#!/usr/bin/env bash

# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# Test-InstallScript.sh - Run install.sh as a user does, in a scratch account, and check what it did
# for documentation: Test-InstallScript.sh --help

set -o errexit -o errtrace -o nounset -o pipefail

# Declare-BashScript's functions and constants this suite uses, as NobleFactor/noblefactor-ops c942f54 has them,
# copied by hand: nothing here sources the helper (#1037). Copy them again when this suite needs a newer one.

readonly EX_USAGE=64    # command line usage error
readonly EX_NOINPUT=66  # cannot open input
readonly EX_SOFTWARE=70 # internal software error
readonly Heavy_ballot='✘' Heavy_check_mark='✔'
script_name="$(basename "$0")" && readonly script_name

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
    cat <<'EOF'
Usage: scripts/Test-InstallScript.sh [--keep-path] [--help]

Runs the checkout's install.sh the way a user does, piped into bash, in a scratch account: HOME, the XDG homes and
TMPDIR are under one temporary directory, so this machine's own installation and layer registrations are never
touched. On macOS the installer runs under /bin/bash with PATH=/usr/bin:/bin:/usr/sbin:/sbin, which is macOS's own
bash 3.2, bsdtar, BSD grep and sed, and shasum, whatever else is installed. Reports each check passed or failed on
stderr, in Declare-BashScript's form, with the installer's output under each failure, and exits 1 if any check failed.
It exits 64 on a usage error, 66 when DEVLORE_TEST_DIST lacks this platform's archive, and 70 when make dist fails.

Every install runs against a faux channel (#1031): a stand-in for GitHub, a fake curl first on PATH, that answers the
installer from releases made here. Its release carries this checkout's lore, star and writ, as `make dist` builds and
packs them for a release, so the installer under test installs the programs under test.
The other releases are each a way a release can fail the installer or mislead it (#1002), each with GitHub's JSON
indented and again on one line, as GitHub sends it either way (#1008). Nothing in install.sh is overridden; it asks
for what it always asks for.

Run by .github/workflows/installers.yaml on every pull request, on every platform the installers serve (#950, #1031),
with DEVLORE_TEST_DIST the archives its build job made.

  --keep-path       on macOS, run the installer with this PATH and the bash it finds, not macOS's own: the Installers
                    workflow's MacPorts leg puts MacPorts' GNU tools first, as on the owner's Macs (#1002)
  -h, --help        show this help and exit

Environment:
  GH_TOKEN          optional; passed to the installer, which sends it to the faux channel's API
  DEVLORE_TEST_DIST optional; a directory holding what `make dist DEVLORE_VERSION=v0.0.0-test.1002` made: this
                    platform's archive and the checksums file. Without it, the suite runs that for this platform
                    itself, which needs Go and GNU make 3.82 or later
EOF
}

# The suite's scratch directory, made once the arguments are parsed; cleanup removes it on every way out.
scratch=""

# cleanup removes the suite's scratch directory, whichever way the suite ends.
#
# Parameters:
#   - none.
#
# Returns:
#   - 0.
function cleanup {
    [[ -z "${scratch}" ]] || rm -rf "${scratch}"
}

Set-Traps cleanup

keep_path=false
for arg in "$@"; do
    case "$arg" in
        --keep-path) keep_path=true ;;
        --help | -h)
            usage
            exit 0
            ;;
        *)
            usage >&2
            error $EX_USAGE "unknown argument: ${arg}"
            ;;
    esac
done

# The checkout this script belongs to, whose install.sh is under test, and which also serves as a personal layer:
# a working-tree root. pwd -P, because writ records a root with its symbolic links resolved.
repo=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)
installer="${repo}/install.sh"
team_url="https://github.com/NobleFactor/noblefactor-ops.git"

# mktemp, rm, mkdir, tail, ln, wc and awk keep their short options: macOS's BSD tools have no long forms.
scratch=$(cd "$(mktemp -d)" && pwd -P)

case "$(uname -s)" in
    Darwin*)
        os=darwin
        installer_path=/usr/bin:/bin:/usr/sbin:/sbin
        installer_bash=/bin/bash
        if [[ "$keep_path" == true ]]; then
            installer_path=$PATH
            installer_bash=bash
        fi
        checksum_check="shasum --algorithm 256 --check"
        ;;
    *)
        os=linux
        installer_path=$PATH
        installer_bash=bash
        checksum_check="sha256sum --check"
        ;;
esac
case "$(uname -m)" in
    x86_64 | amd64) arch=amd64 ;;
    *) arch=arm64 ;;
esac

failures=0

# pass reports a check that passed, in success's form.
#
# Parameters:
#   - `$1`: the check's description.
#
# Returns:
#   - 0.
pass() {
    success "$1"
}

# fail reports a check that failed, in error's form as a warning, counts it, and shows the file it names.
#
# The file holds what the installer printed; it goes to stderr, under the report, between two marker lines.
#
# Parameters:
#   - `$1`: the check's description.
#   - `$2`: the file to show; optional.
#
# Returns:
#   - 0.
fail() {
    error 0 "$1"
    failures=$((failures + 1))
    if [[ -n "${2:-}" && -f "$2" ]]; then
        printf -- '---- %s\n' "${2#"$scratch"/}" >&2
        cat "$2" >&2
        printf -- '----\n' >&2
    fi
}

# expect reports a check: passed when its command succeeds, else failed with its file shown.
#
# Parameters:
#   - `$1`: the check's description.
#   - `$2`: the file to show if it fails.
#   - `${@:3}`: the command that decides it.
#
# Returns:
#   - 0.
expect() {
    local what="$1" show="$2"
    shift 2
    if "$@"; then
        pass "$what"
    else
        fail "$what" "$show"
    fi
}

