// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"errors"
	"fmt"
	"strings"
	"text/template"
)

// RenderError formats a Go template string with positional args and
// keyword args, returning the result as an error.
//
// Template data available to the format string:
//
//	{{ .key }}          — kwargs value
//	{{ index .Args 0 }} — positional arg by index
//
// If the format string contains no template directives, it passes
// through as a plain string.
func RenderError(format string, args []any, kwargs map[string]any) error {
	data := make(map[string]any, len(kwargs)+1)
	for k, v := range kwargs {
		data[k] = v
	}
	data["Args"] = args

	tmpl, err := template.New("msg").Parse(format)
	if err != nil {
		return fmt.Errorf("render: %w", err)
	}
	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("render: %w", err)
	}
	return errors.New(buf.String())
}
