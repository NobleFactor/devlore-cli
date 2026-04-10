// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package bind

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"go.starlark.net/starlark"
)

var (
	_ starlark.Value    = (*Value)(nil) // Interface Guard: ensures *Value implements starlark.Value.
	_ starlark.HasAttrs = (*Value)(nil) // Interface Guard: ensures *Value implements starlark.HasAttrs.
)

// Value wraps an arbitrary Go object for starlark use.
//
// It embeds [executingReceiver] for method dispatch. Field access returns marshaled struct fields. Unlike [Provider] and
// [Resource], a Value has no constructor, no roles, and no identity. It is what you get when a provider method returns
// a Go struct.
type Value struct {
	executingReceiver
	value  op.Value       // carries execution context + reflect.Value
	fields map[string]int // snake_name → struct field index
}

// NewValue creates a [Value] wrapping the given [op.Value].
//
// Parameters:
//   - rt: the receiver type descriptor.
//   - v: the op.Value holding the Go object and execution context.
//
// Returns:
//   - *Value: the starlark-ready wrapper.
func NewValue(rt op.ReceiverType, v op.Value) *Value {

	rv := v.Unwrap()
	base := newExecutingReceiver(rt, rv.Interface())

	// Build field index from exported struct fields.

	fields := make(map[string]int)
	elem := rv
	for elem.Kind() == reflect.Pointer {
		elem = elem.Elem()
	}
	if elem.Kind() == reflect.Struct {
		t := elem.Type()
		for i := range t.NumField() {
			field := t.Field(i)
			if !field.IsExported() {
				continue
			}
			tag := field.Tag.Get("starlark")
			if tag == "-" {
				continue
			}
			name := tag
			if name == "" {
				name = camelToSnake(field.Name)
			}
			fields[name] = i
		}
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

	return &Value{
		executingReceiver: base,
		value:             v,
		fields:            fields,
	}
}

// region EXPORTED METHODS

// region State management

// String implements starlark.Value.
func (v *Value) String() string {

	if stringer, ok := v.value.Unwrap().Interface().(fmt.Stringer); ok {
		return stringer.String()
	}

	var b strings.Builder
	b.WriteString(v.receiverType.ReceiverName())
	b.WriteByte('(')
	for i, name := range v.attrNames {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(name)
		b.WriteString(" = ")
		attr, err := v.Attr(name)
		if err != nil {
			b.WriteString("<error>")
		} else {
			b.WriteString(attr.String())
		}
	}
	b.WriteByte(')')
	return b.String()
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
func (v *Value) Attr(name string) (starlark.Value, error) {

	if idx, ok := v.fields[name]; ok {
		elem := v.value.Unwrap()
		for elem.Kind() == reflect.Pointer {
			elem = elem.Elem()
		}
		return marshalReflect(elem.Field(idx))
	}

	return v.executingReceiver.Attr(name)
}

// endregion

// endregion
