#!/usr/bin/env bash

# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# install.sh - Install lore, star and writ, and register the layers given
# for documentation: install.sh --help

set -o errexit -o nounset -o pipefail

# Where the DevLore site serves this script. The site releases from develop, so this is its develop environment.
INSTALLERS_URL="https://delightful-grass-0ac0a4c1e-develop.westus2.6.azurestaticapps.net"

usage() {
    cat <<EOF
Usage: install.sh [--prefix=<dir>] [--base=<loc>] [--team=<loc>] [--personal=<loc>]

Installs lore, star and writ into <prefix> (default ~/.local), then registers each layer given with
writ repo set, base first. A layer not given is skipped and named at the end. Never asks.

  --prefix=<dir>     installation prefix (default: ~/.local)
  --base=<loc>       the base layer: a working-tree root or a repository URL (or DEVLORE_BASE)
  --team=<loc>       the team layer (or DEVLORE_TEAM)
  --personal=<loc>   the personal layer (or DEVLORE_PERSONAL)
  -h, --help         show this help and exit

A flag wins over its variable. Running the same command again is safe: it is also how to recover from a failure.

Environment:
  DEVLORE_VERSION    a release tag to install (default: the newest release, pre-releases included)
  DEVLORE_TOOLS      all, writ, lore or star (default: all)
  GH_TOKEN           optional; sent to GitHub's API, which lifts its limit of 60 anonymous requests an hour

Served by the DevLore site's develop environment, from which devlore is released today:
  curl --fail --silent --show-error --location ${INSTALLERS_URL}/install.sh | bash -s -- --base=<loc> --team=<loc> --personal=<loc>
EOF
}

# Parse arguments. Each layer's variable is its default; a flag wins over it (#950).
PREFIX=""
BASE="${DEVLORE_BASE:-}"
TEAM="${DEVLORE_TEAM:-}"
PERSONAL="${DEVLORE_PERSONAL:-}"
for arg in "$@"; do
    case "$arg" in
        --prefix=*) PREFIX="${arg#*=}" ;;
        --base=*) BASE="${arg#*=}" ;;
        --team=*) TEAM="${arg#*=}" ;;
        --personal=*) PERSONAL="${arg#*=}" ;;
        --help | -h)
            usage
            exit 0
            ;;
        *)
            # A typo such as --bsae=... would otherwise be dropped, and its layer reported skipped.
            printf 'error: unknown argument: %s\n\n' "$arg" >&2
            usage >&2
            exit 1
            ;;
    esac
done

# The download directory, removed on every exit. Script scope, not local to main: the EXIT trap runs
# after main has returned, and under set -u a local that is gone is an error (#958).
TMP_DIR=""
cleanup() {
    if [[ -n "${TMP_DIR}" ]]; then
        rm -rf "${TMP_DIR}"
    fi
}
trap cleanup EXIT

# Configuration
GITHUB_REPO="NobleFactor/devlore-cli"
GITHUB_API="https://api.github.com/repos/${GITHUB_REPO}"
PREFIX="${PREFIX:-$HOME/.local}"
INSTALL_DIR="${PREFIX}/bin"
VERSION="${DEVLORE_VERSION:-latest}"
TOOLS="${DEVLORE_TOOLS:-all}"

# GitHub authentication (optional). The repository is public; a token only lifts the API's anonymous rate limit.
# Per https://docs.github.com/en/rest/releases/assets
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
error() {
    echo -e "${RED}error:${NC} $*" >&2
    exit 1
}

# Detect OS
detect_os() {
    case "$(uname -s)" in
        Linux*) echo "linux" ;;
        Darwin*) echo "darwin" ;;
        MINGW* | MSYS* | CYGWIN*) echo "windows" ;;
        *) error "Unsupported operating system: $(uname -s)" ;;
    esac
}

# Detect architecture. The releases publish amd64 and arm64 only.
detect_arch() {
    case "$(uname -m)" in
        x86_64 | amd64) echo "amd64" ;;
        arm64 | aarch64) echo "arm64" ;;
        *) error "Unsupported architecture: $(uname -m)" ;;
    esac
}