# account_env <account>: the environment of a scratch account, one VAR=value per line.
account_env() {
    local dir="${scratch}/$1"
    printf '%s\n' \
        "HOME=${dir}/home" \
        "XDG_CONFIG_HOME=${dir}/home/.config" \
        "XDG_DATA_HOME=${dir}/home/.local/share" \
        "XDG_STATE_HOME=${dir}/home/.local/state" \
        "XDG_CACHE_HOME=${dir}/home/.cache" \
        "TMPDIR=${dir}/tmp"
}

# install_pipe <account> <output> [VAR=value ...] -- [install.sh argument ...]: install.sh piped into bash, as
# `curl ... | bash -s -- ...` runs it, in the named scratch account, with its output in <output>. Sets status.
status=0
install_pipe() {
    local account="$1" output="$2"
    shift 2
    local extra=()
    while [[ $# -gt 0 && "$1" != "--" ]]; do
        extra+=("$1")
        shift
    done
    shift
    mkdir -p "${scratch}/${account}/home" "${scratch}/${account}/tmp"
    local environment=()
    while IFS= read -r setting; do
        environment+=("$setting")
    done < <(account_env "$account")
    status=0
    # ${extra[@]+...}: bash 3.2 reports an empty array as unbound under nounset.
    cat "$installer" | env "${environment[@]}" PATH="$installer_path" ${extra[@]+"${extra[@]}"} \
        "$installer_bash" -s -- "$@" >"$output" 2>&1 || status=$?
}

# root_of <account> <layer>: the root writ reports for a layer in that account, by the writ installed there.
root_of() {
    local environment=()
    while IFS= read -r setting; do
        environment+=("$setting")
    done < <(account_env "$1")
    env "${environment[@]}" "${scratch}/$1/home/.local/bin/writ" repo list \
        --jq ".[] | select(.layer == \"$2\") | .root" --output value 2>/dev/null || true
}

# line_from_end <n> <file>: the nth line from the end.
line_from_end() {
    tail -n "$1" "$2" | awk 'NR == 1'
}

# count <fixed string> <file>: how many lines hold it.
count() {
    grep --count --fixed-strings -- "$1" "$2" || true
}

is() { # is <actual> <expected>
    [[ "$1" == "$2" ]]
}

at_least() { # at_least <actual> <minimum>
    [[ "$1" -ge "$2" ]]
}

absent() { # absent <fixed string> <file>
    ! grep --quiet --fixed-strings -- "$1" "$2"
}

holds() { # holds <file> <fixed string>...: every string is in the file
    local file="$1" text
    shift
    for text in "$@"; do
        grep --quiet --fixed-strings -- "$text" "$file" || return 1
    done
}

missing() { # missing <path>
    [[ ! -e "$1" ]]
}

# sha256_of <file>: its SHA-256, by whichever tool this machine has.
sha256_of() {
    if command -v sha256sum >/dev/null; then
        sha256sum "$1"
    else
        shasum --algorithm 256 "$1"
    fi | awk '{ print $1 }'
}

# tools_only <dir> <tool>...: <dir> holding the tools named and nothing else, linked from the installer's PATH. A tool
# the PATH lacks is left out.
tools_only() {
    local dir="$1" tool entry entries
    shift
    IFS=: read -r -a entries <<<"$installer_path"
    mkdir -p "$dir"
    for tool in "$@"; do
        for entry in "${entries[@]}"; do
            if [[ -x "${entry}/${tool}" ]]; then
                ln -s "${entry}/${tool}" "${dir}/${tool}"
                break
            fi
        done
    done
}

# --- Refusals, before any network call ---

output="${scratch}/tools.out"
install_pipe tools "$output" DEVLORE_TOOLS=lore -- --base="$repo"
expect "DEVLORE_TOOLS=lore with --base: exits 1" "$output" is "$status" 1
expect "DEVLORE_TOOLS=lore with --base: refused before any download" "$output" absent "Fetching" "$output"

output="${scratch}/argument.out"
install_pipe argument "$output" -- --bsae=x
expect "an unknown argument: exits 1" "$output" is "$status" 1
expect "an unknown argument: named, with the usage" "$output" \
    grep --quiet --fixed-strings "unknown argument: --bsae=x" "$output"

# Windows, as Git Bash reports it: install.ps1 installs there, and install.sh refuses (#1032).
mkdir -p "${scratch}/windows-uname"
cat >"${scratch}/windows-uname/uname" <<'EOF'
#!/bin/sh
case "$1" in
    -m) echo x86_64 ;;
    *) echo MINGW64_NT-10.0-26100 ;;
esac
EOF
chmod +x "${scratch}/windows-uname/uname"
output="${scratch}/windows.out"
install_pipe windows "$output" PATH="${scratch}/windows-uname:${installer_path}" --
expect "on Windows (Git Bash): exits 1" "$output" is "$status" 1
expect "on Windows (Git Bash): says it requires Linux or macOS" "$output" \
    grep --quiet --fixed-strings "This script requires Linux or macOS (Darwin)." "$output"
expect "on Windows (Git Bash): refused before any download" "$output" absent "Fetching" "$output"

# --- An archive that can't be verified is refused, against a stand-in for GitHub (#1002, #1008) ---
#
# The fake curl answers install.sh as GitHub does, from the fixtures under FAKE_GITHUB:
#
# - GitHub's API, https://api.github.com/repos/NobleFactor/devlore-cli/<path>, from api/<path>, and the request for the
#   newest release from api/releases.json. A fixture is answered with HTTP 200, or with the status in <fixture>.status,
#   as GitHub answers a rate limit with 403; a path without one gets GitHub's 404 and its JSON. GitHub sends its JSON
#   indented or on one line (#1008): the fixtures are indented, and FAKE_GITHUB_JSON=one-line sends them on one line.
# - A release's public links, https://github.com/NobleFactor/devlore-cli/releases/download/<tag>/<name>, from
#   download/<tag>/<name>. As GitHub's download host does, it takes the name in any case, redirects to the file, and
#   names the file it serves in its Content-Disposition; a name it has no file for gets 404 and "Not Found", in plain
#   text.
#
# It exits 0 whatever it answers, as curl does without --fail. It appends each URL it is asked for to FAKE_GITHUB_LOG,
# and each it was sent a token with to FAKE_GITHUB_LOG.token, so a test can tell what was downloaded and where the
# token went.

