// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Noble Factor. All rights reserved.

// Command lore is the tribal knowledge package deployer.
package main

import (
	"os"

	"github.com/NobleFactor/devlore-cli/internal/lore"
)

func main() {
	cmd := lore.NewRootCmd()
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
