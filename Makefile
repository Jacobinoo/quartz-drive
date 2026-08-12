.PHONY: all build rust clean setup-env test

# Default target
all: build

# Build the Rust OPAQUE bindings
rust:
	cd internal/bindings/opaque_rust && cargo build --release

# Build the Go backend (depends on rust bindings)
build: rust
	go build -o build/quartz-account ./cmd/app

# Build the backend for production (depends on rust bindings)
build-prod: rust
	CGO_ENABLED=1 go build -ldflags="-s -w" -trimpath -o build/quartz-account ./cmd/app

# Run the Go backend
run: rust
	go run ./cmd/app

# Generate the OPAQUE_SERVER_SETUP string for .env
setup-env: rust
	@echo "Generating OPAQUE_SERVER_SETUP..."
	@go run ./cmd/setup_opaque/main.go

# Run Rust and Go tests
test: rust
	@echo "Running Rust tests..."
	cd internal/bindings/opaque_rust && cargo test
	@echo "Running Go tests..."
	go test -v ./...

# Clean build artifacts
clean:
	rm -rf build/
	cd internal/bindings/opaque_rust && cargo clean
