// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Code generated from gen-receiver templates; DO NOT EDIT.

package starlark

import (
	"fmt"
	"io"

	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/internal/host"
)

// ArchiveReceiver provides the archive.* Starlark namespace.
//
// Backing implementation: host.Host (ExpandPath, RunCommand).
// Extraction delegates to tar via shell.
type ArchiveReceiver struct {
	Receiver
	host   host.Host
	output io.Writer
}

// NewArchiveReceiver creates a new archive receiver.
func NewArchiveReceiver(h host.Host, output io.Writer) *ArchiveReceiver {
	return &ArchiveReceiver{
		Receiver: NewReceiver("archive"),
		host:     h,
		output:   output,
	}
}

// Attr implements starlark.HasAttrs.
func (r *ArchiveReceiver) Attr(name string) (starlark.Value, error) {
	switch name {
	case "extract":
		return MakeAttr("archive.extract", r.extract), nil
	default:
		return nil, NoSuchAttrError("archive", name)
	}
}

// AttrNames implements starlark.HasAttrs.
func (r *ArchiveReceiver) AttrNames() []string {
	return []string{"extract"}
}

func (r *ArchiveReceiver) extract(_ *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var path, dest string
	var strip int
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "path", &path, "dest", &dest, "strip?", &strip); err != nil {
		return nil, err
	}
	path = r.host.ExpandPath(path)
	dest = r.host.ExpandPath(dest)

	_, _ = fmt.Fprintf(r.output, "  [archive] Extracting %s -> %s\n", path, dest)

	cmd := fmt.Sprintf("mkdir -p %s && tar -xf %s -C %s", dest, path, dest)
	if strip > 0 {
		cmd = fmt.Sprintf("mkdir -p %s && tar -xf %s -C %s --strip-components=%d", dest, path, dest, strip)
	}

	result := r.host.RunCommand(cmd, false)
	return resultToStarlark(result), nil
}
