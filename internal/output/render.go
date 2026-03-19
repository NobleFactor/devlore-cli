// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package output provides structured-data rendering for CLI commands.
//
// The single entry point is Render, which transforms data into JSON, YAML,
// table, or Go-template format and writes it to the supplied io.Writer.
// Callers control where output goes; the package never assumes stdout.
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"
	"text/tabwriter"
	"text/template"

	"gopkg.in/yaml.v3"
)

// DefaultFormat is the default output format (JSON for scriptability).
const DefaultFormat = "json"

// Options controls how Render transforms and filters data.
type Options struct {
	// Format selects the serializer: "json", "yaml", "table",
	// or a Go text/template string.
	Format string

	// Filter is a set of repeatable key=value predicates (AND logic).
	// Items that do not match every predicate are excluded.
	Filter []string
}

// Render transforms data according to options and writes it to w.
//
// Supported formats:
//   - "json"  — indented JSON
//   - "yaml"  — indented YAML
//   - "table" — aligned columns with headers derived from struct/map fields
//   - anything else is parsed as a Go text/template
func Render(w io.Writer, data any, options Options) error {
	filtered := applyFilter(data, options.Filter)

	switch options.Format {
	case "json":
		return renderJSON(w, filtered)
	case "yaml":
		return renderYAML(w, filtered)
	case "table":
		return renderTable(w, filtered)
	default:
		return renderTemplate(w, filtered, options.Format)
	}
}

// =============================================================================
// Format Renderers
// =============================================================================

func renderJSON(w io.Writer, data any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(data)
}

func renderYAML(w io.Writer, data any) error {
	enc := yaml.NewEncoder(w)
	enc.SetIndent(2)
	defer func() { _ = enc.Close() }() //nolint:errcheck // Close error on yaml encoder is not actionable
	return enc.Encode(data)
}

func renderTable(w io.Writer, data any) error {
	items := toSlice(data)
	if len(items) == 0 {
		return nil
	}

	fields := getFieldNames(items[0])
	if len(fields) == 0 {
		return nil
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)

	headers := make([]string, len(fields))
	for i, f := range fields {
		headers[i] = strings.ToUpper(f)
	}
	_, _ = fmt.Fprintln(tw, strings.Join(headers, "\t")) //nolint:errcheck // tabwriter accumulates errors

	for _, item := range items {
		values := make([]string, len(fields))
		for i, f := range fields {
			values[i] = formatFieldValue(getFieldValue(item, f))
		}
		_, _ = fmt.Fprintln(tw, strings.Join(values, "\t")) //nolint:errcheck // tabwriter accumulates errors
	}

	return tw.Flush()
}

func renderTemplate(w io.Writer, data any, tmplStr string) error {
	tmpl, err := template.New("output").Parse(tmplStr)
	if err != nil {
		return fmt.Errorf("invalid template: %w", err)
	}

	items := toSlice(data)
	for _, item := range items {
		if err := tmpl.Execute(w, item); err != nil {
			return fmt.Errorf("template execution: %w", err)
		}
		_, _ = fmt.Fprintln(w) //nolint:errcheck // best-effort newline after template row
	}
	return nil
}

// =============================================================================
// Filtering
// =============================================================================

func applyFilter(data any, filters []string) any {
	if len(filters) == 0 {
		return data
	}

	items := toSlice(data)
	if len(items) == 0 {
		return data
	}

	var result []any
	for _, item := range items {
		if matchesAllFilters(item, filters) {
			result = append(result, item)
		}
	}
	return result
}

func matchesAllFilters(item any, filters []string) bool {
	for _, filter := range filters {
		if !matchesFilter(item, filter) {
			return false
		}
	}
	return true
}

func matchesFilter(item any, filter string) bool {
	idx := strings.Index(filter, "=")
	if idx == -1 {
		return true // invalid filter, skip
	}

	field := strings.TrimSpace(filter[:idx])
	expected := strings.TrimSpace(filter[idx+1:])
	value := formatFieldValue(getFieldValue(item, field))

	return strings.EqualFold(value, expected)
}

// =============================================================================
// Reflection Helpers
// =============================================================================

func toSlice(data any) []any {
	v := reflect.ValueOf(data)

	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	if v.Kind() != reflect.Slice {
		return []any{data}
	}

	result := make([]any, v.Len())
	for i := 0; i < v.Len(); i++ {
		result[i] = v.Index(i).Interface()
	}
	return result
}

func getFieldNames(item any) []string {
	v := reflect.ValueOf(item)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	switch v.Kind() {
	case reflect.Struct:
		t := v.Type()
		names := make([]string, 0, t.NumField())
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if f.PkgPath == "" { // exported
				name := f.Tag.Get("json")
				if name == "" || name == "-" {
					name = strings.ToLower(f.Name)
				} else {
					name = strings.Split(name, ",")[0]
				}
				names = append(names, name)
			}
		}
		return names

	case reflect.Map:
		keys := v.MapKeys()
		names := make([]string, len(keys))
		for i, k := range keys {
			names[i] = fmt.Sprintf("%v", k.Interface())
		}
		return names

	default:
		return nil
	}
}

func getFieldValue(item any, field string) any {
	v := reflect.ValueOf(item)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	switch v.Kind() {
	case reflect.Struct:
		t := v.Type()
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			jsonName := strings.Split(f.Tag.Get("json"), ",")[0]
			if jsonName == field || strings.EqualFold(f.Name, field) {
				return v.Field(i).Interface()
			}
		}
		fv := v.FieldByName(field)
		if fv.IsValid() {
			return fv.Interface()
		}

	case reflect.Map:
		mv := v.MapIndex(reflect.ValueOf(field))
		if mv.IsValid() {
			return mv.Interface()
		}

	default:
		// No fields for non-struct/non-map kinds.
	}

	return nil
}

func formatFieldValue(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}
