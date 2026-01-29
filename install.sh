#!/bin/bash
# SPDX-License-Identifier: MIT
# Copyright (c) 2025 Noble Factor. All rights reserved.
#
# DevLore CLI Installer
# Usage: curl -sSL https://devlore.noblefactor.com/install.sh | bash
#        curl -sSL https://devlore.noblefactor.com/install.sh | bash -s -- --prefix=/opt/devlore
#
# For private repo (requires GitHub token):
#   curl -sSL https://devlore.noblefactor.com/install.sh | GH_TOKEN=$(unset GITHUB_TOKEN GH_TOKEN; gh auth token) bash
#
# Arguments:
#   --prefix=<dir>       - Installation prefix (default: ~/.local per XDG)
#                          Binaries go to <prefix>/bin, man pages to <prefix>/share/man, etc.
#
# Environment variables:
#   GH_TOKEN             - GitHub token for private repo access
#                          Use: GH_TOKEN=$(gh auth token) for OAuth token
#   DEVLORE_VERSION      - Version to install (default: latest)
#                          "latest" installs the most recent release (including prereleases)
#                          Set explicitly (e.g., "v1.0.0") for a specific version
#   DEVLORE_TOOLS        - Tools to install: "all", "writ", "lore" (default: all)
#
# Documentation references:
#   - GitHub Releases API: https://docs.github.com/en/rest/releases/releases
#   - GitHub Release Assets API: https://docs.github.com/en/rest/releases/assets

set -euo pipefail

# Parse arguments
PREFIX=""
for arg in "$@"; do
    case "$arg" in
        --prefix=*) PREFIX="${arg#*=}" ;;
        --help|-h)
            echo "Usage: install.sh [--prefix=<dir>]"
            echo "  --prefix=<dir>  Installation prefix (default: ~/.local)"
            exit 0
            ;;
    esac
done

# Configuration
GITHUB_REPO="NobleFactor/devlore-cli"
GITHUB_API="https://api.github.com/repos/${GITHUB_REPO}"
PREFIX="${PREFIX:-$HOME/.local}"
INSTALL_DIR="${PREFIX}/bin"
VERSION="${DEVLORE_VERSION:-latest}"
TOOLS="${DEVLORE_TOOLS:-all}"

# GitHub authentication (required for private repo)
# Per https://docs.github.com/en/rest/releases/assets - requires "Contents" read permission
# Note: Use "token" not "Bearer" for OAuth tokens from gh auth
AUTH_HEADER=""
if [[ -n "${GH_TOKEN:-}" ]]; then
    AUTH_HEADER="Authorization: token ${GH_TOKEN}"
fi

# Colors (disabled if not a terminal)
if [[ -t 1 ]]; then
    RED='\033[0;31m'
    GREEN='\033[0;32m'
    YELLOW='\033[0;33m'
    BLUE='\033[0;34m'
    NC='\033[0m' # No Color
else
    RED=''
    GREEN=''
    YELLOW=''
    BLUE=''
    NC=''
fi

info() { echo -e "${BLUE}info:${NC} $*"; }
success() { echo -e "${GREEN}success:${NC} $*"; }
warn() { echo -e "${YELLOW}warning:${NC} $*"; }
error() { echo -e "${RED}error:${NC} $*" >&2; exit 1; }

# Detect OS
detect_os() {
    case "$(uname -s)" in
        Linux*)  echo "linux" ;;
        Darwin*) echo "darwin" ;;
        MINGW*|MSYS*|CYGWIN*) echo "windows" ;;
        *) error "Unsupported operating system: $(uname -s)" ;;
    esac
}

# Detect architecture
detect_arch() {
    case "$(uname -m)" in
        x86_64|amd64) echo "amd64" ;;
        arm64|aarch64) echo "arm64" ;;
        armv7l) echo "armv7" ;;
        *) error "Unsupported architecture: $(uname -m)" ;;
    esac
}

# Make authenticated API request
# Per https://docs.github.com/en/rest/releases/releases
api_get() {
    local url="$1"
    if command -v curl &>/dev/null; then
        if [[ -n "$AUTH_HEADER" ]]; then
            curl -sSL -H "Accept: application/vnd.github+json" -H "$AUTH_HEADER" "$url"
        else
            curl -sSL -H "Accept: application/vnd.github+json" "$url"
        fi
    elif command -v wget &>/dev/null; then
        if [[ -n "$AUTH_HEADER" ]]; then
            wget -qO- --header="Accept: application/vnd.github+json" --header="$AUTH_HEADER" "$url"
        else
            wget -qO- --header="Accept: application/vnd.github+json" "$url"
        fi
    else
        error "Neither curl nor wget found. Please install one of them."
    fi
}

