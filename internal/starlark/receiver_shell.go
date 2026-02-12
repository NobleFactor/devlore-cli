// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlark

import (
	"fmt"
	"io"
	"os/exec"

	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/internal/host"
)

// ShellReceiver provides the shell.* Starlark namespace.
//
// Backing implementation: host.Host (RunCommand) for exec/run,
// os/exec.LookPath for which.
type ShellReceiver struct {
	Receiver
	host   host.Host
	output io.Writer
}

// NewShellReceiver creates a new shell receiver.
func NewShellReceiver(h host.Host, output io.Writer) *ShellReceiver {
	return &ShellReceiver{
		Receiver: NewReceiver("shell"),
		host:     h,
		output:   output,
	}
}

func (r *ShellReceiver) Attr(name string) (starlark.Value, error) {
	switch name {
	case "exec":
		return MakeAttr("shell.exec", r.shellExec), nil
	case "run":
		return MakeAttr("shell.run", r.shellRun), nil
	case "which":
		return MakeAttr("shell.which", r.shellWhich), nil
	default:
		return nil, NoSuchAttrError("shell", name)
	}
}

func (r *ShellReceiver) AttrNames() []string {
	return []string{"exec", "run", "which"}
}

func (r *ShellReceiver) shellExec(_ *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var command string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "command", &command); err != nil {
		return nil, err
	}

	_, _ = fmt.Fprintf(r.output, "  [shell] %s\n", command)
	result := r.host.RunCommand(command, false)
	return resultToStarlark(result), nil
}

func (r *ShellReceiver) shellRun(_ *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var command string
	var shell bool
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "command", &command, "shell?", &shell); err != nil {
		if len(args) >= 1 {
			if s, ok := starlark.AsString(args[0]); ok {
				command = s
			}
		}
	}

	_, _ = fmt.Fprintf(r.output, "  [shell] %s\n", command)
	result := r.host.RunCommand(command, false)
	return resultToStarlark(result), nil
}

func (r *ShellReceiver) shellWhich(_ *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "name", &name); err != nil {
		if len(args) >= 1 {
			if s, ok := starlark.AsString(args[0]); ok {
				name = s
			}
		}
	}

	path, err := exec.LookPath(name)
	if err != nil {
		return starlark.String(""), nil
	}
	return starlark.String(path), nil
}
