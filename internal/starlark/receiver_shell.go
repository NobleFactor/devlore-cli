// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlark

import (
	"io"
	"os/exec"

	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/internal/execution/provider/shell"
)

// ShellReceiver provides the shell.* Starlark namespace.
// Forward operations (exec, run) delegate to shell.Provider.
// Query operations (which) use os/exec directly.
type ShellReceiver struct {
	Receiver
	provider *shell.Provider
	output   io.Writer
}

// NewShellReceiver creates a new shell receiver.
func NewShellReceiver(provider *shell.Provider, output io.Writer) *ShellReceiver {
	return &ShellReceiver{
		Receiver: NewReceiver("shell"),
		provider: provider,
		output:   output,
	}
}

// Attr implements starlark.HasAttrs.
func (r *ShellReceiver) Attr(name string) (starlark.Value, error) {
	switch name {
	case "exec":
		return MakeAttr("shell.exec", r.shellExec), nil
	case "power_shell":
		return MakeAttr("shell.power_shell", r.powerShell), nil
	case "run":
		return MakeAttr("shell.run", r.shellRun), nil
	case "which":
		return MakeAttr("shell.which", r.shellWhich), nil
	default:
		return nil, NoSuchAttrError("shell", name)
	}
}

// AttrNames implements starlark.HasAttrs.
func (r *ShellReceiver) AttrNames() []string {
	return []string{"exec", "power_shell", "run", "which"}
}

func (r *ShellReceiver) shellExec(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var command string
	if err := starlark.UnpackArgs("exec", args, kwargs, "command", &command); err != nil {
		return nil, err
	}
	if err := r.provider.Shell(command, r.output); err != nil {
		return nil, err
	}
	return starlark.None, nil
}

func (r *ShellReceiver) shellRun(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var command string
	if err := starlark.UnpackArgs("run", args, kwargs, "command", &command); err != nil {
		return nil, err
	}
	if err := r.provider.Shell(command, r.output); err != nil {
		return nil, err
	}
	return starlark.None, nil
}

func (r *ShellReceiver) powerShell(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var command string
	if err := starlark.UnpackArgs("power_shell", args, kwargs, "command", &command); err != nil {
		return nil, err
	}
	if err := r.provider.PowerShell(command, r.output); err != nil {
		return nil, err
	}
	return starlark.None, nil
}

func (r *ShellReceiver) shellWhich(_ *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "name", &name); err != nil {
		return nil, err
	}
	path, err := exec.LookPath(name)
	if err != nil {
		return starlark.String(""), nil
	}
	return starlark.String(path), nil
}
