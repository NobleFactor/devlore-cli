// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import "reflect"

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

// SetContext replaces the context for this provider.
func (p *ProviderBase) SetContext(ctx Context) {
	p.ctx = ctx
}

func (p *ProviderBase) providerBase() *ProviderBase { return p }

// AttributeResolver is implemented by providers that expose dynamic attributes beyond their own methods.
//
// When the bridge encounters an unknown attribute, it checks if the provider implements AttributeResolver and delegates
// to it. The returned value is marshaled to a starlark.Value.
type AttributeResolver interface {
	ResolveAttr(name string) any
}

// ContextProvider allows a provider to access its scoped execution [Context].
//
// Actions that need access to the execution environment implement this interface to receive the [Context] during graph
// execution. The [Context] includes execution parameters, platform abstractions, and runtime state.
//
// Types should satisfy this interface by embedding [ProviderBase].
type ContextProvider interface {
	Context() Context
	providerBase() *ProviderBase
}

// ReceiverFactory is the required interface for all provider descriptors--generated and handwritten alike.
//
// Every provider implements this to announce its name and register its actions with the framework.
type ReceiverFactory interface {
	GetOrCreateProvider(ctx Context) ContextProvider
	MethodParams() map[string][]string
	MethodParamsFor(name string) []string
	ProviderType() reflect.Type
	ReceiverName() string
	Register(ctx Context, registry *ReceiverRegistry)
}