# Get latest release version from GitHub API
# Per https://docs.github.com/en/rest/releases/releases#list-releases
# Uses /releases?per_page=1 to get the most recent release (including prereleases)
# Note: /releases/latest excludes prereleases, so we use the list endpoint instead
get_latest_version() {
    local url="${GITHUB_API}/releases?per_page=1"
    local response
    response=$(api_get "$url")
    # Extract tag_name from JSON response (first item in array)
    echo "$response" | grep -o '"tag_name"[[:space:]]*:[[:space:]]*"[^"]*"' | head -1 | sed 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/'
}

# Get release by tag
# Per https://docs.github.com/en/rest/releases/releases#get-a-release-by-tag-name
get_release_by_tag() {
    local tag="$1"
    local url="${GITHUB_API}/releases/tags/${tag}"
    api_get "$url"
}

# Extract asset ID from release JSON by filename
# The release response contains an "assets" array with id, name, browser_download_url
get_asset_id() {
    local release_json="$1"
    local asset_name="$2"
    # Extract asset id where name matches
    echo "$release_json" | grep -B5 "\"name\"[[:space:]]*:[[:space:]]*\"${asset_name}\"" | grep -o '"id"[[:space:]]*:[[:space:]]*[0-9]*' | head -1 | sed 's/.*:[[:space:]]*//'
}

# Download release asset by ID
# Per https://docs.github.com/en/rest/releases/assets#get-a-release-asset
# Must use Accept: application/octet-stream to get binary content
download_asset() {
    local asset_id="$1"
    local dest="$2"
    local url="${GITHUB_API}/releases/assets/${asset_id}"

    if command -v curl &>/dev/null; then
        if [[ -n "$AUTH_HEADER" ]]; then
            curl -sSL -H "Accept: application/octet-stream" -H "$AUTH_HEADER" "$url" -o "$dest"
        else
            curl -sSL -H "Accept: application/octet-stream" "$url" -o "$dest"
        fi
    elif command -v wget &>/dev/null; then
        if [[ -n "$AUTH_HEADER" ]]; then
            wget -q --header="Accept: application/octet-stream" --header="$AUTH_HEADER" "$url" -O "$dest"
        else
            wget -q --header="Accept: application/octet-stream" "$url" -O "$dest"
        fi
    else
        error "Neither curl nor wget found. Please install one of them."
    fi
}

# Verify checksum
verify_checksum() {
    local file="$1"
    local expected="$2"

    local actual
    if command -v sha256sum &>/dev/null; then
        actual=$(sha256sum "$file" | awk '{print $1}')
    elif command -v shasum &>/dev/null; then
        actual=$(shasum -a 256 "$file" | awk '{print $1}')
    else
        warn "No sha256sum or shasum found, skipping checksum verification"
        return 0
    fi

    if [[ "$actual" != "$expected" ]]; then
        error "Checksum verification failed!\nExpected: $expected\nActual:   $actual"
    fi
}

