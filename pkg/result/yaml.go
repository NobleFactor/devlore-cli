// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package result

import (
	"io"

	"gopkg.in/yaml.v3"
)

// YAMLFormatter renders the value as indented YAML. The default encoding settings match the existing
// internal/output package's `yaml` format: two-space indentation, latest stable yaml.v3 from
// go-yaml.
type YAMLFormatter struct{}

// Compile-time interface guard.
var _ Formatter = YAMLFormatter{}

// Format encodes value as indented YAML to w.
//
// The yaml.Encoder is closed before return to flush trailing bytes. Close errors are wrapped in the
// returned error chain via the deferred sequence.
func (YAMLFormatter) Format(value any, w io.Writer) (err error) {

	enc := yaml.NewEncoder(w)
	enc.SetIndent(2)
	defer func() {
		if closeErr := enc.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()
	return enc.Encode(value)
}
