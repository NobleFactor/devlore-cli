// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package result

import (
	"fmt"
	"reflect"
	"strings"
)

// FieldFilter narrows a slice of structs/maps to those whose named field matches a target value.
// The expression form is `field=value`, mirroring `gh`'s `--filter` flag and `kubectl`'s field
// selectors. Multiple expressions AND together — every expression must match for a row to pass.
//
// Comparison is done after rendering both sides through fmt.Sprint, so primitives, fmt.Stringer
// implementations, and time.Time values all compare correctly. Numeric strings ("42") and numeric
// values (42) compare equal — the expression dialect is string-typed.
//
// Construct via [NewFieldFilter]; surface parse errors directly to the user.
type FieldFilter struct {
	predicates []fieldPredicate
}

// fieldPredicate is one parsed `field=value` clause.
type fieldPredicate struct {
	field string
	value string
}

// Compile-time interface guard.
var _ Filter = (*FieldFilter)(nil)

// region Construction

// NewFieldFilter parses zero or more `field=value` expressions into a [FieldFilter]. Empty
// expressions are skipped silently, supporting the common pattern where the caller does
// `strings.Split(flag, ",")` against an empty flag value. An expression missing `=` is a parse
// error.
//
// Parameters:
//   - exprs: zero or more `field=value` expressions.
//
// Returns:
//   - *FieldFilter: the constructed filter. With zero predicates, [Filter.Apply] is a pass-through.
//   - error: when any expression fails to parse.
func NewFieldFilter(exprs ...string) (*FieldFilter, error) {

	predicates := make([]fieldPredicate, 0, len(exprs))
	for _, expr := range exprs {

		expr = strings.TrimSpace(expr)
		if expr == "" {
			continue
		}

		field, value, ok := strings.Cut(expr, "=")
		if !ok {
			return nil, fmt.Errorf("result.FieldFilter: expression %q is missing '='", expr)
		}

		field = strings.TrimSpace(field)
		if field == "" {
			return nil, fmt.Errorf("result.FieldFilter: expression %q has empty field name", expr)
		}

		predicates = append(predicates, fieldPredicate{field: field, value: value})
	}

	return &FieldFilter{predicates: predicates}, nil
}

// endregion

// region Filter

// Apply returns the subset of value's elements for which every predicate matches. Non-slice values
// pass through unchanged when the filter has zero predicates; otherwise non-slice values error
// loudly.
func (f *FieldFilter) Apply(value any) (any, error) {

	if len(f.predicates) == 0 {
		return value, nil
	}
	if value == nil {
		return value, nil
	}

	rv := reflect.ValueOf(value)
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return value, nil
		}
		rv = rv.Elem()
	}

	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		return nil, fmt.Errorf("result.FieldFilter: expected slice or array, got %T", value)
	}

	out := reflect.MakeSlice(reflect.SliceOf(rv.Type().Elem()), 0, rv.Len())
	for i := range rv.Len() {
		element := rv.Index(i)
		match, err := f.matches(element)
		if err != nil {
			return nil, err
		}
		if match {
			out = reflect.Append(out, element)
		}
	}
	return out.Interface(), nil
}

// matches reports whether every predicate matches against the named field/key of element.
func (f *FieldFilter) matches(element reflect.Value) (bool, error) {

	resolved := indirect(element)
	if !resolved.IsValid() {
		return false, nil
	}

	for _, predicate := range f.predicates {
		ok, err := fieldEquals(resolved, predicate.field, predicate.value)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}
	return true, nil
}

// endregion

// region Field lookup

// fieldEquals reports whether resolved's field/key named name has value (compared via fmt.Sprint).
// Returns false (and no error) when the field/key is absent — absence is "doesn't match", not "bad
// query".
func fieldEquals(resolved reflect.Value, name, value string) (bool, error) {

	switch resolved.Kind() {

	case reflect.Struct:
		field := lookupStructField(resolved, name)
		if !field.IsValid() {
			return false, nil
		}
		return cellMatches(field, value), nil

	case reflect.Map:
		entry := csvMapLookup(resolved, name)
		if !entry.IsValid() {
			return false, nil
		}
		return cellMatches(entry, value), nil

	default:
		return false, fmt.Errorf("result.FieldFilter: element kind %s is not struct or map", resolved.Kind())
	}
}

// lookupStructField returns the field of rv with the given name, honoring `csv:"name"` overrides
// and embedded-field promotion. Returns the zero reflect.Value if the field is absent or skipped.
func lookupStructField(rv reflect.Value, name string) reflect.Value {

	for _, field := range reflect.VisibleFields(rv.Type()) {
		if !field.IsExported() {
			continue
		}
		fieldName, skip := csvFieldName(field)
		if skip {
			continue
		}
		if fieldName == name {
			return rv.FieldByIndex(field.Index)
		}
	}
	return reflect.Value{}
}

// cellMatches reports whether the rendered string of rv equals target.
func cellMatches(rv reflect.Value, target string) bool {

	return csvCellValue(rv) == target
}

// endregion
