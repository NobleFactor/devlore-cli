#!/usr/bin/env bash

# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# install.sh - Install lore, star and writ from an archive it has verified, and register the layers given
# for documentation: install.sh --help

set -o errexit -o nounset -o pipefail

# Where the DevLore site serves this script. The site releases from develop, so this is its develop environment.
INSTALLERS_URL="https://delightful-grass-0ac0a4c1e-develop.westus2.6.azurestaticapps.net"

usage() {
    cat <<EOF
Usage: install.sh [--prefix=<dir>] [--base=<loc>] [--team=<loc>] [--personal=<loc>]

Installs lore, star and writ into <prefix> (default ~/.local), then registers each layer given with
writ repo set, base first. A layer not given is skipped and named at the end. Never asks.

The release's archive is verified against its checksums file, with sha256sum or shasum, before anything is
extracted. An archive that can't be verified is not installed: a release without the checksums file, a checksums
file without the archive's line, a machine with neither tool and a mismatch are each an error. Both files are
downloaded with curl, from the release's public links.

  --prefix=<dir>     installation prefix (default: ~/.local)
  --base=<loc>       the base layer: a working-tree root or a repository URL (or DEVLORE_BASE)
  --team=<loc>       the team layer (or DEVLORE_TEAM)
  --personal=<loc>   the personal layer (or DEVLORE_PERSONAL)
  -h, --help         show this help and exit

A flag wins over its variable. Running the same command again is safe: it is also how to recover from a failure.

Environment:
  DEVLORE_VERSION    a release tag to install (default: the newest release, pre-releases included)
  DEVLORE_TOOLS      all, writ, lore or star (default: all)
  GH_TOKEN           optional; sent to GitHub's API alone, which lifts its limit of 60 anonymous requests an hour

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
# A release's files, each by its public link, <tag>/<name>: its browser_download_url, which self upgrade downloads too.
GITHUB_DOWNLOAD="https://github.com/${GITHUB_REPO}/releases/download"
PREFIX="${PREFIX:-$HOME/.local}"
INSTALL_DIR="${PREFIX}/bin"
VERSION="${DEVLORE_VERSION:-latest}"
# Whether GitHub has shown that release VERSION exists, by listing it as the newest or by serving one of its files. A
# release DEVLORE_VERSION names is not looked up before its files are downloaded: GitHub's API is asked for it only
# when a link answers 404 before then (#1008).
RELEASE_FOUND=false
TOOLS="${DEVLORE_TOOLS:-all}"

# GitHub authentication (optional). The repository is public; a token only lifts the API's anonymous rate limit, and is
# sent to the API alone, never to a download link.
# Per https://docs.github.com/en/rest/using-the-rest-api/rate-limits-for-the-rest-api
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

# GitHub's own message in the JSON on stdin, which its API sends when it refuses a request, or nothing
# Per https://docs.github.com/en/rest/using-the-rest-api/troubleshooting-the-rest-api
github_message() {
    grep --only-matching '"message"[[:space:]]*:[[:space:]]*"[^"]*"' | awk 'NR == 1' | sed 's/^"message"[[:space:]]*:[[:space:]]*"\(.*\)"$/\1/' || true
}

# The value of header name, given in lower case, in the last answer in a headers file curl dumped, or nothing.
header_value() {
    awk -v name="$2" '
        { sub(/\r$/, "") }
        /^HTTP\// { value = "" }
        tolower(substr($0, 1, length(name) + 1)) == name ":" {
            value = substr($0, length(name) + 2)
            sub(/^[ \t]+/, "", value)
        }
        END { print value }
    ' "$1"
}

# What to tell the user when GitHub's API has refused for its rate limit, from the headers file of its answer, or
# nothing when it refused for another reason. GitHub refuses a spent limit with 403 or 429, x-ratelimit-remaining 0, and
# x-ratelimit-reset, the time the limit resets in seconds since the epoch, which is said as this machine's clock shows
# it, by GNU date or BSD date, and in minutes, rounded up. GH_TOKEN raises the limit, which is said when it is unset.
# Per https://docs.github.com/en/rest/using-the-rest-api/rate-limits-for-the-rest-api
rate_limit_message() {
    local code="$1"
    local headers="$2"
    if [[ "$code" != "403" && "$code" != "429" ]] || [[ "$(header_value "$headers" x-ratelimit-remaining)" != "0" ]]; then
        return 0
    fi
    local reset
    reset=$(header_value "$headers" x-ratelimit-reset)
    if [[ ! "$reset" =~ ^[0-9]+$ ]]; then
        return 0
    fi
    # date's -r is BSD's, which has no long form; GNU date reads -r as a file, so --date is tried first.
    local clock
    clock=$(date --date="@${reset}" +%H:%M 2>/dev/null) || clock=$(date -r "$reset" +%H:%M 2>/dev/null) || clock=""
    if [[ ! "$clock" =~ ^[0-9][0-9]:[0-9][0-9]$ ]]; then
        return 0
    fi
    local now
    now=$(date +%s)
    local minutes=$(((reset - now + 59) / 60))
    if [[ $minutes -lt 1 ]]; then
        minutes=1
    fi
    local unit="minutes"
    if [[ $minutes -eq 1 ]]; then
        unit="minute"
    fi
    # GitHub counts the limit per token when one is sent, and per address when none is.
    local whose="this address"
    if [[ -n "${GH_TOKEN:-}" ]]; then
        whose="your token"
    fi
    printf "GitHub's API limit for %s is used up. It resets at %s (in %s %s); run the installer again after that." \
        "$whose" "$clock" "$minutes" "$unit"
    if [[ -z "${GH_TOKEN:-}" ]]; then
        printf ' Setting GH_TOKEN raises the limit.'
    fi
    printf '\n'
}

# Make an API request, authenticated when GH_TOKEN is set, and print GitHub's answer. A refusal is an error saying
# "Could not <what>", with GitHub's answer: curl reports the HTTP status rather than failing on it (no --fail), and a
# refusal's body is GitHub's message, which the error reports, followed, when the refusal is for GitHub's rate limit, by
# when to run the installer again (#1002).
# Per https://docs.github.com/en/rest/releases/releases
api_get() {
    local url="$1"
    local what="$2"
    local headers
    headers=$(mktemp "${TMP_DIR}/api.XXXXXX") || error "Could not ${what}"
    local response
    if [[ -n "$AUTH_HEADER" ]]; then
        response=$(curl --silent --show-error --location --header "Accept: application/vnd.github+json" --header "$AUTH_HEADER" --dump-header "$headers" --write-out '%{http_code}' "$url") || error "Could not ${what}"
    else
        response=$(curl --silent --show-error --location --header "Accept: application/vnd.github+json" --dump-header "$headers" --write-out '%{http_code}' "$url") || error "Could not ${what}"
    fi
    # --write-out puts the status, three digits, after the body.
    local code="${response: -3}"
    response="${response%???}"
    if [[ "$code" != "200" ]]; then
        local message
        message=$(printf '%s\n' "$response" | github_message)
        local limit
        limit=$(rate_limit_message "$code" "$headers")
        error "Could not ${what}: GitHub answered HTTP ${code}${message:+: ${message}}${limit:+\n${limit}}"
    fi
    printf '%s\n' "$response"
}

# Get latest release version from GitHub API: the newest release's tag, or nothing when the repository has none.
# GitHub sends its JSON indented or on one line (#1008); grep --only-matching finds the tag in either.
# Per https://docs.github.com/en/rest/releases/releases#list-releases
# Uses /releases?per_page=1 to get the most recent release (including prereleases)
# Note: /releases/latest excludes prereleases, so we use the list endpoint instead
get_latest_version() {
    local url="${GITHUB_API}/releases?per_page=1"
    local response
    # exit, not errexit: bash clears errexit in a command substitution, which is where this runs. api_get has said why.
    response=$(api_get "$url" "list the releases of ${GITHUB_REPO}") || exit 1
    # Extract tag_name from JSON response (first item in array)
    echo "$response" | grep --only-matching '"tag_name"[[:space:]]*:[[:space:]]*"[^"]*"' | awk 'NR == 1' | sed 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/' || true
}

# Require the release tag, which is an error, with GitHub's message, when GitHub has no such release. Its JSON is not
# read: a download link answers 404 alike for a release that doesn't exist and for a file it doesn't have, and this is
# how a release that doesn't exist is told apart, and reported as that.
# Per https://docs.github.com/en/rest/releases/releases#get-a-release-by-tag-name
require_release() {
    local tag="$1"
    api_get "${GITHUB_API}/releases/tags/${tag}" "fetch release ${tag} of ${GITHUB_REPO}" >/dev/null
}

# The name of the file GitHub served, from the Content-Disposition of the last answer in a headers file curl dumped,
# or nothing when that answer names none.
served_name() {
    awk '
        { sub(/\r$/, "") }
        /^HTTP\// { name = "" }
        tolower(substr($0, 1, 20)) == "content-disposition:" && match($0, /filename="?[^";]+/) {
            name = substr($0, RSTART + 9, RLENGTH - 9)
            sub(/^"/, "", name)
        }
        END { print name }
    ' "$1"
}

# Download the release's file name, by its public link, to dest, whose name is the file's. The link is public, so
# GH_TOKEN is not sent; it redirects to where GitHub keeps the file, and curl follows it. curl reports the HTTP status
# rather than failing on it (no --fail): a refusal is an error naming the file and the release, with GitHub's message,
# which its download host sends as plain text ("Not Found"), unless it is a 404 before the release is found and GitHub's
# API has no such release, which require_release reports. That host takes a name in any case, and names the file it
# served in its Content-Disposition: a file by another name is not this one, and is refused (#1002).
# Per https://docs.github.com/en/repositories/releasing-projects-on-github/linking-to-releases
download_asset() {
    local name="$1"
    local dest="$2"
    local headers="${dest}.headers"
    local answer
    answer=$(curl --silent --show-error --location --dump-header "$headers" --write-out '%{http_code} %{content_type}' "${GITHUB_DOWNLOAD}/${VERSION}/${name}" --output "$dest") || error "Could not download ${name} from release ${VERSION}"
    local code="${answer%% *}"
    if [[ "$code" != "200" ]]; then
        if [[ "$code" == "404" && "$RELEASE_FOUND" == false ]]; then
            require_release "$VERSION"
        fi
        local message=""
        if [[ -f "$dest" ]]; then
            case "${answer#* }" in
                text/plain*) message=$(awk '{ sub(/\r$/, ""); print; exit }' "$dest") ;;
                *) message=$(github_message <"$dest") ;;
            esac
        fi
        error "Could not download ${name} from release ${VERSION}: GitHub answered HTTP ${code}${message:+: ${message}}"
    fi
    local served
    served=$(served_name "$headers")
    if [[ -n "$served" && "$served" != "$name" ]]; then
        error "Could not download ${name} from release ${VERSION}: GitHub served ${served}, a file by another name"
    fi
    RELEASE_FOUND=true
}

