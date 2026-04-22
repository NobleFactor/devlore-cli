// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlarkbridge

import "go.starlark.net/starlark"

// Marshaler is implemented by Go types that want to marshal themselves into a Starlark value.
type Marshaler interface {
	MarshalStarlark() (starlark.Value, error)
}

// Unmarshaler is implemented by Go types that want to unmarshal that can unmarshal themselves from a Starlark value.
type Unmarshaler interface {
	UnmarshalStarlark(starlark.Value) error
}
