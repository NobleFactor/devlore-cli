#!/usr/bin/env bash

# SPDX-License-Identifier: MIT
# Copyright (c) 2025-2026 Noble Factor. All rights reserved.

# generate-portfile.sh - Generate MacPorts Portfile from template
# Called by GoReleaser as a post hook

set -euo pipefail

VERSION="${1:-}"
if [[ -z "$VERSION" ]]; then
	echo "Usage: generate-portfile.sh <version>" >&2
	exit 1
fi

# Remove 'v' prefix if present
VERSION="${VERSION#v}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TEMPLATE="$SCRIPT_DIR/Portfile.template"
OUTPUT="${SCRIPT_DIR}/../../dist/Portfile"

TARBALL_URL="https://github.com/NobleFactor/devlore-cli/archive/v${VERSION}.tar.gz"
TMPFILE=$(mktemp)
trap 'rm -f "$TMPFILE"' EXIT

echo "Downloading source tarball for checksums..."
curl -sSL "$TARBALL_URL" -o "$TMPFILE"

# Calculate checksums
SHA256=$(shasum -a 256 "$TMPFILE" | cut -d' ' -f1)
RMD160=$(openssl dgst -rmd160 "$TMPFILE" 2>/dev/null | awk '{print $NF}')
SIZE=$(stat -f%z "$TMPFILE" 2>/dev/null || stat -c%s "$TMPFILE" 2>/dev/null)

echo "  SHA256: $SHA256"
echo "  RMD160: $RMD160"
echo "  SIZE:   $SIZE"

# Generate Portfile from template
mkdir -p "$(dirname "$OUTPUT")"
sed -e "s/{{VERSION}}/$VERSION/g" \
	-e "s/{{SHA256}}/$SHA256/g" \
	-e "s/{{RMD160}}/$RMD160/g" \
	-e "s/{{SIZE}}/$SIZE/g" \
	"$TEMPLATE" >"$OUTPUT"

echo "Generated: $OUTPUT"
