// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"fmt"
	"reflect"
)

// Convert transforms value into the target Go type. The cascade:
//
//  1. Identity — source type equals target type.
//  2. Assignability — source type is assignable to target (covers concrete →
//     interface, aliases, etc.).
//  3. Source polymorphism — if value implements Converter, delegate to
//     value.Convert(target).
//  4. Target instantiation — if ctx.Registry has a ResourceReceiverType for
//     target, call its ctx-aware constructor with value as the source. This
//     is the ctx-passing site: target-side conversion implies instantiation,
//     which consults the registry with ctx in hand.
//  5. Slice lift — if both types are slices, recurse element-wise via Convert.
//  6. No path found — error.
//
// Convert is the op-side counterpart to bind.ToUnmarshaler: bind produces a
// raw Go value from a starlark source; Convert reshapes that Go value into
// the target parameter type expected by a method signature.
func Convert(ctx *ExecutionContext, value any, target reflect.Type) (any, error) {

	if value == nil {
		return reflect.Zero(target).Interface(), nil
	}

	source := reflect.TypeOf(value)

	// 1. Identity.
	if source == target {
		return value, nil
	}

	// 2. Assignability.
	if source.AssignableTo(target) {
		return reflect.ValueOf(value).Convert(target).Interface(), nil
	}

	// 3. Source polymorphism.
	if c, ok := value.(Converter); ok {
		if out, err := c.Convert(target); err == nil {
			return out, nil
		}
		// Converter didn't accept this target; fall through to other options.
	}

	// 4. Target instantiation via the registry (ctx-aware).
	if ctx != nil && ctx.Registry != nil {
		if targetRT, ok := ctx.Registry.TypeByReflection(target); ok {
			if rrt, ok := targetRT.(ResourceReceiverType); ok {
				return rrt.Construct()(ctx, value)
			}
		}
		// Pointer-to-resource: target may be *T where T is the registered resource.
		if target.Kind() == reflect.Pointer {
			if targetRT, ok := ctx.Registry.TypeByReflection(target.Elem()); ok {
				if rrt, ok := targetRT.(ResourceReceiverType); ok {
					return rrt.Construct()(ctx, value)
				}
			}
		}
	}

	// 5. Slice lift.
	if source.Kind() == reflect.Slice && target.Kind() == reflect.Slice {
		return liftSlice(ctx, value, source, target)
	}

	return nil, fmt.Errorf("convert: no path from %s to %s", source, target)
}

func liftSlice(ctx *ExecutionContext, value any, source, target reflect.Type) (any, error) {

	srcValue := reflect.ValueOf(value)
	n := srcValue.Len()
	result := reflect.MakeSlice(target, n, n)
	elemTarget := target.Elem()

	for i := 0; i < n; i++ {
		elem := srcValue.Index(i).Interface()
		converted, err := Convert(ctx, elem, elemTarget)
		if err != nil {
			return nil, fmt.Errorf("slice lift: index %d: %w", i, err)
		}
		result.Index(i).Set(reflect.ValueOf(converted))
	}
	return result.Interface(), nil
}
