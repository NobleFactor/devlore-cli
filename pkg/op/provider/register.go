// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package provider registers all operation graph providers.
package provider

import (
	"github.com/NobleFactor/devlore-cli/pkg/op"

	// Blank imports trigger init() in each provider's generated files,
	// which call op.RegisterBinding() to self-register.
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/archive"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/encryption"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/file"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/git"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/net"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/pkg"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/service"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/shell"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/template"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/ui"
)

// RegisterAll registers all provider actions with the given registry.
// Provider packages self-register via init() when imported above.
func RegisterAll(reg *op.ActionRegistry) {
	op.RegisterAllProviders(reg)
}
