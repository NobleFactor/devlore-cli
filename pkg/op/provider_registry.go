// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

// ProviderRegistrar registers a provider's actions with an ActionRegistry.
// The Context provides execution state (Platform, Writer, etc.) so the
// registrar can create a per-graph provider instance with its context set.
type ProviderRegistrar func(*ActionRegistry, Context)

// RegisterAllProviders calls ActionRegistrar for every registered provider
// binding. Used by consumers that only need action registration without
// Starlark globals (tests, migrate, execution engine).
func RegisterAllProviders(reg *ActionRegistry, ctx Context) {
	for _, b := range AllBindings() {
		if b.ActionRegistrar != nil {
			b.ActionRegistrar(reg, ctx)
		}
	}
}
