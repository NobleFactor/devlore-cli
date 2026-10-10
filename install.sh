#!/usr/bin/env bash

# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# install.sh - Install lore, star and writ from an archive it has verified, and register the layers given
# for documentation: install.sh --help

set -o errexit -o errtrace -o nounset -o pipefail

# Where the DevLore site serves this script. The site releases from develop, so this is its develop environment.
INSTALLERS_URL="https://delightful-grass-0ac0a4c1e-develop.westus2.6.azurestaticapps.net"

# Declare-BashScript's functions and constants this script uses, as NobleFactor/noblefactor-ops c942f54 has them,
# copied by hand (#1037). install.sh runs with what ships with macOS and sources nothing, so it carries none of the
# helper's checks for bash 5.3 or GNU getopt. Copy them again when this script needs a newer one.

readonly EX_USAGE=64       # command line usage error
readonly EX_DATAERR=65     # data format error
readonly EX_UNAVAILABLE=69 # service unavailable (missing dependency)
readonly EX_SOFTWARE=70    # internal software error
readonly EX_TEMPFAIL=75    # temp failure; user is invited to retry
readonly EX_CONFIG=78      # configuration error (unsupported platform)
readonly Heavy_ballot='✘' Heavy_check_mark='✔'
# Under curl ... | bash, $0 is bash; the name the messages carry is this script's.
readonly script_name=install.sh

# Set-Traps sends ERR, HUP, INT and TERM to on_error_or_interrupt, and EXIT to the cleanup function a script names.
#
# A script calls it once, after its cleanup function is defined and before its arguments are parsed: an early exit
# with EXIT naming a function not yet defined fails with "command not found". A script with nothing to undo calls it
# with no argument (#268).
#
# Parameters:
#   - `$1`: the name of the cleanup function EXIT runs; optional.
#
# Returns:
#   - 0.
# shellcheck disable=SC2120 # The cleanup is optional: a script with nothing to undo calls Set-Traps bare (#289).
function Set-Traps {
    trap 'on_error_or_interrupt ERR' ERR
    trap 'on_error_or_interrupt HUP' HUP
    trap 'on_error_or_interrupt INT' INT
    trap 'on_error_or_interrupt TERM' TERM
    # shellcheck disable=SC2064 # The cleanup's name is expanded now, by design: it is what EXIT runs.
    if [[ -n ${1:-} ]]; then trap "$1" EXIT; fi
}

# error prints a message to stderr in the helper's error form, and ends the script with the given status unless it is 0.
#
# The form is `[<script name>] [✘] <message>`, the ✘ in red. A status of 0 makes the message a warning: it is printed,
# and the script carries on.
#
# Parameters:
#   - `$1`: the exit status; 0 prints the message and returns.
#   - `${@:2}`: the message, its words joined by spaces.
#
# Returns:
#   - 0 when `$1` is 0; otherwise it does not return: the script exits with `$1`.
function error {
    local rc=$1
    shift 1
    printf '[%s] [\033[31m%s\033[0m] %s\n' "$script_name" "$Heavy_ballot" "$*" >&2
    if ((rc != 0)); then
        exit "$rc"
    fi
}

# note prints an informational message to stderr in the helper's form, unless SILENT is set.
#
# The form is `[<script name>] [+] <message>`.
#
# Parameters:
#   - `$@`: the message, its words joined by spaces.
#
# Returns:
#   - 0.
function note {
    if [[ -n ${SILENT:-} ]]; then
        return
    fi
    printf "[%s] [+] %s\n" "$script_name" "$*" >&2
}

