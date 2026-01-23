// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"regexp"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// OutputFlags holds the --filter and --format flag values.
type OutputFlags struct {
	Format   string
	Filter   string
	Passthru bool
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
		`Output format: json, table, table(field,...), value(field,...), yaml`)
	cmd.Flags().StringVar(&flags.Filter, "filter", "",
		`Filter expression: field=value, field:prefix, field~regex (AND/OR/NOT supported)`)

	// Add completions for --format
	cmd.RegisterFlagCompletionFunc("format", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"json", "table", "yaml", "value()", "table()"}, cobra.ShellCompDirectiveNoFileComp
	})
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
		`Output format when using --passthru: json, table, value(field,...), yaml`)

	// Add completions for --format
	cmd.RegisterFlagCompletionFunc("format", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"json", "table", "yaml", "value()"}, cobra.ShellCompDirectiveNoFileComp
	})
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

// Render outputs data according to the format specification.
// data should be a slice of structs or maps for list output, or a single item.
func Render(w io.Writer, data interface{}, flags OutputFlags) error {
	// Apply filter first
	filtered, err := applyFilter(data, flags.Filter)
	if err != nil {
		return fmt.Errorf("filter error: %w", err)
	}

	// Parse format
	format, fields := parseFormat(flags.Format)

	switch format {
	case "json":
		return renderJSON(w, filtered)
	case "yaml":
		return renderYAML(w, filtered)
	case "table":
		return renderTable(w, filtered, fields)
	case "value":
		return renderValue(w, filtered, fields)
	default:
		return fmt.Errorf("unknown format: %s", format)
	}
}

// RenderTo is a convenience function that renders to stdout.
func RenderTo(data interface{}, flags OutputFlags) error {
	return Render(os.Stdout, data, flags)
}

// parseFormat parses format strings like "table(name,version)" into format and fields.
func parseFormat(format string) (string, []string) {
	// Check for field specification: format(field1,field2,...)
	if idx := strings.Index(format, "("); idx != -1 {
		if !strings.HasSuffix(format, ")") {
			return format, nil
		}
		name := format[:idx]
		fieldStr := format[idx+1 : len(format)-1]
		fields := strings.Split(fieldStr, ",")
		for i := range fields {
			fields[i] = strings.TrimSpace(fields[i])
		}
		return name, fields
	}
	return format, nil
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
	defer enc.Close()
	return enc.Encode(data)
}

// renderTable outputs data as a formatted table.
func renderTable(w io.Writer, data interface{}, fields []string) error {
	items := toSlice(data)
	if len(items) == 0 {
		return nil
	}

	// Determine fields from first item if not specified
	if len(fields) == 0 {
		fields = getFieldNames(items[0])
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)

	// Header
	headers := make([]string, len(fields))
	for i, f := range fields {
		headers[i] = strings.ToUpper(f)
	}
	fmt.Fprintln(tw, strings.Join(headers, "\t"))

	// Rows
	for _, item := range items {
		values := make([]string, len(fields))
		for i, f := range fields {
			values[i] = formatFieldValue(getFieldValue(item, f))
		}
		fmt.Fprintln(tw, strings.Join(values, "\t"))
	}

	return tw.Flush()
}

// renderValue outputs just field values, no headers (for scripting).
func renderValue(w io.Writer, data interface{}, fields []string) error {
	items := toSlice(data)
	if len(items) == 0 {
		return nil
	}

	// Determine fields from first item if not specified
	if len(fields) == 0 {
		fields = getFieldNames(items[0])
	}

	for _, item := range items {
		values := make([]string, len(fields))
		for i, f := range fields {
			values[i] = formatFieldValue(getFieldValue(item, f))
		}
		fmt.Fprintln(w, strings.Join(values, "\t"))
	}

	return nil
}

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

// applyFilter filters data based on a gcloud-style filter expression.
func applyFilter(data interface{}, filter string) (interface{}, error) {
	if filter == "" {
		return data, nil
	}

	items := toSlice(data)
	if len(items) == 0 {
		return data, nil
	}

	var result []interface{}
	for _, item := range items {
		match, err := matchesFilter(item, filter)
		if err != nil {
			return nil, err
		}
		if match {
			result = append(result, item)
		}
	}

	return result, nil
}

