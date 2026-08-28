#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.
#
# Invoke-WindowsDevloreTest.sh — cross-compile the devlore-test suite and run it on a Windows host.
#
# The path-dialect defects this exists to catch (#395, #548, #600, #719) are unreachable off Windows:
# file.join only emits backslashes there, so no assertion written on a Unix box can fail for them.
# Waiting for a CI leg puts a fortnight between the change and the symptom. This closes that gap to
# about a minute.
#
# The test binary is built here rather than on the host: go.mod requires a newer toolchain than the
# box carries, and cross-compiling avoids a per-run toolchain download. data/ travels with it because
# testdataDir resolves via runtime.Caller, which bakes in THIS machine's source path —
# DEVLORE_TESTDATA overrides that.
#
# Usage:
#   scripts/Invoke-WindowsDevloreTest.sh [host] [-- <go test args>]
#
#   scripts/Invoke-WindowsDevloreTest.sh
#   scripts/Invoke-WindowsDevloreTest.sh danoble-wd11-3.local -- -run TestImmediateFilePathSeam -v

set -euo pipefail

host="${1:-danoble-wd11-3.local}"
shift || true
[[ "${1:-}" == "--" ]] && shift || true

readonly PKG="./cmd/devlore-test/devloretest"
readonly REMOTE='C:/Users/david-noble/devlore-test'

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${repo_root}"

echo "== building ${PKG} for windows/arm64 =="
GOOS=windows GOARCH=arm64 CGO_ENABLED=0 \
    go test -c -o /tmp/devloretest.exe "${PKG}"

echo "== staging on ${host}:${REMOTE} =="
ssh "${host}" "New-Item -ItemType Directory -Force -Path '${REMOTE}' | Out-Null; Remove-Item -Recurse -Force '${REMOTE}/data' -ErrorAction SilentlyContinue"
scp -q /tmp/devloretest.exe "${host}:${REMOTE}/devloretest.exe"
scp -qr cmd/devlore-test/devloretest/data "${host}:${REMOTE}/data"

echo "== running =="
# -test.v etc. are the compiled-binary spellings of go test's flags.
args=""
for a in "$@"; do
    case "$a" in
    -run) args="${args} -test.run" ;;
    -v) args="${args} -test.v" ;;
    -count=*) args="${args} -test.count=${a#-count=}" ;;
    *) args="${args} ${a}" ;;
    esac
done

ssh "${host}" "\$env:DEVLORE_TESTDATA='${REMOTE}/data'; & '${REMOTE}/devloretest.exe' ${args}; exit \$LASTEXITCODE"