# Main installation
main() {
    info "DevLore CLI Installer"
    echo

    # Check for auth token (required for private repo)
    if [[ -z "$AUTH_HEADER" ]]; then
        warn "No GH_TOKEN set. This will fail for private repositories."
        warn "Set GH_TOKEN with a token that has 'Contents' read permission."
    fi

    # Detect platform
    local os=$(detect_os)
    local arch=$(detect_arch)
    info "Detected platform: ${os}/${arch}"

    # Resolve version
    if [[ "$VERSION" == "latest" ]]; then
        info "Fetching latest version..."
        VERSION=$(get_latest_version)
        if [[ -z "$VERSION" ]]; then
            error "Could not determine latest version. Check GH_TOKEN has correct permissions.\nFor private repos, token needs 'Contents' read permission."
        fi
    fi
    info "Version: $VERSION"

    # Get release info
    info "Fetching release info..."
    local release_json
    release_json=$(get_release_by_tag "$VERSION")
    # Check for any API error - GitHub API returns "message" field on errors
    # Per https://docs.github.com/en/rest/releases/releases
    if [[ -z "$release_json" ]]; then
        error "Empty response from GitHub API. Check GH_TOKEN is set and valid."
    fi
    if echo "$release_json" | grep -q '"message"'; then
        local api_error
        api_error=$(echo "$release_json" | grep -o '"message"[[:space:]]*:[[:space:]]*"[^"]*"' | sed 's/.*:\s*"\([^"]*\)".*/\1/')
        error "GitHub API error: $api_error\nCheck GH_TOKEN has Contents read permission."
    fi

    # Determine archive extension
    local ext="tar.gz"
    if [[ "$os" == "windows" ]]; then
        ext="zip"
    fi

    # Build asset names
    local archive_name="devlore-cli_${VERSION}_${os}_${arch}.${ext}"
    local checksums_name="devlore-cli_${VERSION}_checksums.txt"

    # Get asset IDs
    local archive_id
    archive_id=$(get_asset_id "$release_json" "$archive_name")
    if [[ -z "$archive_id" ]]; then
        error "Asset $archive_name not found in release $VERSION"
    fi

    local checksums_id
    checksums_id=$(get_asset_id "$release_json" "$checksums_name")

    # Create temp directory
    local tmp_dir
    tmp_dir=$(mktemp -d)
    trap "rm -rf '$tmp_dir'" EXIT

    # Download archive via GitHub API
    info "Downloading ${archive_name}..."
    download_asset "$archive_id" "${tmp_dir}/${archive_name}"

    # Download and verify checksum
    if [[ -n "$checksums_id" ]]; then
        info "Verifying checksum..."
        download_asset "$checksums_id" "${tmp_dir}/checksums.txt"
        local expected_checksum
        expected_checksum=$(grep "${archive_name}" "${tmp_dir}/checksums.txt" | awk '{print $1}')
        if [[ -n "$expected_checksum" ]]; then
            verify_checksum "${tmp_dir}/${archive_name}" "$expected_checksum"
            success "Checksum verified"
        else
            warn "Checksum not found for ${archive_name}, skipping verification"
        fi
    else
        warn "Checksums file not found, skipping verification"
    fi

    # Extract archive
    info "Extracting..."
    if [[ "$ext" == "tar.gz" ]]; then
        tar -xzf "${tmp_dir}/${archive_name}" -C "${tmp_dir}"
    else
        unzip -q "${tmp_dir}/${archive_name}" -d "${tmp_dir}"
    fi

    # Create install directory
    mkdir -p "$INSTALL_DIR"

    # Install binaries
    local installed=()
    if [[ "$TOOLS" == "all" || "$TOOLS" == "writ" ]]; then
        local writ_bin="writ"
        [[ "$os" == "windows" ]] && writ_bin="writ.exe"
        if [[ -f "${tmp_dir}/${writ_bin}" ]]; then
            mv "${tmp_dir}/${writ_bin}" "${INSTALL_DIR}/${writ_bin}"
            chmod +x "${INSTALL_DIR}/${writ_bin}"
            installed+=("writ")
        fi
    fi

    if [[ "$TOOLS" == "all" || "$TOOLS" == "lore" ]]; then
        local lore_bin="lore"
        [[ "$os" == "windows" ]] && lore_bin="lore.exe"
        if [[ -f "${tmp_dir}/${lore_bin}" ]]; then
            mv "${tmp_dir}/${lore_bin}" "${INSTALL_DIR}/${lore_bin}"
            chmod +x "${INSTALL_DIR}/${lore_bin}"
            installed+=("lore")
        fi
    fi

    if [[ ${#installed[@]} -eq 0 ]]; then
        error "No binaries found in archive"
    fi

    # Run self-install for each tool to install man pages and completions
    for tool in "${installed[@]}"; do
        info "Running ${tool} self-install..."
        "${INSTALL_DIR}/${tool}" self-install "$PREFIX" --unattended || warn "${tool} self-install failed"
    done

    echo
    success "Installed: ${installed[*]}"
    success "Location: ${INSTALL_DIR}"
    echo

    # Check if install dir is in PATH
    if [[ ":$PATH:" != *":${INSTALL_DIR}:"* ]]; then
        warn "${INSTALL_DIR} is not in your PATH"
        echo
        echo "Add it to your shell profile:"
        echo
        echo "  # For bash (~/.bashrc or ~/.bash_profile)"
        echo "  export PATH=\"\$HOME/.local/bin:\$PATH\""
        echo
        echo "  # For zsh (~/.zshrc)"
        echo "  export PATH=\"\$HOME/.local/bin:\$PATH\""
        echo
        echo "  # For fish (~/.config/fish/config.fish)"
        echo "  fish_add_path \$HOME/.local/bin"
        echo
    fi

    # Verify installation
    if [[ ":$PATH:" == *":${INSTALL_DIR}:"* ]]; then
        echo "Verify installation:"
        for tool in "${installed[@]}"; do
            echo "  ${tool} --version"
        done
    fi

    echo
    info "Next steps:"
    echo "  Adopt files:      writ adopt --project <name> <file>..."
    echo "  Migrate existing: writ migrate <directory>"
    echo
    info "Documentation: https://devlore.noblefactor.com"
}

main "$@"
