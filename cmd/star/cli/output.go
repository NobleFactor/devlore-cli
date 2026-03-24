// SPDX-License-Identifier: MIT
// Copyright Noble Factor. All rights reserved.

// Package cli provides CLI utilities for output formatting and user interaction.
package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"text/tabwriter"
	"text/template"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/NobleFactor/devlore-cli/pkg/op/provider/ui"
)

// =============================================================================
// Exit Codes (BSD sysexits.h)
// =============================================================================

// Exit codes following BSD sysexits.h conventions.
const (
	ExitOK          = 0  // Success
	ExitError       = 1  // Generic error
	ExitUsage       = 64 // Bad CLI syntax
	ExitDataErr     = 65 // Invalid manifest/config
	ExitNoInput     = 66 // File not found
	ExitUnavailable = 69 // Registry unreachable
	ExitSoftware    = 70 // Internal error (bug)
	ExitCantCreate  = 73 // Can't create file/symlink
	ExitIOErr       = 74 // Read/write failure
	ExitNoPerm      = 77 // Permission denied
)

// =============================================================================
// Exit Code Errors
// =============================================================================

// exitError wraps an error with a specific exit code.
type exitError struct {
	code int
	err  error
}

func (e *exitError) Error() string { return e.err.Error() }
func (e *exitError) Unwrap() error { return e.err }

// ExitWith returns an error that carries a specific exit code.
func ExitWith(code int, err error) error {
	return &exitError{code: code, err: err}
}

// ExitCode extracts the exit code from an error.
func ExitCode(err error) int {
	if err == nil {
		return ExitOK
	}
	var coded *exitError
	if errors.As(err, &coded) {
		return coded.code
	}
	return ExitError
}

// =============================================================================
// Output Flags
// =============================================================================

// OutputFlags holds the --filter and --format flag values.
type OutputFlags struct {
	Format string
	Filter []string
}

// DefaultFormat is the default output format (JSON for scriptability).
const DefaultFormat = "json"

// AddOutputFlags adds --filter and --format flags to a command.
func AddOutputFlags(cmd *cobra.Command, flags *OutputFlags) {
	cmd.Flags().StringVar(&flags.Format, "format", DefaultFormat,
		`Output format: json, table, yaml, or a Go template (e.g. '{{.Name}}\t{{.Version}}')`)
	cmd.Flags().StringArrayVar(&flags.Filter, "filter", nil,
		`Filter expression: field=value (repeatable, AND logic)`)
}

// MutationFlags holds flags for mutating commands (--passthru).
type MutationFlags struct {
	Passthru bool
	Format   string
}

// AddMutationFlags adds --passthru and --format flags for mutating commands.
func AddMutationFlags(cmd *cobra.Command, flags *MutationFlags) {
	cmd.Flags().BoolVar(&flags.Passthru, "passthru", false,
		`Output what was changed (for pipelines)`)
	cmd.Flags().StringVar(&flags.Format, "format", DefaultFormat,
		`Output format when using --passthru: json, table, yaml, or a Go template`)
}

// RenderMutation outputs data only if --passthru is set.
func RenderMutation(w io.Writer, data interface{}, flags MutationFlags) error {
	if !flags.Passthru {
		return nil
	}
	return Render(w, data, OutputFlags{Format: flags.Format})
}

// RenderMutationTo is a convenience function that renders to stdout if --passthru is set.
func RenderMutationTo(data interface{}, flags MutationFlags) error {
	return RenderMutation(os.Stdout, data, flags)
}

// =============================================================================
// Rendering
// =============================================================================

// Render outputs data according to the format specification.
func Render(w io.Writer, data interface{}, flags OutputFlags) error {
	filtered := applyFilter(data, flags.Filter)

	switch flags.Format {
	case "json":
		return renderJSON(w, filtered)
	case "yaml":
		return renderYAML(w, filtered)
	case "table":
		return renderTable(w, filtered)
	default:
		return renderTemplate(w, filtered, flags.Format)
	}
}

// RenderTo is a convenience function that renders to stdout.
func RenderTo(data interface{}, flags OutputFlags) error {
	return Render(os.Stdout, data, flags)
}

