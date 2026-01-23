// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

// Command writ is the dotfiles manager with platform-aware symlinks.
package main

import (
	"os"

	"github.com/NobleFactor/devlore-cli/internal/cli"
	"github.com/NobleFactor/devlore-cli/internal/writ"
)

func main() {
	cmd := writ.NewRootCmd()
	if err := cmd.Execute(); err != nil {
		os.Exit(cli.ExitCode(err))
	}
}
