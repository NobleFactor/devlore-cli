// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package flow

import "github.com/NobleFactor/devlore-cli/internal/execution"

// Register registers all flow actions into the given registry.
func Register(reg *execution.ActionRegistry) {
	reg.Register(&Choose{})
	reg.Register(&Gather{})
	reg.Register(&Elevate{})
	reg.Register(&WaitUntil{})
}
