//! Prairie imageutil WASI helper.
//!
//! Reads an image from `--input`, writes WebP (or PNG) variants into `--outdir`,
//! and emits a small JSON manifest on stdout. Built for wasm32-wasip1 and run
//! in-process by `internal/imageutil` via wazero.

use std::fs;
use std::path::{Path, PathBuf};
use std::process::ExitCode;

use clap::{Parser, ValueEnum};
use image::imageops::FilterType;
use image::{DynamicImage, GenericImageView, ImageBuffer, ImageFormat, Rgba, RgbaImage};
use resvg::tiny_skia;
use resvg::usvg::{Options as UsvgOptions, Tree};
use serde::Serialize;
use zenwebp::{EncodeRequest, LossyConfig, PixelLayout};

#[derive(Debug, Clone, ValueEnum)]
enum Mode {
    /// Re-encode original (capped) + width variants as WebP.
    Variants,
    /// Center-crop to square, then emit square original + size variants as WebP.
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

    /// WebP quality 1–100 (default 90).
    #[arg(long, default_value_t = 90)]
    quality: u8,

    /// Cap longest original dimension for `variants` (default 1920).
    #[arg(long, default_value_t = 1920)]
    max_original: u32,

    /// Cap longest dimension for `normalize-png` (default 100).
    #[arg(long, default_value_t = 100)]
    max_dim: u32,
}

#[derive(Serialize)]
struct Manifest {
    ext: String,
    variants: Vec<ManifestVariant>,
}

#[derive(Serialize)]
struct ManifestVariant {
    key: String,
    file: String,
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

fn process_variants(img: &DynamicImage, args: &Args) -> Result<Manifest, String> {
    let mut variants = Vec::with_capacity(args.widths.len() + 1);

    let original = fit_within(img, args.max_original);
    let original_name = "original.webp";
    write_webp(&args.outdir.join(original_name), &original, args.quality)?;
    variants.push(ManifestVariant {
        key: "original".into(),
        file: original_name.into(),
    });

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
        let name = format!("w{w}.webp");
        write_webp(&args.outdir.join(&name), &out, args.quality)?;
        variants.push(ManifestVariant {
            key: format!("w{w}"),
            file: name,
        });
    }

    Ok(Manifest {
        ext: ".webp".into(),
        variants,
    })
}

fn process_square_variants(img: &DynamicImage, args: &Args) -> Result<Manifest, String> {
    let square = center_crop_square(img)?;
    let square_size = square.width();
    let mut variants = Vec::with_capacity(args.sizes.len() + 1);

    let original_name = "original.webp";
    write_webp(&args.outdir.join(original_name), &square, args.quality)?;
    variants.push(ManifestVariant {
        key: "original".into(),
        file: original_name.into(),
    });

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
        let name = format!("w{size}.webp");
        write_webp(&args.outdir.join(&name), &out, args.quality)?;
        variants.push(ManifestVariant {
            key: format!("w{size}"),
            file: name,
        });
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
        }],
    })
}

fn decode_image(data: &[u8]) -> Result<DynamicImage, String> {
    if looks_like_svg(data) {
        return rasterize_svg(data);
    }
    image::load_from_memory(data).map_err(|e| format!("decode image: {e}"))
}

fn looks_like_svg(data: &[u8]) -> bool {
    let prefix = data.iter().take(256).copied().collect::<Vec<_>>();
    let s = String::from_utf8_lossy(&prefix).to_ascii_lowercase();
    let trimmed = s.trim_start();
    trimmed.starts_with("<svg")
        || trimmed.starts_with("<?xml") && trimmed.contains("<svg")
        || trimmed.contains("<svg")
}

fn rasterize_svg(data: &[u8]) -> Result<DynamicImage, String> {
    let opt = UsvgOptions::default();
    let tree = Tree::from_data(data, &opt).map_err(|e| format!("parse svg: {e}"))?;
    let size = tree.size().to_int_size();
    let mut width = size.width().max(1);
    let mut height = size.height().max(1);
    // Cap enormous SVGs before allocating a pixmap (matches artwork cache needs).
    const MAX_SVG_EDGE: u32 = 4096;
    if width > MAX_SVG_EDGE || height > MAX_SVG_EDGE {
        let scale = MAX_SVG_EDGE as f32 / width.max(height) as f32;
        // Use float scale via pixmap size only; resvg fit is handled by pixmap dims.
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
    // method 4 balances encode time vs size for artwork caching.
    let config = LossyConfig::new()
        .with_quality(f32::from(quality))
        .with_method(4);
    let encoded = EncodeRequest::lossy(&config, rgba.as_raw(), PixelLayout::Rgba8, w, h)
        .encode()
        .map_err(|e| format!("encode webp: {e}"))?;
    fs::write(path, encoded).map_err(|e| format!("write {}: {e}", path.display()))
}
