// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package result

import (
	"bytes"
	"strings"
	"testing"
	"text/template"
)

// region NewTemplateFormatter

func TestNewTemplateFormatterParsesValidTemplate(t *testing.T) {

	got, err := NewTemplateFormatter("{{.}}")
	if err != nil {
		t.Fatalf("NewTemplateFormatter: %v", err)
	}
	if got == nil {
		t.Fatal("NewTemplateFormatter returned nil formatter")
	}
}

func TestNewTemplateFormatterReportsParseError(t *testing.T) {

	_, err := NewTemplateFormatter("{{.unterminated")
	if err == nil {
		t.Fatal("expected parse error for malformed template; got nil")
	}
	if !strings.Contains(err.Error(), "parse") {
		t.Errorf("error text = %q, want substring 'parse'", err.Error())
	}
}

// endregion

// region Format — happy paths

func TestTemplateFormatterRendersStringValue(t *testing.T) {

	f, err := NewTemplateFormatter("hello {{.}}")
	if err != nil {
		t.Fatalf("NewTemplateFormatter: %v", err)
	}

	var buf bytes.Buffer
	if err := f.Format("world", &buf); err != nil {
		t.Fatalf("Format: %v", err)
	}

	if got := buf.String(); got != "hello world" {
		t.Errorf("Format = %q, want %q", got, "hello world")
	}
}

func TestTemplateFormatterRendersStructFields(t *testing.T) {

	type point struct {
		X, Y int
	}

	f, err := NewTemplateFormatter("({{.X}},{{.Y}})")
	if err != nil {
		t.Fatalf("NewTemplateFormatter: %v", err)
	}

	var buf bytes.Buffer
	if err := f.Format(point{X: 3, Y: 4}, &buf); err != nil {
		t.Fatalf("Format: %v", err)
	}

	if got := buf.String(); got != "(3,4)" {
		t.Errorf("Format = %q, want %q", got, "(3,4)")
	}
}

func TestTemplateFormatterRendersSliceWithRange(t *testing.T) {

	f, err := NewTemplateFormatter("{{range .}}{{.}}\n{{end}}")
	if err != nil {
		t.Fatalf("NewTemplateFormatter: %v", err)
	}

	var buf bytes.Buffer
	if err := f.Format([]string{"a", "b", "c"}, &buf); err != nil {
		t.Fatalf("Format: %v", err)
	}

	if got := buf.String(); got != "a\nb\nc\n" {
		t.Errorf("Format = %q, want %q", got, "a\nb\nc\n")
	}
}

// endregion

// region Format — execution errors

func TestTemplateFormatterReportsExecutionError(t *testing.T) {

	// {{.NotAField}} on a struct without that field — execution-time error.
	f, err := NewTemplateFormatter(`{{.NotAField}}`)
	if err != nil {
		t.Fatalf("NewTemplateFormatter: %v", err)
	}

	type empty struct{}

	var buf bytes.Buffer
	err = f.Format(empty{}, &buf)
	if err == nil {
		t.Fatal("expected execution error for missing field; got nil")
	}
	if !strings.Contains(err.Error(), "execute") {
		t.Errorf("error text = %q, want substring 'execute'", err.Error())
	}
}

// endregion

// region NewTemplateFormatterFromTemplate

func TestNewTemplateFormatterFromTemplateUsesProvidedTemplate(t *testing.T) {

	tmpl := template.Must(template.New("custom").Funcs(template.FuncMap{
		"upper": strings.ToUpper,
	}).Parse(`{{. | upper}}`))

	f := NewTemplateFormatterFromTemplate(tmpl)

	var buf bytes.Buffer
	if err := f.Format("hello", &buf); err != nil {
		t.Fatalf("Format: %v", err)
	}

	if got := buf.String(); got != "HELLO" {
		t.Errorf("Format = %q, want %q", got, "HELLO")
	}
}

// endregion
