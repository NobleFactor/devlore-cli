// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

// Command devlore-test is the graph test harness for Starlark plan + execute + verify.
package main

import (
	"os"

	"github.com/NobleFactor/devlore-cli/cmd/devlore-test/devloretest"
	"github.com/NobleFactor/devlore-cli/cmd/internal/cli"

	_ "github.com/NobleFactor/devlore-cli/pkg/op/inventory"
)

func main() {
	cmd := devloretest.NewRootCmd()

	// Parse, validate, then run. Cobra resolves and dispatches in one pass, and a group it cannot run is
	// abandoned before its arguments are validated -- so `repo add` printed help and exited 0 once `add` was
	// retired. This refuses first, on the same resolution cobra is about to repeat (#897).
	if err := cli.ValidateCommandLine(cmd, os.Args[1:]); err != nil {
		os.Exit(cli.ExitCode(err)) // the refusal is already on stderr; this is its exit code
	}

	if err := cmd.Execute(); err != nil {
		os.Exit(cli.ExitCode(err))
	}
}
