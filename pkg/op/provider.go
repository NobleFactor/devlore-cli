// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"reflect"

	"go.starlark.net/starlark"
)

// ReceiverFactory is the required interface for all provider descriptors--generated and handwritten alike.
//
// Every provider implements this to announce its name and register its actions with the framework.
type ReceiverFactory interface {
	GetOrCreateProvider(ctx Context) ContextProvider
	ReceiverName() string
	ProviderType() reflect.Type
	Register(registry *ActionRegistry, ctx Context)
}

// PlanningReceiverFactory is optional. Checked via type assertion during InitAll.
//
// Providers that contribute a plan sub-namespace (e.g., plan.file) implement this.
type PlanningReceiverFactory interface {
	NewPlanning(graph *Graph, project string, reg *ActionRegistry) starlark.Value
}

// ExecutingReceiverFactory is optional. Checked via type assertion during InitAll.
// Providers that contribute an immediate receiver (e.g., file, ui) implement this.
type ExecutingReceiverFactory interface {
	NewExecuting(ctx Context) starlark.Value
}

// ContextProvider is an interface for objects that can supply an execution [Context].
//
// Actions that need access to the execution environment implement this interface to receive the [Context] during graph
// execution. The [Context] includes execution parameters, platform abstractions, and runtime state.
//
// Types should satisfy this interface by embedding [ProviderBase].
type ContextProvider interface {
	Context() Context
	providerBase() *ProviderBase
}

// ProviderBase provides a standardized implementation of the [ContextProvider] interface.
//
// It must be embedded in all domain-specific providers to ensure they adhere to the execution graph's strictly enforced
// lifetime.
//
// ProviderBase should only be instantiated by authorized builders using [NewExecuting].
type ProviderBase struct {
	ctx Context
}

// NewProviderBase returns a new ProviderBase provider instance with the given [Context].
func NewProviderBase(ctx Context) ProviderBase {
	return ProviderBase{ctx: ctx}
}

// Context returns the context associated with this provider's lifetime.
func (p *ProviderBase) Context() Context {
	return p.ctx
}

func (p *ProviderBase) providerBase() *ProviderBase { return p }
