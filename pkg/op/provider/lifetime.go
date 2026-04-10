// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package provider

// Lifetime declares a provider's lifecycle semantics.
// The value is set via a "// +devlore:lifetime=" directive on the ReceiverFactory struct and emitted by the code generator
// into provider descriptor registrations.
type Lifetime string

// Lifetime constants define caching, sharing, and cleanup behavior.
const (
	Stateless Lifetime = "stateless" // safe to cache indefinitely and share across threads
	Subgraph  Lifetime = "subgraph"  // fresh instance per subgraph; Close() at subgraph boundary
	Session   Lifetime = "session"   // single instance for the session; Close() at session end
)
