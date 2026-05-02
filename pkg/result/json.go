// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package result

import (
	"encoding/json"
	"io"
)

// JSONFormatter renders the value as indented JSON. The default encoding settings match the existing
// internal/output package's `json` format: two-space indentation, no HTML escaping concerns (the
// stream is for tooling consumption, not browser embedding).
type JSONFormatter struct{}

// Compile-time interface guard.
var _ Formatter = JSONFormatter{}

// Format encodes value as indented JSON to w.
func (JSONFormatter) Format(value any, w io.Writer) error {

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}
