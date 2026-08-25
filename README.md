# Quartz Account Server (Go)

[![Deploy API to Cloud](https://github.com/Jacobinoo/QuartzAccountGo/actions/workflows/deploy.yml/badge.svg?event=push)](https://github.com/Jacobinoo/QuartzAccountGo/actions/workflows/deploy.yml)

A high-performance account authentication and key management server written in Go. It relies on the [OPAQUE](https://datatracker.ietf.org/doc/draft-irtf-cfrg-opaque/) cryptographic protocol to provide strong, asymmetric password-authenticated key exchange (PAKE). The backend relies on native Rust bindings (`opaque-ke`) via CGO to process the cryptographic heavy lifting.

## Prerequisites

To build and run this project, you must have the following installed on your machine:

1. **Go** (v1.20+ recommended)
2. **Rust** and **Cargo** (for compiling the OPAQUE cryptography bindings)
3. A **C compiler** (e.g. `gcc` or `clang` for CGO support)
4. **Redis** (running locally or accessible via URL)
5. **PostgreSQL** (running locally or accessible via URL)

## Setup & Compilation

This project uses a Makefile to simplify the build process. Before building the Go application, the Rust bindings must be compiled into a static library.

1. **Clone the repository:**
   ```bash
   git clone <your-repo-url>
   cd QuartzAccountGo
   ```

2. **Copy the example environment variables:**
   ```bash
   cp .env.example .env
   ```
   *(Be sure to fill in your `DB_HOST`, `DB_PASSWORD`, `KV_URL`, etc.)*

3. **Generate a new OPAQUE Server Setup Key:**
   The server requires a long-lived cryptographic seed to operate. You must generate this once and store it safely.
   ```bash
   make setup-env
   ```
   *Copy the output string and add it to your `.env` file as `OPAQUE_SERVER_SETUP=...`*

4. **Run the server:**
   The Makefile will automatically compile the Rust bindings first, and then build/run the Go server.
   ```bash
   make run
   ```

## Development

To run the test suites (which execute both the Rust unit tests and the Go backend tests):
```bash
make test
```

If you're making changes to the Rust bindings located in `internal/bindings/opaque_rust/`, you can rebuild them manually:
```bash
make rust
```

To clean all build artifacts (including the compiled Rust library and Go binary):
```bash
make clean
```

## License
This project is licensed under the **GNU Affero General Public License v3.0 (AGPLv3)**. See the `LICENSE` file for details.
