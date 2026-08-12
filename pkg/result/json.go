// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package result

import (
	"encoding/json"
	"io"
)

// JSONFormatter renders the value as indented JSON. Two-space indentation; no HTML escaping
// concerns (the stream is for tooling consumption, not browser embedding).
type JSONFormatter struct{}

// Compile-time interface guard.
var _ Formatter = JSONFormatter{}

// Format encodes value as indented JSON to w.
func (JSONFormatter) Format(value any, w io.Writer) error {

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}