api="https://api.github.com/repos/NobleFactor/devlore-cli"
download="https://github.com/NobleFactor/devlore-cli/releases/download"
fixture_tag="v0.0.0-test.1002"
fixture_archive="devlore-cli_${fixture_tag}_${os}_${arch}.tar.gz"
fixture_checksums="devlore-cli_${fixture_tag}_checksums.txt"
archives=(darwin_amd64.tar.gz darwin_arm64.tar.gz linux_amd64.tar.gz linux_arm64.tar.gz windows_amd64.zip
    windows_arm64.zip)
github="${scratch}/github"
fake_bin="${github}/bin"
files="${github}/files"
mkdir -p "$fake_bin" "$files"

cat >"${github}/not-found.json" <<'EOF'
{
  "message": "Not Found",
  "documentation_url": "https://docs.github.com/rest/releases/releases#get-a-release-by-tag-name",
  "status": "404"
}
EOF
printf 'Not Found' >"${github}/not-found.txt"

cat >"${fake_bin}/curl" <<'EOF'
#!/usr/bin/env bash
# curl as GitHub answers install.sh, from the fixtures under FAKE_GITHUB (scripts/Test-InstallScript.sh).
set -o errexit -o nounset -o pipefail
api="https://api.github.com/repos/NobleFactor/devlore-cli"
download="https://github.com/NobleFactor/devlore-cli/releases/download"
output=""
headers=""
write_out=""
token=""
url=""
while [[ $# -gt 0 ]]; do
    case "$1" in
        --output)
            output="$2"
            shift 2
            ;;
        --dump-header)
            headers="$2"
            shift 2
            ;;
        --write-out)
            write_out="$2"
            shift 2
            ;;
        --header)
            case "$2" in
                Authorization:*) token=sent ;;
            esac
            shift 2
            ;;
        --*)
            shift
            ;;
        *)
            url="$1"
            shift
            ;;
    esac
done
printf '%s\n' "$url" >>"$FAKE_GITHUB_LOG"
if [[ -n "$token" ]]; then
    printf '%s\n' "$url" >>"${FAKE_GITHUB_LOG}.token"
fi

# body <content type> <fixture>: the fixture's bytes; JSON on one line, unindented, when FAKE_GITHUB_JSON=one-line.
body() {
    if [[ "$1" == application/json* && "${FAKE_GITHUB_JSON:-indented}" == one-line ]]; then
        awk '{ sub(/^[ \t]+/, ""); gsub(/": /, "\":"); printf "%s", $0 }' "$2"
    else
        cat "$2"
    fi
}

# answer <code> <content type> <fixture> [<served name>]: GitHub's answer. With a served name, it answers as the
# download host does: a redirect, then the file, named in its Content-Disposition. The headers in <fixture>.headers,
# one a line, where there is one, are sent too, as GitHub's API sends its x-ratelimit-* headers.
answer() {
    local code="$1" type="$2" fixture="$3" served="${4:-}" header
    if [[ -n "$headers" ]]; then
        {
            if [[ -n "$served" ]]; then
                printf 'HTTP/2 302\r\nlocation: https://release-assets.githubusercontent.com/%s\r\n\r\n' "$served"
                printf 'HTTP/2 %s\r\ncontent-type: %s\r\ncontent-disposition: attachment; filename=%s\r\n' \
                    "$code" "$type" "$served"
            else
                printf 'HTTP/2 %s\r\ncontent-type: %s\r\n' "$code" "$type"
            fi
            if [[ -f "${fixture}.headers" ]]; then
                while IFS= read -r header; do
                    printf '%s\r\n' "$header"
                done <"${fixture}.headers"
            fi
            printf '\r\n'
        } >"$headers"
    fi
    if [[ -n "$output" ]]; then
        body "$type" "$fixture" >"$output"
    else
        body "$type" "$fixture"
    fi
    case "$write_out" in
        "") ;;
        "%{http_code}") printf '%s' "$code" ;;
        "%{http_code} %{content_type}") printf '%s %s' "$code" "$type" ;;
        *)
            printf 'curl: the fake curl does not know --write-out %s\n' "$write_out" >&2
            exit 2
            ;;
    esac
}

# api_answer <fixture> <content type>: GitHub's API answering with the fixture, or its 404 when there is none.
api_answer() {
    if [[ ! -f "$1" ]]; then
        answer 404 "application/json; charset=utf-8" "${0%/*}/../not-found.json"
    elif [[ -f "${1}.status" ]]; then
        answer "$(<"${1}.status")" "$2" "$1"
    else
        answer 200 "$2" "$1"
    fi
}

case "$url" in
    "${download}/"*/*)
        tag="${url#"${download}"/}"
        name="${tag#*/}"
        tag="${tag%%/*}"
        shopt -s nocasematch
        for file in "${FAKE_GITHUB}/download/${tag}"/*; do
            if [[ -f "$file" && "${file##*/}" == "$name" ]]; then
                shopt -u nocasematch
                answer 200 application/octet-stream "$file" "${file##*/}"
                exit 0
            fi
        done
        shopt -u nocasematch
        answer 404 "text/plain; charset=utf-8" "${0%/*}/../not-found.txt"
        ;;
    "${api}/releases?per_page=1") api_answer "${FAKE_GITHUB}/api/releases.json" "application/json; charset=utf-8" ;;
    "${api}/releases/assets/"*) api_answer "${FAKE_GITHUB}/api/${url#"${api}"/}" application/octet-stream ;;
    "${api}/"*) api_answer "${FAKE_GITHUB}/api/${url#"${api}"/}" "application/json; charset=utf-8" ;;
    *)
        printf 'curl: (6) the fake curl answers GitHub only, not %s\n' "$url" >&2
        exit 6
        ;;
