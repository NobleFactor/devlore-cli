// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import "sync"

var (
	announceMu sync.Mutex
	announced  []Provider
)

// Announce records a provider descriptor. Called in init().
// Does zero initialization — stores the value for later InitAll callback.
func Announce(p Provider) {
	announceMu.Lock()
	defer announceMu.Unlock()
	announced = append(announced, p)
}

// InitAll calls Register on every announced provider.
// Called once by the framework when it is ready to build an ActionRegistry.
func InitAll(reg *ActionRegistry, ctx Context) {
	announceMu.Lock()
	providers := make([]Provider, len(announced))
	copy(providers, announced)
	announceMu.Unlock()

	for _, p := range providers {
		p.Register(reg, ctx)
	}
}

// Providers returns all announced providers (for introspection/debugging).
func Providers() []Provider {
	announceMu.Lock()
	defer announceMu.Unlock()
	out := make([]Provider, len(announced))
	copy(out, announced)
	return out
}

// resetAnnounced clears the announced list. For testing only.
func resetAnnounced() {
	announceMu.Lock()
	defer announceMu.Unlock()
	announced = nil
}
