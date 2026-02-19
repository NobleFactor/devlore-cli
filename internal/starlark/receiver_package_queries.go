// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlark

import (
	"io"

	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/internal/execution/provider/pkg"
	"github.com/NobleFactor/devlore-cli/internal/host"
)

// PackageReceiver provides the package.* Starlark namespace.
// Forward operations (install, upgrade, remove, update) delegate to
// pkg.Provider. Query operations (manager, installed, version, feature,
// setting) use the platform's PackageManager and manifest data directly.
type PackageReceiver struct {
	Receiver
	provider *pkg.Provider
	pm       host.PackageManager
	features []string
	settings map[string]string
	output   io.Writer
}

// NewPackageReceiver creates a new package receiver.
func NewPackageReceiver(pm host.PackageManager, features []string, settings map[string]string, output io.Writer) *PackageReceiver {
	return &PackageReceiver{
		Receiver: NewReceiver("package"),
		provider: &pkg.Provider{},
		pm:       pm,
		features: features,
		settings: settings,
		output:   output,
	}
}

func (r *PackageReceiver) queryAttr(name string) (starlark.Value, error) {
	switch name {
	case "manager":
		return MakeAttr("package.manager", r.manager), nil
	case "installed":
		return MakeAttr("package.installed", r.installed), nil
	case "version":
		return MakeAttr("package.version", r.version), nil
	case "feature":
		return MakeAttr("package.feature", r.feature), nil
	case "setting":
		return MakeAttr("package.setting", r.setting), nil
	default:
		return nil, NoSuchAttrError("package", name)
	}
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