esac
EOF
chmod +x "${fake_bin}/curl"

# The faux channel's archive for this platform is this checkout's lore, star and writ, as make dist builds and packs
# them for a release, named for the fixtures' tag, with the checksums file make dist writes (#1031): the installer under
# test installs the programs under test. In CI, one job builds them for every platform and DEVLORE_TEST_DIST names them;
# without it, this suite builds this platform's. The other archives are never downloaded, so any bytes do.
dist="${DEVLORE_TEST_DIST:-}"
if [[ -z "$dist" ]]; then
    note "Building the faux channel: make dist PLATFORM=${os}/${arch} DEVLORE_VERSION=${fixture_tag}"
    if ! make -C "$repo" dist PLATFORM="${os}/${arch}" DEVLORE_VERSION="$fixture_tag" >"${scratch}/dist.out" 2>&1; then
        cat "${scratch}/dist.out" >&2
        error $EX_SOFTWARE "make dist failed, so there is no release to install from"
    fi
    dist="${repo}/dist"
fi
for built in "$fixture_archive" "$fixture_checksums"; do
    if [[ ! -f "${dist}/${built}" ]]; then
        error $EX_NOINPUT "${dist} has no ${built}:" \
            "it holds no make dist DEVLORE_VERSION=${fixture_tag} for this platform"
    fi
done
cp "${dist}/${fixture_archive}" "${files}/${fixture_archive}"
cp "${dist}/${fixture_checksums}" "${files}/checksums.dist"
for suffix in "${archives[@]}"; do
    name="devlore-cli_${fixture_tag}_${suffix}"
    if [[ "$name" != "$fixture_archive" ]]; then
        printf 'the %s archive, never downloaded here\n' "$suffix" >"${files}/${name}"
    fi
done
printf 'not %s\n' "$fixture_archive" >"${files}/not-the-archive"
archive_sum=$(sha256_of "${files}/${fixture_archive}")
wrong_sum=$(sha256_of "${files}/not-the-archive")

# upper <text>: the text in upper case. tr, not ${text^^}: macOS's bash is 3.2.
upper() {
    printf '%s' "$1" | tr '[:lower:]' '[:upper:]'
}

# The checksums files, written as the release job's shasum writes them, <hex>, two spaces, the name; and as Windows
# writes them, with CRLF; and as sha256sum --binary writes them, <hex>, a space, '*', the name.
for suffix in "${archives[@]}"; do
    name="devlore-cli_${fixture_tag}_${suffix}"
    sum=$(sha256_of "${files}/${name}")
    printf '%s  %s\n' "$sum" "$name" >>"${files}/checksums.listed"
    printf '%s  %s\r\n' "$sum" "$name" >>"${files}/checksums.crlf"
    printf '%s *%s\n' "$sum" "$name" >>"${files}/checksums.star"
    if [[ "$name" == "$fixture_archive" ]]; then
        printf '%s  %s\n' "$wrong_sum" "$name" >>"${files}/checksums.mismatch"
        continue
    fi
    printf '%s  %s\n' "$sum" "$name" >>"${files}/checksums.mismatch"
    printf '%s  %s\n' "$sum" "$name" >>"${files}/checksums.unlisted"
done

# lined <file> <name>: the checksums file without the archive's line, then a line giving the archive's hash to <name>,
# which is not the archive's name: an installer that takes that line verifies the archive by it, and installs.
lined() {
    cp "${files}/checksums.unlisted" "$1"
    printf '%s  %s\n' "$archive_sum" "$2" >>"$1"
}
lined "${files}/checksums.contains" "${fixture_archive}.sig"
lined "${files}/checksums.pattern" "${fixture_archive%.gz}Xgz"
lined "${files}/checksums.case" "$(upper "$fixture_archive")"

# The archive's line first, behind a UTF-8 byte-order mark. The mark is part of the line, as self upgrade, sha256sum and
# shasum read it: what stands before the name is then not 64 hex characters, so it is no line for the archive.
printf '\357\273\277%s  %s\n' "$archive_sum" "$fixture_archive" >"${files}/checksums.bom"
cat "${files}/checksums.unlisted" >>"${files}/checksums.bom"

# release <dir> <name>=<file>...: a fixture of the release fixture_tag carrying the assets named, each answered with
# <file>'s bytes, by its public link and, as GitHub's API serves it too, by its id. An asset with no file is listed but
# not served, as GitHub answers an asset it no longer has. The release is also the newest, in api/releases.json. Each
# asset's id and name are listed in <dir>/asset-ids, one "<id> <name>" a line, for the tests that ask what was
# downloaded.
release() {
    local dir="$1"
    shift
    local count=$# id=2000 entry name source size digest comma
    mkdir -p "${dir}/api/releases/tags" "${dir}/api/releases/assets" "${dir}/download/${fixture_tag}"
    : >"${dir}/asset-ids"
    {
        cat <<EOF
{
  "url": "${api}/releases/1002",
  "html_url": "https://github.com/NobleFactor/devlore-cli/releases/tag/${fixture_tag}",
  "id": 1002,
  "node_id": "RE_kwTEST1002",
  "tag_name": "${fixture_tag}",
  "target_commitish": "develop",
  "name": "${fixture_tag}",
  "draft": false,
  "prerelease": true,
  "created_at": "2026-10-01T00:00:00Z",
  "published_at": "2026-10-01T00:00:00Z",
  "assets": [
EOF
        for entry in "$@"; do
            name="${entry%%=*}"
            source="${entry#*=}"
            id=$((id + 1))
            printf '%s %s\n' "$id" "$name" >>"${dir}/asset-ids"
            size=0
            digest=""
            if [[ -n "$source" ]]; then
                # Hard links, not copies: this platform's archive is a real release's, and every fixture serves it.
                ln -f "$source" "${dir}/api/releases/assets/${id}"
                ln -f "$source" "${dir}/download/${fixture_tag}/${name}"
                size=$(($(wc -c <"$source")))
                digest=$(sha256_of "$source")
            fi
            comma=","
            if [[ $((id - 2000)) -eq $count ]]; then
                comma=""
            fi
            cat <<EOF
    {
      "url": "${api}/releases/assets/${id}",
      "id": ${id},
      "node_id": "RA_kwTEST${id}",
      "name": "${name}",
      "label": "",
      "uploader": {
        "login": "github-actions[bot]",
        "id": 41898282,
        "type": "Bot",
        "site_admin": false
      },
      "content_type": "application/octet-stream",
      "state": "uploaded",
      "size": ${size},
      "digest": "sha256:${digest}",
      "download_count": 0,
      "created_at": "2026-10-01T00:00:00Z",
      "updated_at": "2026-10-01T00:00:00Z",
      "browser_download_url": "${download}/${fixture_tag}/${name}"
    }${comma}
EOF
        done
        cat <<EOF
  ],
  "tarball_url": "${api}/tarball/${fixture_tag}",
  "zipball_url": "${api}/zipball/${fixture_tag}",
  "body": ""
}
EOF
    } >"${dir}/api/releases/tags/${fixture_tag}"
    {
        printf '[\n'
        cat "${dir}/api/releases/tags/${fixture_tag}"
        printf ']\n'
    } >"${dir}/api/releases.json"
}

