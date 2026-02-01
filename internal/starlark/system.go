// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlark

import (
	"os"
	"os/exec"
	"strings"

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
		"git":      s.gitStruct(),
		"fs":       s.fsStruct(),
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
		"installed": starlark.NewBuiltin("system.package.installed", func(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			var name string
			if err := starlark.UnpackArgs("installed", args, kwargs, "name", &name); err != nil {
				return nil, err
			}
			return starlark.Bool(pm.Installed(name)), nil
		}),
		"version": starlark.NewBuiltin("system.package.version", func(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			var name string
			if err := starlark.UnpackArgs("version", args, kwargs, "name", &name); err != nil {
				return nil, err
			}
			return starlark.String(pm.Version(name)), nil
		}),
		"manager": starlark.NewBuiltin("system.package.manager", func(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
			return starlark.String(pm.Name()), nil
		}),
	})
}

// serviceStruct returns service queries as a Starlark struct.
func (s *systemBindings) serviceStruct() starlark.Value {
	sm := s.host.ServiceManager()
	return starlarkstruct.FromStringDict(starlark.String("service"), starlark.StringDict{
		"exists": starlark.NewBuiltin("system.service.exists", func(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			var name string
			if err := starlark.UnpackArgs("exists", args, kwargs, "name", &name); err != nil {
				return nil, err
			}
			return starlark.Bool(sm.Exists(name)), nil
		}),
		"running": starlark.NewBuiltin("system.service.running", func(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			var name string
			if err := starlark.UnpackArgs("running", args, kwargs, "name", &name); err != nil {
				return nil, err
			}
			return starlark.Bool(sm.Status(name) == "running"), nil
		}),
		"enabled": starlark.NewBuiltin("system.service.enabled", func(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
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

// gitStruct returns git queries as a Starlark struct.
func (s *systemBindings) gitStruct() starlark.Value {
	return starlarkstruct.FromStringDict(starlark.String("git"), starlark.StringDict{
		"installed": starlark.NewBuiltin("system.git.installed", func(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
			_, err := exec.LookPath("git")
			return starlark.Bool(err == nil), nil
		}),
		"version": starlark.NewBuiltin("system.git.version", func(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
			cmd := exec.Command("git", "--version")
			output, err := cmd.Output()
			if err != nil {
				return starlark.String(""), nil
			}
			parts := strings.Fields(string(output))
			if len(parts) >= 3 {
				return starlark.String(parts[2]), nil
			}
			return starlark.String(strings.TrimSpace(string(output))), nil
		}),
		"repo_root": starlark.NewBuiltin("system.git.repo_root", func(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
			cmd := exec.Command("git", "rev-parse", "--show-toplevel")
			output, err := cmd.Output()
			if err != nil {
				return starlark.String(""), nil
			}
			return starlark.String(strings.TrimSpace(string(output))), nil
		}),
		"current_branch": starlark.NewBuiltin("system.git.current_branch", func(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
			cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
			output, err := cmd.Output()
			if err != nil {
				return starlark.String(""), nil
			}
			return starlark.String(strings.TrimSpace(string(output))), nil
		}),
		"is_clean": starlark.NewBuiltin("system.git.is_clean", func(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
			cmd := exec.Command("git", "status", "--porcelain")
			output, err := cmd.Output()
			if err != nil {
				return starlark.False, nil
			}
			return starlark.Bool(len(strings.TrimSpace(string(output))) == 0), nil
		}),
	})
}

// fsStruct returns filesystem queries as a Starlark struct.
func (s *systemBindings) fsStruct() starlark.Value {
	return starlarkstruct.FromStringDict(starlark.String("fs"), starlark.StringDict{
		"exists": starlark.NewBuiltin("system.fs.exists", func(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			var path string
			if err := starlark.UnpackArgs("exists", args, kwargs, "path", &path); err != nil {
				return nil, err
			}
			path = s.host.ExpandPath(path)
			_, err := os.Stat(path)
			return starlark.Bool(err == nil), nil
		}),
		"is_dir": starlark.NewBuiltin("system.fs.is_dir", func(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			var path string
			if err := starlark.UnpackArgs("is_dir", args, kwargs, "path", &path); err != nil {
				return nil, err
			}
			path = s.host.ExpandPath(path)
			info, err := os.Stat(path)
			if err != nil {
				return starlark.False, nil
			}
			return starlark.Bool(info.IsDir()), nil
		}),
		"which": starlark.NewBuiltin("system.fs.which", func(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			var name string
			if err := starlark.UnpackArgs("which", args, kwargs, "name", &name); err != nil {
				return nil, err
			}
			path, err := exec.LookPath(name)
			if err != nil {
				return starlark.String(""), nil
			}
			return starlark.String(path), nil
		}),
		"home": starlark.NewBuiltin("system.fs.home", func(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
			return starlark.String(s.host.HomeDir()), nil
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
