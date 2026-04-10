// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"strings"
	"unicode"
)

// CamelToSnake converts a CamelCase Go identifier to snake_case.
//
// Parameters:
//   - s: the CamelCase string (e.g., "WriteText", "ReadBytes").
//
// Returns:
//   - string: the snake_case equivalent (e.g., "write_text", "read_bytes").
func CamelToSnake(s string) string {

	runes := []rune(s)
	var b strings.Builder
	b.Grow(len(s) + 4)

	for i, r := range runes {
		if unicode.IsUpper(r) {
			if i > 0 {
				prev := runes[i-1]
				if unicode.IsLower(prev) || unicode.IsDigit(prev) {
					b.WriteRune('_')
				} else if unicode.IsUpper(prev) && i+1 < len(runes) && unicode.IsLower(runes[i+1]) {
					b.WriteRune('_')
				}
			}
			b.WriteRune(unicode.ToLower(r))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}
