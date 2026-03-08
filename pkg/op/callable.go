// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"fmt"
	"reflect"

	"go.starlark.net/starlark"
)

// CallableResource is the interface that mem.Callable satisfies.
// It allows pkg/op to work with callables without importing the mem package.
type CallableResource interface {
	Resource
	Init(thread *starlark.Thread) error
	Fn() starlark.Callable
	FuncTypeName() string
}

// callableExtractorFn is the registered function that extracts a
// *starlark.Function into a CallableResource. Registered by the mem
// package in init(). Returns the extracted, compiled callable.
var callableExtractorFn func(fn *starlark.Function, funcType string) (CallableResource, error)

// RegisterCallableExtractor registers the function that extracts a
// *starlark.Function into a CallableResource. Called by the mem package
// during init().
func RegisterCallableExtractor(fn func(*starlark.Function, string) (CallableResource, error)) {
	callableExtractorFn = fn
}

// ExtractCallable extracts a *starlark.Function into a CallableResource
// using the registered extractor. Returns an error if no extractor is
// registered.
func ExtractCallable(fn *starlark.Function, funcType string) (CallableResource, error) {
	if callableExtractorFn == nil {
		return nil, fmt.Errorf("no callable extractor registered (mem package not imported?)")
	}
	return callableExtractorFn(fn, funcType)
}

// isCallableResource returns true if the value implements CallableResource.
func isCallableResource(v any) bool {
	_, ok := v.(CallableResource)
	return ok
}

// isFuncType returns true if the reflect.Type is a function type.
func isFuncType(t reflect.Type) bool {
	return t.Kind() == reflect.Func
}

// initCallableSlots finds CallableResource values in slots that target
// func-typed method parameters, initializes them, and replaces the slot
// value with an adapted Go function. This runs in Do() before coerceArgs
// so the standard coercion path sees a directly-assignable func value.
func initCallableSlots(ctx *Context, slots map[string]any, methodType reflect.Type, paramNames []string) error {
	for i, name := range paramNames {
		callable, ok := slots[name].(CallableResource)
		if !ok {
			continue
		}
		paramIdx := i + 1 // skip receiver
		if paramIdx >= methodType.NumIn() {
			continue
		}
		paramType := methodType.In(paramIdx)
		if paramType.Kind() != reflect.Func {
			continue
		}

		if err := callable.Init(ctx.Thread); err != nil {
			return fmt.Errorf("param %s: init callable: %w", name, err)
		}

		adapted, err := buildCallableFunc(callable.Fn(), ctx.Thread, paramType)
		if err != nil {
			return fmt.Errorf("param %s: adapt callable: %w", name, err)
		}
		slots[name] = adapted
	}
	return nil
}

// buildCallableFunc creates a Go function value matching targetType that
// delegates to the Starlark callable. Arguments are marshaled Go→Starlark,
// truncated to the callable's arity (supporting swallowed trailing params).
// Returns are unmarshaled Starlark→Go.
//
// Target func signature: func(p0 T0, p1 T1, ...) (R0, R1, ...)
// The Starlark callable receives min(targetParams, callableArity) arguments.
func buildCallableFunc(fn starlark.Callable, thread *starlark.Thread, targetType reflect.Type) (any, error) {
	numIn := targetType.NumIn()
	numOut := targetType.NumOut()

	// Determine how many args the Starlark function accepts.
	starArity := numIn // default: pass all
	if starFn, ok := fn.(*starlark.Function); ok {
		starArity = starFn.NumParams()
		if starFn.HasVarargs() {
			starArity = numIn // varargs: pass all
		}
	}
	if starArity > numIn {
		starArity = numIn
	}

	// Build adapter function via reflect.MakeFunc.
	adapter := reflect.MakeFunc(targetType, func(args []reflect.Value) []reflect.Value {
		// Marshal Go args → Starlark, truncated to callable arity.
		starArgs := make(starlark.Tuple, starArity)
		for i := range starArity {
			sv, err := marshal(args[i].Interface())
			if err != nil {
				return makeErrorReturn(targetType, numOut,
					fmt.Errorf("callable arg %d: marshal: %w", i, err))
			}
			starArgs[i] = sv
		}

		// Call the Starlark function.
		result, err := starlark.Call(thread, fn, starArgs, nil)
		if err != nil {
			return makeErrorReturn(targetType, numOut,
				fmt.Errorf("callable %s: %w", fn.Name(), err))
		}

		// Unmarshal return values.
		return unmarshalReturn(targetType, numOut, result)
	})

	return adapter.Interface(), nil
}

// makeErrorReturn builds a reflect.Value slice for an error return.
// Convention: the last return is error; preceding returns are zero values.
func makeErrorReturn(funcType reflect.Type, numOut int, err error) []reflect.Value {
	out := make([]reflect.Value, numOut)
	for i := range numOut - 1 {
		out[i] = reflect.Zero(funcType.Out(i))
	}
	if numOut > 0 {
		out[numOut-1] = reflect.ValueOf(&err).Elem()
	}
	return out
}

// unmarshalReturn converts a Starlark result into the target func's return
// values. Convention: first return is the result (via unmarshalToAny), last
// return is error (nil on success). Middle returns are zero values.
func unmarshalReturn(funcType reflect.Type, numOut int, result starlark.Value) []reflect.Value {
	out := make([]reflect.Value, numOut)

	if numOut == 0 {
		return out
	}

	// Last return is error — set to nil.
	if numOut > 0 && funcType.Out(numOut-1).Implements(errorType) {
		out[numOut-1] = reflect.Zero(funcType.Out(numOut - 1))
	}

	// First return is the result value.
	if numOut >= 1 {
		goVal, err := unmarshalToAny(result)
		if err != nil {
			return makeErrorReturn(funcType, numOut, err)
		}
		if goVal == nil {
			out[0] = reflect.Zero(funcType.Out(0))
		} else {
			rv := reflect.ValueOf(goVal)
			if rv.Type().AssignableTo(funcType.Out(0)) {
				out[0] = rv
			} else if rv.Type().ConvertibleTo(funcType.Out(0)) {
				out[0] = rv.Convert(funcType.Out(0))
			} else {
				out[0] = reflect.Zero(funcType.Out(0))
			}
		}
	}

	// Middle returns (between first result and last error) are zero.
	for i := 1; i < numOut-1; i++ {
		out[i] = reflect.Zero(funcType.Out(i))
	}

	return out
}
