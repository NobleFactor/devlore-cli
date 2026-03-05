// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

// Provider is an interface for objects that can supply an execution [Context].
//
// Actions that need access to the execution environment implement this interface to receive the [Context] during graph
// execution. The [Context] includes execution parameters, platform abstractions, and runtime state.
//
// Types should satisfy this interface by embedding [ProviderBase].
type Provider interface {
	Context() Context
	providerBase() *ProviderBase
}

// ProviderBase provides a standardized implementation of the [Provider] interface.
//
// It must be embedded in all domain-specific providers to ensure they adhere to the execution graph's strictly enforced
// lifetime.
//
// ProviderBase should only be instantiated by authorized builders using [New].
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

// InitProvider sets the execution [Context] on any provider that embeds [ProviderBase].
//
// For providers that do not embed ProviderBase, this is a no-op.
func InitProvider(p any, ctx Context) {
	if provider, ok := p.(Provider); ok {
		provider.providerBase().ctx = ctx
	}
}