# on_error_or_interrupt reports a failure or a signal that ends the script, once, in error's own form (#268).
#
# A failure the script tolerates, under set +o errexit, ends nothing and goes unreported. Inside a subshell, the shell
# that started it reports the failure when the command holding the subshell fails. A signal ends the script with 128
# plus its number, so the EXIT trap's cleanup runs. Set-Traps installs it; a script does not call it itself.
#
# Parameters:
#   - `$1`: the trapped event: ERR, HUP, INT or TERM.
#
# Returns:
#   - 0 for a failure under set +o errexit; otherwise it does not return: a subshell exits with the failure's status,
#     unreported, and the script exits with the failing command's status, or 128 plus the signal's number.
function on_error_or_interrupt {
    local status=$? event=$1

    case $event in
        ERR)
            [[ $- == *e* ]] || return 0
            ((BASH_SUBSHELL == 0)) || exit "$status"
            error "$status" "exited with status ${status} at line ${BASH_LINENO[0]}: ${BASH_COMMAND}"
            ;;
        HUP | INT | TERM)
            error $((128 + $(kill -l "$event"))) "interrupted by SIG${event}"
            ;;
    esac
}

# success prints a success message to stderr in the helper's form.
#
# The form is `[<script name>] [✔] <message>`, the ✔ in green.
#
# Parameters:
#   - `$@`: the message, its words joined by spaces.
#
# Returns:
#   - 0.
function success {
    printf '[%s] [\033[32m%s\033[0m] %s\n' "$script_name" "$Heavy_check_mark" "$*" >&2
}

usage() {
    cat <<EOF
Usage: install.sh [--prefix <dir>] [--base <loc>] [--team <loc>] [--personal <loc>]

Installs lore, star and writ into <prefix> (default ~/.local), then registers each layer given with
writ repo set, base first. A layer not given is skipped and named at the end, unless writ already has it
registered. Never asks. Runs on Linux and macOS; on Windows, install.ps1 installs.

The release's archive is verified against its checksums file, with sha256sum or shasum, before anything is
extracted. An archive that can't be verified is not installed: a release without the checksums file, a checksums
file without the archive's line, a machine with neither tool and a mismatch are each an error. Both files are
downloaded with curl, from the release's public links.

  --prefix <dir>     installation prefix (default: ~/.local)
  --base <loc>       the base layer: a working-tree root or a repository URL (or DEVLORE_BASE)
  --team <loc>       the team layer (or DEVLORE_TEAM)
  --personal <loc>   the personal layer (or DEVLORE_PERSONAL)
  -h, --help         show this help and exit

Each option takes its value as --option <value> or as --option=<value>. A flag wins over its variable. Running the
same command again is safe: it is also how to recover from a failure.

Exit status: 0 installed; 64 a usage error; 65 an archive it cannot verify; 69 a missing tool, or a release GitHub
refuses or does not have; 70 a failed self install; 75 GitHub's rate limit used up; 78 an unsupported platform.

Environment:
  DEVLORE_VERSION    a release tag to install (default: the newest release, pre-releases included)
  DEVLORE_TOOLS      all, writ, lore or star (default: all)
  GH_TOKEN           optional; sent to GitHub's API alone, which lifts its limit of 60 anonymous requests an hour

Served by the DevLore site's develop environment, from which devlore is released today:
  curl --fail --silent --show-error --location ${INSTALLERS_URL}/install.sh |
      bash -s -- --base=<loc> --team=<loc> --personal=<loc>
EOF
}

# The download directory, removed on every exit. Script scope, not local to main: the EXIT trap runs
# after main has returned, and under set -u a local that is gone is an error (#958).
TMP_DIR=""

# cleanup removes the download directory, whichever way the script ends.
#
# Parameters:
#   - none.
#
# Returns:
#   - 0.
cleanup() {
    if [[ -n "${TMP_DIR}" ]]; then
        rm -rf "${TMP_DIR}"
    fi
}

Set-Traps cleanup

