// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

// Command writ is the portable environment orchestrator.
package main

import (
	"os"

	"github.com/NobleFactor/devlore-cli/cmd/internal/cli"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ"

	// Blank-import the op inventory so every provider's gen package init() runs and registers its
	// ProviderReceiverType — the plan provider resolves actions through the receiver registry.
	_ "github.com/NobleFactor/devlore-cli/pkg/op/inventory"
)

func main() {
	cmd := writ.NewRootCmd()
	if err := cmd.Execute(); err != nil {
		os.Exit(cli.ExitCode(err))
	}
}
