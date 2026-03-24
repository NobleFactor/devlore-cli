// SPDX-License-Identifier: MIT
// Copyright Noble Factor. All rights reserved.

package commands

// CommandTree provides access to the command hierarchy. The Runtime implements this
// interface and injects it via context Data["command_tree"].
type CommandTree interface {
	// CommandNames returns all registered command names (space-separated).
	CommandNames() []string

	// RunCommand executes a command by space-separated name with the given flags
	// and optional positional arguments.
	RunCommand(name string, flags map[string]string, positional ...string) error

	// CommandHelp returns the help text for a command.
	CommandHelp(name string) string

	// CommandFlags returns the flags for a command as (name, help, default) tuples.
	CommandFlags(name string) []CommandFlag
}

// CommandFlag represents a command flag.
type CommandFlag struct {
	Name    string `starlark:"name"`
	Help    string `starlark:"help"`
	Default string `starlark:"default"`
}

// RunResult holds the outcome of a command execution.
type RunResult struct {
	Passed bool   `starlark:"passed"`
	Error  string `starlark:"error"`
}

// HookCheckResult holds the status of a hook check.
type CommandInfo struct {
	Name  string        `starlark:"name"`
	Help  string        `starlark:"help"`
	Flags []CommandFlag `starlark:"flags"`
}
