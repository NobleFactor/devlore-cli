// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package starvalue defines interfaces for custom Starlark value
// serialization. Types that implement Marshaler or Unmarshaler control
// how they are converted to and from starlark.Value representations.
package starvalue

import "go.starlark.net/starlark"

// Marshaler is implemented by types that can marshal themselves into a
// Starlark value. marshalReflect checks for this interface before walking
// struct fields via reflection — same pattern as encoding/json.Marshaler.
type Marshaler interface {
	MarshalStarvalue() (starlark.Value, error)
}

// Unmarshaler is implemented by types that can unmarshal a Starlark value
// into themselves. Unmarshal checks for this interface before assigning
// fields via reflection.
type Unmarshaler interface {
	UnmarshalStarvalue(starlark.Value) error
}
