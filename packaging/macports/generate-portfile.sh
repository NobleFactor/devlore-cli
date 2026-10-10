#!/usr/bin/env bash

# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# generate-portfile.sh - Generate MacPorts Portfile from template
# Called by GoReleaser as a post hook

set -o errexit -o errtrace -o nounset -o pipefail

# Declare-BashScript's functions and constants this script uses, as NobleFactor/noblefactor-ops c942f54 has them,
# copied by hand: nothing here sources the helper (#1037). Copy them again when this script needs a newer one.

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

# The downloaded source tarball, made once the version is known; cleanup removes it on every way out.
TMPFILE=""

# cleanup removes the downloaded source tarball, whichever way the script ends.
#
# Parameters:
#   - none.
#
# Returns:
#   - 0.
function cleanup {
    [[ -z "${TMPFILE}" ]] || rm -f "${TMPFILE}"
}

Set-Traps cleanup

VERSION="${1:-}"
if [[ -z "$VERSION" ]]; then
    error $EX_USAGE "usage: generate-portfile.sh <version>"
fi

# Remove 'v' prefix if present
VERSION="${VERSION#v}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TEMPLATE="$SCRIPT_DIR/Portfile.template"
OUTPUT="${SCRIPT_DIR}/../../dist/Portfile"

TARBALL_URL="https://github.com/NobleFactor/devlore-cli/archive/v${VERSION}.tar.gz"
TMPFILE=$(mktemp)

note "Downloading source tarball for checksums..."
curl -sSL "$TARBALL_URL" -o "$TMPFILE"

# Calculate checksums
SHA256=$(shasum -a 256 "$TMPFILE" | cut -d' ' -f1)
RMD160=$(openssl dgst -rmd160 "$TMPFILE" 2>/dev/null | awk '{print $NF}')
SIZE=$(stat -f%z "$TMPFILE" 2>/dev/null || stat -c%s "$TMPFILE" 2>/dev/null)

note "  SHA256: $SHA256"
note "  RMD160: $RMD160"
note "  SIZE:   $SIZE"

# Generate Portfile from template
mkdir -p "$(dirname "$OUTPUT")"
sed -e "s/{{VERSION}}/$VERSION/g" \
    -e "s/{{SHA256}}/$SHA256/g" \
    -e "s/{{RMD160}}/$RMD160/g" \
    -e "s/{{SIZE}}/$SIZE/g" \
    "$TEMPLATE" >"$OUTPUT"

success "Generated: $OUTPUT"
