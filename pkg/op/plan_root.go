// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"fmt"
	"sort"

	"go.starlark.net/starlark"
)

// PlanRoot implements the top-level plan namespace. It delegates method
// lookups to the plan Provider's executing receiver and routes sub-namespace
// lookups (plan.file, plan.git, etc.) to PlanningReceiverFactory implementations.
type PlanRoot struct {
	// planReceiver is the plan Provider wrapped as an ExecutingReceiver.
	// Handles: choose, source, gather, complete, degraded, fatal, wait_until.
	planReceiver *ExecutingReceiver

	// Sub-namespaces built from announced PlanningReceiverFactory implementations.
	plans map[string]starlark.Value
}

// NewPlanRoot creates a PlanRoot from a plan executing receiver and announced
// PlanningReceiverFactory implementations.
func NewPlanRoot(planReceiver *ExecutingReceiver, providers map[string]PlanningReceiverFactory, graph *Graph, project string, reg *ActionRegistry) *PlanRoot {
	plans := make(map[string]starlark.Value, len(providers))
	for name, p := range providers {
		plans[name] = p.NewPlanning(graph, project, reg)
	}
	return &PlanRoot{
		planReceiver: planReceiver,
		plans:        plans,
	}
}

// String implements starlark.Value.
func (p *PlanRoot) String() string { return "plan" }

// Type implements starlark.Value.
func (p *PlanRoot) Type() string { return "plan" }

// Freeze implements starlark.Value.
func (p *PlanRoot) Freeze() {}

// Truth implements starlark.Value.
func (p *PlanRoot) Truth() starlark.Bool { return true }

// Hash implements starlark.Value.
func (p *PlanRoot) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable: plan") }

// Attr implements starlark.HasAttrs. Sub-namespaces are checked first,
// then the plan Provider's executing receiver handles method lookups.
func (p *PlanRoot) Attr(name string) (starlark.Value, error) {
	// Sub-namespace routing (plan.file, plan.git, etc.)
	if plan, ok := p.plans[name]; ok {
		return plan, nil
	}

	// Delegate to plan Provider's executing receiver.
	return p.planReceiver.Attr(name)
}

// AttrNames implements starlark.HasAttrs.
func (p *PlanRoot) AttrNames() []string {
	names := make([]string, 0, len(p.plans))
	for name := range p.plans {
		names = append(names, name)
	}
	names = append(names, p.planReceiver.AttrNames()...)
	sort.Strings(names)
	return names
}
