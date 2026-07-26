# imageutil.wasm — artwork resize/encode module

`imageutil.wasm` is a small Rust WASI CLI that decodes JPEG/PNG/GIF/WebP/SVG,
resizes, center-crops squares, and encodes WebP + AVIF (lossy) or PNG. Prairie
Server runs it **in-process** via [`wazero`](https://github.com/tetratelabs/wazero)
from `internal/imageutil`.

## Why this shape

- **No cgo / no libvips.** The main Go binary can build with `CGO_ENABLED=0`.
- **Architecture-independent.** One `wasm32-wasip1` artifact runs on amd64/arm64
  through wazero.
- **Sandboxed decode.** Untrusted upload/provider artwork is decoded inside the
  WASM memory sandbox (same pattern as `tools/mobitool-wasm`).

## Dependencies

| Component | Notes |
|-----------|--------|
| Rust      | stable (tested with 1.97+) |
| `image`   | decode JPEG/PNG/GIF/WebP |
| `resvg`   | SVG → raster (provider logos) |
| `zenwebp` | pure-Rust lossy WebP encode (AGPL-3.0) |
| `ravif`   | pure-Rust lossy AVIF encode |

## Rebuild + install the artifact

```sh
# from repository root
rustup target add wasm32-wasip1
cargo build --release --target wasm32-wasip1 --manifest-path tools/imageutil-wasm/Cargo.toml
cp tools/imageutil-wasm/target/wasm32-wasip1/release/imageutil-wasm.wasm \
  internal/imageutil/imageutil.wasm
sha256sum internal/imageutil/imageutil.wasm | tee internal/imageutil/imageutil.wasm.sha256
```

Or use the Docker builder (pinned Rust, no host toolchain required):

```sh
docker build --platform linux/amd64 -t imageutil-wasm tools/imageutil-wasm
id=$(docker create imageutil-wasm)
docker cp "$id:/imageutil.wasm"        internal/imageutil/imageutil.wasm
docker cp "$id:/imageutil.wasm.sha256" internal/imageutil/imageutil.wasm.sha256
docker rm "$id"
```

Commit both the `.wasm` and the `.sha256` sidecar. `go test ./internal/imageutil`
checks the embed matches the sidecar and smoke-tests resize/encode.

## CLI surface (used by Go)

```text
imageutil-wasm --mode variants|square-variants|normalize-png \
  --input /in/src.bin --outdir /out \
  [--widths 500,300] [--sizes 512,256] \
  [--quality 90] [--max-original 1920] [--max-dim 100] \
  [--formats webp,avif]
```

Prints a JSON manifest on stdout. `ext` stays `.webp` (canonical cache key);
AVIF siblings are listed per variant:

```json
{"ext":".webp","variants":[{"key":"original","file":"original.webp","avif_file":"original.avif"}]}
```
