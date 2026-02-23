// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

// ProviderRegistrar registers a provider's actions with an ActionRegistry.
type ProviderRegistrar func(*ActionRegistry)

// providerRegistrars collects self-registration functions from provider
// packages. Populated by init() in generated actions_gen.go files.
var providerRegistrars []ProviderRegistrar

// RegisterProvider adds a provider registrar. Called from init() in each
// provider's generated actions_gen.go.
func RegisterProvider(fn ProviderRegistrar) {
	providerRegistrars = append(providerRegistrars, fn)
}

// RegisterAllProviders calls all registered provider registrars.
func RegisterAllProviders(reg *ActionRegistry) {
	for _, fn := range providerRegistrars {
		fn(reg)
	}
}
