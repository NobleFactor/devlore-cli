// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlark

import (
	"fmt"
	"io"

	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/internal/host"
)

// PackageReceiver provides the package.* Starlark namespace.
//
// Backing implementation: host.PackageManager for package operations.
// Also provides access to package features and settings from the manifest.
type PackageReceiver struct {
	Receiver
	pm       host.PackageManager
	features []string
	settings map[string]string
	output   io.Writer
}

// NewPackageReceiver creates a new package receiver.
func NewPackageReceiver(pm host.PackageManager, features []string, settings map[string]string, output io.Writer) *PackageReceiver {
	return &PackageReceiver{
		Receiver: NewReceiver("package"),
		pm:       pm,
		features: features,
		settings: settings,
		output:   output,
	}
}

// Attr implements starlark.HasAttrs.
func (r *PackageReceiver) Attr(name string) (starlark.Value, error) {
	switch name {
	case "manager":
		return MakeAttr("package.manager", r.manager), nil
	case "installed":
		return MakeAttr("package.installed", r.installed), nil
	case "version":
		return MakeAttr("package.version", r.version), nil
	case "install":
		return MakeAttr("package.install", r.install), nil
	case "remove":
		return MakeAttr("package.remove", r.remove), nil
	case "update":
		return MakeAttr("package.update", r.update), nil
	case "feature":
		return MakeAttr("package.feature", r.feature), nil
	case "setting":
		return MakeAttr("package.setting", r.setting), nil
	default:
		return nil, NoSuchAttrError("package", name)
	}
}

// AttrNames implements starlark.HasAttrs.
func (r *PackageReceiver) AttrNames() []string {
	return []string{"feature", "install", "installed", "manager", "remove", "setting", "update", "version"}
}

func (r *PackageReceiver) manager(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	return starlark.String(r.pm.Name()), nil
}

func (r *PackageReceiver) installed(_ *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "name", &name); err != nil {
		return nil, err
	}
	return starlark.Bool(r.pm.Installed(name)), nil
}

func (r *PackageReceiver) version(_ *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "name", &name); err != nil {
		return nil, err
	}
	return starlark.String(r.pm.Version(name)), nil
}

func (r *PackageReceiver) install(_ *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	var manager string
	var cask bool
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "name", &name, "manager?", &manager, "cask?", &cask); err != nil {
		return nil, err
	}

	_, _ = fmt.Fprintf(r.output, "  [package] Installing %s", name)
	if manager != "" {
		_, _ = fmt.Fprintf(r.output, " via %s", manager)
	}
	if cask {
		_, _ = fmt.Fprintf(r.output, " (cask)")
	}
	_, _ = fmt.Fprintln(r.output)

	result := r.pm.Install(name)
	return resultToStarlark(result), nil
}

func (r *PackageReceiver) remove(_ *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "name", &name); err != nil {
		return nil, err
	}

	_, _ = fmt.Fprintf(r.output, "  [package] Removing %s\n", name)
	result := r.pm.Remove(name)
	return resultToStarlark(result), nil
}

func (r *PackageReceiver) update(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	_, _ = fmt.Fprintln(r.output, "  [package] Updating package index")
	result := r.pm.Update()
	return resultToStarlark(result), nil
}

func (r *PackageReceiver) feature(_ *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "name", &name); err != nil {
		return nil, err
	}

	for _, f := range r.features {
		if f == name {
			return starlark.True, nil
		}
	}
	return starlark.False, nil
}

func (r *PackageReceiver) setting(_ *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	var defaultValue string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "name", &name, "default?", &defaultValue); err != nil {
		return nil, err
	}

	if val, ok := r.settings[name]; ok {
		return starlark.String(val), nil
	}
	return starlark.String(defaultValue), nil
}
