# Artwork pipeline: drop libvips/CGO via Rust WASI — design

**Goal:** Make Prairie Server’s Go build `CGO_ENABLED=0` by moving image
decode/resize/WebP encode out of `github.com/h2non/bimg` (libvips) into an
embedded WASI module, matching the Kindle→EPUB `ebookconvert` hybrid.

**Status:** Implemented in `internal/imageutil` + `tools/imageutil-wasm`.

## Why

- libvips/`bimg` was the **only** production CGO dependency and forced
  `libvips-dev` in Docker/CI and `libvips42` at runtime.
- The Kindle conversion design already chose wazero + committed `.wasm` for
  “no cgo, arch-independent, sandboxed hostile bytes.” Artwork uploads and
  provider caches are the same class of untrusted input.
- Heavy media CPU remains in FFmpeg subprocesses; this change does **not**
  rewrite playback.

## Shape

1. **`tools/imageutil-wasm`** — Rust `wasm32-wasip1` CLI (`image`, `zenwebp`).
2. **Committed artifact** — `internal/imageutil/imageutil.wasm` + sha256 sidecar.
3. **`internal/imageutil`** — same public API (`GenerateVariants`,
   `GenerateSquareVariants`, `Thumbhash`); implementation instantiates the
   module per job with confined `/in` + `/out` mounts.

## Non-goals (this change)

- Server-side CBR/RAR extraction (still client-side / future WASM if product
  needs it — see Kindle design’s CBR note).
- Replacing FFmpeg probe/transcode/subtitle extract.
- Micro-optimizing WebP encode vs libvips; correctness + CGO removal first.

## Verification

- `go test ./internal/imageutil` (provenance + resize/encode smoke).
- Production `Dockerfile` builds with `CGO_ENABLED=0` and no libvips packages.
