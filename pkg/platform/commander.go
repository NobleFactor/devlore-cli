// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package platform

import "context"

// commander is what a leaf or a service manager runs commands through: the [CommandRunner] the platform was built
// with, and the environment the manager's commands carry. Embedded by [driver] and by the service managers, so a
// manager's methods say `m.run(argv, account)` and nothing else about how a command runs or as whom it is made to.
type commander struct {

	// runner runs the commands; set by [New] from the [Spec], never by a manager.
	runner CommandRunner

	// env joins every command's environment: apt's `DEBIAN_FRONTEND=noninteractive`, and nothing for most.
	env map[string]string
}

// region UNEXPORTED METHODS

// region State management

// setRunner installs the runner, at [New] time.
//
// Parameters:
//   - `runner`: the runner.
func (c *commander) setRunner(runner CommandRunner) {
	c.runner = runner
}

// endregion

// region Behaviors

// run runs a command as an account through the platform's runner, with the manager's environment and the
// auto-confirm stdin every package manager's command gets.
//
// Parameters:
//   - `argv`: the command and its arguments; `argv[0]` is the executable.
//   - `account`: [AccountCaller] or [AccountRoot].
//
// Returns:
//   - `Result`: the captured stdout, stderr and exit code; a launch failure or a refusal is a Result with Code -1.
//
//nolint:unused // called by the Unix-tagged leaves; the Windows leaves reach their runner at lane 8 of #1127.
func (c commander) run(argv []string, account string) Result {

	if c.runner == nil {
		return Result{Code: -1, Stderr: "platform: no command runner; build the platform with New"}
	}
	return c.runner(context.Background(), CommandRequest{Account: account, Argv: argv, Env: c.env, AutoConfirm: true})
}

// endregion

// endregion

// region SUPPORTING TYPES

// runnerSetter is implemented by every manager that runs commands, through [commander]; [New] hands each its runner.
type runnerSetter interface {
	setRunner(runner CommandRunner)
}

// endregion
