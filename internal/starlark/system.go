// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlark

import (
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/NobleFactor/devlore-cli/internal/host"
)

// =============================================================================
// System Bindings Implementation
// =============================================================================

// systemBindings implements SystemBindings by wrapping host.Host.
type systemBindings struct {
	host host.Host
}

// NewSystemBindings creates a new SystemBindings from a host.Host.
func NewSystemBindings(h host.Host) SystemBindings {
	return &systemBindings{host: h}
}

// Platform returns information about the current system.
func (s *systemBindings) Platform() host.Platform {
	return s.host.Platform()
}

// Package returns package manager queries.
func (s *systemBindings) Package() PackageQueries {
	return &packageQueries{pm: s.host.PackageManager()}
}

// Service returns service manager queries.
func (s *systemBindings) Service() ServiceQueries {
	return &serviceQueries{sm: s.host.ServiceManager()}
}

// ToStarlark converts the system bindings to a Starlark value.
func (s *systemBindings) ToStarlark() starlark.Value {
	return starlarkstruct.FromStringDict(starlark.String("system"), starlark.StringDict{
		"platform": s.platformStruct(),
		"package":  s.packageStruct(),
		"service":  s.serviceStruct(),
	})
}

// platformStruct returns the platform information as a Starlark struct.
func (s *systemBindings) platformStruct() starlark.Value {
	p := s.host.Platform()
	return starlarkstruct.FromStringDict(starlark.String("platform"), starlark.StringDict{
		"os":       starlark.String(p.OS),
		"arch":     starlark.String(p.Arch),
		"distro":   starlark.String(p.Distro),
		"version":  starlark.String(p.Version),
		"hostname": starlark.String(p.Hostname),
	})
}

// packageStruct returns package queries as a Starlark struct.
func (s *systemBindings) packageStruct() starlark.Value {
	pm := s.host.PackageManager()
	return starlarkstruct.FromStringDict(starlark.String("package"), starlark.StringDict{
		"installed": starlark.NewBuiltin("installed", func(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			var name string
			if err := starlark.UnpackArgs("installed", args, kwargs, "name", &name); err != nil {
				return nil, err
			}
			return starlark.Bool(pm.Installed(name)), nil
		}),
		"version": starlark.NewBuiltin("version", func(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			var name string
			if err := starlark.UnpackArgs("version", args, kwargs, "name", &name); err != nil {
				return nil, err
			}
			return starlark.String(pm.Version(name)), nil
		}),
		"manager": starlark.String(pm.Name()),
	})
}

// serviceStruct returns service queries as a Starlark struct.
func (s *systemBindings) serviceStruct() starlark.Value {
	sm := s.host.ServiceManager()
	return starlarkstruct.FromStringDict(starlark.String("service"), starlark.StringDict{
		"exists": starlark.NewBuiltin("exists", func(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			var name string
			if err := starlark.UnpackArgs("exists", args, kwargs, "name", &name); err != nil {
				return nil, err
			}
			return starlark.Bool(sm.Exists(name)), nil
		}),
		"running": starlark.NewBuiltin("running", func(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			var name string
			if err := starlark.UnpackArgs("running", args, kwargs, "name", &name); err != nil {
				return nil, err
			}
			return starlark.Bool(sm.Status(name) == "running"), nil
		}),
		"enabled": starlark.NewBuiltin("enabled", func(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			var name string
			if err := starlark.UnpackArgs("enabled", args, kwargs, "name", &name); err != nil {
				return nil, err
			}
			// Check if status contains "enabled" - implementation depends on service manager
			status := sm.Status(name)
			return starlark.Bool(status == "enabled" || status == "running"), nil
		}),
	})
}

// =============================================================================
// Package Queries Implementation
// =============================================================================

type packageQueries struct {
	pm host.PackageManager
}

func (p *packageQueries) Installed(name string) bool {
	return p.pm.Installed(name)
}

func (p *packageQueries) Version(name string) string {
	return p.pm.Version(name)
}

// =============================================================================
// Service Queries Implementation
// =============================================================================

type serviceQueries struct {
	sm host.ServiceManager
}

func (s *serviceQueries) Exists(name string) bool {
	return s.sm.Exists(name)
}

func (s *serviceQueries) Running(name string) bool {
	return s.sm.Status(name) == "running"
}

func (s *serviceQueries) Enabled(name string) bool {
	status := s.sm.Status(name)
	return status == "enabled" || status == "running"
}
