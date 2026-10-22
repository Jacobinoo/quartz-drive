FROM golang:1.27.2-bookworm AS builder

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

# Fetch the latest disposable email blocklist and compare with local
RUN curl -sSL -o remote.conf "https://raw.githubusercontent.com/disposable-email-domains/disposable-email-domains/refs/heads/main/disposable_email_blocklist.conf" || true && \
    if [ -s remote.conf ]; then \
        REMOTE_COUNT=$(wc -l < remote.conf); \
        LOCAL_COUNT=$(wc -l < pkg/email/disposable_email_blocklist.conf 2>/dev/null || echo "0"); \
        if [ "$REMOTE_COUNT" -gt "$LOCAL_COUNT" ]; then \
            echo "CI INFO: Using remote blocklist ($REMOTE_COUNT entries vs local $LOCAL_COUNT entries)"; \
            mv remote.conf pkg/email/disposable_email_blocklist.conf; \
        else \
            echo "CI INFO: Using local blocklist (Remote is smaller or identical)"; \
            rm remote.conf; \
        fi \
    else \
        echo "CI INFO: Failed to fetch remote list, falling back to local list"; \
    fi

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
