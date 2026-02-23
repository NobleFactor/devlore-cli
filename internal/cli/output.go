// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

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

// Exit codes follow BSD sysexits.h conventions for portable process status.
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
//
// Commands return errors via Cobra's RunE. To propagate a specific exit code,
// wrap the error with ExitWith:
//
//	return cli.ExitWith(cli.ExitNoInput, fmt.Errorf("file not found: %s", path))
//	return cli.ExitWith(cli.ExitNoPerm, fmt.Errorf("permission denied: %s", path))
//
// In main(), extract the code with ExitCode:
//
//	if err := cmd.Execute(); err != nil {
//	    os.Exit(cli.ExitCode(err))
//	}
//
// Plain errors (without ExitWith) default to exit code 1 (ExitError).
// Commands only need ExitWith when the distinction matters to callers
// checking $? in scripts.

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
// Returns the wrapped code if present, or ExitError (1) for plain errors.
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
// Usage:
//
//	var output cli.OutputFlags
//	cli.AddOutputFlags(cmd, &output)
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
// Mutating commands are silent by default; --passthru outputs what was changed.
// Usage:
//
//	var flags cli.MutationFlags
//	cli.AddMutationFlags(cmd, &flags)
func AddMutationFlags(cmd *cobra.Command, flags *MutationFlags) {
	cmd.Flags().BoolVar(&flags.Passthru, "passthru", false,
		`Output what was changed (for pipelines)`)
	cmd.Flags().StringVar(&flags.Format, "format", DefaultFormat,
		`Output format when using --passthru: json, table, yaml, or a Go template`)
}

// RenderMutation outputs data only if --passthru is set.
// Returns nil without output if passthru is false.
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
// Supported formats: json, yaml, table, or a Go template string.
func Render(w io.Writer, data interface{}, flags OutputFlags) error {
	// Apply filter first
	filtered := applyFilter(data, flags.Filter)

	switch flags.Format {
	case "json":
		return renderJSON(w, filtered)
	case "yaml":
		return renderYAML(w, filtered)
	case "table":
		return renderTable(w, filtered)
	default:
		// Treat as Go template
		return renderTemplate(w, filtered, flags.Format)
	}
}

// RenderTo is a convenience function that renders to stdout.
func RenderTo(data interface{}, flags OutputFlags) error {
	return Render(os.Stdout, data, flags)
}

// renderJSON outputs data as JSON.
func renderJSON(w io.Writer, data interface{}) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(data)
}

// renderYAML outputs data as YAML.
func renderYAML(w io.Writer, data interface{}) error {
	enc := yaml.NewEncoder(w)
	enc.SetIndent(2)
	defer func() { _ = enc.Close() }() //nolint:errcheck // Close error on yaml encoder is not actionable
	return enc.Encode(data)
}

// renderTable outputs data as a formatted table with all fields.
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

	// Header
	headers := make([]string, len(fields))
	for i, f := range fields {
		headers[i] = strings.ToUpper(f)
	}
	_, _ = fmt.Fprintln(tw, strings.Join(headers, "\t")) //nolint:errcheck // tabwriter accumulates errors

	// Rows
	for _, item := range items {
		values := make([]string, len(fields))
		for i, f := range fields {
			values[i] = formatFieldValue(getFieldValue(item, f))
		}
		_, _ = fmt.Fprintln(tw, strings.Join(values, "\t")) //nolint:errcheck // tabwriter accumulates errors
	}

	return tw.Flush()
}

// renderTemplate outputs data using a Go text/template string.
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
		_, _ = fmt.Fprintln(w) //nolint:errcheck // tabwriter accumulates errors
	}
	return nil
}

// =============================================================================
// Filtering
// =============================================================================

// applyFilter filters data using repeatable key=value pairs (AND logic).
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

// matchesAllFilters checks if an item matches all filter expressions (AND).
func matchesAllFilters(item interface{}, filters []string) bool {
	for _, filter := range filters {
		if !matchesFilter(item, filter) {
			return false
		}
	}
	return true
}

// matchesFilter checks if an item matches a single key=value filter.
func matchesFilter(item interface{}, filter string) bool {
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

// toSlice converts data to a slice of interfaces.
func toSlice(data interface{}) []interface{} {
	v := reflect.ValueOf(data)

	// Handle pointer
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	// If not a slice, wrap as single item
	if v.Kind() != reflect.Slice {
		return []interface{}{data}
	}

	result := make([]interface{}, v.Len())
	for i := 0; i < v.Len(); i++ {
		result[i] = v.Index(i).Interface()
	}
	return result
}

// getFieldNames returns field names from a struct or map.
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
	}

	return nil
}

// getFieldValue retrieves a field value from a struct or map.
func getFieldValue(item interface{}, field string) interface{} {
	v := reflect.ValueOf(item)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	switch v.Kind() {
	case reflect.Struct:
		// Try by json tag first
		t := v.Type()
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			jsonName := strings.Split(f.Tag.Get("json"), ",")[0]
			if jsonName == field || strings.EqualFold(f.Name, field) {
				return v.Field(i).Interface()
			}
		}
		// Try direct field name
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

// formatFieldValue converts a value to string for display.
func formatFieldValue(v interface{}) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}

// =============================================================================
// Status Output Functions
// =============================================================================
//
// Thin wrappers around ui.Provider. All 136 call sites remain unchanged.
// The ui.Provider is the single implementation for both Go callers and
// Starlark immediate receivers.

// output is the package-level ui.Provider instance.
var output = &ui.Provider{
	Writer: os.Stderr,
	Color:  true,
}

// SetProgramName sets the program name used in output prefixes.
func SetProgramName(name string) {
	output.ProgramName = name
}

// SetSilent enables or disables silent mode.
func SetSilent(s bool) {
	output.Silent = s
}

// AddSilentFlag adds the --silent flag to a root command.
func AddSilentFlag(cmd *cobra.Command) {
	cmd.PersistentFlags().BoolVar(&output.Silent, "silent", false,
		`Suppress all status messages (stderr)`)
}

// Note prints an informational message to stderr.
func Note(format string, args ...interface{}) {
	output.Note(fmt.Sprintf(format, args...))
}

// Warn prints a warning message to stderr.
func Warn(format string, args ...interface{}) {
	output.Warn(fmt.Sprintf(format, args...))
}

// Error prints an error message to stderr.
// Unlike Failure, this does not return an error—use for non-fatal errors.
func Error(format string, args ...interface{}) {
	output.Error(fmt.Sprintf(format, args...))
}

// Failure prints an error message to stderr and returns an error.
// Use when the operation cannot continue.
func Failure(format string, args ...interface{}) error {
	return output.Fail(fmt.Sprintf(format, args...))
}

// Success prints a success message to stderr.
func Success(format string, args ...interface{}) {
	output.Success(fmt.Sprintf(format, args...))
}
