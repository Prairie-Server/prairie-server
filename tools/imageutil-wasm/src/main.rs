//! Prairie imageutil WASI helper.
//!
//! Reads an image from `--input`, writes WebP plus optional AVIF/PNG siblings
//! into `--outdir`, and emits a small JSON manifest on stdout. Built for
//! wasm32-wasip1 and run in-process by `internal/imageutil` via wazero.

use std::fs;
use std::path::{Path, PathBuf};
use std::process::ExitCode;

use clap::{Parser, ValueEnum};
use image::imageops::FilterType;
use image::{DynamicImage, GenericImageView, ImageBuffer, ImageFormat, Rgba, RgbaImage};
use ravif::{Encoder as AvifEncoder, Img, RGBA8};
use resvg::tiny_skia;
use resvg::usvg::{Options as UsvgOptions, Tree};
use serde::Serialize;
use zenwebp::{EncodeRequest, LossyConfig, PixelLayout};

#[derive(Debug, Clone, ValueEnum)]
enum Mode {
    /// Re-encode original (capped) + width variants.
    Variants,
    /// Center-crop to square, then emit square original + size variants.
    SquareVariants,
    /// Re-encode as PNG scaled within max-dim (thumbhash normalize path).
    NormalizePng,
}

#[derive(Debug, Parser)]
#[command(
    name = "imageutil-wasm",
    about = "Prairie WASI image resize/encode helper"
)]
struct Args {
    #[arg(long, value_enum)]
    mode: Mode,

    #[arg(long)]
    input: PathBuf,

    #[arg(long)]
    outdir: PathBuf,

    /// Target widths for `variants` (descending order not required).
    #[arg(long, value_delimiter = ',')]
    widths: Vec<u32>,

    /// Target square sizes for `square-variants`.
    #[arg(long, value_delimiter = ',')]
    sizes: Vec<u32>,

    /// WebP/AVIF quality 1–100 (default 90).
    #[arg(long, default_value_t = 90)]
    quality: u8,

    /// rav1e AVIF speed preset 1..=10 (10 = fastest; default 10).
    /// Photographic posters/backdrops stay visually identical at speed 10 vs
    /// slower presets; encode time drops multi-fold in WASM.
    #[arg(long, default_value_t = DEFAULT_AVIF_SPEED)]
    avif_speed: u8,

    /// Cap longest original dimension for `variants` (default 1920).
    #[arg(long, default_value_t = 1920)]
    max_original: u32,

    /// Cap longest dimension for `normalize-png` (default 100).
    #[arg(long, default_value_t = 100)]
    max_dim: u32,

    /// Output formats for variants/square-variants: webp,avif,png.
    /// Default is webp,avif — PNG remains available for callers that need it
    /// (thumbhash normalize still uses PNG) but is not dual-written by default.
    #[arg(long, value_delimiter = ',', default_value = "webp,avif")]
    formats: Vec<String>,

    /// Variant keys that should keep WebP but skip AVIF encode (comma-separated).
    /// GenerateAVIFSiblings passes `original` so only the display ladder
    /// (w300/w500/…) pays for rav1e — the largest frame is the slowest.
    #[arg(long, value_delimiter = ',', default_value = "")]
    no_avif_keys: Vec<String>,
}

/// Fastest rav1e preset. Tunable via `--avif-speed` / Go `avifSpeed`.
const DEFAULT_AVIF_SPEED: u8 = 10;

#[derive(Serialize)]
struct Manifest {
    /// Canonical extension used for cache keys / revision identity (WebP).
    ext: String,
    variants: Vec<ManifestVariant>,
}

#[derive(Serialize)]
struct ManifestVariant {
    key: String,
    file: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    avif_file: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    png_file: Option<String>,
}

struct FormatFlags {
    webp: bool,
    avif: bool,
    png: bool,
}

fn main() -> ExitCode {
    match run() {
        Ok(()) => ExitCode::SUCCESS,
        Err(err) => {
            eprintln!("imageutil-wasm: {err}");
            ExitCode::FAILURE
        }
    }
}

fn run() -> Result<(), String> {
    let args = Args::parse();
    if !(1..=100).contains(&args.quality) {
        return Err(format!("quality must be 1..=100, got {}", args.quality));
    }
    if !(1..=10).contains(&args.avif_speed) {
        return Err(format!(
            "avif-speed must be 1..=10, got {}",
            args.avif_speed
        ));
    }
    fs::create_dir_all(&args.outdir).map_err(|e| format!("create outdir: {e}"))?;

    let data = fs::read(&args.input).map_err(|e| format!("read input: {e}"))?;
    let img = decode_image(&data)?;

    let manifest = match args.mode {
        Mode::Variants => process_variants(&img, &args)?,
        Mode::SquareVariants => process_square_variants(&img, &args)?,
        Mode::NormalizePng => process_normalize_png(&img, &args)?,
    };

    let json = serde_json::to_string(&manifest).map_err(|e| format!("encode manifest: {e}"))?;
    println!("{json}");
    Ok(())
}

