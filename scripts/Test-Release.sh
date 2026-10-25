#!/usr/bin/env bash

# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# Test-Release.sh - Check that a rerun of a release run produces the release the first run made (#980)
# for documentation: Test-Release.sh --help

set -o errexit -o errtrace -o nounset -o pipefail

# Declare-BashScript's functions and constants this suite uses, as NobleFactor/noblefactor-ops c942f54 has them,
# copied by hand: nothing here sources the helper (#1037). Copy them again when this suite needs a newer one.

readonly EX_USAGE=64 # command line usage error
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
    cat <<'TEXT'
Usage: scripts/Test-Release.sh [--help]

Checks the three things a rerun of .github/workflows/release.yaml depends on (#980), each by comparing what it
observes with what it expects, exactly:

  1. .github/scripts/release-version.sh gives each kind of ref the version its commit's time makes, in a scratch
     repository whose commit has a known time, and gives it again on a second run.
  2. .github/scripts/release-publish.sh, against a stand-in gh first on PATH that records each call, creates a
     release that is not there, changes nothing in one that has every asset, and replaces every asset in one that
     lacks one.
  3. make dist, run twice a few seconds apart for this machine's platform and for windows/amd64, so a .tar.gz and a
     .zip, writes the same checksums file, and so the same archives, byte for byte.

Reports each check passed or failed on stderr, in Declare-BashScript's form, and exits 1 if any check failed. It
exits 64 on a usage error. Run by make quality-gate, which CI's quality-gate job and make check-all run.
TEXT
}

scratch=""

# cleanup removes the scratch directory.
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

for arg in "$@"; do
    case "$arg" in
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

repo=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)
version_script="${repo}/.github/scripts/release-version.sh"
publish_script="${repo}/.github/scripts/release-publish.sh"
scratch=$(cd "$(mktemp -d)" && pwd -P)
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

# fail reports a check that failed, in error's form as a warning, and counts it, with what was observed and expected.
#
# Parameters:
#   - `$1`: the check's description.
#   - `$2`: what was observed.
#   - `$3`: what was expected.
#
# Returns:
#   - 0.
fail() {
    error 0 "$1"
    failures=$((failures + 1))
    printf -- '---- observed\n%s\n---- expected\n%s\n----\n' "$2" "$3" >&2
}

# is reports a check: passed when what was observed equals what was expected, exactly.
#
# Parameters:
#   - `$1`: the check's description.
#   - `$2`: what was observed.
#   - `$3`: what was expected.
#
# Returns:
#   - 0.
is() {
    if [[ "$2" == "$3" ]]; then
        pass "$1"
    else
        fail "$1" "$2" "$3"
    fi
}

# --- 1. The version comes from the commit ---

# A scratch repository with one commit whose committer time is known, in UTC.
fixture="${scratch}/fixture"
mkdir -p "$fixture"
git -C "$fixture" init --quiet
GIT_AUTHOR_DATE=2026-10-23T12:34:56Z GIT_COMMITTER_DATE=2026-10-23T12:34:56Z \
    git -C "$fixture" -c user.name=Test -c user.email=test@example.invalid commit --quiet --allow-empty -m fixture
stamp=20261023123456

# version_of runs the version script for a ref in the fixture and prints what it wrote, or why it could not run.
#
# Parameters:
#   - `$1`: GITHUB_REF.
#   - `$2`: GITHUB_REF_NAME.
#
# Returns:
#   - 0.
version_of() {
    if [[ ! -x "$version_script" ]]; then
        printf 'no executable %s\n' "${version_script#"$repo"/}"
        return 0
    fi
    (cd "$fixture" && GITHUB_REF="$1" GITHUB_REF_NAME="$2" "$version_script" 2>&1) || printf 'exit %s\n' "$?"
}

for ref in "refs/heads/develop develop v0.1.0-dev.${stamp} true develop" \
    "refs/heads/main main v0.1.0-rc.${stamp} true release" \
    "refs/heads/release/1.2 release/1.2 v1.2-rc.${stamp} true release" \
    "refs/tags/v1.2.3 v1.2.3 v1.2.3 false release"; do
    read -r full name version prerelease channel <<<"$ref"
    expected=$(printf 'version=%s\nprerelease=%s\nchannel=%s' "$version" "$prerelease" "$channel")
    is "the version of ${full} comes from its commit: ${version}" "$(version_of "$full" "$name")" "$expected"
    # A second later, so a version the clock made would differ; compared with the expected, not the first run, so
    # two runs that both fail the same way do not pass.
    sleep 1
    is "a rerun of ${full} gives the same version: ${version}" "$(version_of "$full" "$name")" "$expected"
