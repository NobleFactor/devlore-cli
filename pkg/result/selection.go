// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package result

import (
	"fmt"
	"strings"
)

// FormatterByName returns the [Formatter] registered under name. The known names are "json", "yaml",
// "csv", and "template". The "template" form requires a non-empty templateText; the others ignore
// it.
//
// Parameters:
//   - name: the formatter name; case-insensitive.
//   - templateText: the body for the "template" formatter; ignored otherwise.
//
// Returns:
//   - Formatter: the constructed formatter.
//   - error: when name is unknown or when the template fails to parse.
func FormatterByName(name, templateText string) (Formatter, error) {

	switch strings.ToLower(strings.TrimSpace(name)) {

	case "json":
		return JSONFormatter{}, nil

	case "yaml":
		return YAMLFormatter{}, nil

	case "csv":
		return CSVFormatter{}, nil

	case "template":
		if templateText == "" {
			return nil, fmt.Errorf("result.FormatterByName: 'template' requires non-empty template text")
		}
		return NewTemplateFormatter(templateText)

	default:
		return nil, fmt.Errorf("result.FormatterByName: unknown formatter %q; expected one of json, yaml, csv, template", name)
	}
}

// FilterByExprs returns a composed [Filter] from optional `field=value` expressions and an optional
// jq expression. When both are present, the field filter runs first (cheap predicate elimination),
// then the jq filter (full transform). With both empty, returns a [NoOpFilter].
//
// Parameters:
//   - fieldExprs: zero or more `field=value` expressions for [FieldFilter].
//   - jqExpr: an optional jq expression for [JQFilter]; empty disables the jq stage.
//
// Returns:
//   - Filter: the composed filter.
//   - error: when any expression fails to parse.
func FilterByExprs(fieldExprs []string, jqExpr string) (Filter, error) {

	stages := make([]Filter, 0, 2)

	field, err := NewFieldFilter(fieldExprs...)
	if err != nil {
		return nil, err
	}
	if len(field.predicates) > 0 {
		stages = append(stages, field)
	}

	if strings.TrimSpace(jqExpr) != "" {
		jq, err := NewJQFilter(jqExpr)
		if err != nil {
			return nil, err
		}
		stages = append(stages, jq)
	}

	switch len(stages) {

	case 0:
		return NoOpFilter{}, nil

	case 1:
		return stages[0], nil

	default:
		return chainFilter(stages), nil
	}
}

// chainFilter is a [Filter] that runs each stage in order, threading the output of one as the input
// of the next.
type chainFilter []Filter

// Compile-time interface guard.
var _ Filter = chainFilter(nil)

// Apply runs each stage in order; the first error short-circuits.
func (c chainFilter) Apply(value any) (any, error) {

	current := value
	for _, stage := range c {
		next, err := stage.Apply(current)
		if err != nil {
			return nil, err
		}
		current = next
	}
	return current, nil
}
