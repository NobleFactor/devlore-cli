// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlark

import (
	"fmt"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/NobleFactor/devlore-cli/internal/host"
)

// SystemRoot implements the top-level system namespace using the Attr receiver pattern.
// It provides access to sub-namespaces (package, service, git, file) and
// the platform struct.
//
// Unlike plan.*, system.* bindings query current system state immediately
// rather than building an execution graph.
type SystemRoot struct {
	host host.Host

	// Sub-namespaces (cached)
	packageSystem *SystemPackage
	serviceSystem *SystemService
	gitSystem     *SystemGit
	fileSystem    *SystemFile

	// Platform struct (cached)
	platformValue starlark.Value
}

// NewSystemRoot creates a new SystemRoot for the given host.
func NewSystemRoot(h host.Host) *SystemRoot {
	p := h.Platform()
	platformValue := starlarkstruct.FromStringDict(starlark.String("platform"), starlark.StringDict{
		"os":       starlark.String(p.OS),
		"arch":     starlark.String(p.Arch),
		"distro":   starlark.String(p.Distro),
		"version":  starlark.String(p.Version),
		"hostname": starlark.String(p.Hostname),
	})

	return &SystemRoot{
		host:          h,
		packageSystem: NewSystemPackage(h.PackageManager()),
		serviceSystem: NewSystemService(h.ServiceManager()),
		gitSystem:     NewSystemGit(),
		fileSystem:    NewSystemFile(h),
		platformValue: platformValue,
	}
}

// Starlark Value interface
func (s *SystemRoot) String() string        { return "system" }
func (s *SystemRoot) Type() string          { return "system" }
func (s *SystemRoot) Freeze()               {}
func (s *SystemRoot) Truth() starlark.Bool  { return true }
func (s *SystemRoot) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable: system") }

// Starlark HasAttrs interface
func (s *SystemRoot) Attr(name string) (starlark.Value, error) {
	switch name {
	// Sub-namespaces
	case "package":
		return s.packageSystem, nil
	case "service":
		return s.serviceSystem, nil
	case "git":
		return s.gitSystem, nil
	case "file":
		return s.fileSystem, nil
	// Platform struct (immediate value, not a namespace)
	case "platform":
		return s.platformValue, nil
	default:
		return nil, starlark.NoSuchAttrError(fmt.Sprintf("system has no attribute %q", name))
	}
}

func (s *SystemRoot) AttrNames() []string {
	return []string{"file", "git", "package", "platform", "service"}
}