fn parse_formats(raw: &[String]) -> Result<FormatFlags, String> {
    let mut flags = FormatFlags {
        webp: false,
        avif: false,
        png: false,
    };
    for item in raw {
        match item.trim().to_ascii_lowercase().as_str() {
            "" => {}
            "webp" => flags.webp = true,
            "avif" => flags.avif = true,
            "png" => flags.png = true,
            other => {
                return Err(format!(
                    "unknown format {other:?} (want webp, avif, and/or png)"
                ))
            }
        }
    }
    if !flags.webp && !flags.avif && !flags.png {
        return Err("formats must include webp, avif, and/or png".into());
    }
    // Canonical cache keys stay WebP; AVIF/PNG are always sibling upgrades.
    if (flags.avif || flags.png) && !flags.webp {
        flags.webp = true;
    }
    Ok(flags)
}

fn process_variants(img: &DynamicImage, args: &Args) -> Result<Manifest, String> {
    let formats = parse_formats(&args.formats)?;
    let mut variants = Vec::with_capacity(args.widths.len() + 1);

    let original = fit_within(img, args.max_original);
    variants.push(write_variant_pair(
        &args.outdir,
        "original",
        &original,
        args.quality,
        args.avif_speed,
        &formats_for_key("original", &formats, &args.no_avif_keys),
    )?);

    let mut widths = args.widths.clone();
    widths.sort_unstable_by(|a, b| b.cmp(a));
    let (src_w, _) = img.dimensions();
    for w in widths {
        if w == 0 {
            continue;
        }
        let out = if src_w > w {
            img.resize(w, u32::MAX, FilterType::Triangle)
        } else {
            img.clone()
        };
        let key = format!("w{w}");
        variants.push(write_variant_pair(
            &args.outdir,
            &key,
            &out,
            args.quality,
            args.avif_speed,
            &formats_for_key(&key, &formats, &args.no_avif_keys),
        )?);
    }

    Ok(Manifest {
        ext: ".webp".into(),
        variants,
    })
}

fn process_square_variants(img: &DynamicImage, args: &Args) -> Result<Manifest, String> {
    let formats = parse_formats(&args.formats)?;
    let square = center_crop_square(img)?;
    let square_size = square.width();
    let mut variants = Vec::with_capacity(args.sizes.len() + 1);

    variants.push(write_variant_pair(
        &args.outdir,
        "original",
        &square,
        args.quality,
        args.avif_speed,
        &formats_for_key("original", &formats, &args.no_avif_keys),
    )?);

    let mut sizes = args.sizes.clone();
    sizes.sort_unstable_by(|a, b| b.cmp(a));
    for size in sizes {
        if size == 0 {
            continue;
        }
        let out = if square_size == size {
            square.clone()
        } else {
            DynamicImage::ImageRgba8(image::imageops::resize(
                square.as_rgba8().ok_or("square image missing rgba8")?,
                size,
                size,
                FilterType::Triangle,
            ))
        };
        let key = format!("w{size}");
        variants.push(write_variant_pair(
            &args.outdir,
            &key,
            &out,
            args.quality,
            args.avif_speed,
            &formats_for_key(&key, &formats, &args.no_avif_keys),
        )?);
    }

    Ok(Manifest {
        ext: ".webp".into(),
        variants,
    })
}

fn formats_for_key(key: &str, formats: &FormatFlags, no_avif_keys: &[String]) -> FormatFlags {
    let mut out = FormatFlags {
        webp: formats.webp,
        avif: formats.avif,
        png: formats.png,
    };
    if out.avif
        && no_avif_keys
            .iter()
            .any(|k| !k.is_empty() && k.eq_ignore_ascii_case(key))
    {
        out.avif = false;
    }
    out
}

fn process_normalize_png(img: &DynamicImage, args: &Args) -> Result<Manifest, String> {
    let out = fit_within(img, args.max_dim);
    let name = "normalized.png";
    out.save_with_format(args.outdir.join(name), ImageFormat::Png)
        .map_err(|e| format!("encode png: {e}"))?;
    Ok(Manifest {
        ext: ".png".into(),
        variants: vec![ManifestVariant {
            key: "normalized".into(),
            file: name.into(),
            avif_file: None,
            png_file: None,
        }],
    })
}

