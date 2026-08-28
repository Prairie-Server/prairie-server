# Rebuild the committed fingerprint_compare.wasm artifact.
# Usage: see tools/fingerprint-compare-wasm/README.md
FROM rust:1.97-bookworm AS build
RUN rustup target add wasm32-wasip1
WORKDIR /src
COPY Cargo.toml ./
COPY src ./src
RUN cargo build --release --target wasm32-wasip1 \
 && cp target/wasm32-wasip1/release/fingerprint-compare-wasm.wasm /fingerprint_compare.wasm \
 && sha256sum /fingerprint_compare.wasm | awk '{print $1"  fingerprint_compare.wasm"}' > /fingerprint_compare.wasm.sha256

FROM scratch
COPY --from=build /fingerprint_compare.wasm /fingerprint_compare.wasm
COPY --from=build /fingerprint_compare.wasm.sha256 /fingerprint_compare.wasm.sha256
