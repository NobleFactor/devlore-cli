// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlark

import (
	"fmt"
	"os"
	"os/exec"

	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/internal/host"
)

// SystemFile implements system.file.* bindings for filesystem queries.
// These are immediate queries - they execute during analysis, not deferred.
type SystemFile struct {
	host host.Host
}

// NewSystemFile creates a new SystemFile for the given host.
func NewSystemFile(h host.Host) *SystemFile {
	return &SystemFile{host: h}
}

// Starlark Value interface
func (f *SystemFile) String() string        { return "system.file" }
func (f *SystemFile) Type() string          { return "system.file" }
func (f *SystemFile) Freeze()               {}
func (f *SystemFile) Truth() starlark.Bool  { return true }
func (f *SystemFile) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable: system.file") }

// Starlark HasAttrs interface
func (f *SystemFile) Attr(name string) (starlark.Value, error) {
	switch name {
	case "exists":
		return starlark.NewBuiltin("system.file.exists", f.exists), nil
	case "is_dir":
		return starlark.NewBuiltin("system.file.is_dir", f.isDir), nil
	case "which":
		return starlark.NewBuiltin("system.file.which", f.which), nil
	case "home":
		return starlark.NewBuiltin("system.file.home", f.home), nil
	default:
		return nil, starlark.NoSuchAttrError(fmt.Sprintf("system.file has no attribute %q", name))
	}
}

func (f *SystemFile) AttrNames() []string {
	return []string{"exists", "home", "is_dir", "which"}
}

// exists checks if a path exists.
// Usage: system.file.exists(path)
//
// Arguments:
//   - path: Path to check (with ~ expansion)
//
// Returns: True if path exists
func (f *SystemFile) exists(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var path string
	if err := starlark.UnpackArgs("exists", args, kwargs, "path", &path); err != nil {
		return nil, err
	}
	path = f.host.ExpandPath(path)
	_, err := os.Stat(path)
	return starlark.Bool(err == nil), nil
}

// isDir checks if a path is a directory.
// Usage: system.file.is_dir(path)
//
// Arguments:
//   - path: Path to check (with ~ expansion)
//
// Returns: True if path exists and is a directory
func (f *SystemFile) isDir(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var path string
	if err := starlark.UnpackArgs("is_dir", args, kwargs, "path", &path); err != nil {
		return nil, err
	}
	path = f.host.ExpandPath(path)
	info, err := os.Stat(path)
	if err != nil {
		return starlark.False, nil
	}
	return starlark.Bool(info.IsDir()), nil
}

// which finds an executable in PATH.
// Usage: system.file.which(name)
//
// Arguments:
//   - name: Executable name to find
//
// Returns: Full path to executable, or empty string if not found
func (f *SystemFile) which(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	if err := starlark.UnpackArgs("which", args, kwargs, "name", &name); err != nil {
		return nil, err
	}
	path, err := exec.LookPath(name)
	if err != nil {
		return starlark.String(""), nil
	}
	return starlark.String(path), nil
}

// home returns the user's home directory.
// Usage: system.file.home()
//
// Returns: Path to user's home directory
func (f *SystemFile) home(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	return starlark.String(f.host.HomeDir()), nil
}
