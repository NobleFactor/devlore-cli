#!/bin/bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2025 Noble Factor. All rights reserved.
#
# DevLore CLI Installer
# Usage: curl -sSL https://devlore.noblefactor.com/install.sh | bash
#
# Options (via environment variables):
#   DEVLORE_INSTALL_DIR  - Installation directory (default: ~/.local/bin)
#   DEVLORE_VERSION      - Specific version to install (default: latest)
#   DEVLORE_TOOLS        - Tools to install: "all", "writ", "lore" (default: all)

set -euo pipefail

# Configuration
GITHUB_REPO="NobleFactor/devlore-cli"
INSTALL_DIR="${DEVLORE_INSTALL_DIR:-$HOME/.local/bin}"
VERSION="${DEVLORE_VERSION:-latest}"
TOOLS="${DEVLORE_TOOLS:-all}"

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

# Get latest version from GitHub API
get_latest_version() {
    local url="https://api.github.com/repos/${GITHUB_REPO}/releases/latest"
    if command -v curl &>/dev/null; then
        curl -sSL "$url" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/'
    elif command -v wget &>/dev/null; then
        wget -qO- "$url" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/'
    else
        error "Neither curl nor wget found. Please install one of them."
    fi
}

# Download file
download() {
    local url="$1"
    local dest="$2"

    if command -v curl &>/dev/null; then
        curl -sSL "$url" -o "$dest"
    elif command -v wget &>/dev/null; then
        wget -q "$url" -O "$dest"
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

    # Detect platform
    local os=$(detect_os)
    local arch=$(detect_arch)
    info "Detected platform: ${os}/${arch}"

    # Resolve version
    if [[ "$VERSION" == "latest" ]]; then
        info "Fetching latest version..."
        VERSION=$(get_latest_version)
        if [[ -z "$VERSION" ]]; then
            error "Could not determine latest version. GitHub API may be rate-limited.\nTry setting DEVLORE_VERSION explicitly."
        fi
    fi
    info "Version: $VERSION"

    # Determine archive extension
    local ext="tar.gz"
    if [[ "$os" == "windows" ]]; then
        ext="zip"
    fi

    # Build download URL
    local version_num="${VERSION#v}"  # Strip leading 'v' if present
    local archive_name="devlore-cli_${version_num}_${os}_${arch}.${ext}"
    local checksums_name="devlore-cli_${version_num}_checksums.txt"
    local base_url="https://github.com/${GITHUB_REPO}/releases/download/${VERSION}"
    local archive_url="${base_url}/${archive_name}"
    local checksums_url="${base_url}/${checksums_name}"

    # Create temp directory
    local tmp_dir
    tmp_dir=$(mktemp -d)
    trap "rm -rf '$tmp_dir'" EXIT

    # Download archive
    info "Downloading ${archive_name}..."
    download "$archive_url" "${tmp_dir}/${archive_name}"

    # Download and verify checksum
    info "Verifying checksum..."
    download "$checksums_url" "${tmp_dir}/checksums.txt"
    local expected_checksum
    expected_checksum=$(grep "${archive_name}" "${tmp_dir}/checksums.txt" | awk '{print $1}')
    if [[ -n "$expected_checksum" ]]; then
        verify_checksum "${tmp_dir}/${archive_name}" "$expected_checksum"
        success "Checksum verified"
    else
        warn "Checksum not found for ${archive_name}, skipping verification"
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
    echo "  1. Run 'writ migrate <your-dotfiles-dir>' to migrate existing dotfiles"
    echo "  2. Run 'writ add <projects>' to deploy configurations"
    echo "  3. Run 'lore install <package>' to install software"
    echo
    info "Documentation: https://devlore.noblefactor.com"
}

main "$@"
