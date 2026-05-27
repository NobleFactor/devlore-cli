// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"fmt"
	"reflect"
)

var (
	// resourceInterfaceType is the [reflect.Type] of [Resource], cached for [Convert]'s registered-Resource step.
	resourceInterfaceType = reflect.TypeFor[Resource]()

	// sourceConverterType is the cached [reflect.Type] of [SourceConverter].
	//
	// Used by [typesAreInterconvertible] to test whether a candidate source type opts into the source-side
	// conversion contract.
	sourceConverterType = reflect.TypeFor[SourceConverter]()

	// targetConverterType is the cached [reflect.Type] of [TargetConverter].
	//
	// Used by [typesAreInterconvertible] to test whether a candidate target type opts into the target-side
	// conversion contract.
	targetConverterType = reflect.TypeFor[TargetConverter]()
)

// Convert projects a Go value into the target type via the type-matching cascade.
//
// Convert is the single source of truth for Go↔Go projection in the framework. Every starlark-bridge entry point
// (wrapper extraction, plan-mode slot fill, immediate-mode dispatch) and method-dispatch site ([Method.Invoke]) routes
// through here so type-matching semantics stay in one place.
//
// The cascade:
//
//  1. Identity — value's type is the target type. Return as-is.
//  2. Assignability — value's underlying type is assignable to target ([reflect.Type.AssignableTo]).
//  3. Slice element conversion — both source and target are slices; recurse element-wise.
//  4. Map element conversion — both source and target are maps; recurse key-and-value-wise.
//  5. Source-side opt-in — value implements [SourceConverter] and advertises the target type.
//  6. Registered Resource construction — target implements [Resource], and a constructor is registered in
//     [RuntimeEnvironment.Registry]; the constructor is run with (runtimeEnvironment, value).
//  7. Target-side opt-in — fresh target probe implements [TargetConverter] and advertises the source type.
//  8. Error — no path through the cascade succeeds.
//
// Parameters:
//   - `runtimeEnvironment`: the ambient [RuntimeEnvironment]. Step 6 uses its [Registry] for registered
//     Resource construction.
//   - `value`: the source value to project. `nil` yields the zero value of `target`.
//   - `target`: the [`reflect.Type`] of the desired result.
//
// Returns:
//   - `any`: the projected value, ready to assign to a target of type `target`.
//   - `error`: non-nil if no path through the cascade succeeds.
func Convert(runtimeEnvironment *RuntimeEnvironment, value any, target reflect.Type) (any, error) {

	// Step 0: nil → zero of target.

	if value == nil {
		return reflect.Zero(target).Interface(), nil
	}

	// Step 1: identity. Pointer-equal reflect.Type means the same underlying `*rtype`, so `==` is a single pointer
	// comparison and subsumes the assignability identity case without paying for reflect.ValueOf, the deref walk,
	// or the Interface() round-trip. Hot path for slot-fill from Parameter.Default (already at p.Type) and for any
	// caller-supplied value whose dynamic type already matches target exactly.

	if reflect.TypeOf(value) == target {
		return value, nil
	}

	// Step 1.5: empty interface (`any`) target. Any non-nil value satisfies `any` — return as-is. Crucially,
	// SKIP the pointer-deref of step 2: a *T value passed to an `any`-typed target must preserve its pointer
	// shape, because callers downstream (e.g., a method whose signature is `[]*T`) need the pointer back. The
	// bridge's early-projection path uses this when goReceiver.Project(reflect.TypeFor[any]()) asks for the
	// natural Go form of a wrapped instance — *op.Invocation must come back as *op.Invocation, not op.Invocation.

	if target.Kind() == reflect.Interface && target.NumMethod() == 0 {
		return value, nil
	}

	// Step 2: assignability with pointer-deref. Dereference pointers so a *T value reaches a T target through the
	// underlying assignability rule.

	elem := reflect.ValueOf(value)

	for elem.Kind() == reflect.Pointer {
		elem = elem.Elem()
	}

	if elem.IsValid() {
		if elem.Type().AssignableTo(target) {
			return elem.Interface(), nil
		}

		if elem.Type().ConvertibleTo(target) {
			return elem.Convert(target).Interface(), nil
		}
	}

	if reflect.TypeOf(value).AssignableTo(target) {
		return value, nil
	}

	// Step 3: slice element conversion.

	if v, ok, err := tryConvertSlice(runtimeEnvironment, elem, target); ok {
		return v, err
	}

	// Step 4: map element conversion.

	if v, ok, err := tryConvertMap(runtimeEnvironment, elem, target); ok {
		return v, err
	}

	// Step 5: source-side opt-in.

	if c, ok := value.(SourceConverter); ok && c.CanConvertTo(target) {
		return c.ConvertTo(target)
	}

	// Step 6: registered Resource construction.

	if v, ok, err := tryConstructResource(runtimeEnvironment, value, target); ok {
		return v, err
	}

	// Step 7: target-side opt-in.

	if v, ok, err := tryTargetConverter(value, target); ok {
		return v, err
	}

	// Step 8: not convertible.

	return nil, fmt.Errorf("%T value is neither assignable nor convertible to %s", value, target)
}

