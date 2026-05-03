// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package result

import (
	"fmt"
	"io"
	"text/template"
)

// TemplateFormatter renders the value through a [text/template.Template]. The template text is
// supplied at construction; the value passed to [Format] is the template's `.` (dot) data binding.
//
// Construct via [NewTemplateFormatter]. The template is parsed once at construction; parse errors
// surface there rather than per-Emit. Callers passing user-supplied template text should surface
// parse errors directly to the user (no silent fallback).
type TemplateFormatter struct {
	template *template.Template
}

// Compile-time interface guard.
var _ Formatter = (*TemplateFormatter)(nil)

// NewTemplateFormatter parses text into a [text/template.Template] and returns a [Formatter] that
// applies it to each emitted value. The template's name is "result" — referenced by other templates
// via {{template "result" .}}. Functions registered via the standard text/template helpers are not
// pre-installed; callers needing helpers can construct a template separately and pass it via
// [NewTemplateFormatterFromTemplate].
//
// Parameters:
//   - text: the template body.
//
// Returns:
//   - *TemplateFormatter: the formatter.
//   - error: when text fails to parse.
func NewTemplateFormatter(text string) (*TemplateFormatter, error) {

	tmpl, err := template.New("result").Parse(text)
	if err != nil {
		return nil, fmt.Errorf("result.TemplateFormatter: parse: %w", err)
	}
	return &TemplateFormatter{template: tmpl}, nil
}

// NewTemplateFormatterFromTemplate wraps an already-constructed [text/template.Template]. Use this
// when the caller needs to register custom Funcs, parse multiple files, or otherwise configure the
// template beyond what [NewTemplateFormatter] supports.
//
// Parameters:
//   - tmpl: the pre-constructed template; must not be nil.
//
// Returns:
//   - *TemplateFormatter: the formatter.
func NewTemplateFormatterFromTemplate(tmpl *template.Template) *TemplateFormatter {
	return &TemplateFormatter{template: tmpl}
}

// Format applies the template to value, writing the rendered bytes to w. Execution errors (e.g.,
// missing fields with template option "missingkey=error") are propagated as-is.
func (t *TemplateFormatter) Format(value any, w io.Writer) error {

	if err := t.template.Execute(w, value); err != nil {
		return fmt.Errorf("result.TemplateFormatter: execute: %w", err)
	}
	return nil
}