# release_of <dir> <checksums> <archive> [<checksums name> [<archive name>]]: a release carrying every platform's
# archive and the checksums file, where <checksums> and <archive>, this platform's, are each a file, "refused" (listed,
# but GitHub answers 404) or "absent", published under their own names unless others are given.
release_of() {
    local dir="$1" checksums="$2" archive="$3"
    local checksums_as="${4:-$fixture_checksums}" archive_as="${5:-$fixture_archive}" entries=() suffix name
    case "$checksums" in
        absent) ;;
        refused) entries+=("${checksums_as}=") ;;
        *) entries+=("${checksums_as}=${checksums}") ;;
    esac
    for suffix in "${archives[@]}"; do
        name="devlore-cli_${fixture_tag}_${suffix}"
        if [[ "$name" != "$fixture_archive" ]]; then
            entries+=("${name}=${files}/${name}")
            continue
        fi
        case "$archive" in
            absent) ;;
            refused) entries+=("${archive_as}=") ;;
            *) entries+=("${archive_as}=${archive}") ;;
        esac
    done
    release "$dir" "${entries[@]}"
}

release_of "${github}/verified" "${files}/checksums.listed" "${files}/${fixture_archive}"
release_of "${github}/verified-crlf" "${files}/checksums.crlf" "${files}/${fixture_archive}"
release_of "${github}/verified-star" "${files}/checksums.star" "${files}/${fixture_archive}"
release_of "${github}/no-checksums" absent "${files}/${fixture_archive}"
release_of "${github}/checksums-case" "${files}/checksums.listed" "${files}/${fixture_archive}" \
    "$(upper "$fixture_checksums")"
release_of "${github}/no-line" "${files}/checksums.unlisted" "${files}/${fixture_archive}"
release_of "${github}/contains" "${files}/checksums.contains" "${files}/${fixture_archive}"
release_of "${github}/pattern" "${files}/checksums.pattern" "${files}/${fixture_archive}"
release_of "${github}/line-case" "${files}/checksums.case" "${files}/${fixture_archive}"
release_of "${github}/bom" "${files}/checksums.bom" "${files}/${fixture_archive}"
release_of "${github}/mismatch" "${files}/checksums.mismatch" "${files}/${fixture_archive}"
release_of "${github}/refused-checksums" refused "${files}/${fixture_archive}"
release_of "${github}/refused-archive" "${files}/checksums.listed" refused
release_of "${github}/no-archive" "${files}/checksums.listed" absent
release_of "${github}/archive-case" "${files}/checksums.listed" "${files}/${fixture_archive}" \
    "$fixture_checksums" "$(upper "$fixture_archive")"

# Two releases whose first asset is named as the checksums file, or as the archive, is, but for one character where its
# name has a dot. That asset is another file: the one like the checksums file lacks the archive's line, and the one
# like the archive is other bytes.
release "${github}/near-checksums" "${fixture_checksums%.txt}Xtxt=${files}/checksums.unlisted" \
    "${fixture_checksums}=${files}/checksums.listed" "${fixture_archive}=${files}/${fixture_archive}"
release "${github}/near-archive" "${fixture_archive%.gz}Xgz=${files}/not-the-archive" \
    "${fixture_checksums}=${files}/checksums.listed" "${fixture_archive}=${files}/${fixture_archive}"

# A repository with no release, which GitHub lists as none; and one whose anonymous limit is spent, which GitHub
# refuses with 403, its message, and x-ratelimit-remaining 0 and x-ratelimit-reset, the time the limit resets, which
# rate_limited writes just before each run, so that the reset is a known number of minutes away.
mkdir -p "${github}/no-release/api" "${github}/rate-limited/api"
printf '[]\n' >"${github}/no-release/api/releases.json"
cat >"${github}/rate-limited/api/releases.json" <<'EOF'
{
  "message": "API rate limit exceeded for 203.0.113.7. (But here's the good news: Authenticated requests get a higher rate limit. Check out the documentation for more details.)",
  "documentation_url": "https://docs.github.com/rest/overview/rate-limits-for-the-rest-api",
  "status": "403"
}
EOF
printf '403\n' >"${github}/rate-limited/api/releases.json.status"

# The tools install.sh, the fake curl and the stand-in programs run, without sha256sum or shasum; and the same with
# sha256sum, shasum and wget, where this machine has them, but without curl, which install.sh alone downloads with.
tools_only "${github}/tools" bash uname grep awk sed mktemp rm mkdir tar gzip mv chmod cp cat
tools_only "${github}/no-curl" bash uname grep awk sed mktemp rm mkdir tar gzip mv chmod cp cat sha256sum shasum wget

