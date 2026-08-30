// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package result

import (
	"fmt"
	"strings"
)

// FormatterByName returns the [Formatter] named by spec.
//
// spec is a format name, or `NAME=ARGUMENT` for a format that takes one. The split is on the FIRST `=`, so an
// argument containing `=` survives intact. A format needing an argument carries it here rather than in a
// second flag: a sidecar would give the format stage two inputs and a mutual-exclusion rule to enforce, where
// this makes the conflict impossible by construction. `kubectl` ships the same form -- `-o go-template=`,
// `-o jsonpath=`, `-o custom-columns=`.
//
// The names are "csv", "json", "none", "template=BODY", "tsv", "value", and "yaml". Reshaping a value is the
// filter stage's job -- see [FilterByExprs] -- so only "template" takes an argument.
//
// Parameters:
//   - `spec`: the format name, or `NAME=ARGUMENT`; the name is case-insensitive.
//
// Returns:
//   - `Formatter`: the constructed formatter.
//   - `error`: when the name is unknown, or an argument is required and missing, or given and unwanted.
func FormatterByName(spec string) (Formatter, error) {

	name, argument, hasArgument := strings.Cut(strings.TrimSpace(spec), "=")
	name = strings.ToLower(strings.TrimSpace(name))

	if hasArgument && argument == "" {
		return nil, fmt.Errorf("result.FormatterByName: %q gives an empty argument; drop the '=' or supply one", spec)
	}

	if hasArgument && name != "template" {
		return nil, fmt.Errorf("result.FormatterByName: %q takes no argument", name)
	}

	switch name {

	case "json":
		return JSONFormatter{}, nil

	case "yaml":
		return YAMLFormatter{}, nil

	case "csv":
		return NewCSVFormatter(), nil

	case "tsv":
		return NewTSVFormatter(), nil

	case "none":
		return NoneFormatter{}, nil

	case "value":
		return NewValueFormatter(), nil

	case "template":
		if !hasArgument {
			return nil, fmt.Errorf("result.FormatterByName: template needs a body: --output template=<body>")
		}
		return NewTemplateFormatter(argument)

	default:
		return nil, fmt.Errorf(
			"result.FormatterByName: unknown formatter %q; expected one of csv, json, none, template=BODY, "+
				"tsv, value, yaml", name)
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
