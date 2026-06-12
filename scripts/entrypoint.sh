#!/bin/bash

DATA_DIR="${REBECCA_DATA_DIR:-/var/lib/rebecca}"
XRAY_DIR="${DATA_DIR}/xray-core"
XRAY_BIN="${XRAY_DIR}/xray"

mkdir -p "$XRAY_DIR"

# Check and install Xray into persistent host-mounted storage
if [ ! -x "$XRAY_BIN" ]; then
    echo "Persisted Xray executable not found at ${XRAY_BIN}, installing..."
    if [ -f "/code/scripts/rebecca/install_latest_xray.sh" ]; then
        XRAY_INSTALL_DIR="$XRAY_DIR" XRAY_ASSETS_DIR="$XRAY_DIR" bash /code/scripts/rebecca/install_latest_xray.sh
    elif [ -f "/code/scripts/install_latest_xray.sh" ]; then
        XRAY_INSTALL_DIR="$XRAY_DIR" XRAY_ASSETS_DIR="$XRAY_DIR" bash /code/scripts/install_latest_xray.sh
    else
        # Fallback: download and install directly
        ARCH=$(uname -m)
        case "$ARCH" in
            x86_64) ARCH="64" ;;
            aarch64|arm64) ARCH="arm64-v8a" ;;
            armv7l) ARCH="arm32-v7a" ;;
            *) ARCH="64" ;;
        esac
        
        TMP_DIR=$(mktemp -d)
        ZIP_FILE="${TMP_DIR}/Xray-linux-${ARCH}.zip"
        
        echo "Downloading Xray for architecture: ${ARCH}"
        if curl -L -o "$ZIP_FILE" "https://github.com/XTLS/Xray-core/releases/latest/download/Xray-linux-${ARCH}.zip"; then
            if command -v unzip >/dev/null 2>&1; then
                unzip -q "$ZIP_FILE" -d "$TMP_DIR"
                install -m 755 "${TMP_DIR}/xray" "$XRAY_BIN"
                install -m 644 "${TMP_DIR}/geoip.dat" "${XRAY_DIR}/geoip.dat" 2>/dev/null || true
                install -m 644 "${TMP_DIR}/geosite.dat" "${XRAY_DIR}/geosite.dat" 2>/dev/null || true
                echo "Xray installed successfully"
            else
                echo "Warning: unzip not found, cannot install Xray"
            fi
        else
            echo "Warning: Failed to download Xray"
        fi
        rm -rf "$TMP_DIR"
    fi
fi

# Keep compatibility paths, but always run panel using persistent core/assets.
mkdir -p /usr/local/bin /usr/local/share
ln -sf "$XRAY_BIN" /usr/local/bin/xray
ln -sfn "$XRAY_DIR" /usr/local/share/xray
export XRAY_EXECUTABLE_PATH="$XRAY_BIN"
export XRAY_ASSETS_PATH="$XRAY_DIR"

# Wait for database to be ready
echo "Waiting for database to be ready..."
max_attempts=30
attempt=0
while [ $attempt -lt $max_attempts ]; do
    if python -c "from app.db.base import engine; from sqlalchemy import text; engine.connect().execute(text('SELECT 1'))" 2>/dev/null; then
        echo "Database is ready!"
        break
    fi
    attempt=$((attempt + 1))
    echo "Attempt $attempt/$max_attempts: Database not ready yet, waiting 2 seconds..."
    sleep 2
done

if [ $attempt -eq $max_attempts ]; then
    echo "Warning: Database connection timeout, proceeding anyway..."
fi

# Run Go/goose migrations with timeout. The Go gateway also runs migrations
# before serving, but this keeps container startup explicit and compatible with
# older launch flows that still invoke the Python process.
echo "Running database migrations..."
if command -v rebecca >/dev/null 2>&1; then
    MIGRATE_CMD="rebecca migrate up"
elif [ -x "/usr/local/bin/rebecca" ]; then
    MIGRATE_CMD="/usr/local/bin/rebecca migrate up"
else
    MIGRATE_CMD=""
fi

if [ -n "$MIGRATE_CMD" ]; then
    timeout 300 $MIGRATE_CMD || {
        echo "Migration failed or timed out, but continuing..."
        echo "You may need to run migrations manually: rebecca migrate up"
    }
else
    echo "Rebecca Go CLI not found; skipping explicit entrypoint migration."
    echo "The Go gateway runs migrations on startup when it is the active process."
fi

# Start the application
python main.py
