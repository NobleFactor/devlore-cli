// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package bind

import (
	"fmt"

	"go.starlark.net/starlark"
)

// receiver provides common implementations for Starlark binding namespaces.
// Embed this in concrete types to satisfy starlark.Value. Concrete types
// must implement starlark.HasAttrs (Attr and AttrNames) themselves.
type receiver struct {
	name string
}

// newReceiver creates a new receiver with the given namespace name.
func newReceiver(name string) receiver {
	return receiver{name: name}
}

// String implements starlark.Value.
func (r receiver) String() string {
	return r.name
}

// Type implements starlark.Value.
func (r receiver) Type() string {
	return r.name
}

// Freeze implements starlark.Value.
func (r receiver) Freeze() {}

// Truth implements starlark.Value.
func (r receiver) Truth() starlark.Bool {
	return true
}

// Hash implements starlark.Value.
func (r receiver) Hash() (uint32, error) {
	return 0, fmt.Errorf("unhashable type: %s", r.name)
}

// NoSuchAttrError returns an error for an unknown attribute.
func NoSuchAttrError(receiver, attr string) error {
	return fmt.Errorf("%s has no .%s attribute", receiver, attr)
}

// builtinFunc is the signature for builtin function implementations.
type builtinFunc func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error)
