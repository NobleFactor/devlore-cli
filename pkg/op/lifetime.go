// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

// ProviderLifetime declares a provider's lifecycle semantics.
// The value is set via a "// +devlore:lifetime=" directive on the ReceiverFactory struct and emitted by the code generator
// into provider descriptor registrations.
type ProviderLifetime string

// Lifetime constants define caching, sharing, and cleanup behavior.
const (
	LifetimeStateless ProviderLifetime = "stateless" // safe to cache indefinitely and share across threads
	LifetimePhase     ProviderLifetime = "phase"     // fresh instance per phase; Close() at phase boundary
	LifetimeSession   ProviderLifetime = "session"   // single instance for the session; Close() at session end
)
