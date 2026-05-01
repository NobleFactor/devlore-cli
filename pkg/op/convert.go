// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"fmt"
	"reflect"
)

// resourceInterfaceType is the [reflect.Type] of [Resource], cached for [Convert]'s registered-Resource step.
var resourceInterfaceType = reflect.TypeFor[Resource]()

// Convert projects a Go value into the target type via the type-matching cascade.
//
// Convert is the single source of truth for Go↔Go projection in the framework. Every starlark-bridge entry
// point (wrapper extraction, plan-mode slot fill, immediate-mode dispatch) and method-dispatch site
// ([Method.Invoke]) routes through here so type-matching semantics stay in one place.
//
// The cascade:
//
//  1. Identity — value's type is the target type. Return as-is.
//  2. Assignability — value's underlying type is assignable to target ([reflect.Type.AssignableTo]).
//  3. Slice element conversion — both source and target are slices; recurse element-wise.
//  4. Map element conversion — both source and target are maps; recurse key-and-value-wise.
//  5. Source-side opt-in — value implements [SourceConverter] and advertises target type.
//  6. Target-side opt-in — fresh target probe implements [TargetConverter] and advertises source type.
//  7. Registered Resource construction — target implements [Resource] and a constructor is registered in
//     [RuntimeEnvironment.Registry]; the constructor is run with (ctx, value).
//  8. Error — no path through the cascade succeeds.
//
// Parameters:
//   - ctx: the ambient [RuntimeEnvironment]. Used by step 7 to look up registered Resource constructors.
//   - value: the source value to project. nil yields the zero value of target.
//   - target: the [reflect.Type] of the desired result.
//
// Returns:
//   - any: the projected value, ready to assign to a target of type target.
//   - error: non-nil if no path through the cascade succeeds.
func Convert(ctx *RuntimeEnvironment, value any, target reflect.Type) (any, error) {

	// Step 0: nil → zero of target.

	if value == nil {
		return reflect.Zero(target).Interface(), nil
	}

	// Step 1: identity / assignability. Dereference pointers so a *T value reaches a T target through the
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

	// Step 2: slice element conversion. Heterogeneous-shaped sources ([]any from starlark lists) cannot
	// satisfy AssignableTo against typed Go slices ([]string, []*file.Resource). Recursively project each
	// element through the same cascade so element-level conversions compose.

	if elem.Kind() == reflect.Slice && target.Kind() == reflect.Slice {

		n := elem.Len()
		out := reflect.MakeSlice(target, n, n)

		for i := range n {
			converted, err := Convert(ctx, elem.Index(i).Interface(), target.Elem())
			if err != nil {
				return nil, fmt.Errorf("slice index %d: %w", i, err)
			}
			out.Index(i).Set(reflect.ValueOf(converted))
		}

		return out.Interface(), nil
	}

	// Step 3: map element conversion. Same shape as slice — heterogeneous-shaped sources (map[any]any,
	// map[string]any from starlark dicts) cannot satisfy AssignableTo against typed Go maps. Recursively
	// project keys and values so map-element conversions compose.

	if elem.Kind() == reflect.Map && target.Kind() == reflect.Map {

		out := reflect.MakeMapWithSize(target, elem.Len())

		iter := elem.MapRange()
		for iter.Next() {

			convertedKey, err := Convert(ctx, iter.Key().Interface(), target.Key())
			if err != nil {
				return nil, fmt.Errorf("map key %v: %w", iter.Key().Interface(), err)
			}

			convertedValue, err := Convert(ctx, iter.Value().Interface(), target.Elem())
			if err != nil {
				return nil, fmt.Errorf("map value for %v: %w", iter.Key().Interface(), err)
			}

			out.SetMapIndex(reflect.ValueOf(convertedKey), reflect.ValueOf(convertedValue))
		}

		return out.Interface(), nil
	}

	// Step 4: source-side opt-in.

	if c, ok := value.(SourceConverter); ok && c.CanConvertTo(target) {
		return c.ConvertTo(target)
	}

	// Step 5: target-side opt-in. Probe must be a *target or target-as-already-pointer, since converter methods
	// conventionally sit on the pointer receiver.

	var probe any
	if target.Kind() == reflect.Pointer {
		probe = reflect.New(target.Elem()).Interface()
	} else {
		probe = reflect.New(target).Interface()
	}

	if t, ok := probe.(TargetConverter); ok && t.CanConvertFrom(reflect.TypeOf(value)) {
		return t.ConvertFrom(value)
	}

	// Step 6: registered Resource construction. When the target is a registered Resource type, the registry
	// holds a constructor that knows how to build a fresh Resource from a string (or other shape) — typically
	// minting the canonical tag URI from a path or scheme-prefixed string. The constructor's source-shape
	// permissiveness lives inside it; Convert just routes the call.

	if target.Implements(resourceInterfaceType) && ctx != nil && ctx.Registry != nil {

		// Resources are typically announced under the value type (file.Resource) but the parameter type is the
		// pointer (*file.Resource). Try the pointer-or-element fallback the registry's other lookups use.
		rt, ok := ctx.Registry.TypeByReflection(target)
		if !ok && target.Kind() == reflect.Pointer {
			rt, ok = ctx.Registry.TypeByReflection(target.Elem())
		}
		if !ok && target.Kind() != reflect.Pointer {
			rt, ok = ctx.Registry.TypeByReflection(reflect.PointerTo(target))
		}
		if !ok {
			return nil, fmt.Errorf("Resource type %s not registered — must be announced via op.AnnounceResource", target)
		}

		rrt, ok := rt.(ResourceReceiverType)
		if !ok {
			return nil, fmt.Errorf("type %s registered as %T, not as ResourceReceiverType", target, rt)
		}

		v, err := rrt.Construct()(ctx, value)
		if err != nil {
			return nil, fmt.Errorf("construct %s from %T: %w", target, value, err)
		}

		return v, nil
	}

	// Step 7: not convertible.

	return nil, fmt.Errorf("%T value is neither assignable nor convertible to %s", value, target)
}