fn write_variant_pair(
    outdir: &Path,
    key: &str,
    img: &DynamicImage,
    quality: u8,
    avif_speed: u8,
    formats: &FormatFlags,
) -> Result<ManifestVariant, String> {
    if !formats.webp {
        return Err("webp output is required for canonical cache keys".into());
    }
    let webp_name = format!("{key}.webp");
    write_webp(&outdir.join(&webp_name), img, quality)?;
    let avif_file = if formats.avif {
        let avif_name = format!("{key}.avif");
        write_avif(&outdir.join(&avif_name), img, quality, avif_speed)?;
        Some(avif_name)
    } else {
        None
    };
    let png_file = if formats.png {
        let png_name = format!("{key}.png");
        img.save_with_format(outdir.join(&png_name), ImageFormat::Png)
            .map_err(|e| format!("encode png: {e}"))?;
        Some(png_name)
    } else {
        None
    };
    Ok(ManifestVariant {
        key: key.into(),
        file: webp_name,
        avif_file,
        png_file,
    })
}

fn decode_image(data: &[u8]) -> Result<DynamicImage, String> {
    if looks_like_svg(data) {
        return rasterize_svg(data);
    }
    image::load_from_memory(data).map_err(|e| format!("decode image: {e}"))
}

fn looks_like_svg(data: &[u8]) -> bool {
    let prefix = data.iter().take(512).copied().collect::<Vec<_>>();
    let s = String::from_utf8_lossy(&prefix).to_ascii_lowercase();
    let trimmed = s.trim_start_matches(['\u{feff}', ' ', '\t', '\r', '\n']);
    // Require a real SVG root near the start. Avoid matching arbitrary XML/HTML
    // that merely mentions "<svg" deeper in the payload.
    if trimmed.starts_with("<svg") {
        return true;
    }
    if let Some(after_xml) = trimmed.strip_prefix("<?xml") {
        return after_xml.find("<svg").is_some_and(|idx| idx < 400);
    }
    false
}

fn rasterize_svg(data: &[u8]) -> Result<DynamicImage, String> {
    let opt = UsvgOptions::default();
    let tree = Tree::from_data(data, &opt).map_err(|e| format!("parse svg: {e}"))?;
    let size = tree.size().to_int_size();
    let mut width = size.width().max(1);
    let mut height = size.height().max(1);
    const MAX_SVG_EDGE: u32 = 4096;
    if width > MAX_SVG_EDGE || height > MAX_SVG_EDGE {
        let scale = MAX_SVG_EDGE as f32 / width.max(height) as f32;
        width = ((width as f32) * scale).round().max(1.0) as u32;
        height = ((height as f32) * scale).round().max(1.0) as u32;
    }
    let mut pixmap = tiny_skia::Pixmap::new(width, height)
        .ok_or_else(|| format!("svg pixmap {width}x{height}"))?;
    let scale_x = width as f32 / tree.size().width();
    let scale_y = height as f32 / tree.size().height();
    let transform = tiny_skia::Transform::from_scale(scale_x, scale_y);
    resvg::render(&tree, transform, &mut pixmap.as_mut());
    let rgba = ImageBuffer::<Rgba<u8>, _>::from_raw(width, height, pixmap.take())
        .ok_or_else(|| "svg rgba buffer".to_string())?;
    Ok(DynamicImage::ImageRgba8(rgba))
}

fn fit_within(img: &DynamicImage, max_dim: u32) -> DynamicImage {
    if max_dim == 0 {
        return img.clone();
    }
    let (w, h) = img.dimensions();
    if w <= max_dim && h <= max_dim {
        return img.clone();
    }
    if w >= h {
        img.resize(max_dim, u32::MAX, FilterType::Triangle)
    } else {
        img.resize(u32::MAX, max_dim, FilterType::Triangle)
    }
}

fn center_crop_square(img: &DynamicImage) -> Result<DynamicImage, String> {
    let (w, h) = img.dimensions();
    if w == 0 || h == 0 {
        return Err("invalid image size".into());
    }
    let side = w.min(h);
    let x = (w - side) / 2;
    let y = (h - side) / 2;
    let rgba: RgbaImage = img.to_rgba8();
    let cropped = image::imageops::crop_imm(&rgba, x, y, side, side).to_image();
    Ok(DynamicImage::ImageRgba8(cropped))
}

fn write_webp(path: &Path, img: &DynamicImage, quality: u8) -> Result<(), String> {
    let rgba = img.to_rgba8();
    let (w, h) = rgba.dimensions();
    let config = LossyConfig::new()
        .with_quality(f32::from(quality))
        .with_method(4);
    let encoded = EncodeRequest::lossy(&config, rgba.as_raw(), PixelLayout::Rgba8, w, h)
        .encode()
        .map_err(|e| format!("encode webp: {e}"))?;
    fs::write(path, encoded).map_err(|e| format!("write {}: {e}", path.display()))
}

