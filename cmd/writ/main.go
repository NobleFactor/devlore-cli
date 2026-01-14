// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Noble Factor. All rights reserved.

// Command writ is the dotfiles manager with platform-aware symlinks.
package main

import (
	"os"

	"github.com/NobleFactor/devlore-cli/internal/writ"
)

func main() {
	cmd := writ.NewRootCmd()
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