# Make an API request, authenticated when GH_TOKEN is set
# Per https://docs.github.com/en/rest/releases/releases
api_get() {
    local url="$1"
    if command -v curl &>/dev/null; then
        if [[ -n "$AUTH_HEADER" ]]; then
            curl --silent --show-error --location --header "Accept: application/vnd.github+json" --header "$AUTH_HEADER" "$url"
        else
            curl --silent --show-error --location --header "Accept: application/vnd.github+json" "$url"
        fi
    elif command -v wget &>/dev/null; then
        if [[ -n "$AUTH_HEADER" ]]; then
            wget --quiet --output-document=- --header="Accept: application/vnd.github+json" --header="$AUTH_HEADER" "$url"
        else
            wget --quiet --output-document=- --header="Accept: application/vnd.github+json" "$url"
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
    echo "$response" | grep --only-matching '"tag_name"[[:space:]]*:[[:space:]]*"[^"]*"' | awk 'NR == 1' | sed 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/'
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
    echo "$release_json" | grep --before-context=5 "\"name\"[[:space:]]*:[[:space:]]*\"${asset_name}\"" | grep --only-matching '"id"[[:space:]]*:[[:space:]]*[0-9]*' | awk 'NR == 1' | sed 's/.*:[[:space:]]*//'
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
            curl --silent --show-error --location --header "Accept: application/octet-stream" --header "$AUTH_HEADER" "$url" --output "$dest"
        else
            curl --silent --show-error --location --header "Accept: application/octet-stream" "$url" --output "$dest"
        fi
    elif command -v wget &>/dev/null; then
        if [[ -n "$AUTH_HEADER" ]]; then
            wget --quiet --header="Accept: application/octet-stream" --header="$AUTH_HEADER" "$url" --output-document="$dest"
        else
            wget --quiet --header="Accept: application/octet-stream" "$url" --output-document="$dest"
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
        actual=$(shasum --algorithm 256 "$file" | awk '{print $1}')
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

    # A layer is registered by the writ this run installs, so a run that leaves writ out cannot register one. Refused
    # before anything is downloaded (#950).
    if [[ -n "${BASE}${TEAM}${PERSONAL}" && "$TOOLS" != "all" && "$TOOLS" != "writ" ]]; then
        error "--base, --team and --personal register layers with writ, which DEVLORE_TOOLS=${TOOLS} does not install"
    fi

    # Detect platform
    local os
    local arch
    os=$(detect_os)
    arch=$(detect_arch)
    info "Detected platform: ${os}/${arch}"

    # Resolve version
    if [[ "$VERSION" == "latest" ]]; then
        info "Fetching latest version..."
        VERSION=$(get_latest_version)
        if [[ -z "$VERSION" ]]; then
            error "Could not determine the latest release of ${GITHUB_REPO}"
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
        error "Empty response from GitHub's API"
    fi
    if echo "$release_json" | grep --quiet '"message"'; then
        local api_error
        api_error=$(echo "$release_json" | grep --only-matching '"message"[[:space:]]*:[[:space:]]*"[^"]*"' | sed 's/.*:[[:space:]]*"\([^"]*\)".*/\1/')
        error "GitHub API error: $api_error"
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

    # Create temp directory; cleanup, trapped at script scope, removes it on every exit. mktemp, rm, mkdir and unzip
    # keep their short options: macOS's BSD tools have no long forms, and Info-ZIP has none anywhere.
    TMP_DIR=$(mktemp -d)

    # Download archive via GitHub API
    info "Downloading ${archive_name}..."
    download_asset "$archive_id" "${TMP_DIR}/${archive_name}"

    # Download and verify checksum
    if [[ -n "$checksums_id" ]]; then
        info "Verifying checksum..."
        download_asset "$checksums_id" "${TMP_DIR}/checksums.txt"
        local expected_checksum
        expected_checksum=$(grep "${archive_name}" "${TMP_DIR}/checksums.txt" | awk '{print $1}')
        if [[ -n "$expected_checksum" ]]; then
            verify_checksum "${TMP_DIR}/${archive_name}" "$expected_checksum"
            success "Checksum verified"
        else
            warn "Checksum not found for ${archive_name}, skipping verification"
        fi
    else
        warn "Checksums file not found, skipping verification"
    fi

    # Extract archive
    #
    # The archive holds the products at its root and star's extensions under share/ (#903). The products move to
    # pkg/bin so that each one's `self install` finds pkg/share at <exeDir>/../share, the path star copies its
    # extensions from.
    info "Extracting..."
    local pkg="${TMP_DIR}/pkg"
    mkdir -p "${pkg}/bin"
    if [[ "$ext" == "tar.gz" ]]; then
        tar --extract --gzip --file "${TMP_DIR}/${archive_name}" --directory "${pkg}"
    else
        unzip -q "${TMP_DIR}/${archive_name}" -d "${pkg}"
    fi

    # Install binaries
    #
    # Every file at the archive root is a product, so this list is the archive's and not a second copy of the
    # Makefile's. Each product installs itself: `self install <prefix>` copies the binary to <prefix>/bin and adds
    # its man pages, completions and, for star, its extensions. A failure means that product is not installed.
    mkdir -p "$INSTALL_DIR"
    local installed=()
    local file name product
    for file in "${pkg}"/*; do
        [[ -f "$file" ]] || continue
        name="${file##*/}"
        product="${name%.exe}"
        [[ "$TOOLS" == "all" || "$TOOLS" == "$product" ]] || continue
        mv "$file" "${pkg}/bin/${name}"
        chmod +x "${pkg}/bin/${name}"
        info "Installing ${product}..."
        (cd "$pkg" && "bin/${name}" self install "$PREFIX" --unattended) || error "${product} self install failed"
        installed+=("$product")
    done

    if [[ ${#installed[@]} -eq 0 ]]; then
        error "No binaries found in archive for DEVLORE_TOOLS=${TOOLS}"
    fi

    # Register the layers given, base first, with the writ just installed (#950). The call runs in the user's working
    # directory, so a relative location resolves where it was typed. writ's output is its own, and so are its errors:
    # a failure ends the run, and running the same command again is the recovery. --unattended is writ's contract
    # for a run nobody is there to answer.
    local writ="${INSTALL_DIR}/writ"
    if [[ "$os" == "windows" ]]; then
        writ="${writ}.exe"
    fi
    local registered=()
    local skipped=()
    local layer location
    for layer in base team personal; do
        case "$layer" in
            base) location="$BASE" ;;
            team) location="$TEAM" ;;
            *) location="$PERSONAL" ;;
        esac
        if [[ -z "$location" ]]; then
            skipped+=("$layer")
            continue
        fi
        info "Registering ${layer}: ${location}"
        "$writ" repo set "$layer" "$location" --unattended
        registered+=("$layer")
    done

    # The summary comes last, after writ's output, and its last lines are the layers skipped.
    echo
    success "Installed: ${installed[*]}"
    success "Location: ${INSTALL_DIR}"
    if [[ ${#registered[@]} -gt 0 ]]; then
        success "Registered: ${registered[*]}"
    fi
    echo

    # Check if install dir is in PATH. The advice names the directory this run installed into, written from $HOME
    # when it is under it, so that --prefix gets advice that works.
    if [[ ":$PATH:" != *":${INSTALL_DIR}:"* ]]; then
        local shown="$INSTALL_DIR"
        if [[ "$INSTALL_DIR" == "$HOME"/* ]]; then
            shown="\$HOME/${INSTALL_DIR#"$HOME"/}"
        fi
        warn "${INSTALL_DIR} is not in your PATH"
        echo
        echo "Add it to your shell profile:"
        echo
        echo "  # For bash (~/.bashrc or ~/.bash_profile)"
        echo "  export PATH=\"${shown}:\$PATH\""
        echo
        echo "  # For zsh (~/.zshrc)"
        echo "  export PATH=\"${shown}:\$PATH\""
        echo
        echo "  # For fish (~/.config/fish/config.fish)"
        echo "  fish_add_path ${shown}"
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
    info "Documentation: https://github.com/NobleFactor/devlore-cli#readme"

    if [[ ${#registered[@]} -gt 0 || ${#skipped[@]} -gt 0 ]]; then
        echo
        info "Next steps:"
    fi
    if [[ ${#registered[@]} -gt 0 ]]; then
        echo "  writ deploy"
    fi
    if [[ ${#skipped[@]} -gt 0 ]]; then
        for layer in "${skipped[@]}"; do
            echo "skipped: ${layer}; to register it later:"
            echo "  writ repo set ${layer} <working-tree-root>|<repository-url>"
        done
    fi
}

main "$@"
