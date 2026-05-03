// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package result

import (
	"reflect"
	"strings"
	"testing"
)

// region FormatterByName

func TestFormatterByNameKnownFormatters(t *testing.T) {

	for _, tc := range []struct {
		name string
		want any
	}{
		{"json", JSONFormatter{}},
		{"JSON", JSONFormatter{}},
		{"  yaml  ", YAMLFormatter{}},
		{"csv", CSVFormatter{}},
	} {
		t.Run(tc.name, func(t *testing.T) {

			got, err := FormatterByName(tc.name, "")
			if err != nil {
				t.Fatalf("FormatterByName(%q): %v", tc.name, err)
			}
			if reflect.TypeOf(got) != reflect.TypeOf(tc.want) {
				t.Errorf("FormatterByName(%q) = %T, want %T", tc.name, got, tc.want)
			}
		})
	}
}

func TestFormatterByNameTemplateRequiresText(t *testing.T) {

	_, err := FormatterByName("template", "")
	if err == nil {
		t.Fatal("expected error for empty template text; got nil")
	}
	if !strings.Contains(err.Error(), "non-empty") {
		t.Errorf("error text = %q, want substring 'non-empty'", err.Error())
	}
}

func TestFormatterByNameTemplateUsesText(t *testing.T) {

	got, err := FormatterByName("template", "{{.}}")
	if err != nil {
		t.Fatalf("FormatterByName: %v", err)
	}
	if _, ok := got.(*TemplateFormatter); !ok {
		t.Errorf("FormatterByName(template) = %T, want *TemplateFormatter", got)
	}
}

func TestFormatterByNameTemplateReportsParseError(t *testing.T) {

	_, err := FormatterByName("template", "{{.unterminated")
	if err == nil {
		t.Fatal("expected parse error; got nil")
	}
}

func TestFormatterByNameUnknownErrors(t *testing.T) {

	_, err := FormatterByName("xml", "")
	if err == nil {
		t.Fatal("expected error for unknown formatter; got nil")
	}
	if !strings.Contains(err.Error(), "unknown formatter") {
		t.Errorf("error text = %q, want substring 'unknown formatter'", err.Error())
	}
}

// endregion

// region FilterByExprs

func TestFilterByExprsEmptyReturnsNoOpFilter(t *testing.T) {

	got, err := FilterByExprs(nil, "")
	if err != nil {
		t.Fatalf("FilterByExprs: %v", err)
	}
	if _, ok := got.(NoOpFilter); !ok {
		t.Errorf("FilterByExprs(empty) = %T, want NoOpFilter", got)
	}
}

func TestFilterByExprsFieldOnlyReturnsFieldFilter(t *testing.T) {

	got, err := FilterByExprs([]string{"k=v"}, "")
	if err != nil {
		t.Fatalf("FilterByExprs: %v", err)
	}
	if _, ok := got.(*FieldFilter); !ok {
		t.Errorf("FilterByExprs(field-only) = %T, want *FieldFilter", got)
	}
}

func TestFilterByExprsJQOnlyReturnsJQFilter(t *testing.T) {

	got, err := FilterByExprs(nil, ".name")
	if err != nil {
		t.Fatalf("FilterByExprs: %v", err)
	}
	if _, ok := got.(*JQFilter); !ok {
		t.Errorf("FilterByExprs(jq-only) = %T, want *JQFilter", got)
	}
}

func TestFilterByExprsBothComposes(t *testing.T) {

	got, err := FilterByExprs([]string{"kind=file"}, ".[].name")
	if err != nil {
		t.Fatalf("FilterByExprs: %v", err)
	}
	if _, ok := got.(chainFilter); !ok {
		t.Errorf("FilterByExprs(both) = %T, want chainFilter", got)
	}
}

func TestFilterByExprsRunsStagesInOrder(t *testing.T) {

	in := []map[string]any{
		{"kind": "file", "name": "a"},
		{"kind": "dir", "name": "b"},
		{"kind": "file", "name": "c"},
	}

	f, err := FilterByExprs([]string{"kind=file"}, ".[].name")
	if err != nil {
		t.Fatalf("FilterByExprs: %v", err)
	}

	got, err := f.Apply(in)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	want := []any{"a", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Apply = %v, want %v", got, want)
	}
}

func TestFilterByExprsReportsFieldParseError(t *testing.T) {

	_, err := FilterByExprs([]string{"missingequals"}, "")
	if err == nil {
		t.Fatal("expected field-parse error; got nil")
	}
	if !strings.Contains(err.Error(), "missing '='") {
		t.Errorf("error text = %q, want substring \"missing '='\"", err.Error())
	}
}

func TestFilterByExprsReportsJQParseError(t *testing.T) {

	_, err := FilterByExprs(nil, "(((")
	if err == nil {
		t.Fatal("expected jq-parse error; got nil")
	}
	if !strings.Contains(err.Error(), "parse") {
		t.Errorf("error text = %q, want substring 'parse'", err.Error())
	}
}

// endregion
