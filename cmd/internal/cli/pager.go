// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/NobleFactor/devlore-cli/pkg/result"
	"github.com/spf13/viper"
)

// The pager's defaults.
const (

	// pagerVariable is this suite's own pager, consulted before the `pager` configuration key and before
	// `PAGER`. It is to these programs what `GIT_PAGER` is to git: a pager for this suite that differs from
	// the one everything else uses.
	pagerVariable = "DEVLORE_PAGER"

	// pagerFallback is what runs when neither variable is set.
	//
	// `-F` quits if the output fits on one screen, which is what makes "only when it is long" true without
	// this package measuring anything; `-R` keeps the escape codes `terminal` emits; `-X` leaves the output
	// on screen after the pager exits.
	pagerFallback = "less -FRX"
)

// activePager is the pager this command invocation opened, or nil. A package-level value for the same reason
// [restoreStoreRoot] is one: it is scoped to a single command invocation, which is all a cobra process runs.
var activePager *pagerSession

// region Helpers

// shouldPage reports whether a result rendered by a formatter of this group goes to a pager.
//
// Four conditions, all of them (10-command-line-interface.md §10):
//
//   - stdout is a terminal. A pipe or a file never pages, so a rendering is the same bytes either way.
//   - the rendering is one a person reads: [result.GroupRecords] or [result.GroupDocument]. Serialized and
//     Composed exist to be consumed by a program, and Nothing emits nothing.
//   - the output does not fit on one screen, which `less -F` decides and this package does not measure.
//   - nothing refused it: `--no-pager` refuses, and `--paginate` asks for a pager whatever the group.
//
// Parameters:
//   - `isTTY`: whether the result is going to a terminal.
//   - `group`: the rendering's group.
//   - `paginate`: whether `--paginate` was given.
//   - `noPager`: whether `--no-pager` was given.
//
// Returns:
//   - `bool`: true when the result should be paged.
func shouldPage(isTTY bool, group result.Group, paginate, noPager bool) bool {

	if noPager || !isTTY {
		return false
	}

	if paginate {
		return true
	}

	return group == result.GroupRecords || group == result.GroupDocument
}

// pagerCommand returns the pager to run and its arguments, or nil when paging is disabled.
//
// Which pager runs and whether to page at all are two questions, as they are in git, and this answers only the
// first. The chain is git's, name for name:
//
//	GIT_PAGER  ->  core.pager  ->  PAGER  ->  less
//	DEVLORE_PAGER  ->  pager  ->  PAGER  ->  less -FRX
//
// The second question belongs to [shouldPage] and to the `--paginate` and `--no-pager` flags, none of which
// touches the configured pager: refusing one today leaves it in place for tomorrow.
//
// An empty value anywhere in the chain disables paging, as `GIT_PAGER=` and `core.pager=` do. The value is
// split on spaces rather than handed to a shell: a pager is a command and its flags, and running the user's
// string through a shell on four platforms buys nothing this suite needs.
//
// Parameters:
//   - `program`: the program's name, which is the section its configuration lives under.
//
// Returns:
//   - `[]string`: the command and its arguments, or nil when paging is disabled.
func pagerCommand(program string) []string {

	if value, set := os.LookupEnv(pagerVariable); set {
		return strings.Fields(value)
	}

	if key := program + ".pager"; viper.IsSet(key) {
		return strings.Fields(viper.GetString(key))
	}

	if value, set := os.LookupEnv("PAGER"); set {
		return strings.Fields(value)
	}

	return strings.Fields(pagerFallback)
}

// closeActivePager closes the pager this invocation opened, if any, and waits for it.
//
// The pager owns the terminal until the reader quits it, so a command waits here rather than exiting into a
// shell prompt that fights the pager for the screen. Called from the post-run [addOutputFlags] installs.
//
// Returns:
//   - `error`: a close or wait error; nil when no pager was opened.
func closeActivePager() error {

	if activePager == nil {
		return nil
	}

	pager := activePager
	activePager = nil

	return pager.Close()
}

// endregion

// region pagerSession

// pagerSession is a running pager and the pipe a result is written to.
type pagerSession struct {
	command *exec.Cmd
	stdin   io.WriteCloser
}

// openPager starts the pager, with `out` as its own output.
//
// Parameters:
//   - `out`: where the pager writes, which is the stdout the result was bound for.
//   - `program`: the program's name, for the configuration section [pagerCommand] reads.
//
// Returns:
//   - `*pagerSession`: the running pager.
//   - `error`: the pager could not be started.
func openPager(out io.Writer, program string) (*pagerSession, error) {

	argv := pagerCommand(program)
	if len(argv) == 0 {
		return nil, fmt.Errorf("cli.openPager: paging is disabled")
	}

	command := exec.CommandContext(context.Background(), argv[0], argv[1:]...) //nolint:gosec // G204: the pager is the user's own, named by the environment
	command.Stdout = out
	command.Stderr = os.Stderr

	stdin, err := command.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("cli.openPager: %w", err)
	}

	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("cli.openPager: start %s: %w", argv[0], err)
	}

	return &pagerSession{command: command, stdin: stdin}, nil
}

// Write sends bytes to the pager.
//
// Parameters:
//   - `p`: the bytes to write.
//
// Returns:
//   - `int`: the count written.
//   - `error`: any write error.
func (s *pagerSession) Write(p []byte) (int, error) { return s.stdin.Write(p) }

// Close closes the pipe and waits for the pager to exit.
//
// Waiting is the point: a pager owns the terminal until the reader quits it, and a program that exits first
// leaves the shell prompt fighting the pager for the screen.
//
// Returns:
//   - `error`: a close or wait error. A pager the reader quit early closes its input, and that is not an
//     error to report: the reader said they were done reading.
func (s *pagerSession) Close() error {

	_ = s.stdin.Close() //nolint:errcheck // a pager quit early has already closed its end

	if err := s.command.Wait(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return nil
		}
		return fmt.Errorf("cli.pagerSession: wait: %w", err)
	}

	return nil
}

// endregion
