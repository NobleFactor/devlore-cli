// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package provider

import (
	"github.com/NobleFactor/devlore-cli/internal/execution"

	// Blank imports trigger init() in each provider's actions_gen.go,
	// which calls execution.RegisterProvider(Register).
	_ "github.com/NobleFactor/devlore-cli/internal/execution/provider/archive"
	_ "github.com/NobleFactor/devlore-cli/internal/execution/provider/encryption"
	_ "github.com/NobleFactor/devlore-cli/internal/execution/provider/file"
	_ "github.com/NobleFactor/devlore-cli/internal/execution/provider/git"
	_ "github.com/NobleFactor/devlore-cli/internal/execution/provider/net"
	_ "github.com/NobleFactor/devlore-cli/internal/execution/provider/pkg"
	_ "github.com/NobleFactor/devlore-cli/internal/execution/provider/service"
	_ "github.com/NobleFactor/devlore-cli/internal/execution/provider/shell"
	_ "github.com/NobleFactor/devlore-cli/internal/execution/provider/template"
)

// RegisterAll registers all provider actions with the given registry.
// Provider packages self-register via init() when imported above.
func RegisterAll(reg *execution.ActionRegistry) {
	execution.RegisterAllProviders(reg)
}