// tryConvertSlice handles [Convert]'s step 3: slice → slice element-wise recursion.
//
// Heterogeneous-shaped sources ([]any from starlark lists) cannot satisfy AssignableTo against typed Go slices
// ([]string, []*file.Resource). Each element is recursed through the full [Convert] cascade so element-level
// conversions compose. Returns (nil, false, nil) when either input is not a slice.
//
// Parameters:
//   - `runtimeEnvironment`: forwarded to the recursive [Convert] calls for env-sensitive steps.
//   - `elem`: the source value as a [reflect.Value] (already pointer-dereferenced by Convert step 2).
//   - `target`: the desired slice target type.
//
// Returns:
//   - `any`: the constructed slice when applicable; nil otherwise.
//   - `bool`: true when this step applied (regardless of error); false when neither input is a slice.
//   - `error`: non-nil when an element conversion failed.
func tryConvertSlice(
	runtimeEnvironment *RuntimeEnvironment,
	elem reflect.Value,
	target reflect.Type,
) (any, bool, error) {

	if elem.Kind() != reflect.Slice || target.Kind() != reflect.Slice {
		return nil, false, nil
	}

	n := elem.Len()
	out := reflect.MakeSlice(target, n, n)

	for i := range n {
		converted, err := Convert(runtimeEnvironment, elem.Index(i).Interface(), target.Elem())
		if err != nil {
			return nil, true, fmt.Errorf("slice index %d: %w", i, err)
		}
		out.Index(i).Set(reflect.ValueOf(converted))
	}

	return out.Interface(), true, nil
}

// tryConvertMap handles [Convert]'s step 4: map → map key-and-value recursion.
//
// Heterogeneous-shaped sources (map[any]any, map[string]any from starlark dictionaries) cannot satisfy AssignableTo
// against typed Go maps. Keys and values are recursed through the full [Convert] cascade so map-element conversions
// compose. Returns (nil, false, nil) when either input is not a map.
//
// Parameters:
//   - `runtimeEnvironment`: forwarded to the recursive [Convert] calls for env-sensitive steps.
//   - `elem`: the source value as a [reflect.Value] (already pointer-dereferenced by Convert step 2).
//   - `target`: the desired map target type.
//
// Returns:
//   - `any`: the constructed map when applicable; nil otherwise.
//   - `bool`: true when this step applied (regardless of error); false when neither input is a map.
//   - `error`: non-nil when a key or value conversion failed.
func tryConvertMap(
	runtimeEnvironment *RuntimeEnvironment,
	elem reflect.Value,
	target reflect.Type,
) (any, bool, error) {

	if elem.Kind() != reflect.Map || target.Kind() != reflect.Map {
		return nil, false, nil
	}

	out := reflect.MakeMapWithSize(target, elem.Len())

	iter := elem.MapRange()
	for iter.Next() {

		convertedKey, err := Convert(runtimeEnvironment, iter.Key().Interface(), target.Key())
		if err != nil {
			return nil, true, fmt.Errorf("map key %v: %w", iter.Key().Interface(), err)
		}

		convertedValue, err := Convert(runtimeEnvironment, iter.Value().Interface(), target.Elem())
		if err != nil {
			return nil, true, fmt.Errorf("map value for %v: %w", iter.Key().Interface(), err)
		}

		out.SetMapIndex(reflect.ValueOf(convertedKey), reflect.ValueOf(convertedValue))
	}

	return out.Interface(), true, nil
}

