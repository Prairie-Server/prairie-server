# imageutil.wasm — artwork resize/encode module

`imageutil.wasm` is a small Rust WASI CLI that decodes JPEG/PNG/GIF/WebP/SVG,
resizes, center-crops squares, and encodes WebP (+ optional PNG). Prairie Server
runs it **in-process** via [`wazero`](https://github.com/tetratelabs/wazero)
from `internal/imageutil` for sandboxed decode/resize/WebP.

**AVIF siblings** default to a native backend (`metadata.avif_encoder=auto|svt|nvenc`)
using ffmpeg `libsvtav1` (or optional `av1_nvenc`) — see `internal/imageutil`
(`ConfigureAVIFEncoder`). The WASM `ravif`/rav1e path remains as
`metadata.avif_encoder=wasm` for hermetic fallback. Tradeoff: native AVIF needs
debian `ffmpeg` + libsvtav1 in the runtime image (still `CGO_ENABLED=0`).

## Why this shape

- **No cgo / no libvips.** The main Go binary can build with `CGO_ENABLED=0`.
- **Architecture-independent WebP path.** One `wasm32-wasip1` artifact runs on
  amd64/arm64 through wazero.
- **Sandboxed decode.** Untrusted upload/provider artwork is decoded inside the
  WASM memory sandbox (same pattern as `tools/mobitool-wasm`).
- **Native AVIF throughput.** rav1e-in-WASM cannot match SVT-AV1 SIMD/threads on
  small nodes; native encode is the production default.

## Dependencies

| Component | Notes |
|-----------|--------|
| Rust      | stable (tested with 1.97+) |
| `image`   | decode JPEG/PNG/GIF/WebP |
| `resvg`   | SVG → raster (provider logos) |
| `zenwebp` | pure-Rust lossy WebP encode (AGPL-3.0) |
| `ravif`   | legacy AVIF encode (`avif_encoder=wasm` only) |

## Rebuild + install the artifact

```sh
# from repository root
rustup target add wasm32-wasip1
# SIMD accelerates rav1e inside wazero (Core Features V2 enables simd128).
RUSTFLAGS="-C target-feature=+simd128" \
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

Host-side unit tests (no WASI target required):

```sh
cargo test --manifest-path tools/imageutil-wasm/Cargo.toml
cargo fmt --manifest-path tools/imageutil-wasm/Cargo.toml --check
cargo clippy --manifest-path tools/imageutil-wasm/Cargo.toml --all-targets -- -D warnings
```

CI runs the same gates in the `Rust imageutil` job. Canonical keys stay `.webp`;
the web client prefers AVIF siblings, then WebP (PNG remains a signed fallback
for legacy caches / older clients via 404 fall-through).

## CLI surface (used by Go)

```text
imageutil-wasm --mode variants|square-variants|normalize-png \
  --input /in/src.bin --outdir /out \
  [--widths 500,300] [--sizes 512,256] \
  [--quality 90] [--avif-speed 10] [--max-original 1920] [--max-dim 100] \
  [--formats webp,avif] [--no-avif-keys original]
```

`--avif-speed` is the rav1e preset (1..=10; 10 = fastest). Go passes
`avifSpeed=10` by default. `--no-avif-keys` skips AVIF for listed variant keys
while still writing WebP; `GenerateAVIFSiblings` passes `original` so only the
display ladder (w300/w500/…) is encoded as AVIF.

Prints a JSON manifest on stdout. `ext` stays `.webp` (canonical cache key);
AVIF (and optional PNG) siblings are listed per variant:

```json
{"ext":".webp","variants":[{"key":"original","file":"original.webp","avif_file":"original.avif"}]}
```
