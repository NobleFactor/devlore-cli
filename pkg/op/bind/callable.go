// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package bind

import (
	"fmt"
	"reflect"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"go.starlark.net/starlark"
)

// callableResourceType is the [reflect.Type] for the CallableResource interface.
//
// It is used as the key in the constructor registry for callable extraction.
var callableResourceType = reflect.TypeOf((*CallableResource)(nil)).Elem()

// CallableInput carries the parameters needed for callable extraction.
//
// Used as the constructor input for the CallableResource constructor registered by the mem package via
// AnnounceResource.
type CallableInput struct {
	Fn       *starlark.Function
	FuncType string
}

// CallableResource is the interface that mem.Callable satisfies.
//
// It allows pkg/op to work with callables without importing the mem package.
type CallableResource interface {
	op.Resource
	Init(thread *starlark.Thread) error
	Fn() starlark.Callable
	FuncTypeName() string
}

// region UNEXPORTED FUNCTIONS

// buildCallableFunc creates a Go function value matching targetType that delegates to a Starlark callable.
//
// All arguments are marshaled Go→Starlark and passed to the callable. The single Starlark return value is
// unmarshaled into the first return slot; the last return (if error-typed) is set to nil on success.
//
// Parameters:
//   - fn: the Starlark callable to wrap.
//   - thread: the Starlark thread for function calls.
//   - targetType: the Go func [reflect.Type] to produce.
//
// Returns:
//   - any: the adapted Go function value matching targetType.
//   - error: non-nil if adaptation fails.
func buildCallableFunc(fn starlark.Callable, thread *starlark.Thread, targetType reflect.Type) (any, error) {

	numIn := targetType.NumIn()
	numOut := targetType.NumOut()

	// Build adapter function via reflect.MakeFunc.
	adapter := reflect.MakeFunc(targetType, func(args []reflect.Value) []reflect.Value {
		// Marshal all Go args → Starlark.
		starArgs := make(starlark.Tuple, numIn)
		for i := range numIn {
			sv, err := Marshal(args[i].Interface())
			if err != nil {
				return makeErrorReturn(targetType, numOut, fmt.Errorf("callable arg %d: marshal: %w", i, err))
			}
			starArgs[i] = sv
		}

		// Call the Starlark function.
		result, err := starlark.Call(thread, fn, starArgs, nil)
		if err != nil {
			return makeErrorReturn(targetType, numOut, fmt.Errorf("callable %s: %w", fn.Name(), err))
		}

		// Unmarshal return values.
		return unmarshalReturn(targetType, numOut, result)
	})

	return adapter.Interface(), nil
}

// unmarshalReturn converts a Starlark result into the target func's return values.
//
// The callable returns a single starlark.Value. This function places the unmarshaled value in the first return slot
// and sets the last return to nil (if error-typed).
//
// Parameters:
//   - funcType: the Go func [reflect.Type] for return type introspection.
//   - numOut: the number of return values.
//   - result: the Starlark value to unmarshal into the first return slot.
//
// Returns:
//   - []reflect.Value: the populated return values.
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

	// Remaining returns (between first result and last error) are zero.

	for i := 1; i < numOut-1; i++ {
		out[i] = reflect.Zero(funcType.Out(i))
	}

	return out
}

// makeErrorReturn builds a [reflect.Value] slice for an error return.
//
// The last return holds the error; all preceding returns are zero values.
//
// Parameters:
//   - funcType: the Go func [reflect.Type] for return type introspection.
//   - numOut: the number of return values.
//   - err: the error to place in the last return slot.
//
// Returns:
//   - []reflect.Value: zero values for all returns except the last, which holds err.
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

// endregion