// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlark

import (
	"fmt"

	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/internal/host"
)

// SystemPackage implements system.package.* bindings for package manager queries.
// These are immediate queries - they execute during analysis, not deferred.
type SystemPackage struct {
	pm host.PackageManager
}

// NewSystemPackage creates a new SystemPackage for the given package manager.
func NewSystemPackage(pm host.PackageManager) *SystemPackage {
	return &SystemPackage{pm: pm}
}

// Starlark Value interface
func (p *SystemPackage) String() string        { return "system.package" }
func (p *SystemPackage) Type() string          { return "system.package" }
func (p *SystemPackage) Freeze()               {}
func (p *SystemPackage) Truth() starlark.Bool  { return true }
func (p *SystemPackage) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable: system.package") }

// Starlark HasAttrs interface
func (p *SystemPackage) Attr(name string) (starlark.Value, error) {
	switch name {
	case "installed":
		return starlark.NewBuiltin("system.package.installed", p.installed), nil
	case "version":
		return starlark.NewBuiltin("system.package.version", p.version), nil
	case "manager":
		return starlark.NewBuiltin("system.package.manager", p.manager), nil
	default:
		return nil, starlark.NoSuchAttrError(fmt.Sprintf("system.package has no attribute %q", name))
	}
}

func (p *SystemPackage) AttrNames() []string {
	return []string{"installed", "manager", "version"}
}

// installed checks if a package is installed.
// Usage: system.package.installed(name)
//
// Arguments:
//   - name: Package name to check
//
// Returns: True if the package is installed
func (p *SystemPackage) installed(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	if err := starlark.UnpackArgs("installed", args, kwargs, "name", &name); err != nil {
		return nil, err
	}
	return starlark.Bool(p.pm.Installed(name)), nil
}

// version returns the installed version of a package.
// Usage: system.package.version(name)
//
// Arguments:
//   - name: Package name to check
//
// Returns: Version string, or empty if not installed
func (p *SystemPackage) version(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	if err := starlark.UnpackArgs("version", args, kwargs, "name", &name); err != nil {
		return nil, err
	}
	return starlark.String(p.pm.Version(name)), nil
}

// manager returns the name of the system package manager.
// Usage: system.package.manager()
//
// Returns: Package manager name (e.g., "apt", "brew", "dnf")
func (p *SystemPackage) manager(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	return starlark.String(p.pm.Name()), nil
}
