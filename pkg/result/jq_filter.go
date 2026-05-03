// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package result

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/itchyny/gojq"
)

// JQFilter applies a jq expression to the value via [github.com/itchyny/gojq].
//
// The input is normalized to gojq's native shape (map[string]any / []any / primitives) by
// round-tripping through encoding/json. This costs a serialization pass per Apply but lets callers
// hand any JSON-serializable Go value to the filter without first converting it.
//
// The expression is parsed and compiled at construction; parse errors surface there. Execution
// errors from gojq propagate through Apply. Single-result expressions return the unwrapped value;
// multi-result expressions (jq's `,` operator, `..` recursive descent, etc.) collect into a slice.
type JQFilter struct {
	expression string
	code       *gojq.Code
}

// Compile-time interface guard.
var _ Filter = (*JQFilter)(nil)

// region Construction

// NewJQFilter parses and compiles a jq expression into a [JQFilter].
//
// Parameters:
//   - expression: the jq query, e.g. ".[] | select(.kind == \"file\")".
//
// Returns:
//   - *JQFilter: the constructed filter.
//   - error: when expression fails to parse or compile.
func NewJQFilter(expression string) (*JQFilter, error) {

	query, err := gojq.Parse(expression)
	if err != nil {
		return nil, fmt.Errorf("result.JQFilter: parse %q: %w", expression, err)
	}

	code, err := gojq.Compile(query)
	if err != nil {
		return nil, fmt.Errorf("result.JQFilter: compile %q: %w", expression, err)
	}

	return &JQFilter{expression: expression, code: code}, nil
}

// endregion

// region Filter

// Apply normalizes value to gojq's native shape, runs the compiled query, and returns the result.
//
// A single-result expression returns the unwrapped value. A multi-result expression collects every
// emitted value into an []any. An expression that emits no values returns nil. Execution errors are
// returned as-is.
func (f *JQFilter) Apply(value any) (any, error) {

	normalized, err := jqNormalize(value)
	if err != nil {
		return nil, fmt.Errorf("result.JQFilter: normalize input: %w", err)
	}

	results := make([]any, 0, 1)
	iter := f.code.Run(normalized)
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if asErr, isErr := v.(error); isErr {
			var halt *gojq.HaltError
			if errors.As(asErr, &halt) && halt.Value() == nil {
				break
			}
			return nil, fmt.Errorf("result.JQFilter: execute %q: %w", f.expression, asErr)
		}
		results = append(results, v)
	}

	switch len(results) {
	case 0:
		return nil, nil
	case 1:
		return results[0], nil
	default:
		return results, nil
	}
}

// endregion

// region Helpers

// jqNormalize converts value into gojq's native shape via JSON round-trip. gojq operates on
// map[string]any / []any / primitives; struct values pass through json.Marshal which honors `json:`
// tags. The conversion is needed because gojq cannot reflect on arbitrary Go structs.
func jqNormalize(value any) (any, error) {

	if value == nil {
		return nil, nil
	}

	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}

	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()

	var out any
	if err := decoder.Decode(&out); err != nil {
		return nil, err
	}

	return jqUnwrapNumbers(out), nil
}

// jqUnwrapNumbers walks the decoded value and converts json.Number values to int64 or float64. gojq
// understands both directly; the conversion keeps `==` comparisons stable across integer and float
// branches.
func jqUnwrapNumbers(value any) any {

	switch v := value.(type) {

	case json.Number:
		if i, err := v.Int64(); err == nil {
			return i
		}
		if f, err := v.Float64(); err == nil {
			return f
		}
		return v.String()

	case map[string]any:
		for key, child := range v {
			v[key] = jqUnwrapNumbers(child)
		}
		return v

	case []any:
		for i, child := range v {
			v[i] = jqUnwrapNumbers(child)
		}
		return v

	default:
		return v
	}
}

// endregion
