#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GO_DIR="$ROOT_DIR/go"

if [ "${REBECCA_SKIP_GO_CLI:-0}" = "1" ]; then
    echo "Skipping Rebecca Go CLI build."
    exit 2
fi

if [[ "${OS:-}" == "Windows_NT" ]]; then
    echo "Skipping Rebecca Go CLI build on Windows."
    exit 2
fi

if ! command -v go >/dev/null 2>&1; then
    echo "Go toolchain is required to build the Rebecca Go CLI." >&2
    exit 2
fi

mkdir -p "$ROOT_DIR/dist"

(
    cd "$GO_DIR"
    CGO_ENABLED=1 go build -trimpath -buildvcs=false -o "$ROOT_DIR/dist/rebecca-cli" ./cmd/rebecca_cli
)

echo "Rebecca Go CLI built at $ROOT_DIR/dist/rebecca-cli"
