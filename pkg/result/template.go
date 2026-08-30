// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package result

import (
	"fmt"
	"io"
	"text/template"
)

// TemplateFormatter renders a value through a [text/template.Template].
//
// Reached as `--output template=<body>`, never as a separate flag: the format value carries its own argument,
// so there is no pairing to get wrong and no state where a body is supplied and ignored.
//
// The value passed to [Format] is the template's `.` binding. The template is parsed once at construction, so
// a malformed body fails when the pipeline is built rather than per emission.
//
// Most reshaping belongs in the filter stage instead. `--jq` selects, maps, and interpolates, and composes
// with every format; a template earns its place only for text layout a query cannot express.
type TemplateFormatter struct {
	template *template.Template
}

// Compile-time interface guard.
var _ Formatter = (*TemplateFormatter)(nil)

// NewTemplateFormatter parses body and returns the formatter that renders through it.
//
// Parameters:
//   - `body`: the template text; the value being rendered is its `.` binding.
//
// Returns:
//   - `*TemplateFormatter`: the formatter.
//   - `error`: when the body does not parse.
func NewTemplateFormatter(body string) (*TemplateFormatter, error) {

	parsed, err := template.New("output").Parse(body)
	if err != nil {
		return nil, fmt.Errorf("result.NewTemplateFormatter: %w", err)
	}

	return &TemplateFormatter{template: parsed}, nil
}

// region Formatter

// Format executes the template against value, writing to w.
//
// Parameters:
//   - `value`: the template's `.` binding.
//   - `w`: the destination.
//
// Returns:
//   - `error`: any execution or write error.
func (t *TemplateFormatter) Format(value any, w io.Writer) error {

	if err := t.template.Execute(w, value); err != nil {
		return fmt.Errorf("result.TemplateFormatter: %w", err)
	}

	return nil
}

// endregion
