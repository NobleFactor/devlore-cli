#!/usr/bin/env bash

# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# Test-InstallScript.sh - Run install.sh as a user does, in a scratch account, and check what it did
# for documentation: Test-InstallScript.sh --help

set -o errexit -o nounset -o pipefail

usage() {
    cat <<'EOF'
Usage: scripts/Test-InstallScript.sh [--help]

Runs the checkout's install.sh the way a user does, piped into bash, in a scratch account: HOME, the XDG homes and
TMPDIR are under one temporary directory, so this machine's own installation and layer registrations are never
touched. On macOS the installer runs under /bin/bash with PATH=/usr/bin:/bin:/usr/sbin:/sbin, which is macOS's own
bash 3.2, bsdtar, BSD grep and sed, and shasum, whatever else is installed. Prints PASS or FAIL for each check, with
the installer's output under each failure, and exits 1 if any check failed.

Run by .github/workflows/installers.yaml on every platform the installers serve (#950).

Environment:
  GH_TOKEN          optional; passed to the installer, which sends it to GitHub's API
  DEVLORE_VERSION   optional; the release tag to install (default: the newest release, pre-releases included)
EOF
}

for arg in "$@"; do
    case "$arg" in
        --help | -h)
            usage
            exit 0
            ;;
        *)
            printf 'error: unknown argument: %s\n\n' "$arg" >&2
            usage >&2
            exit 1
            ;;
    esac
done

# The checkout this script belongs to, whose install.sh is under test, and which also serves as a personal layer:
# a working-tree root. pwd -P, because writ records a root with its symbolic links resolved.
repo=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)
installer="${repo}/install.sh"
team_url="https://github.com/NobleFactor/noblefactor-ops.git"

# mktemp, rm, mkdir and tail keep their short options: macOS's BSD tools have no long forms.
scratch=$(cd "$(mktemp -d)" && pwd -P)
trap 'rm -rf "$scratch"' EXIT

case "$(uname -s)" in
    Darwin*)
        os=darwin
        installer_path=/usr/bin:/bin:/usr/sbin:/sbin
        installer_bash=/bin/bash
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

pass() {
    printf 'PASS %s\n' "$1"
}

# fail <description> [<file>]: counts the failure and shows the file, which holds what the installer printed.
fail() {
    printf 'FAIL %s\n' "$1"
    failures=$((failures + 1))
    if [[ -n "${2:-}" && -f "$2" ]]; then
        printf -- '---- %s\n' "${2#"$scratch"/}"
        cat "$2"
        printf -- '----\n'
    fi
}

# expect <description> <file> <command...>: PASS when the command succeeds, else FAIL with the file shown.
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

# --- A path layer and a URL layer, run twice: the second run changes nothing ---

run1="${scratch}/layers.run1"
run2="${scratch}/layers.run2"
install_pipe layers "$run1" -- --personal="$repo" --team="$team_url"
expect "layers: the first run exits 0" "$run1" is "$status" 0
install_pipe layers "$run2" -- --personal="$repo" --team="$team_url"
expect "layers: the second run exits 0" "$run2" is "$status" 0
expect "layers: personal is the checkout" "$run2" is "$(root_of layers personal)" "$repo"
expect "layers: team is cloned into the account" "$run2" \
    is "$(root_of layers team)" "${scratch}/layers/home/.local/share/devlore/writ/repos/noblefactor-ops"
expect "layers: the second run is unchanged for both" "$run2" at_least "$(count ": unchanged," "$run2")" 2
expect "layers: the skipped base is last" "$run1" \
    is "$(line_from_end 2 "$run1")" "skipped: base; to register it later:"

# --- No flags: nothing registered, and the three skipped layers are the last lines ---

output="${scratch}/none.out"
install_pipe none "$output" --
expect "no flags: exits 0" "$output" is "$status" 0
expect "no flags: the three skipped layers are last" "$output" is "$(tail -n 6 "$output" | grep --count '^skipped: ')" 3
expect "no flags: personal's command is the last line" "$output" \
    is "$(line_from_end 1 "$output")" "  writ repo set personal <working-tree-root>|<repository-url>"

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

# --- The guide's checksum line, against the release just installed ---

output="${scratch}/checksum.out"
tag=$("${scratch}/layers/home/.local/bin/writ" --version 2>/dev/null | awk 'NR == 1 { sub(/,$/, "", $3); print $3 }')
archive="devlore-cli_${tag}_${os}_${arch}.tar.gz"
release="https://github.com/NobleFactor/devlore-cli/releases/download/${tag}"
mkdir -p "${scratch}/checksum"
status=0
(
    cd "${scratch}/checksum"
    curl --fail --silent --show-error --location --remote-name "${release}/${archive}"
    curl --fail --silent --show-error --location --remote-name "${release}/devlore-cli_${tag}_checksums.txt"
    # shellcheck disable=SC2086 # checksum_check is a command and its options, split on purpose
    grep "$archive" "devlore-cli_${tag}_checksums.txt" | env PATH="$installer_path" $checksum_check
) >"$output" 2>&1 || status=$?
expect "the guide's checksum line (${checksum_check}) verifies ${archive}" "$output" is "$status" 0

if [[ $failures -gt 0 ]]; then
    printf '%s check(s) failed\n' "$failures"
    exit 1
fi
printf 'every check passed\n'
