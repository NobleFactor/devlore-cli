// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package provider

import (
	"github.com/NobleFactor/devlore-cli/internal/execution"
	"github.com/NobleFactor/devlore-cli/internal/execution/provider/archive"
	"github.com/NobleFactor/devlore-cli/internal/execution/provider/content"
	"github.com/NobleFactor/devlore-cli/internal/execution/provider/encryption"
	"github.com/NobleFactor/devlore-cli/internal/execution/provider/file"
	"github.com/NobleFactor/devlore-cli/internal/execution/provider/git"
	"github.com/NobleFactor/devlore-cli/internal/execution/provider/net"
	"github.com/NobleFactor/devlore-cli/internal/execution/provider/pkg"
	"github.com/NobleFactor/devlore-cli/internal/execution/provider/service"
	"github.com/NobleFactor/devlore-cli/internal/execution/provider/shell"
)

// RegisterAll registers all provider actions into the given registry.
func RegisterAll(reg *execution.ActionRegistry) {
	file.Register(reg)
	encryption.Register(reg)
	pkg.Register(reg)
	shell.Register(reg)
	service.Register(reg)
	content.Register(reg)
	net.Register(reg)
	archive.Register(reg)
	git.Register(reg)
}
