#!/usr/bin/env bash

# SPDX-License-Identifier: SSPL-1.0
# Copyright (c) 2025-2026 Noble Factor. All rights reserved.

# shell-lint.sh - Lint shell scripts with shfmt and shellcheck
#
# Finds all files with shell shebangs or shellcheck directives,
# then runs shfmt and shellcheck on each file.

failed=0
files=$(
    find . -path ./.git -prune -o -type f -print 2>/dev/null |
        while read -r file; do
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
    shellcheck -x --severity=warning "$f" >/dev/null 2>&1 || shellcheck_ok=false
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