// tryConstructResource handles [Convert]'s step 6: registered Resource construction.
//
// Tried before [TargetConverter] (step 7) so Resources with both a registered constructor and a [TargetConverter]
// opt-in use the env-aware canonical path at dispatch: the registered constructor receives the full
// [RuntimeEnvironment] (catalog, root, registry, etc.) and can produce a fully-canonicalized Resource.
// [TargetConverter] (step 7) is reached only when no registered constructor applies — env-less library callers, tests,
// or non-Resource target types — and serves as the framework's plan-time convertibility probe via
// [typesAreInterconvertible]. Resources without a registered constructor still get the [TargetConverter] path;
// Resources with one always prefer the canonical.
//
// Returns (nil, false, nil) when `target` is not a Resource type or no [RuntimeEnvironment.Registry] is
// available.
//
// Parameters:
//   - `runtimeEnvironment`: provides [RuntimeEnvironment.Registry] for type lookup and is passed to the
//     registered constructor.
//   - `value`: the source value to project.
//   - `target`: the Resource target type.
//
// Returns:
//   - `any`: the constructed Resource when applicable; nil otherwise.
//   - `bool`: true when this step applied (regardless of error).
//   - `error`: non-nil when registry lookup, type assertion, or constructor execution fails.
func tryConstructResource(
	runtimeEnvironment *RuntimeEnvironment,
	value any,
	target reflect.Type,
) (any, bool, error) {

	if !target.Implements(resourceInterfaceType) || runtimeEnvironment == nil || runtimeEnvironment.Registry == nil {
		return nil, false, nil
	}

	// Resources are typically announced under the value type (file.Resource), but the parameter type is the pointer
	// (*file.Resource). Try the pointer-or-element fallback the registry's other lookups use.
	rt, ok := runtimeEnvironment.Registry.TypeByReflection(target)
	if !ok && target.Kind() == reflect.Pointer {
		rt, ok = runtimeEnvironment.Registry.TypeByReflection(target.Elem())
	}
	if !ok && target.Kind() != reflect.Pointer {
		rt, ok = runtimeEnvironment.Registry.TypeByReflection(reflect.PointerTo(target))
	}
	if !ok {
		return nil, true, fmt.Errorf("resource type %s not registered — must be announced via op.AnnounceResource", target)
	}

	rrt, isResourceReceiverType := rt.(ResourceReceiverType)
	if !isResourceReceiverType {
		return nil, true, fmt.Errorf("type %s registered as %T, not as ResourceReceiverType", target, rt)
	}

	v, err := rrt.Construct()(runtimeEnvironment, value)
	if err != nil {
		return nil, true, fmt.Errorf("construct %s from %T: %w", target, value, err)
	}

	return v, true, nil
}

// tryTargetConverter handles [Convert]'s step 7: target-side opt-in.
//
// Probe must be a *target or target-as-already-pointer, since converter methods conventionally sit on the pointer
// receiver. Reached after step 6, so registered-Resource canonicalization always wins at dispatch when env is
// available; step 7 fires for RuntimeEnvironment-les callers, non-Resource target types, and Resources whose registry
// entry is missing.
//
// Returns (nil, false, nil) when the target probe does not implement [TargetConverter] or declines to
// absorb `value`'s type via [TargetConverter.CanConvertFrom].
//
// Parameters:
//   - `value`: the source value to project.
//   - `target`: the desired target type.
//
// Returns:
//   - `any`: the constructed value when applicable; nil otherwise.
//   - `bool`: true when this step applied; false when the target does not opt into [TargetConverter] for
//     `value`'s type.
//   - `error`: non-nil when [TargetConverter.ConvertFrom] fails.
func tryTargetConverter(value any, target reflect.Type) (any, bool, error) {

	var probe any
	if target.Kind() == reflect.Pointer {
		probe = reflect.New(target.Elem()).Interface()
	} else {
		probe = reflect.New(target).Interface()
	}

	t, ok := probe.(TargetConverter)
	if !ok || !t.CanConvertFrom(reflect.TypeOf(value)) {
		return nil, false, nil
	}

	v, err := t.ConvertFrom(value)
	return v, true, err
}

