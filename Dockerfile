FROM golang:1.26-bookworm AS builder

# Install Rust and build dependencies
RUN apt-get update && apt-get install -y curl build-essential \
    && curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y
ENV PATH="/root/.cargo/bin:${PATH}"

WORKDIR /app

# Cache Go dependencies
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Copy the rest of the source code
COPY . .

# Build the Rust bindings and the Go application
RUN --mount=type=cache,target=/root/.cargo/registry \
    --mount=type=cache,target=/root/.cargo/git \
    --mount=type=cache,target=/app/internal/bindings/opaque_rust/target \
    --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    make build && \
    mkdir -p /app/libs && \
    cp /app/internal/bindings/opaque_rust/target/release/libopaque_rust.so /app/libs/

# Final runtime image
FROM debian:bookworm-slim
WORKDIR /app

# Install CA certificates for outbound HTTPS connections and curl for healthchecks
RUN apt-get update && apt-get install -y ca-certificates curl && rm -rf /var/lib/apt/lists/*

# Copy the compiled binary and necessary directories
COPY --from=builder /app/build/quartz-account /usr/local/bin/quartz-account
COPY --from=builder /app/libs/libopaque_rust.so /usr/lib/
COPY --from=builder /app/cert /app/cert

# Update shared library cache
RUN ldconfig

# Expose the API port
EXPOSE 3100

CMD ["quartz-account"]
