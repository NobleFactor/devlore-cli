// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

// Provider allows a provider to access its scoped execution [RuntimeEnvironment].
//
// Actions that need access to the execution environment implement this interface to receive the [RuntimeEnvironment] during graph
// execution. The [RuntimeEnvironment] includes execution parameters, platform abstractions, and runtime state.
//
// Types should satisfy this interface by embedding [ProviderBase].
type Provider interface {
	RuntimeEnvironment() *RuntimeEnvironment
	providerBase() *ProviderBase
}

// ProviderBase provides a standardized implementation of the [Provider] interface.
//
// It must be embedded in all domain-specific providers to ensure they adhere to the execution graph's strictly enforced
// lifetime.
//
// All providers constructed from the same RuntimeEnvironment share a pointer to it. Per-invocation state changes (DryRun,
// Data) propagate to all providers without reconstruction.
type ProviderBase struct {
	ctx *RuntimeEnvironment
}

// NewProviderBase returns a new ProviderBase provider instance with the given [RuntimeEnvironment].
func NewProviderBase(runtimeEnvironment *RuntimeEnvironment) ProviderBase {
	return ProviderBase{ctx: runtimeEnvironment}
}

// RuntimeEnvironment returns the shared context associated with this provider's lifetime.
func (p *ProviderBase) RuntimeEnvironment() *RuntimeEnvironment {
	return p.ctx
}

func (p *ProviderBase) providerBase() *ProviderBase { return p }

// AttributeResolver is implemented by providers that expose dynamic attributes beyond their own methods.
//
// When the bridge encounters an unknown attribute, it checks if the provider implements AttributeResolver and delegates
// to it. The returned value is marshaled to a starlark.Value.
type AttributeResolver interface {
	ResolveAttr(name string) any
}
