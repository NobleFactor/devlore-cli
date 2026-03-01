// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package provider registers all operation graph providers.
package provider

import (
	"github.com/NobleFactor/devlore-cli/pkg/op"

	// Blank imports trigger init() in each provider's gen package,
	// which call op.RegisterBinding() to self-register.
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/archive/gen"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/encryption/gen"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/file/gen"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/git/gen"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/net/gen"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/pkg/gen"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/service/gen"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/shell/gen"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/starcode/gen"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/template/gen"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/ui/gen"
)

// RegisterAll registers all provider actions with the given registry.
// Provider packages self-register via init() when imported above.
func RegisterAll(reg *op.ActionRegistry) {
	op.RegisterAllProviders(reg)
}