# The SHA-256 a checksums file lists for name, lower-cased, or nothing. A line is 64 hex characters, a space, a space
# or '*', then the name, exactly and with case, as sha256sum and shasum write it; a trailing carriage return is ignored
# and the first line that names it wins. A name that merely contains this one is a different file. This is self
# upgrade's rule (listedChecksum, cmd/internal/cli/selfupgrade_archive.go), in awk that BSD awk runs too.
listed_checksum() {
    local checksums="$1"
    local name="$2"
    awk -v name="$name" '
        { sub(/\r+$/, "") }
        substr($0, 65, 1) == " " && (substr($0, 66, 1) == " " || substr($0, 66, 1) == "*") && substr($0, 67) == name &&
            substr($0, 1, 64) ~ /^[0-9A-Fa-f]+$/ {
            print tolower(substr($0, 1, 64))
            exit
        }
    ' "$checksums"
}

# Verify checksum: the file's SHA-256 against the one its line lists. Without sha256sum or shasum the file can't be
# verified, which is an error: nothing is installed that isn't verified.
verify_checksum() {
    local file="$1"
    local expected="$2"

    local actual
    if command -v sha256sum &>/dev/null; then
        actual=$(sha256sum "$file" | awk '{print $1}')
    elif command -v shasum &>/dev/null; then
        actual=$(shasum --algorithm 256 "$file" | awk '{print $1}')
    else
        error "Neither sha256sum nor shasum found, so ${file##*/} cannot be verified. Please install one of them."
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

    # Every request is curl's, as the published one-liner's is (#1008).
    if ! command -v curl &>/dev/null; then
        error "curl not found: install.sh downloads with it. Please install curl."
    fi

    # Detect platform
    local os
    local arch
    os=$(detect_os)
    arch=$(detect_arch)
    info "Detected platform: ${os}/${arch}"

    # Create temp directory, which holds what GitHub sends from here on; cleanup, trapped at script scope, removes it on
    # every exit. mktemp, rm, mkdir, awk, date and unzip keep their short options: macOS's BSD tools have no long forms,
    # and Info-ZIP has none anywhere.
    TMP_DIR=$(mktemp -d)

    # Resolve version
    if [[ "$VERSION" == "latest" ]]; then
        info "Fetching latest version..."
        # A refusal, such as a rate limit, has been reported by api_get, with GitHub's message.
        VERSION=$(get_latest_version) || exit 1
        if [[ -z "$VERSION" ]]; then
            error "Could not determine the latest release of ${GITHUB_REPO}: GitHub lists none"
        fi
        RELEASE_FOUND=true
    fi
    info "Version: $VERSION"

    # Determine archive extension
    local ext="tar.gz"
    if [[ "$os" == "windows" ]]; then
        ext="zip"
    fi

    # Build asset names
    local archive_name="devlore-cli_${VERSION}_${os}_${arch}.${ext}"
    local checksums_name="devlore-cli_${VERSION}_checksums.txt"

    # Download the checksums file, then the archive, each by its public link (#1008). An archive the release gives no
    # way to verify is not installed (#1002): as self upgrade does, the archive is downloaded only when the checksums
    # file has its line.
    info "Downloading ${checksums_name}..."
    download_asset "$checksums_name" "${TMP_DIR}/${checksums_name}"
    local expected_checksum
    expected_checksum=$(listed_checksum "${TMP_DIR}/${checksums_name}" "$archive_name")
    if [[ -z "$expected_checksum" ]]; then
        error "${checksums_name} has no line for ${archive_name}, so the archive cannot be verified"
    fi

    info "Downloading ${archive_name}..."
    download_asset "$archive_name" "${TMP_DIR}/${archive_name}"

    # Verify the archive before anything is extracted
    info "Verifying checksum..."
    verify_checksum "${TMP_DIR}/${archive_name}" "$expected_checksum"
    success "Checksum verified"

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
