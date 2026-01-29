// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Command lore is the tribal knowledge package deployer.
package main

import (
	"os"

	"github.com/NobleFactor/devlore-cli/internal/cli"
	"github.com/NobleFactor/devlore-cli/internal/lore"
)

func main() {
	cmd := lore.NewRootCmd()
	if err := cmd.Execute(); err != nil {
		os.Exit(cli.ExitCode(err))
	}
}