fn write_avif(path: &Path, img: &DynamicImage, quality: u8, speed: u8) -> Result<(), String> {
    let rgba = img.to_rgba8();
    let (w, h) = rgba.dimensions();
    let pixels: Vec<RGBA8> = rgba
        .as_raw()
        .chunks_exact(4)
        .map(|px| RGBA8 {
            r: px[0],
            g: px[1],
            b: px[2],
            a: px[3],
        })
        .collect();
    // Speed 10 = fastest rav1e preset; artwork is cache-once so favor WASM latency.
    let encoded = AvifEncoder::new()
        .with_quality(f32::from(quality))
        .with_speed(speed)
        .encode_rgba(Img::new(pixels.as_slice(), w as usize, h as usize))
        .map_err(|e| format!("encode avif: {e}"))?;
    fs::write(path, encoded.avif_file).map_err(|e| format!("write {}: {e}", path.display()))
}

#[cfg(test)]
mod tests {
    use super::*;
    use image::{Rgba, RgbaImage};

    fn solid(w: u32, h: u32) -> DynamicImage {
        DynamicImage::ImageRgba8(RgbaImage::from_pixel(w, h, Rgba([10, 20, 30, 255])))
    }

    #[test]
    fn parse_formats_defaults_and_unknown() {
        let all = parse_formats(&["webp".into(), "avif".into(), "png".into()]).unwrap();
        assert!(all.webp && all.avif && all.png);

        let both = parse_formats(&["webp".into(), "avif".into()]).unwrap();
        assert!(both.webp && both.avif && !both.png);

        let webp_only = parse_formats(&["webp".into()]).unwrap();
        assert!(webp_only.webp && !webp_only.avif && !webp_only.png);

        // AVIF/PNG-only still force WebP so canonical cache keys stay .webp.
        let avif_only = parse_formats(&["avif".into()]).unwrap();
        assert!(avif_only.webp && avif_only.avif && !avif_only.png);

        let png_only = parse_formats(&["png".into()]).unwrap();
        assert!(png_only.webp && !png_only.avif && png_only.png);

        assert!(parse_formats(&[]).is_err());
        assert!(parse_formats(&["gif".into()]).is_err());
    }

    #[test]
    fn looks_like_svg_requires_root_near_start() {
        assert!(looks_like_svg(
            b"<svg xmlns='http://www.w3.org/2000/svg'></svg>"
        ));
        assert!(looks_like_svg(
            b"<?xml version=\"1.0\"?><svg xmlns='http://www.w3.org/2000/svg'></svg>"
        ));
        assert!(!looks_like_svg(b"<html><body><svg></svg></body></html>"));
        assert!(!looks_like_svg(b"\x89PNG\r\n\x1a\n"));
    }

    #[test]
    fn fit_within_caps_longest_edge() {
        let img = solid(2000, 1000);
        let out = fit_within(&img, 1000);
        assert_eq!(out.dimensions(), (1000, 500));

        let already = solid(800, 600);
        assert_eq!(fit_within(&already, 1000).dimensions(), (800, 600));
    }

    #[test]
    fn center_crop_square_is_centered() {
        let img = solid(300, 100);
        let square = center_crop_square(&img).unwrap();
        assert_eq!(square.dimensions(), (100, 100));
    }

    #[test]
    fn write_variant_pair_emits_webp_and_optional_avif() {
        let dir = tempfile_dir("imageutil-pair");
        let img = solid(32, 32);
        let all = FormatFlags {
            webp: true,
            avif: true,
            png: true,
        };
        let manifest =
            write_variant_pair(&dir, "original", &img, 80, DEFAULT_AVIF_SPEED, &all).unwrap();
        assert_eq!(manifest.file, "original.webp");
        assert_eq!(manifest.avif_file.as_deref(), Some("original.avif"));
        assert_eq!(manifest.png_file.as_deref(), Some("original.png"));
        assert!(dir.join("original.webp").is_file());
        assert!(dir.join("original.avif").is_file());
        assert!(dir.join("original.png").is_file());

        let webp_bytes = fs::read(dir.join("original.webp")).unwrap();
        assert!(webp_bytes.windows(4).any(|w| w == b"WEBP") || webp_bytes.starts_with(b"RIFF"));
        let avif_bytes = fs::read(dir.join("original.avif")).unwrap();
        assert!(avif_bytes.windows(4).any(|w| w == b"ftyp"));
        let png_bytes = fs::read(dir.join("original.png")).unwrap();
        assert!(png_bytes.starts_with(b"\x89PNG"));
    }

    fn tempfile_dir(prefix: &str) -> PathBuf {
        let mut dir = std::env::temp_dir();
        dir.push(format!(
            "{prefix}-{}-{}",
            std::process::id(),
            std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .unwrap()
                .as_nanos()
        ));
        fs::create_dir_all(&dir).unwrap();
        dir
    }
}
