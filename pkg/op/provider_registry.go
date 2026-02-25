// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

// ProviderRegistrar registers a provider's actions with an ActionRegistry.
type ProviderRegistrar func(*ActionRegistry)

// RegisterAllProviders calls ActionRegistrar for every registered provider
// binding. Used by consumers that only need action registration without
// Starlark globals (tests, migrate, execution engine).
func RegisterAllProviders(reg *ActionRegistry) {
	for _, b := range AllBindings() {
		if b.ActionRegistrar != nil {
			b.ActionRegistrar(reg)
		}
	}
}
