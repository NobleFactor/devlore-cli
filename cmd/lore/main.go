// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Command lore is the tribal knowledge package deployer.
package main

import (
	"os"

	"github.com/NobleFactor/devlore-cli/cmd/lore/lore"
	"github.com/NobleFactor/devlore-cli/internal/cli"

	// Blank-import the op inventory so every provider's gen package init() runs and registers its
	// ProviderReceiverType — the plan provider resolves actions through the receiver registry.
	_ "github.com/NobleFactor/devlore-cli/pkg/op/inventory"
)

func main() {
	cmd := lore.NewRootCmd()
	if err := cmd.Execute(); err != nil {
		os.Exit(cli.ExitCode(err))
	}
}
