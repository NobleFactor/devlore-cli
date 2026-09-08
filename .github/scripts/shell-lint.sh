#!/usr/bin/env bash

# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# shell-lint.sh - Lint shell scripts with shfmt and shellcheck
#
# Finds all files with shell shebangs or shellcheck directives,
# then runs shfmt and shellcheck on each file.
#
# A consumer's `source=Declare-BashScript` directive resolves through -P. The helper ships from the base
# layer (noblefactor-ops), so the directory is the developer's deployed ~/.local/bin by default and
# DECLARE_BASHSCRIPT_DIR wherever that is not where it lives. Without it every consumer reports its
# variables unassigned (SC2154) and this gate disagrees with CI. (Do not begin a comment line with the
# word this tool is named after: it is parsed as a directive.)

failed=0
# Discovery is the git tree, not a filesystem walk. What CI lints is what CI checks out, so a build
# artifact or a scratch file has no business deciding whether this gate passes. It is also the difference
# between reading 1,617 files and reading 55,153 of them in a built clone, where build/ alone is 1.1 GB.
files=$(
    git ls-files -z |
        while IFS= read -r -d '' file; do
            [ -f "$file" ] || continue
            head -n1 "$file" 2>/dev/null |
                grep -qE "^#!/(usr/bin/env[[:space:]]+)?(sh|bash)\b|^# shellcheck shell=" &&
                echo "$file"
        done |
        sort
)

for f in $files; do
    shfmt_ok=true
    shellcheck_ok=true
    shfmt -d -i 4 -ci "$f" >/dev/null 2>&1 || shfmt_ok=false
    shellcheck -x --severity=warning -P "${DECLARE_BASHSCRIPT_DIR:-$HOME/.local/bin}" "$f" >/dev/null 2>&1 || shellcheck_ok=false
    if $shfmt_ok && $shellcheck_ok; then
        echo "  ok $f"
    else
        echo "FAIL $f"
        $shfmt_ok || echo "  shfmt: FAIL"
        $shellcheck_ok || echo "  shellcheck: FAIL"
        failed=1
    fi
done

exit $failed
