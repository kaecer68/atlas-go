#!/usr/bin/env bash
# =============================================================================
# install-atlas-mcp-from-release.sh — 投資人 hermes/openclaw agent 一鍵安裝
#
# 對象: 沒有 Go toolchain 的外部 AI agent operator
# 下載: 從 GitHub Releases 拉 atlas-mcp binary + SHA256 verify → ~/.local/bin/
# 不依賴: atlas-go source tree（投資人不該需要 clone repo）
#
# 用法:
#   curl -fsSL https://raw.githubusercontent.com/kaecer68/atlas-go/main/scripts/install-atlas-mcp-from-release.sh | bash
#
#   # 指定版本（推薦，reproducible）
#   curl -fsSL ... | bash -s -- --version v0.0.0.33
#
#   # 預覽不安裝
#   curl -fsSL ... | bash -s -- --dry-run
#
#   # 指定安裝路徑
#   curl -fsSL ... | bash -s -- --prefix /opt/atlas
#
# Exit codes:
#   0 success | 1 user error | 2 network | 3 checksum mismatch | 4 unsupported OS
# =============================================================================

set -euo pipefail

# ---------- defaults ----------
GITHUB_REPO="kaecer68/atlas-go"
VERSION="latest"
PREFIX="${HOME}/.local/bin"
DRY_RUN=false
VERBOSE=false

# ---------- usage ----------
usage() {
    cat <<EOF
Usage: $0 [OPTIONS]

Options:
  --version <tag>    Install specific version (e.g. v0.0.0.33). Default: latest
  --prefix <dir>     Install to <dir>/atlas-mcp. Default: \$HOME/.local/bin
  --dry-run          Show what would be done, do not install
  --verbose          Print extra debugging
  -h, --help         Show this help

Examples:
  # Install latest to ~/.local/bin
  $0

  # Install specific version
  $0 --version v0.0.0.33

  # Preview without installing
  $0 --dry-run

EOF
}

# ---------- arg parse ----------
while [[ $# -gt 0 ]]; do
    case "$1" in
        --version)  VERSION="$2"; shift 2 ;;
        --prefix)   PREFIX="$2"; shift 2 ;;
        --dry-run)  DRY_RUN=true; shift ;;
        --verbose)  VERBOSE=true; shift ;;
        -h|--help)   usage; exit 0 ;;
        *)          echo "Unknown option: $1" >&2; usage; exit 1 ;;
    esac
done

log() { echo "[install-atlas-mcp] $*" >&2; }
vlog() { $VERBOSE && echo "[install-atlas-mcp] DEBUG: $*" >&2 || true; }

# ---------- OS / arch detection ----------
detect_os_arch() {
    local os arch
    case "$(uname -s)" in
        Darwin)  os="darwin" ;;
        Linux)   os="linux" ;;
        *)       log "Unsupported OS: $(uname -s). Only macOS and Linux."; exit 4 ;;
    esac
    case "$(uname -m)" in
        x86_64)  arch="amd64" ;;
        arm64)   arch="arm64" ;;
        aarch64) arch="arm64" ;;
        *)       log "Unsupported arch: $(uname -m). Only amd64 and arm64."; exit 4 ;;
    esac
    echo "${os}-${arch}"
}

OS_ARCH=$(detect_os_arch)
vlog "Detected: ${OS_ARCH}"

# ---------- resolve version → tag ----------
resolve_version_tag() {
    if [[ "${VERSION}" != "latest" ]]; then
        echo "${VERSION}"
        return
    fi
    local latest_url="https://api.github.com/repos/${GITHUB_REPO}/releases/latest"
    vlog "Fetching ${latest_url}"
    local tag
    tag=$(curl -fsS --max-time 15 -H 'Accept: application/vnd.github+json' \
        "${latest_url}" 2>/dev/null \
        | grep -m1 '"tag_name"' | sed -E 's/.*"tag_name":[[:space:]]*"([^"]+)".*/\1/')
    if [[ -z "${tag}" ]]; then
        log "ERROR: Could not resolve 'latest' from GitHub API."
        log "  Hint: network issue, or no release published yet."
        log "  Workaround: --version v0.0.0.XX with a known-published tag."
        exit 2
    fi
    echo "${tag}"
}

TAG=$(resolve_version_tag)
log "Target version: ${TAG} (${OS_ARCH})"

# ---------- download URLs ----------
ASSET_NAME="atlas-mcp-${OS_ARCH}.tar.gz"
ASSET_URL="https://github.com/${GITHUB_REPO}/releases/download/${TAG}/${ASSET_NAME}"
CHECKSUM_URL="https://github.com/${GITHUB_REPO}/releases/download/${TAG}/checksums.txt"

