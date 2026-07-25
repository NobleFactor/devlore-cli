// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package function

import (
	"fmt"

	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

var _ op.Provider = (*Provider)(nil) // Interface Guard

// Provider implements actions over content-addressed function resources.
//
// The package was resource-only until phase-8 step 10 added [Provider.Call] — the leaf action that evaluates a
// callable — so a starlark function or lambda passes through planning as a [*Resource] and is invoked at dispatch.
//
// +devlore:access=planned
type Provider struct {
	op.ProviderBase
}

// NewProvider creates a function provider bound to the given context.
func NewProvider(runtimeEnvironment *op.RuntimeEnvironment) *Provider {

	return &Provider{ProviderBase: op.NewProviderBase(runtimeEnvironment)}
}

// region EXPORTED METHODS

// region Behaviors

// Fallible actions

// Call invokes the function `callable` with `args` and `kwargs` and returns its result.
//
// The plan-time surface is `plan.function.call(<callable>, ...)`: a starlark function or lambda passed as the first
// argument crosses the bridge intact and [op.ActionPlanner] resolves it to a [*Resource] via the registry's source
// constructor, so the lambda lives in the graph and catalog as a content-addressed function resource. The remaining
// positional and keyword arguments fill the function's own parameters at the call, converted and invoked through the
// resource's [starlarkbridge.Invoker] — the semantics of a native starlark call site, including the callee's own
// defaults and *args / **kwargs. Phase-8 step 10 desugars a lambda `when` / `then` / `default` body to this leaf;
// WaitUntil's step-12 rebuild is the next consumer.
//
// Parameters:
//   - `callable`: the function resource to invoke.
//   - `args`: positional arguments for the function, as native Go values; converted to starlark at the call.
//   - `kwargs`: keyword arguments for the function, as native Go values; converted to starlark at the call.
//
// Returns:
//   - `any`: the function's result, converted to a native Go value.
//   - `error`: non-nil when the resource fails to initialize, an argument or the result cannot be converted, or the
//     call itself fails.
func (p *Provider) Call(callable *Resource, args []any, kwargs map[string]any) (any, error) {

	if callable == nil {
		return nil, fmt.Errorf("function.Call: callable is nil")
	}
	if callable.invoker == nil {
		return nil, fmt.Errorf("function.Call: invoker not initialized on the resource")
	}

	starlarkCallable, err := callable.Init(&starlark.Thread{Name: "function.Call"})
	if err != nil {
		return nil, fmt.Errorf("function.Call: %w", err)
	}

	return callable.invoker.CallStarlark(starlarkCallable, args, kwargs)
}

// endregion

// endregion
