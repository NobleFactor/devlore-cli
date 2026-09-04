// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

// Package devloretest implements the devlore-test CLI commands.
package devloretest

import (
	"github.com/spf13/cobra"

	"github.com/NobleFactor/devlore-cli/cmd/internal/cli"
	"github.com/NobleFactor/devlore-cli/pkg/application"
	"github.com/NobleFactor/devlore-cli/schema"
)

// Version information, stamped once for every command in [application].
var (
	version   = application.Version
	commit    = application.Commit
	buildDate = application.BuildDate
)

// NewRootCmd creates the root devlore-test command with all subcommands.
//
// The root is the shared one (#757): the common set, `--config`, `--dry-run`, `--verbose`, `--silent`, the
// narrator, the configuration, and `version`, `man`, `config` and `self` all come from [cli.NewRootCmd],
// so a repair there reaches devlore-test the day it lands.
//
// Returns:
//   - `*cobra.Command`: the root, with `run` attached.
func NewRootCmd() *cobra.Command {

	rootCmd := cli.NewRootCmd(cli.RootConfig{
		Name:  "devlore-test",
		Short: "Graph test harness for Starlark plan + execute + verify",
		Long: `devlore-test is the graph test harness for the devlore execution engine.

It executes a Starlark test script that builds an execution graph, runs the
graph through the engine, and verifies expectations against the results.

The result goes to stdout as JSON; narration goes to stderr; the definition
and its traces go to the execution store.

  devlore-test run test.star
  devlore-test run -o yaml test.star
  devlore-test run --store ./run test.star
  devlore-test run -o none test.star      # exit code only`,
		DefaultConfig: schema.TestDefaultConfig,
		Version:       version,
		Commit:        commit,
		BuildDate:     buildDate,
	})

	rootCmd.AddCommand(newRunCmd())

	return rootCmd
}