vlog "Asset URL: ${ASSET_URL}"
vlog "Checksum URL: ${CHECKSUM_URL}"

# ---------- download to tmpdir ----------
TMPDIR_INSTALL=$(mktemp -d -t atlas-mcp-install.XXXXXX)
trap 'rm -rf "${TMPDIR_INSTALL}"' EXIT
TAR_PATH="${TMPDIR_INSTALL}/${ASSET_NAME}"
CHECKSUM_PATH="${TMPDIR_INSTALL}/checksums.txt"

download() {
    local url="$1"; local out="$2"; local label="$3"
    log "Downloading ${label}..."
    if ! curl -fsSL --max-time 120 --retry 3 --retry-delay 5 \
            -o "${out}" "${url}"; then
        log "ERROR: Failed to download ${label} from ${url}"
        log "  Hint: check network, or verify ${TAG} is a published release."
        exit 2
    fi
    vlog "  → ${out} ($(wc -c < "${out}") bytes)"
}

download "${ASSET_URL}" "${TAR_PATH}" "${ASSET_NAME}"
download "${CHECKSUM_URL}" "${CHECKSUM_PATH}" "checksums.txt"

# ---------- SHA256 verify ----------
EXPECTED_SHA=$(grep -E "^[^ ]+[[:space:]]+${ASSET_NAME}\$" "${CHECKSUM_PATH}" | awk '{print $1}')
if [[ -z "${EXPECTED_SHA}" ]]; then
    log "ERROR: ${ASSET_NAME} not found in checksums.txt"
    log "  The release may not include this asset, or the file is malformed."
    exit 3
fi
ACTUAL_SHA=$(shasum -a 256 "${TAR_PATH}" 2>/dev/null | awk '{print $1}') || \
ACTUAL_SHA=$(sha256sum "${TAR_PATH}" | awk '{print $1}')

log "Verifying SHA256..."
vlog "  expected: ${EXPECTED_SHA}"
vlog "  actual:   ${ACTUAL_SHA}"
if [[ "${EXPECTED_SHA}" != "${ACTUAL_SHA}" ]]; then
    log "ERROR: SHA256 mismatch!"
    log "  expected: ${EXPECTED_SHA}"
    log "  actual:   ${ACTUAL_SHA}"
    log "  Possible MITM or compromised release. Aborting."
    exit 3
fi
log "  ✓ SHA256 OK"

# ---------- extract ----------
EXTRACT_DIR="${TMPDIR_INSTALL}/extracted"
mkdir -p "${EXTRACT_DIR}"
tar -xzf "${TAR_PATH}" -C "${EXTRACT_DIR}"
EXTRACTED_BIN="${EXTRACT_DIR}/atlas-mcp"
if [[ ! -f "${EXTRACTED_BIN}" ]]; then
    log "ERROR: ${ASSET_NAME} does not contain 'atlas-mcp' binary"
    log "  Contents: $(ls -la "${EXTRACT_DIR}")"
    exit 3
fi

# ---------- install ----------
TARGET_PATH="${PREFIX%/}/atlas-mcp"

if ${DRY_RUN}; then
    log "[DRY-RUN] Would install:"
    log "  ${EXTRACTED_BIN} → ${TARGET_PATH}"
    log "  Mode: 0755 (executable)"
    log "  Verified SHA256: ${ACTUAL_SHA:0:16}..."
    exit 0
fi

log "Installing to ${TARGET_PATH}..."
mkdir -p "${PREFIX}"
install -m 0755 "${EXTRACTED_BIN}" "${TARGET_PATH}"
log "  ✓ Installed $(command -v atlas-mcp 2>/dev/null || echo "${TARGET_PATH}")"

# ---------- post-install hints ----------
echo ""
cat <<EOF
✅ atlas-mcp ${TAG} installed successfully.

Next steps:
  1. Make sure ${PREFIX} is in your PATH:
       export PATH="${PREFIX}:\$PATH"
       # or add to ~/.zshrc / ~/.bashrc

  2. Verify the binary works:
       atlas-mcp --help

  3. Configure your MCP client (Hermes / OpenClaw / Claude Desktop):
       See https://github.com/${GITHUB_REPO}#atlas-as-mcp-server

  4. The MCP server entry should use key 'atlas-mcp' (not 'atlas-go').
       \$ATLAS_BASE_URL must point to a running atlas-go backend.

Done.
EOF