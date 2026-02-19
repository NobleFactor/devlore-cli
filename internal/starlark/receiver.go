// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlark

import (
	"fmt"

	"go.starlark.net/starlark"
)

// Receiver provides common implementations for Starlark binding namespaces.
// Embed this in concrete types to satisfy starlark.Value. Concrete types
// must implement starlark.HasAttrs (Attr and AttrNames) themselves.
type Receiver struct {
	name string
}

// NewReceiver creates a new Receiver with the given namespace name.
func NewReceiver(name string) Receiver {
	return Receiver{name: name}
}

// String implements starlark.Value.
func (r Receiver) String() string {
	return r.name
}

// Type implements starlark.Value.
func (r Receiver) Type() string {
	return r.name
}

// Freeze implements starlark.Value.
func (r Receiver) Freeze() {}

// Truth implements starlark.Value.
func (r Receiver) Truth() starlark.Bool {
	return true
}

// Hash implements starlark.Value.
func (r Receiver) Hash() (uint32, error) {
	return 0, fmt.Errorf("unhashable type: %s", r.name)
}

// listToStringSlice converts a Starlark list to a Go string slice.
// Used by generated receivers to convert []string parameters.
func listToStringSlice(list *starlark.List) []string {
	result := make([]string, list.Len())
	for i := 0; i < list.Len(); i++ {
		s, _ := starlark.AsString(list.Index(i))
		result[i] = s
	}
	return result
}

// NoSuchAttrError returns an error for an unknown attribute.
func NoSuchAttrError(receiver, attr string) error {
	return fmt.Errorf("%s has no .%s attribute", receiver, attr)
}

// BuiltinFunc is the signature for builtin function implementations.
type BuiltinFunc func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error)

// MakeAttr creates a starlark.Builtin from a receiver method.
func MakeAttr(name string, fn BuiltinFunc) starlark.Value {
	return starlark.NewBuiltin(name, fn)
}
