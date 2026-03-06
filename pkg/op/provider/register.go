// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package provider triggers init() in all provider packages via blank imports.
// Importing this package causes every provider to call op.Announce(), making
// them available for op.InitAll().
package provider

import (
	// Blank imports trigger init() in each provider package,
	// which call op.Announce() to self-register.
	_ "github.com/NobleFactor/devlore-cli/internal/execution/flow"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/archive/gen"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/encryption/gen"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/file/gen"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/git/gen"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/json/gen"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/net/gen"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/pkg/gen"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/regexp/gen"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/service/gen"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/shell/gen"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/staranalysis/gen"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/starcode/gen"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/starcomplexity/gen"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/starindex/gen"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/starstats/gen"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/template/gen"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/ui/gen"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/yaml/gen"
)
