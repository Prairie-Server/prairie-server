#!/usr/bin/env bash
# One-time setup for the rsync+air dev workflow on the Prairie LXC.
# Installs Go, Node, pnpm, libvips, jellyfin-ffmpeg7, air, and tmux.
# Usage: DEV_HOST=root@prairie-dev.example.invalid ./scripts/dev-remote-setup.sh
set -euo pipefail

DEV_HOST="${DEV_HOST:-root@prairie-dev.example.invalid}"
DEV_DIR="${DEV_DIR:-/opt/git/prairie-dev}"

echo "==> Setting up dev environment on ${DEV_HOST}..."

ssh "${DEV_HOST}" bash -s -- "${DEV_DIR}" <<'REMOTE'
set -euo pipefail
DEV_DIR="$1"

export DEBIAN_FRONTEND=noninteractive

# --- Go 1.26 ---
if ! command -v go &>/dev/null; then
    echo "==> Installing Go 1.26..."
    curl -fsSL https://go.dev/dl/go1.26.0.linux-amd64.tar.gz | tar -C /usr/local -xz
    cat >> /etc/profile.d/golang.sh <<'EOF'
export PATH=$PATH:/usr/local/go/bin:/root/go/bin
EOF
    export PATH=$PATH:/usr/local/go/bin:/root/go/bin
else
    echo "==> Go already installed: $(go version)"
fi
export PATH=$PATH:/usr/local/go/bin:/root/go/bin

# --- System packages (libvips, tmux, build tools) ---
echo "==> Installing system packages..."
apt-get update -qq
apt-get install -y --no-install-recommends \
    build-essential pkg-config \
    libvips-dev \
    tmux \
    ca-certificates curl gnupg

# --- ffmpeg (symlinked to jellyfin-ffmpeg path for compatibility) ---
if ! command -v ffmpeg &>/dev/null; then
    echo "==> Installing ffmpeg..."
    apt-get install -y --no-install-recommends ffmpeg
else
    echo "==> ffmpeg already installed: $(ffmpeg -version 2>&1 | head -1)"
fi
mkdir -p /usr/lib/jellyfin-ffmpeg
ln -sf /usr/bin/ffmpeg /usr/lib/jellyfin-ffmpeg/ffmpeg
ln -sf /usr/bin/ffprobe /usr/lib/jellyfin-ffmpeg/ffprobe

# --- Node 22 + pnpm ---
if ! command -v node &>/dev/null; then
    echo "==> Installing Node 22..."
    curl -fsSL https://deb.nodesource.com/setup_22.x | bash -
    apt-get install -y --no-install-recommends nodejs
else
    echo "==> Node already installed: $(node --version)"
fi

if ! command -v pnpm &>/dev/null; then
    echo "==> Installing pnpm..."
    corepack enable
    corepack prepare pnpm@latest --activate
else
    echo "==> pnpm already installed: $(pnpm --version)"
fi

# --- air (Go hot-reload) ---
if ! command -v air &>/dev/null; then
    echo "==> Installing air..."
    go install github.com/air-verse/air@latest
else
    echo "==> air already installed: $(air -v 2>&1 | head -1)"
fi

# --- Create directories ---
mkdir -p "${DEV_DIR}/Prairie/web/dist"
mkdir -p "${DEV_DIR}/prairie-plugin-sdk"
mkdir -p /tmp/prairie-transcode
mkdir -p /opt/prairie/plugins /opt/prairie/transcode /opt/prairie/postgres /opt/prairie/redis

# Jellycompat debug logging bind-mounts a file, not a directory.
DEBUG_LOG_PATH="/opt/prairie/jellycompat-debug.log"
if [ -d "${DEBUG_LOG_PATH}" ]; then
    if ! rmdir "${DEBUG_LOG_PATH}" 2>/dev/null; then
        mv "${DEBUG_LOG_PATH}" "${DEBUG_LOG_PATH}.dir.$(date +%s)"
    fi
fi
touch "${DEBUG_LOG_PATH}"

# Symlink plugin cache so DB paths (using Docker's /var/lib/prairie/plugins)
# resolve correctly when running natively.
mkdir -p /var/lib/prairie
if [ ! -L /var/lib/prairie/plugins ] && [ -d /opt/prairie/plugins ]; then
    ln -sf /opt/prairie/plugins /var/lib/prairie/plugins
fi

# --- Create .env for native execution ---
ENV_FILE="${DEV_DIR}/Prairie/.env"
if [ ! -f "${ENV_FILE}" ]; then
    echo "==> Creating ${ENV_FILE}..."
    cat > "${ENV_FILE}" <<'EOF'
DATABASE_URL=postgres://prairie:prairie@localhost:5432/prairie?sslmode=disable
REDIS_URL=redis://localhost:6379
PORT=8090
JF_PORT=8096
MODE=integrated
PRAIRIE_PLUGIN_CACHE_DIR=/var/lib/prairie/plugins
EOF
else
    echo "==> ${ENV_FILE} already exists, skipping"
fi

echo ""
echo "=== Setup complete ==="
echo "Go:       $(go version)"
echo "Node:     $(node --version)"
echo "pnpm:     $(pnpm --version)"
echo "air:      $(air -v 2>&1 | head -1)"
echo "ffmpeg:   $(/usr/lib/jellyfin-ffmpeg/ffmpeg -version 2>&1 | head -1)"
echo "Dev dir:  ${DEV_DIR}"
REMOTE

echo "==> Done! Run 'make dev-deploy' to start developing."