# clock_of <seconds since the epoch>: that time as this machine's clock shows it, HH:MM, by GNU date or BSD date.
clock_of() {
    date --date="@$1" +%H:%M 2>/dev/null || date -r "$1" +%H:%M
}

# rate_limited: GitHub's rate-limit headers for the fixture rate-limited, its limit resetting 23 minutes from now, and
# that reset as this machine's clock shows it.
rate_limited() {
    local reset
    reset=$(($(date +%s) + 23 * 60))
    printf '%s\n' "x-ratelimit-limit: 60" "x-ratelimit-remaining: 0" "x-ratelimit-reset: ${reset}" \
        "x-ratelimit-used: 60" "x-ratelimit-resource: core" >"${github}/rate-limited/api/releases.json.headers"
    clock_of "$reset"
}

# against <fixture> <account> <output> [VAR=value ...] [-- install.sh argument ...]: install.sh in its own scratch
# account, answered by the fake curl from <fixture> with its JSON laid out as layout says, with DEVLORE_VERSION the
# fixtures' tag. Sets status. Every URL the fake curl was asked for is then in ${scratch}/<account>.requests, and every
# one it was sent a token with in ${scratch}/<account>.requests.token.
against() {
    local fixture="$1" account="$2" output="$3" settings=()
    shift 3
    while [[ $# -gt 0 && "$1" != "--" ]]; do
        settings+=("$1")
        shift
    done
    if [[ $# -gt 0 ]]; then
        shift
    fi
    : >"${scratch}/${account}.requests"
    : >"${scratch}/${account}.requests.token"
    # ${settings[@]+...}: bash 3.2 reports an empty array as unbound under nounset.
    install_pipe "$account" "$output" PATH="${fake_bin}:${installer_path}" FAKE_GITHUB="$fixture" \
        FAKE_GITHUB_JSON="$layout" FAKE_GITHUB_LOG="${scratch}/${account}.requests" DEVLORE_VERSION="$fixture_tag" \
        ${settings[@]+"${settings[@]}"} -- ${@+"$@"}
}

# refused <account> <what> <fixture> <message> [VAR=value ...]: install.sh, against the fixture, exits 1, prints the
# message, and extracts and installs nothing.
refused() {
    local account="$1" what="$2" fixture="$3" message="$4"
    shift 4
    local output="${scratch}/${account}.out"
    against "$fixture" "$account" "$output" ${@+"$@"}
    expect "${what}: exits 1" "$output" is "$status" 1
    expect "${what}: says \"${message}\"" "$output" holds "$output" "$message"
    expect "${what}: extracts nothing" "$output" absent "Extracting" "$output"
    expect "${what}: installs nothing" "$output" missing "${scratch}/${account}/home/.local/bin"
}

# unasked <fixture> <requests>: the requests never include the archive's download, by its public link or, where the
# fixture's release lists it, by its asset id.
unasked() {
    local id
    id=$(awk -v name="$fixture_archive" '$2 == name { print $1 }' "$1/asset-ids")
    ! grep --quiet --line-regexp --fixed-strings -- "${download}/${fixture_tag}/${fixture_archive}" "$2" &&
        { [[ -z "$id" ]] || ! grep --quiet --line-regexp --fixed-strings -- "${api}/releases/assets/${id}" "$2"; }
}

# refused_before_archive <account> <what> <fixture> <message> [VAR=value ...]: refused, and the archive, which the
# fixture's release has, was never downloaded: the checksums file, and the archive's line in it, come first
# (Requirement 1).
refused_before_archive() {
    local account="$1" what="$2" fixture="$3"
    refused "$@"
    expect "${what}: never downloads the archive" "${scratch}/${account}.requests" \
        unasked "$fixture" "${scratch}/${account}.requests"
}

# by_link <requests>: the checksums file and the archive were each downloaded by its public link, and nothing by an
# asset id through GitHub's API (Requirement 3a).
by_link() {
    grep --quiet --line-regexp --fixed-strings -- "${download}/${fixture_tag}/${fixture_checksums}" "$1" &&
        grep --quiet --line-regexp --fixed-strings -- "${download}/${fixture_tag}/${fixture_archive}" "$1" &&
        ! grep --quiet --fixed-strings -- "${api}/releases/assets/" "$1"
}

# asked <requests> <url>...: the requests are the URLs given, in that order, and no others.
asked() {
    local requests="$1"
    shift
    [[ "$(cat "$requests")" == "$(printf '%s\n' "$@")" ]]
}

# api_asked <requests>: the requests made of GitHub's API, one a line, or nothing.
api_asked() {
    grep --fixed-strings -- "${api}/" "$1" || true
}

# installs <account> <what> <fixture> [VAR=value ...]: install.sh, against the fixture, exits 0, says it verified the
# archive, installs the archive's programs, and downloaded both files by their public links.
installs() {
    local account="$1" what="$2" fixture="$3"
    shift 3
    local output="${scratch}/${account}.out"
    against "$fixture" "$account" "$output" ${@+"$@"}
    expect "${what}: exits 0" "$output" is "$status" 0
    expect "${what}: says Checksum verified" "$output" holds "$output" "Checksum verified"
    expect "${what}: installs its programs" "$output" test -x "${scratch}/${account}/home/.local/bin/writ"
    expect "${what}: downloads both files by their public links" "${scratch}/${account}.requests" \
        by_link "${scratch}/${account}.requests"
}

# The layer cases, against the faux channel's release: the writ that registers the layers is the writ under test
# (#1031).

layout=indented

# --- A path layer and a URL layer, run twice: the second run changes nothing ---

run1="${scratch}/layers.run1"
run2="${scratch}/layers.run2"
against "${github}/verified" layers "$run1" -- --personal="$repo" --team="$team_url"
expect "layers: the first run exits 0" "$run1" is "$status" 0
expect "layers: the first run verified the archive" "$run1" holds "$run1" "Checksum verified"
against "${github}/verified" layers "$run2" -- --personal="$repo" --team="$team_url"
expect "layers: the second run exits 0" "$run2" is "$status" 0
expect "layers: personal is the checkout" "$run2" is "$(root_of layers personal)" "$repo"
expect "layers: team is cloned into the account" "$run2" \
    is "$(root_of layers team)" "${scratch}/layers/home/.local/share/devlore/writ/repos/noblefactor-ops"
expect "layers: the second run is unchanged for both" "$run2" at_least "$(count ": unchanged," "$run2")" 2
expect "layers: the skipped base is last" "$run1" \
    is "$(line_from_end 2 "$run1")" "skipped: base; to register it later:"

# --- No flags, where writ already has team and personal: they are named as registered, and only base is skipped ---

run3="${scratch}/layers.run3"
against "${github}/verified" layers "$run3"
expect "layers, no flags: exits 0" "$run3" is "$status" 0
expect "layers, no flags: names the layers writ already has" "$run3" \
    holds "$run3" "Already registered: team personal"
expect "layers, no flags: skips base alone" "$run3" is "$(count "skipped: " "$run3")" 1
expect "layers, no flags: the skipped base is last" "$run3" \
    is "$(line_from_end 2 "$run3")" "skipped: base; to register it later:"

# --- No flags: nothing registered, and the three skipped layers are the last lines ---

output="${scratch}/none.out"
against "${github}/verified" none "$output"
expect "no flags: exits 0" "$output" is "$status" 0
expect "no flags: verified the archive" "$output" holds "$output" "Checksum verified"
expect "no flags: the three skipped layers are last" "$output" is "$(tail -n 6 "$output" | grep --count '^skipped: ')" 3
expect "no flags: personal's command is the last line" "$output" \
    is "$(line_from_end 1 "$output")" "  writ repo set personal <working-tree-root>|<repository-url>"

# The one list of cases both installers' tests run, each check named by its case's number in the plan
# (docs/plans/fix/1002-installers-install-an-archive.md, Requirement 6), then install.sh's own case, then the checks of
# Requirement 3a: all of it once with GitHub's JSON indented and once with it on one line, as GitHub sends it (#1008).

no_line="${fixture_checksums} has no line for ${fixture_archive}, so the archive cannot be verified"
no_checksums="Could not download ${fixture_checksums} from release ${fixture_tag}: GitHub answered HTTP 404: Not Found"
no_archive="Could not download ${fixture_archive} from release ${fixture_tag}: GitHub answered HTTP 404: Not Found"
no_tag="v0.0.0-test.no-such-release"
rate_limit="Could not list the releases of NobleFactor/devlore-cli: GitHub answered HTTP 403: API rate limit exceeded"
rate_limit+=" for 203.0.113.7. (But here's the good news: Authenticated requests get a higher rate limit. Check out the"
rate_limit+=" documentation for more details.)"

for layout in indented one-line; do
    json="(${layout} JSON)"
    refused_before_archive "${layout}.no-checksums" "case 1 ${json}: no checksums file in the release" \
        "${github}/no-checksums" "$no_checksums"
    refused_before_archive "${layout}.no-line" "case 2 ${json}: no line for the archive" "${github}/no-line" "$no_line"
    refused_before_archive "${layout}.contains" "case 3 ${json}: a line only for ${fixture_archive}.sig" \
        "${github}/contains" "$no_line"
    refused_before_archive "${layout}.pattern" "case 4 ${json}: a line that matches only as a pattern, a dot replaced" \
        "${github}/pattern" "$no_line"
    refused_before_archive "${layout}.line-case" "case 5 ${json}: a line naming the archive in another case" \
        "${github}/line-case" "$no_line"
    refused "${layout}.mismatch" "case 6 ${json}: a mismatch" "${github}/mismatch" "Checksum verification failed"
    expect "case 6 ${json}: a mismatch: names both hashes" "${scratch}/${layout}.mismatch.out" \
        holds "${scratch}/${layout}.mismatch.out" "Expected: ${wrong_sum}" "Actual:   ${archive_sum}"
    refused_before_archive "${layout}.refused-checksums" "case 7 ${json}: the checksums file's download refused" \
        "${github}/refused-checksums" "$no_checksums"
    refused "${layout}.refused-archive" "case 8 ${json}: the archive's download refused" "${github}/refused-archive" \
        "$no_archive"
    refused "${layout}.no-archive" "case 9 ${json}: no archive for ${os}/${arch} in the release" \
        "${github}/no-archive" "$no_archive"
    description="case 9 ${json}: no archive for ${os}/${arch} in the release: asks GitHub's API nothing,"
    description+=" the checksums file having shown the release exists"
    expect "$description" \
        "${scratch}/${layout}.no-archive.requests" is "$(api_asked "${scratch}/${layout}.no-archive.requests")" ""
    refusal="Could not download ${fixture_archive} from release ${fixture_tag}:"
    refusal+=" GitHub served $(upper "$fixture_archive"), a file by another name"
    refused "${layout}.archive-case" "case 10 ${json}: the archive published under a name in another case" \
        "${github}/archive-case" "$refusal"
    installs "${layout}.near-checksums" \
        "case 11 ${json}: a look-alike of the checksums file, a dot replaced, listed first" "${github}/near-checksums"
    installs "${layout}.near-archive" "case 11 ${json}: a look-alike of the archive, a dot replaced, listed first" \
        "${github}/near-archive"
    refused "${layout}.no-release" "case 12 ${json}: no release" "${github}/no-release" \
        "Could not determine the latest release of NobleFactor/devlore-cli: GitHub lists none" DEVLORE_VERSION=
    # The rate limit says when to run again (Requirement 3b): the reset as a clock time and in minutes, and, only when
    # GH_TOKEN is unset, that setting it raises the limit. Each sentence is checked as the whole line.
    clock=$(rate_limited)
    refused "${layout}.rate-limited" "case 12 ${json}: a rate limit" "${github}/rate-limited" "$rate_limit" \
        DEVLORE_VERSION= GH_TOKEN=
    refusal="GitHub's API limit for this address is used up. It resets at ${clock} (in 23 minutes); run the installer"
    refusal+=" again after that. Setting GH_TOKEN raises the limit."
    expect "case 12 ${json}: a rate limit: says when to run again, and that GH_TOKEN raises the limit" \
        "${scratch}/${layout}.rate-limited.out" grep --quiet --line-regexp --fixed-strings -- "$refusal" \
        "${scratch}/${layout}.rate-limited.out"
    clock=$(rate_limited)
    refused "${layout}.rate-limited-token" "case 12 ${json}: a rate limit, with GH_TOKEN set" "${github}/rate-limited" \
        "$rate_limit" DEVLORE_VERSION= GH_TOKEN=fixture-token
    refusal="GitHub's API limit for your token is used up. It resets at ${clock} (in 23 minutes); run the installer"
    refusal+=" again after that."
    expect "case 12 ${json}: a rate limit, with GH_TOKEN set: says when to run again, and nothing of GH_TOKEN" \
        "${scratch}/${layout}.rate-limited-token.out" grep --quiet --line-regexp --fixed-strings -- "$refusal" \
        "${scratch}/${layout}.rate-limited-token.out"
    installs "${layout}.verified" "case 13 ${json}: plain lines" "${github}/verified"
    installs "${layout}.verified-crlf" "case 13 ${json}: CRLF lines" "${github}/verified-crlf"
    installs "${layout}.verified-star" "case 13 ${json}: <hash> *<name> lines" "${github}/verified-star"
    refusal="Could not download ${fixture_checksums} from release ${fixture_tag}:"
    refusal+=" GitHub served $(upper "$fixture_checksums"), a file by another name"
    refused_before_archive "${layout}.checksums-case" \
        "case 14 ${json}: the checksums file published under a name in another case" "${github}/checksums-case" \
        "$refusal"
    refused_before_archive "${layout}.bom" "case 15 ${json}: the archive's line behind a byte-order mark" \
        "${github}/bom" "$no_line"
    refused "${layout}.no-tag" "case 16 ${json}: a pinned DEVLORE_VERSION that names no release" "${github}/verified" \
        "Could not fetch release ${no_tag} of NobleFactor/devlore-cli: GitHub answered HTTP 404: Not Found" \
        DEVLORE_VERSION="$no_tag"
    description="case 16 ${json}: a pinned DEVLORE_VERSION that names no release: asks GitHub's API for it only after"
    description+=" its checksums file's link answers 404"
    expect "$description" \
        "${scratch}/${layout}.no-tag.requests" asked "${scratch}/${layout}.no-tag.requests" \
        "${download}/${no_tag}/devlore-cli_${no_tag}_checksums.txt" "${api}/releases/tags/${no_tag}"

    refused "${layout}.no-sha256" "install.sh's own case ${json}: neither sha256sum nor shasum" "${github}/verified" \
        "Neither sha256sum nor shasum found, so ${fixture_archive} cannot be verified" \
        PATH="${fake_bin}:${github}/tools"

    refused "${layout}.no-curl" "Requirement 3a ${json}: no curl, with wget where this machine has it" \
        "${github}/verified" "curl not found" PATH="${github}/no-curl"
    description="Requirement 3a ${json}: a pinned DEVLORE_VERSION that names a release: asks GitHub's API nothing"
    description+=" (case 13's plain run)"
    expect "$description" \
        "${scratch}/${layout}.verified.requests" is "$(api_asked "${scratch}/${layout}.verified.requests")" ""
    installs "${layout}.token" "Requirement 3a ${json}: the newest release, with GH_TOKEN set" "${github}/verified" \
        DEVLORE_VERSION= GH_TOKEN=fixture-token
    description="Requirement 3a ${json}: the newest release, with GH_TOKEN set: asks GitHub's API for the newest"
    description+=" release's tag alone"
    expect "$description" \
        "${scratch}/${layout}.token.requests" \
        is "$(api_asked "${scratch}/${layout}.token.requests")" "${api}/releases?per_page=1"
    expect "Requirement 3a ${json}: the newest release, with GH_TOKEN set: sends it to GitHub's API" \
        "${scratch}/${layout}.token.requests.token" \
        holds "${scratch}/${layout}.token.requests.token" "${api}/releases?per_page=1"
    expect "Requirement 3a ${json}: the newest release, with GH_TOKEN set: never sends it to a download link" \
        "${scratch}/${layout}.token.requests.token" absent "${download}/" "${scratch}/${layout}.token.requests.token"
done

# --- The guide's checksum line, against the faux channel's release ---
#
# The archive and the checksums file make dist wrote for it, as the release job writes them (#1031).

output="${scratch}/checksum.out"
mkdir -p "${scratch}/checksum"
cp "${files}/${fixture_archive}" "${scratch}/checksum/${fixture_archive}"
cp "${files}/checksums.dist" "${scratch}/checksum/${fixture_checksums}"
status=0
(
    cd "${scratch}/checksum"
    # shellcheck disable=SC2016,SC2086 # $2 is awk's; checksum_check is a command and its options, split on purpose
    env PATH="$installer_path" awk -v archive="$fixture_archive" '$2 == archive' "$fixture_checksums" |
        env PATH="$installer_path" $checksum_check
) >"$output" 2>&1 || status=$?
expect "the guide's checksum line (${checksum_check}) verifies ${fixture_archive}" "$output" is "$status" 0

if [[ $failures -gt 0 ]]; then
    error 1 "${failures} check(s) failed"
fi
success "every check passed"
