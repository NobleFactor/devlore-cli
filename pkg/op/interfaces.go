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
//
// Plan-time obligation: [SourceConverter.CanConvertTo] is consulted by [typesAreInterconvertible] at plan time to
// validate the bubble-up parameter-consistency surface ([Subgraph.mergeBubbled]). The probe is therefore called
// against zero-value or fresh-allocated probes of the source type — never against a populated value. Implementations
// MUST treat the receiver as opaque: do not dereference fields, call methods that touch state, or assume the
// receiver was constructed via a normal constructor. The contract is "given just my type, can I convert to `target`?"
// — the answer must be a pure function of the type pair, computable from the receiver's type identity alone.
type SourceConverter interface {
	CanConvertTo(target reflect.Type) bool
	ConvertTo(target reflect.Type) (any, error)
}

// TargetConverter is implemented by types that know how to absorb specific source Go values into themselves.
//
// [TargetConverter] is the target-side opt-in counterpart to [SourceConverter] — useful when the source is a
// stdlib or third-party type that cannot be retrofitted with methods (`[]any`, `map[string]any`, primitive slices)
// but the target can opt in. The [Convert] cascade consults sources first, then registered Resource constructors
// (when the target is a [Resource] type and a [RuntimeEnvironment.Registry] is available), then this target-side
// opt-in as the final reflective path. [TargetConverter.CanConvertFrom] takes a [reflect.Type] (not a value) so
// the probe stays cheap and value-free; [TargetConverter.ConvertFrom] commits with the actual source value and
// returns the constructed target instance.
//
// Plan-time obligation: [TargetConverter.CanConvertFrom] is consulted by [typesAreInterconvertible] at plan time
// to validate the bubble-up parameter-consistency surface ([Subgraph.mergeBubbled]). The probe is therefore called
// against a freshly-allocated probe of the target type (via [reflect.New]) — its receiver is non-nil but otherwise
// zero. Implementations MUST be safe on a zero receiver: do not dereference receiver fields, call methods that
// touch state, or assume the receiver was constructed via a normal constructor. The contract is "given just my
// type, can I absorb `source`?" — the answer must be a pure function of the type pair, computable from the
// receiver's type identity alone.
//
// Resource implementers: opting into [TargetConverter] advertises that a CLI flag, env var, or config value of the
// declared source type can fill a slot typed as the Resource. The framework wires both halves uniformly:
// [Subgraph.mergeBubbled] honors the convertibility relation at plan time (no false collisions between, say, a
// `string` slot and a [*Resource] slot bound to the same variable name); [Convert] step 6 (registered constructor)
// produces the canonical, env-aware Resource at dispatch when [RuntimeEnvironment.Registry] is available;
// [Convert] step 7 ([TargetConverter.ConvertFrom]) is the env-less fallback for library callers or test fixtures
// that exercise [Convert] outside a runtime session. Resource implementers' [TargetConverter.ConvertFrom] typically
// returns a minimal unlinked Resource (identity field set, catalog interning deferred to the receiving provider
// method's own [NewResource]/[DiscoverResource] call). Content-addressed Resources (mem / function / json / yaml)
// commonly omit [TargetConverter] when their natural source is content bytes or a parsed structure — not a CLI
// string — so the cascade falls back to the registered constructor unconditionally.
type TargetConverter interface {
	CanConvertFrom(source reflect.Type) bool
	ConvertFrom(value any) (any, error)
}
