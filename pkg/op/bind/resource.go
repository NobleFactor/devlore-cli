// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package bind

import (
	"fmt"
	"reflect"
	"sort"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
)

var (
	_ starlark.Value      = (*Resource)(nil) // Interface Guard: ensures *Resource implements starlark.Value.
	_ starlark.HasAttrs   = (*Resource)(nil) // Interface Guard: ensures *Resource implements starlark.HasAttrs.
	_ starlark.Comparable = (*Resource)(nil) // Interface Guard: ensures *Resource implements starlark.Comparable.
)

// Resource wraps a Go resource for starlark use.
//
// It embeds [executingReceiver] for method dispatch. Field access returns marshaled struct fields. Method access
// delegates to the embedded receiver's dispatch. Two resources are equal if they have the same URI.
type Resource struct {
	executingReceiver
	resource op.Resource    // the Go resource (also held as instance in executingReceiver)
	fields   map[string]int // snake_name → struct field index
}

// NewResource creates a [Resource] wrapping the given Go resource.
//
// Parameters:
//   - rt: the receiver type descriptor for the resource.
//   - value: the Go resource.
//
// Returns:
//   - *Resource: the starlark-ready wrapper.
func NewResource(rt op.ResourceReceiverType, value op.Resource) *Resource {

	base := newExecutingReceiver(rt, value)

	// Build field index from exported struct fields.

	fields := make(map[string]int)
	rv := reflect.ValueOf(value)
	t := rv.Elem().Type()
	for i := range t.NumField() {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		fields[camelToSnake(field.Name)] = i
	}

	// Merge field names into attrNames.

	seen := make(map[string]bool, len(base.attrNames)+len(fields))
	for _, name := range base.attrNames {
		seen[name] = true
	}
	for name := range fields {
		if !seen[name] {
			base.attrNames = append(base.attrNames, name)
			seen[name] = true
		}
	}
	sort.Strings(base.attrNames)

	return &Resource{
		executingReceiver: base,
		resource:          value,
		fields:            fields,
	}
}

// region EXPORTED METHODS

// region State management

// String implements starlark.Value.
//
// Delegates to [fmt.Stringer] if the resource implements it.
func (r *Resource) String() string {

	if stringer, ok := r.resource.(fmt.Stringer); ok {
		return stringer.String()
	}
	return r.executingReceiver.String()
}

// Hash implements starlark.Value.
func (r *Resource) Hash() (uint32, error) {

	if uri := r.resource.URI(); uri != "" {
		return hashString(uri), nil
	}
	return 0, fmt.Errorf("unhashable: %s", r.receiverType.ReceiverName())
}

// endregion

// region Behaviors

// Attr implements starlark.HasAttrs.
//
// Fields are resolved first by marshaling the Go struct field. Methods are resolved second and dispatched through the
// embedded [executingReceiver].
//
// Parameters:
//   - name: the snake_case attribute name to look up.
//
// Returns:
//   - starlark.Value: the marshaled field value or a method builtin.
//   - error: non-nil if the attribute does not exist.
func (r *Resource) Attr(name string) (starlark.Value, error) {

	if idx, ok := r.fields[name]; ok {
		rv := reflect.ValueOf(r.resource).Elem()
		return marshalReflect(rv.Field(idx))
	}

	return r.executingReceiver.Attr(name)
}

// CompareSameType implements starlark.Comparable.
//
// Two resources are equal if they have the same URI.
//
// Parameters:
//   - op: the comparison operator.
//   - y: the other value (must be *Resource).
//   - depth: recursion depth (unused).
//
// Returns:
//   - bool: true if the comparison holds.
//   - error: non-nil if the comparison is not supported.
func (r *Resource) CompareSameType(op syntax.Token, y starlark.Value, depth int) (bool, error) {

	other, ok := y.(*Resource)
	if !ok {
		return false, fmt.Errorf("cannot compare %s with %s", r.Type(), y.Type())
	}

	rURI := r.resource.URI()
	oURI := other.resource.URI()

	switch op {
	case syntax.EQL:
		return rURI == oURI, nil
	case syntax.NEQ:
		return rURI != oURI, nil
	default:
		return false, fmt.Errorf("unsupported comparison: %s %s %s", r.Type(), op, y.Type())
	}
}

// endregion

// endregion

// region UNEXPORTED FUNCTIONS

// hashString returns a simple hash of the given string.
//
// Parameters:
//   - s: the string to hash.
//
// Returns:
//   - uint32: the hash value.
func hashString(s string) uint32 {

	var h uint32
	for _, c := range s {
		h = h*31 + uint32(c)
	}
	return h
}

// endregion
