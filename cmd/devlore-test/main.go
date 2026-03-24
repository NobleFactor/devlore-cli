// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Command devlore-test is the graph test harness for Starlark plan + execute + verify.
package main

import (
	"os"

	"github.com/NobleFactor/devlore-cli/cmd/devlore-test/devloretest"
	"github.com/NobleFactor/devlore-cli/internal/cli"

	_ "github.com/NobleFactor/devlore-cli/pkg/op/inventory"
)

func main() {
	cmd := devloretest.NewRootCmd()
	if err := cmd.Execute(); err != nil {
		os.Exit(cli.ExitCode(err))
	}
}
