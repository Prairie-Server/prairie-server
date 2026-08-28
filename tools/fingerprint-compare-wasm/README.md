# fingerprint_compare.wasm — chromaprint intro compare module

`fingerprint_compare.wasm` is a small Rust WASI CLI that runs the CPU-heavy
chromaprint fingerprint comparison used by intro marker detection. Prairie Server
runs it **in-process** via [`wazero`](https://github.com/tetratelabs/wazero) from
`internal/intromarkers`.

## Why this shape

- **No cgo.** The main Go binary builds with `CGO_ENABLED=0`.
- **Architecture-independent.** One `wasm32-wasip1` artifact runs on amd64/arm64
  through wazero.
- **Hot path offload.** Pairwise Hamming-distance matching is O(n²) with nested
  loops; Rust compiles tighter and can use SIMD for popcount.

## Rebuild + install the artifact

```sh
# from repository root
rustup target add wasm32-wasip1
cargo build --release --target wasm32-wasip1 \
  --manifest-path tools/fingerprint-compare-wasm/Cargo.toml
cp tools/fingerprint-compare-wasm/target/wasm32-wasip1/release/fingerprint-compare-wasm.wasm \
  internal/intromarkers/fingerprint_compare.wasm
sha256sum internal/intromarkers/fingerprint_compare.wasm \
  | tee internal/intromarkers/fingerprint_compare.wasm.sha256
```

Or use the Docker builder (pinned Rust, no host toolchain required):

```sh
docker build --platform linux/amd64 -t fingerprint-compare-wasm tools/fingerprint-compare-wasm
id=$(docker create fingerprint-compare-wasm)
docker cp "$id:/fingerprint_compare.wasm"        internal/intromarkers/fingerprint_compare.wasm
docker cp "$id:/fingerprint_compare.wasm.sha256" internal/intromarkers/fingerprint_compare.wasm.sha256
docker rm "$id"
```

Commit both the `.wasm` and the `.sha256` sidecar. `go test ./internal/intromarkers`
checks the embed matches the sidecar.

Host-side unit tests (no WASI target required):

```sh
cargo test --manifest-path tools/fingerprint-compare-wasm/Cargo.toml
cargo fmt --manifest-path tools/fingerprint-compare-wasm/Cargo.toml --check
cargo clippy --manifest-path tools/fingerprint-compare-wasm/Cargo.toml --all-targets -- -D warnings
```

## CLI surface (used by Go)

```text
fingerprint-compare-wasm --input /in/request.json
```

Prints a JSON response on stdout:

```json
{"matches":[{"left_index":0,"right_index":1,"left":{"start":0.0,"end":32.0},"right":{"start":19.7,"end":51.7}}]}
```
