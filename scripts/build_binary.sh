#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

if [ ! -f "dashboard/build/index.html" ] && [ ! -f "dashboard/dist/index.html" ]; then
    echo "Dashboard build is missing. Build dashboard/build or dashboard/dist before creating binaries." >&2
    exit 1
fi

if ! python -c "import PyInstaller" >/dev/null 2>&1; then
    if python -m pip --version >/dev/null 2>&1; then
        python -m pip install --disable-pip-version-check pyinstaller
    else
        echo "PyInstaller is missing from the active environment." >&2
        echo "Sync the locked build dependencies first (for example: uv sync --group build)." >&2
        exit 1
    fi
fi

bash scripts/build_go_bridge.sh

prepare_go_dashboard_embed() {
    local source_dir="dashboard/build"
    local target_dir="go/internal/gateway/static/dashboard/build"
    if [[ ! -f "$source_dir/index.html" && -f "dashboard/dist/index.html" ]]; then
        source_dir="dashboard/dist"
    fi
    if [[ ! -f "$source_dir/index.html" ]]; then
        echo "Dashboard build is missing. Expected dashboard/build/index.html or dashboard/dist/index.html." >&2
        exit 1
    fi
    rm -rf "$target_dir"
    mkdir -p "$target_dir"
    cp -R "$source_dir"/. "$target_dir/"
    touch "$target_dir/.gitkeep"
}

if [[ "${OS:-}" == "Windows_NT" ]]; then
    PYINSTALLER_DATA_SEP=";"
else
    PYINSTALLER_DATA_SEP=":"
fi

pyinstaller_add_data() {
    printf "%s%s%s" "$1" "$PYINSTALLER_DATA_SEP" "$2"
}

JOB_HIDDEN_IMPORT_ARGS=(
    --hidden-import app.jobs.remove_expired_users
    --hidden-import app.jobs.send_notifications
)

COMMON_PYINSTALLER_ARGS=(
    --clean
    --noconfirm
    --onefile
    --add-data "$(pyinstaller_add_data "app/proto" "app/proto")"
    --add-data "$(pyinstaller_add_data "app/templates" "app/templates")"
    --collect-submodules app
    --collect-all apscheduler
    --collect-all fastapi
    --collect-all jinja2
    --collect-all bcrypt
    --collect-all passlib
    --collect-all pydantic
    --collect-all sqlalchemy
    --collect-all starlette
    --collect-all setuptools
    --collect-all uvicorn
    --hidden-import main
    --hidden-import passlib.handlers.bcrypt
    --hidden-import httpx
    --hidden-import pkg_resources
    --hidden-import pymysql
)

GO_BRIDGE_BINARY_ARGS=()
if [[ "${OS:-}" == "Windows_NT" ]]; then
    GO_BRIDGE_PATH="go/build/rebecca_bridge.dll"
elif [[ "$(uname -s)" == "Darwin" ]]; then
    GO_BRIDGE_PATH="go/build/librebecca_bridge.dylib"
else
    GO_BRIDGE_PATH="go/build/librebecca_bridge.so"
fi

if [ -f "$GO_BRIDGE_PATH" ]; then
    GO_BRIDGE_BINARY_ARGS+=(--add-binary "$(pyinstaller_add_data "$GO_BRIDGE_PATH" "go_bridge")")
fi

env REBECCA_SKIP_RUNTIME_INIT=1 DEBUG=false DOCS=false python -m PyInstaller \
    "${COMMON_PYINSTALLER_ARGS[@]}" \
    "${GO_BRIDGE_BINARY_ARGS[@]}" \
    "${JOB_HIDDEN_IMPORT_ARGS[@]}" \
    --name rebecca-python-server \
    packaging/binary_launcher.py

gateway_output="$ROOT_DIR/dist/rebecca-server"
if [[ "${OS:-}" == "Windows_NT" ]]; then
    gateway_output="$ROOT_DIR/dist/rebecca-server.exe"
fi

(
    prepare_go_dashboard_embed
    cd "$ROOT_DIR/go"
    CGO_ENABLED=1 go build -trimpath -buildvcs=false -o "$gateway_output" ./cmd/rebecca_gateway
)

echo "Rebecca Go gateway built at $gateway_output"

bash scripts/build_go_cli.sh
