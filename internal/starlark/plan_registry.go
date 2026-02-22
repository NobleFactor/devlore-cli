// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlark

import (
	"sort"

	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/internal/execution"
	"github.com/NobleFactor/devlore-cli/internal/host"
	"github.com/NobleFactor/devlore-cli/pkg/projection"
)

// PlanFactory creates a plan sub-namespace for the given graph context.
type PlanFactory func(graph *projection.Graph, h host.Host, project string, reg *execution.ActionRegistry) starlark.Value

// planRegistry maps namespace names to plan factories. Populated by init()
// functions in generated plan_*_gen.go files.
var planRegistry = map[string]PlanFactory{}

// registerPlan adds a plan factory to the registry.
func registerPlan(name string, f PlanFactory) {
	planRegistry[name] = f
}

// PlanNames returns sorted namespace names from the registry.
func PlanNames() []string {
	names := make([]string, 0, len(planRegistry))
	for name := range planRegistry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
