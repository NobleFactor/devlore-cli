// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlarkbridge

import (
	"fmt"

	"go.starlark.net/starlark"
)

// Invoker is the provider-facing surface for calling a Starlark callable from Go.
//
// A provider that captured a Starlark callable — e.g. function.Resource holding a *starlark.Function reducer —
// builds its own Invoker via [NewInvoker] and calls through it. The Invoker takes native Go arguments and returns a
// native Go result, owning every Go↔Starlark conversion and the per-goroutine thread discipline, so providers stay
// Go-native and re-roll neither.
type Invoker interface {

	// CallStarlark invokes callable with the given Go arguments on a fresh Starlark thread and returns its result as a
	// native Go value.
	//
	// Each argument is converted to Starlark, the call runs on a thread minted for this invocation (Starlark threads
	// are not safe for concurrent reuse, so a per-call — hence per-goroutine — thread is the only correct choice), and
	// the result is converted back to Go.
	//
	// Parameters:
	//   - `callable`: the Starlark callable to invoke.
	//   - `args`: the positional arguments, as native Go values.
	//
	// Returns:
	//   - `any`: the call's result as a native Go value.
	//   - `error`: non-nil when an argument or the result cannot be converted, or the call itself fails.
	CallStarlark(callable starlark.Callable, args ...any) (any, error)
}

// NewInvoker returns a new [Invoker] over an env-free converter.
//
// The invoker path performs no environment-dependent conversion — toStarlark and toNaturalGo never read the
// converter's environment — so no runtime environment is needed. Each consumer builds its own instance rather than
// sharing one through a registry.
//
// Returns:
//   - `Invoker`: a ready-to-use invoker over an env-free converter.
func NewInvoker() Invoker {
	return invoker{converter: converter{}}
}

// region SUPPORTING TYPES

// invoker is the [Invoker] implementation over a session [converter].
//
// It holds the converter for both conversion directions and mints a fresh Starlark thread per call.
type invoker struct {
	converter converter
}

// Static assertion that invoker satisfies [Invoker].
var _ Invoker = invoker{}

// region Behaviors

// CallStarlark converts args to Starlark, invokes callable on a fresh thread, and converts the result back to Go.
//
// Parameters:
//   - `callable`: the Starlark callable to invoke.
//   - `args`: the positional arguments, as native Go values.
//
// Returns:
//   - `any`: the call's result as a native Go value.
//   - `error`: non-nil when an argument or the result cannot be converted, or the call itself fails.
func (i invoker) CallStarlark(callable starlark.Callable, args ...any) (any, error) {

	tuple := make(starlark.Tuple, len(args))

	for index, arg := range args {

		value, err := i.converter.toStarlark(arg)
		if err != nil {
			return nil, fmt.Errorf("arg %d: %w", index, err)
		}

		tuple[index] = value
	}

	result, err := starlark.Call(&starlark.Thread{Name: "starlarkbridge.Invoker"}, callable, tuple, nil)
	if err != nil {
		return nil, err
	}

	return i.converter.toNaturalGo(result)
}

// endregion

// endregion