# Parse arguments. Each layer's variable is its default; a flag wins over it (#950). Each option takes its value as
# --option value or --option=value (#1038), in a loop that runs on bash 3.2: there is no getopt to lean on.
PREFIX=""
BASE="${DEVLORE_BASE:-}"
TEAM="${DEVLORE_TEAM:-}"
PERSONAL="${DEVLORE_PERSONAL:-}"
while (($# > 0)); do
    case "$1" in
        --prefix=* | --base=* | --team=* | --personal=*)
            option="${1%%=*}"
            value="${1#*=}"
            shift 1
            ;;
        --prefix | --base | --team | --personal)
            if (($# < 2)); then
                usage >&2
                error $EX_USAGE "$1 needs a value"
            fi
            option="$1"
            value="$2"
            shift 2
            ;;
        --help | -h)
            usage
            exit 0
            ;;
        *)
            # A typo such as --bsae=... would otherwise be dropped, and its layer reported skipped.
            usage >&2
            error $EX_USAGE "unknown argument: $1"
            ;;
    esac
    case "$option" in
        --prefix) PREFIX="$value" ;;
        --base) BASE="$value" ;;
        --team) TEAM="$value" ;;
        *) PERSONAL="$value" ;;
    esac
done

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

# Detect OS. install.sh installs on Linux and macOS, and install.ps1 on Windows; neither serves the other's (#1032). The
# refusal is Declare-BashScript's require_nix's.
detect_os() {
    case "$(uname -s)" in
        Linux*) echo "linux" ;;
        Darwin*) echo "darwin" ;;
        *) error $EX_CONFIG "This script requires Linux or macOS (Darwin)." ;;
    esac
}

# Detect architecture. The releases publish amd64 and arm64 only.
detect_arch() {
    case "$(uname -m)" in
        x86_64 | amd64) echo "amd64" ;;
        arm64 | aarch64) echo "arm64" ;;
        *) error $EX_CONFIG "Unsupported architecture: $(uname -m)" ;;
    esac
}

