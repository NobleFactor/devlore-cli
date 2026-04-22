// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import "reflect"

// Comparer is implemented by types that define domain-specific equality.
type Comparer interface {
	Equal(other any) bool
}

// Converter is implemented by source values that know how to project themselves into specific target Go types.
type Converter interface {
	CanConvert(target reflect.Type) bool
	Convert(target reflect.Type) (any, error)
}
