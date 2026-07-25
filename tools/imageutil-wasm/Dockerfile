# Rebuild the committed imageutil.wasm artifact.
# Usage: see tools/imageutil-wasm/README.md
FROM rust:1.97-bookworm AS build
RUN rustup target add wasm32-wasip1
WORKDIR /src
COPY Cargo.toml Cargo.lock ./
COPY src ./src
RUN cargo build --release --target wasm32-wasip1 \
 && cp target/wasm32-wasip1/release/imageutil-wasm.wasm /imageutil.wasm \
 && sha256sum /imageutil.wasm | awk '{print $1"  imageutil.wasm"}' > /imageutil.wasm.sha256

FROM scratch
COPY --from=build /imageutil.wasm /imageutil.wasm
COPY --from=build /imageutil.wasm.sha256 /imageutil.wasm.sha256
