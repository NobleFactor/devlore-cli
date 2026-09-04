// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"golang.org/x/term"
)

// isTerminal reports whether the file is a terminal. A variable so a test can take the handoff branch
// without one.
var isTerminal = func(f *os.File) bool { return term.IsTerminal(int(f.Fd())) }

// RunInteractive hands the terminal to a child the user drives -- an editor, a pager -- and waits for it.
//
// It is the one seam through which stdout leaves this process for a child (10-command-line-interface.md
// §10, ruled 2026-09-03). A child run for its output is captured instead, always, and never comes here.
// With no terminal on both stdin and stdout there is nothing to hand over: the call fails naming the
// alternative, rather than launching an editor into a pipe.
//
// Parameters:
//   - `child`: the command to run; its standard streams are set here.
//   - `alternative`: what the user does instead when there is no terminal, as a clause.
//
// Returns:
//   - `error`: no terminal, or the child's failure.
func RunInteractive(child *exec.Cmd, alternative string) error {

	if !isTerminal(os.Stdin) || !isTerminal(os.Stdout) {
		return fmt.Errorf("%s needs a terminal and there is none; %s", filepath.Base(child.Path), alternative)
	}

	child.Stdin = os.Stdin
	child.Stdout = os.Stdout
	child.Stderr = os.Stderr

	return child.Run()
}
