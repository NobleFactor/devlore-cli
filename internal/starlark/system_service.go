// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlark

import (
	"fmt"

	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/internal/host"
)

// SystemService implements system.service.* bindings for service manager queries.
// These are immediate queries - they execute during analysis, not deferred.
type SystemService struct {
	sm host.ServiceManager
}

// NewSystemService creates a new SystemService for the given service manager.
func NewSystemService(sm host.ServiceManager) *SystemService {
	return &SystemService{sm: sm}
}

// Starlark Value interface
func (s *SystemService) String() string        { return "system.service" }
func (s *SystemService) Type() string          { return "system.service" }
func (s *SystemService) Freeze()               {}
func (s *SystemService) Truth() starlark.Bool  { return true }
func (s *SystemService) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable: system.service") }

// Starlark HasAttrs interface
func (s *SystemService) Attr(name string) (starlark.Value, error) {
	switch name {
	case "exists":
		return starlark.NewBuiltin("system.service.exists", s.exists), nil
	case "running":
		return starlark.NewBuiltin("system.service.running", s.running), nil
	case "enabled":
		return starlark.NewBuiltin("system.service.enabled", s.enabled), nil
	default:
		return nil, starlark.NoSuchAttrError(fmt.Sprintf("system.service has no attribute %q", name))
	}
}

func (s *SystemService) AttrNames() []string {
	return []string{"enabled", "exists", "running"}
}

// exists checks if a service exists.
// Usage: system.service.exists(name)
//
// Arguments:
//   - name: Service name to check
//
// Returns: True if the service exists
func (s *SystemService) exists(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	if err := starlark.UnpackArgs("exists", args, kwargs, "name", &name); err != nil {
		return nil, err
	}
	return starlark.Bool(s.sm.Exists(name)), nil
}

// running checks if a service is running.
// Usage: system.service.running(name)
//
// Arguments:
//   - name: Service name to check
//
// Returns: True if the service is running
func (s *SystemService) running(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	if err := starlark.UnpackArgs("running", args, kwargs, "name", &name); err != nil {
		return nil, err
	}
	return starlark.Bool(s.sm.Status(name) == "running"), nil
}

// enabled checks if a service is enabled to start on boot.
// Usage: system.service.enabled(name)
//
// Arguments:
//   - name: Service name to check
//
// Returns: True if the service is enabled
func (s *SystemService) enabled(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	if err := starlark.UnpackArgs("enabled", args, kwargs, "name", &name); err != nil {
		return nil, err
	}
	// Check if status contains "enabled" - implementation depends on service manager
	status := s.sm.Status(name)
	return starlark.Bool(status == "enabled" || status == "running"), nil
}
