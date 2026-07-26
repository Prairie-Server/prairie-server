//! Prairie imageutil WASI helper.
//!
//! Reads an image from `--input`, writes WebP and optional AVIF variants into
//! `--outdir`, and emits a small JSON manifest on stdout. Built for
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
#[command(name = "imageutil-wasm", about = "Prairie WASI image resize/encode helper")]
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

    /// Cap longest original dimension for `variants` (default 1920).
    #[arg(long, default_value_t = 1920)]
    max_original: u32,

    /// Cap longest dimension for `normalize-png` (default 100).
    #[arg(long, default_value_t = 100)]
    max_dim: u32,

    /// Output formats for variants/square-variants: webp,avif (default both).
    #[arg(long, value_delimiter = ',', default_value = "webp,avif")]
    formats: Vec<String>,
}

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
}

struct FormatFlags {
    webp: bool,
    avif: bool,
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
    };
    for item in raw {
        match item.trim().to_ascii_lowercase().as_str() {
            "" => {}
            "webp" => flags.webp = true,
            "avif" => flags.avif = true,
            other => return Err(format!("unknown format {other:?} (want webp and/or avif)")),
        }
    }
    if !flags.webp && !flags.avif {
        return Err("formats must include webp and/or avif".into());
    }
    // Canonical cache keys stay WebP; AVIF is always a sibling upgrade.
    if flags.avif && !flags.webp {
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
        &formats,
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
        variants.push(write_variant_pair(
            &args.outdir,
            &format!("w{w}"),
            &out,
            args.quality,
            &formats,
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
        &formats,
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
        variants.push(write_variant_pair(
            &args.outdir,
            &format!("w{size}"),
            &out,
            args.quality,
            &formats,
        )?);
    }

    Ok(Manifest {
        ext: ".webp".into(),
        variants,
    })
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
        }],
    })
}

fn write_variant_pair(
    outdir: &Path,
    key: &str,
    img: &DynamicImage,
    quality: u8,
    formats: &FormatFlags,
) -> Result<ManifestVariant, String> {
    if !formats.webp {
        return Err("webp output is required for canonical cache keys".into());
    }
    let webp_name = format!("{key}.webp");
    write_webp(&outdir.join(&webp_name), img, quality)?;
    let avif_file = if formats.avif {
        let avif_name = format!("{key}.avif");
        write_avif(&outdir.join(&avif_name), img, quality)?;
        Some(avif_name)
    } else {
        None
    };
    Ok(ManifestVariant {
        key: key.into(),
        file: webp_name,
        avif_file,
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
    if trimmed.starts_with("<?xml") {
        return trimmed[5..].find("<svg").is_some_and(|idx| idx < 400);
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

fn write_avif(path: &Path, img: &DynamicImage, quality: u8) -> Result<(), String> {
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
        .with_speed(10)
        .encode_rgba(Img::new(pixels.as_slice(), w as usize, h as usize))
        .map_err(|e| format!("encode avif: {e}"))?;
    fs::write(path, encoded.avif_file).map_err(|e| format!("write {}: {e}", path.display()))
}