# GitHub's own message in the JSON on stdin, which its API sends when it refuses a request, or nothing
# Per https://docs.github.com/en/rest/using-the-rest-api/troubleshooting-the-rest-api
github_message() {
    grep --only-matching '"message"[[:space:]]*:[[:space:]]*"[^"]*"' | awk 'NR == 1' |
        sed 's/^"message"[[:space:]]*:[[:space:]]*"\(.*\)"$/\1/' || true
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
    if [[ "$code" != "403" && "$code" != "429" ]] ||
        [[ "$(header_value "$headers" x-ratelimit-remaining)" != "0" ]]; then
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
# when to run the installer again (#1002). It ends the script with 75 for GitHub's rate limit, and 69 for any other
# refusal or a request curl could not make.
# Per https://docs.github.com/en/rest/releases/releases
api_get() {
    local url="$1"
    local what="$2"
    local headers
    headers=$(mktemp "${TMP_DIR}/api.XXXXXX") || error $EX_UNAVAILABLE "Could not ${what}"
    local response
    if [[ -n "$AUTH_HEADER" ]]; then
        response=$(curl --silent --show-error --location --header "Accept: application/vnd.github+json" \
            --header "$AUTH_HEADER" --dump-header "$headers" --write-out '%{http_code}' "$url") ||
            error $EX_UNAVAILABLE "Could not ${what}"
    else
        response=$(curl --silent --show-error --location --header "Accept: application/vnd.github+json" \
            --dump-header "$headers" --write-out '%{http_code}' "$url") ||
            error $EX_UNAVAILABLE "Could not ${what}"
    fi
    # --write-out puts the status, three digits, after the body.
    local code="${response: -3}"
    response="${response%???}"
    if [[ "$code" != "200" ]]; then
        local message
        message=$(printf '%s\n' "$response" | github_message)
        local limit
        limit=$(rate_limit_message "$code" "$headers")
        if [[ -n "$limit" ]]; then
            # The refusal, then when to run again, a line of its own.
            error 0 "Could not ${what}: GitHub answered HTTP ${code}${message:+: ${message}}"
            printf '%s\n' "$limit" >&2
            exit $EX_TEMPFAIL
        fi
        error $EX_UNAVAILABLE "Could not ${what}: GitHub answered HTTP ${code}${message:+: ${message}}"
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
    # exit, not errexit: bash clears errexit in a command substitution, which is where this runs. api_get has said why,
    # and its status is passed on.
    response=$(api_get "$url" "list the releases of ${GITHUB_REPO}") || exit $?
    # Extract tag_name from JSON response (first item in array)
    echo "$response" | grep --only-matching '"tag_name"[[:space:]]*:[[:space:]]*"[^"]*"' | awk 'NR == 1' |
        sed 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/' || true
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
# served in its Content-Disposition: a file by another name is not this one, and is refused (#1002). Each refusal ends
# the script with 69.
# Per https://docs.github.com/en/repositories/releasing-projects-on-github/linking-to-releases
download_asset() {
    local name="$1"
    local dest="$2"
    local headers="${dest}.headers"
    local what="download ${name} from release ${VERSION}"
    local answer
    answer=$(curl --silent --show-error --location --dump-header "$headers" \
        --write-out '%{http_code} %{content_type}' "${GITHUB_DOWNLOAD}/${VERSION}/${name}" --output "$dest") ||
        error $EX_UNAVAILABLE "Could not ${what}"
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
        error $EX_UNAVAILABLE "Could not ${what}: GitHub answered HTTP ${code}${message:+: ${message}}"
    fi
    local served
    served=$(served_name "$headers")
    if [[ -n "$served" && "$served" != "$name" ]]; then
        error $EX_UNAVAILABLE "Could not ${what}: GitHub served ${served}, a file by another name"
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
# verified, which is an error, 69: nothing is installed that isn't verified. A mismatch is an error, 65, followed by
# the two checksums, each a line of its own.
verify_checksum() {
    local file="$1"
    local expected="$2"

    local actual
    if command -v sha256sum &>/dev/null; then
        actual=$(sha256sum "$file" | awk '{print $1}')
    elif command -v shasum &>/dev/null; then
        actual=$(shasum --algorithm 256 "$file" | awk '{print $1}')
    else
        error $EX_UNAVAILABLE "Neither sha256sum nor shasum found, so ${file##*/} cannot be verified." \
            "Please install one of them."
    fi

    if [[ "$actual" != "$expected" ]]; then
        error 0 "Checksum verification failed!"
        printf 'Expected: %s\nActual:   %s\n' "$expected" "$actual" >&2
        exit $EX_DATAERR
    fi
}

# Main installation
main() {
    note "DevLore CLI Installer"
    echo

    # A layer is registered by the writ this run installs, so a run that leaves writ out cannot register one. Refused
    # before anything is downloaded (#950).
    if [[ -n "${BASE}${TEAM}${PERSONAL}" && "$TOOLS" != "all" && "$TOOLS" != "writ" ]]; then
        error $EX_USAGE "--base, --team and --personal register layers with writ," \
            "which DEVLORE_TOOLS=${TOOLS} does not install"
    fi

    # Every request is curl's, as the published one-liner's is (#1008).
    if ! command -v curl &>/dev/null; then
        error $EX_UNAVAILABLE "curl not found: install.sh downloads with it. Please install curl."
    fi

    # Detect platform. exit, not errexit, after a command substitution: each has said why; its status is passed on.
    local os
    local arch
    os=$(detect_os) || exit $?
    arch=$(detect_arch) || exit $?
    note "Detected platform: ${os}/${arch}"

    # Create temp directory, which holds what GitHub sends from here on; cleanup, trapped at script scope, removes it on
    # every exit. mktemp, rm, mkdir, awk and date keep their short options: macOS's BSD tools have no long forms.
    TMP_DIR=$(mktemp -d)

    # Resolve version
    if [[ "$VERSION" == "latest" ]]; then
        note "Fetching latest version..."
        # A refusal, such as a rate limit, has been reported by api_get, with GitHub's message; its status is passed on.
        VERSION=$(get_latest_version) || exit $?
        if [[ -z "$VERSION" ]]; then
            error $EX_UNAVAILABLE "Could not determine the latest release of ${GITHUB_REPO}: GitHub lists none"
        fi
        RELEASE_FOUND=true
    fi
    note "Version: $VERSION"

    # Build asset names
    local archive_name="devlore-cli_${VERSION}_${os}_${arch}.tar.gz"
    local checksums_name="devlore-cli_${VERSION}_checksums.txt"

    # Download the checksums file, then the archive, each by its public link (#1008). An archive the release gives no
    # way to verify is not installed (#1002): as self upgrade does, the archive is downloaded only when the checksums
    # file has its line.
    note "Downloading ${checksums_name}..."
    download_asset "$checksums_name" "${TMP_DIR}/${checksums_name}"
    local expected_checksum
    expected_checksum=$(listed_checksum "${TMP_DIR}/${checksums_name}" "$archive_name")
    if [[ -z "$expected_checksum" ]]; then
        error $EX_DATAERR "${checksums_name} has no line for ${archive_name}, so the archive cannot be verified"
    fi

    note "Downloading ${archive_name}..."
    download_asset "$archive_name" "${TMP_DIR}/${archive_name}"

    # Verify the archive before anything is extracted
    note "Verifying checksum..."
    verify_checksum "${TMP_DIR}/${archive_name}" "$expected_checksum"
    success "Checksum verified"

    # Extract archive
    #
    # The archive holds the products at its root and star's extensions under share/ (#903). The products move to
    # pkg/bin so that each one's `self install` finds pkg/share at <exeDir>/../share, the path star copies its
    # extensions from.
    note "Extracting..."
    local pkg="${TMP_DIR}/pkg"
    mkdir -p "${pkg}/bin"
    tar --extract --gzip --file "${TMP_DIR}/${archive_name}" --directory "${pkg}"

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
        product="$name"
        [[ "$TOOLS" == "all" || "$TOOLS" == "$product" ]] || continue
        mv "$file" "${pkg}/bin/${name}"
        chmod +x "${pkg}/bin/${name}"
        note "Installing ${product}..."
        (cd "$pkg" && "bin/${name}" self install "$PREFIX" --unattended) ||
            error $EX_SOFTWARE "${product} self install failed"
        installed+=("$product")
    done

    if [[ ${#installed[@]} -eq 0 ]]; then
        error $EX_DATAERR "No binaries found in archive for DEVLORE_TOOLS=${TOOLS}"
    fi

    # Register the layers given, base first, with the writ just installed (#950). The call runs in the user's working
    # directory, so a relative location resolves where it was typed. writ's output is its own, and so are its errors:
    # a failure ends the run, and running the same command again is the recovery. --unattended is writ's contract
    # for a run nobody is there to answer.
    local writ="${INSTALL_DIR}/writ"
    local registered=()
    local held=()
    local skipped=()
    local layer location root
    for layer in base team personal; do
        case "$layer" in
            base) location="$BASE" ;;
            team) location="$TEAM" ;;
            *) location="$PERSONAL" ;;
        esac
        if [[ -z "$location" ]]; then
            # A layer not given is skipped unless the writ just installed already has it registered (#1029). A writ
            # this run didn't install, or that can't answer, leaves it skipped.
            root=""
            if [[ -x "$writ" ]]; then
                root=$("$writ" repo list --filter "layer=${layer}" --filter state=registered --jq '.[].root' \
                    --output value 2>/dev/null) || root=""
            fi
            if [[ -n "$root" ]]; then
                held+=("$layer")
            else
                skipped+=("$layer")
            fi
            continue
        fi
        note "Registering ${layer}: ${location}"
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
    if [[ ${#held[@]} -gt 0 ]]; then
        success "Already registered: ${held[*]}"
    fi
    echo

    # Check if install dir is in PATH. The advice names the directory this run installed into, written from $HOME
    # when it is under it, so that --prefix gets advice that works.
    if [[ ":$PATH:" != *":${INSTALL_DIR}:"* ]]; then
        local shown="$INSTALL_DIR"
        if [[ "$INSTALL_DIR" == "$HOME"/* ]]; then
            shown="\$HOME/${INSTALL_DIR#"$HOME"/}"
        fi
        error 0 "${INSTALL_DIR} is not in your PATH"
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
    note "Documentation: https://github.com/NobleFactor/devlore-cli#readme"

    if [[ ${#registered[@]} -gt 0 || ${#skipped[@]} -gt 0 ]]; then
        echo
        note "Next steps:"
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