done

# --- 2. The release step creates, completes, or changes nothing ---

# A stand-in gh, first on PATH: it records each call, one line per call, and answers `release view` as the release
# STAND_IN_RELEASE names would: none, one with every asset, or one lacking the checksums file.
bin="${scratch}/bin"
mkdir -p "$bin"
cat >"${bin}/gh" <<'GH'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"${STAND_IN_CALLS}"
if [[ "$1 $2" == "release view" ]]; then
    case "${STAND_IN_RELEASE}" in
        none)
            echo "release not found" >&2
            exit 1
            ;;
        complete) printf '%s\n' devlore-cli_v1.2.3_checksums.txt devlore-cli_v1.2.3_linux_arm64.tar.gz ;;
        partial) printf '%s\n' devlore-cli_v1.2.3_linux_arm64.tar.gz ;;
    esac
fi
GH
chmod +x "${bin}/gh"
assets="${scratch}/dist"
mkdir -p "$assets"
: >"${assets}/devlore-cli_v1.2.3_checksums.txt"
: >"${assets}/devlore-cli_v1.2.3_linux_arm64.tar.gz"
checksums="${assets}/devlore-cli_v1.2.3_checksums.txt"
archive="${assets}/devlore-cli_v1.2.3_linux_arm64.tar.gz"

# calls_for runs the publish script against the stand-in in one state and prints the calls it made, or why it could
# not run.
#
# Parameters:
#   - `$1`: the release's state: none, complete or partial.
#
# Returns:
#   - 0.
calls_for() {
    local calls="${scratch}/calls-$1"
    : >"$calls"
    if [[ ! -x "$publish_script" ]]; then
        printf 'no executable %s\n' "${publish_script#"$repo"/}"
        return 0
    fi
    PATH="${bin}:${PATH}" STAND_IN_CALLS="$calls" STAND_IN_RELEASE="$1" RELEASE_NOTES="notes" \
        "$publish_script" v1.2.3 0123456789abcdef false "$assets" >/dev/null 2>&1 || printf 'exit %s\n' "$?"
    cat "$calls"
}

view="release view v1.2.3 --json assets --jq .assets[].name"
created="release create v1.2.3 --target 0123456789abcdef --title v1.2.3 --notes notes ${checksums} ${archive}"
is "a release that is not there is created, its assets with it" "$(calls_for none)" \
    "$(printf '%s\n%s' "$view" "$created")"
is "a release with every asset is left as it is" "$(calls_for complete)" "$view"
is "a release lacking an asset has every asset replaced" "$(calls_for partial)" \
    "$(printf '%s\n%s' "$view" "release upload v1.2.3 --clobber ${checksums} ${archive}")"

# --- 3. A rebuild of the commit is the same bytes ---

# This machine's platform, packed as a .tar.gz, and windows/amd64, packed as a .zip: both ways an archive is made.
platforms="$(go env GOOS)/$(go env GOARCH) windows/amd64"
test_version=v0.0.0-test.reproducible
sums="dist/devlore-cli_${test_version}_checksums.txt"

# dist_checksums builds this platform's release with make dist and prints its checksums file, or why it could not.
#
# Parameters:
#   - `$1`: the log to write make's output to.
#
# Returns:
#   - 0.
dist_checksums() {
    if ! (cd "$repo" && env -u VERSION -u COMMIT -u BUILD_DATE make dist PLATFORM="$platforms" \
        DEVLORE_VERSION="$test_version" >"$1" 2>&1); then
        printf 'make dist failed; its output is in %s\n' "$1"
        return 0
    fi
    cat "${repo}/${sums}"
}

note "building ${platforms} twice with make dist, a few seconds apart"
first=$(dist_checksums "${scratch}/dist-1.log")
sleep 2
second=$(dist_checksums "${scratch}/dist-2.log")
is "a second make dist of the commit writes the same archives" "$second" "$first"

if ((failures > 0)); then
    error 1 "${failures} check(s) failed"
fi
success "every check passed"
