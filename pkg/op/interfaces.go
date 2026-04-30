// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import "reflect"

// Comparer is implemented by types that define domain-specific equality.
//
// Go's `==` works on built-in types and pointer identity, but neither captures the equality semantics most domain
// types need — two [Resource] values with the same URI represent the same resource even if they are distinct
// pointers, and two configuration structs that differ only in cached metadata are equivalent for routing decisions.
// Types that implement [Comparer] take ownership of their own equality rule; callers compare via [Comparer.Equal]
// rather than `==` whenever both sides advertise it.
type Comparer interface {
	Equal(other any) bool
}

// SourceConverter is implemented by values that know how to convert themselves into specific target Go types.
//
// Type conversion in op runs through a small cascade: identity, [reflect.Type.AssignableTo], then opt-in interfaces.
// [SourceConverter] is the source-side opt-in — the value being converted advertises which targets it can produce.
// [SourceConverter.CanConvertTo] is a cheap probe the cascade calls before committing; [SourceConverter.ConvertTo]
// does the work and may fail with a domain-specific error. A type that returns true from CanConvertTo for a given
// target must succeed in ConvertTo for the same target on well-formed input — the probe is a contract, not a hint.
type SourceConverter interface {
	CanConvertTo(target reflect.Type) bool
	ConvertTo(target reflect.Type) (any, error)
}

// TargetConverter is implemented by types that know how to absorb specific source Go values into themselves.
//
// [TargetConverter] is the target-side opt-in counterpart to [SourceConverter] — useful when the source is a
// stdlib or third-party type that cannot be retrofitted with methods (`[]any`, `map[string]any`, primitive slices)
// but the target can opt in. The cascade consults sources first; targets are consulted only when the source does
// not advertise the requested conversion. [TargetConverter.CanConvertFrom] takes a [reflect.Type] (not a value) so
// the probe stays cheap and value-free; [TargetConverter.ConvertFrom] commits with the actual source value and
// returns the constructed target instance.
type TargetConverter interface {
	CanConvertFrom(source reflect.Type) bool
	ConvertFrom(value any) (any, error)
}