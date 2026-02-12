// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlark

import (
	"fmt"
	"io"

	"go.starlark.net/starlark"
)

// LogReceiver provides the log.* Starlark namespace and root-level
// output functions (note, warn, error, success, fail).
//
// Backing implementation: io.Writer.
// Bound to root as both the "log" namespace and as individual builtins.
type LogReceiver struct {
	Receiver
	output io.Writer
}

// NewLogReceiver creates a new log receiver.
func NewLogReceiver(output io.Writer) *LogReceiver {
	return &LogReceiver{
		Receiver: NewReceiver("log"),
		output:   output,
	}
}

func (r *LogReceiver) Attr(name string) (starlark.Value, error) {
	switch name {
	case "info":
		return MakeAttr("log.info", r.note), nil
	case "warn":
		return MakeAttr("log.warn", r.warn), nil
	case "error":
		return MakeAttr("log.error", r.errorFunc), nil
	default:
		return nil, NoSuchAttrError("log", name)
	}
}

func (r *LogReceiver) AttrNames() []string {
	return []string{"error", "info", "warn"}
}

// Root-level output builtins — also bound as top-level globals.

func (r *LogReceiver) note(_ *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var msg string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "msg", &msg); err != nil {
		if len(args) >= 1 {
			if s, ok := starlark.AsString(args[0]); ok {
				msg = s
			}
		}
	}
	_, _ = fmt.Fprintf(r.output, "  [note] %s\n", msg)
	return starlark.None, nil
}

func (r *LogReceiver) warn(_ *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var msg string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "msg", &msg); err != nil {
		if len(args) >= 1 {
			if s, ok := starlark.AsString(args[0]); ok {
				msg = s
			}
		}
	}
	_, _ = fmt.Fprintf(r.output, "  [warn] %s\n", msg)
	return starlark.None, nil
}

func (r *LogReceiver) errorFunc(_ *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var msg string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "msg", &msg); err != nil {
		if len(args) >= 1 {
			if s, ok := starlark.AsString(args[0]); ok {
				msg = s
			}
		}
	}
	_, _ = fmt.Fprintf(r.output, "  [ERROR] %s\n", msg)
	return nil, fmt.Errorf("phase error: %s", msg)
}

func (r *LogReceiver) fail(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	var msg string
	if len(args) >= 1 {
		if s, ok := starlark.AsString(args[0]); ok {
			msg = s
		}
	}
	_, _ = fmt.Fprintf(r.output, "  [FAIL] %s\n", msg)
	return nil, fmt.Errorf("phase failed: %s", msg)
}

func (r *LogReceiver) success(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	var msg string
	if len(args) >= 1 {
		if s, ok := starlark.AsString(args[0]); ok {
			msg = s
		}
	}
	_, _ = fmt.Fprintf(r.output, "  [SUCCESS] %s\n", msg)
	return starlark.None, nil
}