// matchesFilter checks if an item matches a filter expression.
// Supports: field=value, field:prefix, field~regex, AND, OR, NOT
func matchesFilter(item interface{}, filter string) (bool, error) {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return true, nil
	}

	// Handle NOT
	if strings.HasPrefix(filter, "NOT ") {
		match, err := matchesFilter(item, filter[4:])
		return !match, err
	}

	// Handle OR (lower precedence than AND)
	if idx := findOperator(filter, " OR "); idx != -1 {
		left, err := matchesFilter(item, filter[:idx])
		if err != nil {
			return false, err
		}
		right, err := matchesFilter(item, filter[idx+4:])
		if err != nil {
			return false, err
		}
		return left || right, nil
	}

	// Handle AND
	if idx := findOperator(filter, " AND "); idx != -1 {
		left, err := matchesFilter(item, filter[:idx])
		if err != nil {
			return false, err
		}
		if !left {
			return false, nil // short-circuit
		}
		return matchesFilter(item, filter[idx+5:])
	}

	// Handle parentheses
	if strings.HasPrefix(filter, "(") && strings.HasSuffix(filter, ")") {
		return matchesFilter(item, filter[1:len(filter)-1])
	}

	// Parse single condition: field=value, field:prefix, field~regex
	return matchCondition(item, filter)
}

// findOperator finds an operator not inside parentheses.
func findOperator(s, op string) int {
	depth := 0
	for i := 0; i < len(s)-len(op)+1; i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
		}
		if depth == 0 && strings.HasPrefix(s[i:], op) {
			return i
		}
	}
	return -1
}

// matchCondition evaluates a single filter condition.
func matchCondition(item interface{}, condition string) (bool, error) {
	condition = strings.TrimSpace(condition)

	// field~regex (regex match)
	if idx := strings.Index(condition, "~"); idx != -1 {
		field := strings.TrimSpace(condition[:idx])
		pattern := strings.TrimSpace(condition[idx+1:])
		value := formatFieldValue(getFieldValue(item, field))
		re, err := regexp.Compile(pattern)
		if err != nil {
			return false, fmt.Errorf("invalid regex %q: %w", pattern, err)
		}
		return re.MatchString(value), nil
	}

	// field:prefix (prefix/contains match)
	if idx := strings.Index(condition, ":"); idx != -1 {
		field := strings.TrimSpace(condition[:idx])
		prefix := strings.TrimSpace(condition[idx+1:])
		value := formatFieldValue(getFieldValue(item, field))
		// Support wildcards: * at end means prefix, otherwise contains
		if strings.HasSuffix(prefix, "*") {
			return strings.HasPrefix(strings.ToLower(value), strings.ToLower(prefix[:len(prefix)-1])), nil
		}
		return strings.Contains(strings.ToLower(value), strings.ToLower(prefix)), nil
	}

	// field=value (exact match)
	if idx := strings.Index(condition, "="); idx != -1 {
		field := strings.TrimSpace(condition[:idx])
		expected := strings.TrimSpace(condition[idx+1:])
		value := formatFieldValue(getFieldValue(item, field))
		return strings.EqualFold(value, expected), nil
	}

	return false, fmt.Errorf("invalid condition: %s (use field=value, field:prefix, or field~regex)", condition)
}

// =============================================================================
// Status Output Functions
// =============================================================================
//
// Consistent output functions for CLI commands. Format matches
// Declare-BashScript for consistency across shell scripts and Go commands.
//
// Format: [program] [symbol] message
//   Note:    [program] [+] message     (light gray +)
//   Warn:    [program] [△] message     (yellow △)
//   Error:   [program] [✖] message     (red ✖)
//   Fail:    [program] [✖] message     (red ✖) + returns error
//   Success: [program] [✔] message     (green ✔)

// ANSI color codes
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorGray   = "\033[37m"
)

// Status symbols
const (
	symbolNote    = "+"
	symbolWarn    = "△"
	symbolError   = "✖"
	symbolSuccess = "✔"
)

// programName holds the current program name for output prefixes.
// Set via SetProgramName at startup.
var programName = "devlore"

// SetProgramName sets the program name used in output prefixes.
func SetProgramName(name string) {
	programName = name
}

// Note prints an informational message to stderr.
// Format: [program] [+] message (light gray +)
func Note(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "[%s] [%s%s%s] %s\n", programName, colorGray, symbolNote, colorReset, msg)
}

// Warn prints a warning message to stderr.
// Format: [program] [△] message (yellow △)
func Warn(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "[%s] [%s%s%s] %s\n", programName, colorYellow, symbolWarn, colorReset, msg)
}

// Error prints an error message to stderr.
// Unlike Fail, this does not return an error—use for non-fatal errors.
// Format: [program] [✖] message (red ✖)
func Error(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "[%s] [%s%s%s] %s\n", programName, colorRed, symbolError, colorReset, msg)
}

// Fail prints an error message to stderr and returns an error.
// Use when the operation cannot continue.
// Format: [program] [✖] message (red ✖)
func Fail(format string, args ...interface{}) error {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "[%s] [%s%s%s] %s\n", programName, colorRed, symbolError, colorReset, msg)
	return fmt.Errorf("%s", msg)
}

// Success prints a success message to stderr.
// Format: [program] [✔] message (green ✔)
func Success(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "[%s] [%s%s%s] %s\n", programName, colorGreen, symbolSuccess, colorReset, msg)
}
