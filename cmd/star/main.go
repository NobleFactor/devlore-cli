// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

// star is the Starlark-powered operations tool for NobleFactor projects.
// Commands are defined as extensions in the star/extensions/ directory.
package main

import (
	"os"

	"github.com/NobleFactor/devlore-cli/cmd/internal/cli"
	"github.com/NobleFactor/devlore-cli/cmd/star/star"
	"github.com/NobleFactor/devlore-cli/pkg/iox"
)

func main() {
	if err := run(); err != nil {
		os.Exit(cli.ExitCode(err))
	}
}

// run builds the star command tree and executes it, returning the execution error.
//
// The extraction keeps main's single os.Exit above every defer: an os.Exit inside this body would
// skip the runtime's Close (gocritic exitAfterDefer).
//
// Returns:
//   - `error`: the command execution error, or nil on success.
func run() (err error) {

	rootCmd, runtime := star.NewRootCmd()
	defer iox.Close(&err, runtime)

	// Parse, validate, then run -- after the extensions have contributed their commands, since their groups
	// are part of the tree this validates (#897).
	if err := cli.ValidateCommandLine(rootCmd, os.Args[1:]); err != nil {
		return err // already on stderr; returning it carries the exit code through run's defer
	}

	return rootCmd.Execute()
}
