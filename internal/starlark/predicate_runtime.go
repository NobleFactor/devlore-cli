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

// RuntimePredicate is a predicate that evaluates at execution time.
// It implements both execution.Predicate (for Choose) and starlark.Value
// (for passing through Starlark scripts).
type RuntimePredicate struct {
	description string
	eval        func(any) (bool, error)
}

// Starlark Value interface
func (p *RuntimePredicate) String() string        { return fmt.Sprintf("predicate(%s)", p.description) }
func (p *RuntimePredicate) Type() string          { return "predicate" }
func (p *RuntimePredicate) Freeze()               {}
func (p *RuntimePredicate) Truth() starlark.Bool  { return true }
func (p *RuntimePredicate) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable: predicate") }

// execution.Predicate interface
func (p *RuntimePredicate) Eval(value any) (bool, error) { return p.eval(value) }

// =============================================================================
// Package predicates — plan.package.installed(), plan.package.not_installed()
// =============================================================================

// packageInstalled returns a predicate that checks if a package is installed.
func packageInstalled(pm host.PackageManager, name string) *RuntimePredicate {
	return &RuntimePredicate{
		description: fmt.Sprintf("package.installed(%q)", name),
		eval:        func(_ any) (bool, error) { return pm.Installed(name), nil },
	}
}

// packageNotInstalled returns a predicate that checks if a package is NOT installed.
func packageNotInstalled(pm host.PackageManager, name string) *RuntimePredicate {
	return &RuntimePredicate{
		description: fmt.Sprintf("package.not_installed(%q)", name),
		eval:        func(_ any) (bool, error) { return !pm.Installed(name), nil },
	}
}

// packageVersionGTE returns a predicate that checks if a package version >= target.
func packageVersionGTE(pm host.PackageManager, name, version string) *RuntimePredicate {
	return &RuntimePredicate{
		description: fmt.Sprintf("package.version_gte(%q, %q)", name, version),
		eval: func(_ any) (bool, error) {
			installed := pm.Version(name)
			if installed == "" {
				return false, nil
			}
			return installed >= version, nil
		},
	}
}

// =============================================================================
// Service predicates — plan.service.exists(), plan.service.running(), plan.service.enabled()
// =============================================================================

// serviceExists returns a predicate that checks if a service exists.
func serviceExists(sm host.ServiceManager, name string) *RuntimePredicate {
	return &RuntimePredicate{
		description: fmt.Sprintf("service.exists(%q)", name),
		eval:        func(_ any) (bool, error) { return sm.Exists(name), nil },
	}
}

// serviceRunning returns a predicate that checks if a service is running.
func serviceRunning(sm host.ServiceManager, name string) *RuntimePredicate {
	return &RuntimePredicate{
		description: fmt.Sprintf("service.running(%q)", name),
		eval:        func(_ any) (bool, error) { return sm.Status(name) == "running", nil },
	}
}

// serviceEnabled returns a predicate that checks if a service is enabled.
func serviceEnabled(sm host.ServiceManager, name string) *RuntimePredicate {
	return &RuntimePredicate{
		description: fmt.Sprintf("service.enabled(%q)", name),
		eval: func(_ any) (bool, error) {
			status := sm.Status(name)
			return status == "enabled" || status == "running", nil
		},
	}
}

// =============================================================================
// File predicates — plan.file.exists(), plan.file.is_dir()
// =============================================================================

// fileExists returns a predicate that checks if a path exists.
func fileExists(path string) *RuntimePredicate {
	return &RuntimePredicate{
		description: fmt.Sprintf("file.exists(%q)", path),
		eval: func(_ any) (bool, error) {
			_, err := os.Stat(path)
			return err == nil, nil
		},
	}
}

// fileIsDir returns a predicate that checks if a path is a directory.
func fileIsDir(path string) *RuntimePredicate {
	return &RuntimePredicate{
		description: fmt.Sprintf("file.is_dir(%q)", path),
		eval: func(_ any) (bool, error) {
			info, err := os.Stat(path)
			if err != nil {
				return false, nil
			}
			return info.IsDir(), nil
		},
	}
}

// =============================================================================
// Git predicates — plan.git.installed()
// =============================================================================

// gitInstalled returns a predicate that checks if git is installed.
func gitInstalled() *RuntimePredicate {
	return &RuntimePredicate{
		description: "git.installed()",
		eval: func(_ any) (bool, error) {
			_, err := exec.LookPath("git")
			return err == nil, nil
		},
	}
}
