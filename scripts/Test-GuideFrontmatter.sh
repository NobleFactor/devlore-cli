#!/usr/bin/env bash
# SPDX-License-Identifier: SSPL-1.0
# Copyright (c) 2025-2026 Noble Factor. All rights reserved.
#
# Validates that all guide markdown files have required Astro frontmatter.
# The devlore.noblefactor.com site requires: title, description, tool, category, order

set -euo pipefail

GUIDES_DIR="docs/guides"
ERRORS=0

# Required frontmatter fields for guides
REQUIRED_FIELDS=("title" "description" "tool" "category" "order")

validate_file() {
    local file="$1"
    local missing_fields=()

    # Check if file starts with ---
    if ! head -1 "$file" | grep -q "^---$"; then
        echo "ERROR: $file - Missing frontmatter (file must start with ---)"
        return 1
    fi

    # Extract frontmatter and check for required fields
    local frontmatter
    frontmatter=$(sed -n '1,/^---$/p' "$file" | tail -n +2 | head -n -1)

    for field in "${REQUIRED_FIELDS[@]}"; do
        if ! echo "$frontmatter" | grep -q "^${field}:"; then
            missing_fields+=("$field")
        fi
    done

    if [[ ${#missing_fields[@]} -gt 0 ]]; then
        echo "ERROR: $file - Missing required fields: ${missing_fields[*]}"
        return 1
    fi

    return 0
}

# Find all guide markdown files
while IFS= read -r -d '' file; do
    if ! validate_file "$file"; then
        ((ERRORS++))
    fi
done < <(find "$GUIDES_DIR" -name "*.md" -print0)

if [[ $ERRORS -gt 0 ]]; then
    echo ""
    echo "Found $ERRORS file(s) with invalid frontmatter."
    echo ""
    echo "Required frontmatter format:"
    echo "---"
    echo 'title: "Page Title"'
    echo 'description: "Brief description"'
    echo 'tool: "lore"  # or "writ" or "devlore"'
    echo 'category: "overview"  # or "tutorial", "concept", "reference"'
    echo 'order: 1'
    echo "---"
    exit 1
fi

echo "All guide files have valid frontmatter."
exit 0