// typesAreInterconvertible reports whether a value of type `a` can fill a slot typed `b` or vice versa.
//
// Symmetrically tests both directions via any of the purely reflective paths of the [Convert] cascade.
//
// Used by [Subgraph.mergeBubbled] (the bubble-up parameter-consistency check), so the same-named-variable-across-
// differently-typed-slots case is not treated as a hard collision when a registered conversion bridges the two
// types. Slot-fill is *defined* as the conversion site; the plan-time check honors the same contract the dispatch-
// time cascade does.
//
// Paths probed (mirroring [Convert] steps 1, 2, 5, 6):
//
//  1. Identity — `a == b`.
//  2. Assignability — `a.AssignableTo(b)` or `b.AssignableTo(a)`.
//  3. Source-side opt-in — a zero-value of `a` implements [SourceConverter] and `CanConvertTo(b)` returns true,
//     or symmetrically for `b → a`.
//  4. Target-side opt-in — a fresh probe of `b` implements [TargetConverter] and `CanConvertFrom(a)` returns true,
//     or symmetrically for `b ← a`.
//
// Paths NOT probed: slice / map element-wise recursion (require a concrete value) and registered-Resource construction
// (requires a [RuntimeEnvironment] handle). Providers whose Resource types want plan-time type-compatibility honored
// for non-Resource sources opt in by implementing [TargetConverter] on the Resource type — the framework then wires
// both the plan-time consistency check (via this function) and the dispatch-time slot-fill (via [Convert] step 6)
// uniformly.
//
// Both [SourceConverter.CanConvertTo] and [TargetConverter.CanConvertFrom] are part of the cheap-probe contract:
// callers MUST be safe on a zero-value receiver (no field dereference), because this function calls them against nil
// pointers and zero structs to determine the existence of a conversion path without producing a value.
//
// Parameters:
//   - `a`: one of the two types to test.
//   - `b`: the other type.
//
// Returns:
//   - `bool`: true if at least one of the probed paths reports interconvertibility in either direction.
func typesAreInterconvertible(a, b reflect.Type) bool {

	if a == nil || b == nil {
		return false
	}

	if a == b {
		return true
	}

	if a.AssignableTo(b) || b.AssignableTo(a) {
		return true
	}

	if sourceSideAdvertises(a, b) || sourceSideAdvertises(b, a) {
		return true
	}

	if targetSideAdvertises(b, a) || targetSideAdvertises(a, b) {
		return true
	}

	return false
}

// sourceSideAdvertises reports whether `source` opts into [SourceConverter] for `target`.
//
// Probes both the value form of `source` (when methods sit on a value receiver) and the pointer form (when methods sit
// on `*source`, the conventional Go choice). When the type implements [SourceConverter], [SourceConverter.CanConvertTo]
// is called on the probe to confirm `target` is an advertised destination.
//
// Pointer-type sources allocate a fresh zero-value via [reflect.New](source.Elem()) — never a nil pointer — so methods
// promoted through embedded structs (e.g., [op.ResourceBase] on a Resource type) can access the embedded field without
// dereferencing nil.
//
// Parameters:
//   - `source`: the candidate source type whose [SourceConverter] opt-in is being probed.
//   - `target`: the destination type the source must advertise convertibility to.
//
// Returns:
//   - `bool`: true when a probe of `source` (or its pointer form) implements [SourceConverter] and reports
//     `CanConvertTo(target)`; false otherwise.
func sourceSideAdvertises(source, target reflect.Type) bool {

	if source.Implements(sourceConverterType) {

		var probe any
		if source.Kind() == reflect.Pointer {
			probe = reflect.New(source.Elem()).Interface()
		} else {
			probe = reflect.Zero(source).Interface()
		}

		if c, ok := probe.(SourceConverter); ok {
			return c.CanConvertTo(target)
		}
	}

	if source.Kind() != reflect.Pointer {
		ptrSource := reflect.PointerTo(source)
		if ptrSource.Implements(sourceConverterType) {
			probe := reflect.New(source).Interface()
			if c, ok := probe.(SourceConverter); ok {
				return c.CanConvertTo(target)
			}
		}
	}

	return false
}

// targetSideAdvertises reports whether `target` opts into [TargetConverter] for `source`.
//
// Mirrors [Convert] step 6's probe construction: when `target` is `*T`, the probe is `*T`; when `target` is a
// non-pointer `T`, the probe is `*T` (TargetConverter methods conventionally sit on the pointer receiver).
// [TargetConverter.CanConvertFrom] is then called on the probe to confirm `source` is an absorbable type.
//
// Parameters:
//   - `source`: the candidate source type the target must advertise convertibility from.
//   - `target`: the destination type whose [TargetConverter] opt-in is being probed.
//
// Returns:
//   - `bool`: true when a fresh probe of `target` (or its pointer form) implements [TargetConverter] and
//     reports `CanConvertFrom(source)`; false otherwise.
func targetSideAdvertises(source, target reflect.Type) bool {

	var probeType reflect.Type
	if target.Kind() == reflect.Pointer {
		probeType = target
	} else {
		probeType = reflect.PointerTo(target)
	}

	if !probeType.Implements(targetConverterType) {
		return false
	}

	var probe any
	if target.Kind() == reflect.Pointer {
		probe = reflect.New(target.Elem()).Interface()
	} else {
		probe = reflect.New(target).Interface()
	}

	t, ok := probe.(TargetConverter)
	if !ok {
		return false
	}

	return t.CanConvertFrom(source)
}