func renderJSON(w io.Writer, data interface{}) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(data)
}

func renderYAML(w io.Writer, data interface{}) error {
	enc := yaml.NewEncoder(w)
	enc.SetIndent(2)
	if err := enc.Encode(data); err != nil {
		return err
	}
	return enc.Close()
}

func renderTable(w io.Writer, data interface{}) error {
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
	if _, err := fmt.Fprintln(tw, strings.Join(headers, "\t")); err != nil {
		return err
	}

	for _, item := range items {
		values := make([]string, len(fields))
		for i, f := range fields {
			values[i] = formatFieldValue(getFieldValue(item, f))
		}
		if _, err := fmt.Fprintln(tw, strings.Join(values, "\t")); err != nil {
			return err
		}
	}

	return tw.Flush()
}

func renderTemplate(w io.Writer, data interface{}, tmplStr string) error {
	tmpl, err := template.New("output").Parse(tmplStr)
	if err != nil {
		return fmt.Errorf("invalid template: %w", err)
	}

	items := toSlice(data)
	for _, item := range items {
		if err := tmpl.Execute(w, item); err != nil {
			return fmt.Errorf("template execution: %w", err)
		}
		if _, err := fmt.Fprintln(w); err != nil {
			return fmt.Errorf("writing newline: %w", err)
		}
	}
	return nil
}

// =============================================================================
// Filtering
// =============================================================================

func applyFilter(data interface{}, filters []string) interface{} {
	if len(filters) == 0 {
		return data
	}

	items := toSlice(data)
	if len(items) == 0 {
		return data
	}

	var result []interface{}
	for _, item := range items {
		if matchesAllFilters(item, filters) {
			result = append(result, item)
		}
	}
	return result
}

func matchesAllFilters(item interface{}, filters []string) bool {
	for _, filter := range filters {
		if !matchesFilter(item, filter) {
			return false
		}
	}
	return true
}

func matchesFilter(item interface{}, filter string) bool {
	idx := strings.Index(filter, "=")
	if idx == -1 {
		return true
	}

	field := strings.TrimSpace(filter[:idx])
	expected := strings.TrimSpace(filter[idx+1:])
	value := formatFieldValue(getFieldValue(item, field))

	return strings.EqualFold(value, expected)
}

// =============================================================================
// Reflection Helpers
// =============================================================================

func toSlice(data interface{}) []interface{} {
	v := reflect.ValueOf(data)

	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	if v.Kind() != reflect.Slice {
		return []interface{}{data}
	}

	result := make([]interface{}, v.Len())
	for i := 0; i < v.Len(); i++ {
		result[i] = v.Index(i).Interface()
	}
	return result
}

func getFieldNames(item interface{}) []string {
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
			if f.PkgPath == "" {
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
	}

	return nil
}

func getFieldValue(item interface{}, field string) interface{} {
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
	}

	return nil
}

func formatFieldValue(v interface{}) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}

// =============================================================================
// Status Output — delegates to ui.Provider
// =============================================================================

var uiProvider = &ui.Provider{
	Writer:      os.Stderr,
	ProgramName: "star",
	Color:       true,
}

// SetUIProvider sets the backing ui.Provider for all status output functions.
func SetUIProvider(p *ui.Provider) {
	uiProvider = p
}

// Note prints an informational message to stderr.
func Note(format string, args ...interface{}) {
	uiProvider.Note(fmt.Sprintf(format, args...))
}

// Warn prints a warning message to stderr.
func Warn(format string, args ...interface{}) {
	uiProvider.Warn(fmt.Sprintf(format, args...))
}

// Error prints an error message to stderr.
func Error(format string, args ...interface{}) {
	uiProvider.Error(fmt.Sprintf(format, args...))
}

// Success prints a success message to stderr.
func Success(format string, args ...interface{}) {
	uiProvider.Success(fmt.Sprintf(format, args...))
}

// Failure prints an error message to stderr and returns an error.
func Failure(format string, args ...interface{}) error {
	return uiProvider.Fail(fmt.Sprintf(format, args...))
}
