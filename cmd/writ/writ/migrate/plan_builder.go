// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package migrate

import (
	"fmt"
	"os"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/plan"
)

// planBuilder accumulates file-operation invocations and ordering constraints, then assembles them into an
// immutable execution graph via [plan.Provider.Assemble].
//
// Each op method (Mkdir / Copy / Rename / Remove) plans one invocation against its file-provider method and
// returns it; [planBuilder.DependsOn] records "from before to" ordering between two returned invocations.
// [planBuilder.Build] topologically sorts the invocations by those constraints and assembles them in that
// order — the sealed executor runs top-level children sequentially, so the sort order is the execution order.
type planBuilder struct {
	planProvider *plan.Provider
	project      string
	invocations  []*op.Invocation
	ordering     []orderingEdge
	err          error // first error encountered while planning; surfaced by Build
}

// orderingEdge is a "from must complete before to" constraint between two accumulated invocations.
type orderingEdge struct {
	from *op.Invocation
	to   *op.Invocation
}

// newPlanBuilder creates a plan builder bound to a planning provider over `env`.
//
// Parameters:
//   - `env`: the planning runtime environment; supplies the receiver registry used for provider-method lookup.
//   - `project`: the project recorded on the assembled graph's [op.Origin] via [planBuilder.Build].
//
// Returns:
//   - *planBuilder: the ready builder.
//   - `error`: always nil today; retained so callers need not change if construction grows fallible.
func newPlanBuilder(env *op.RuntimeEnvironment, project string) (*planBuilder, error) {
	return &planBuilder{
		planProvider: plan.NewProvider(env),
		project:      project,
	}, nil
}

// Mkdir plans a directory-creation invocation.
func (p *planBuilder) Mkdir(path string) *op.Invocation {
	return p.add("file.mkdir", map[string]any{
		"path":  path,
		"chmod": os.FileMode(0o755),
		"chown": "",
	})
}

// Copy plans a file-copy invocation.
func (p *planBuilder) Copy(source, path string) *op.Invocation {
	return p.add("file.copy", map[string]any{
		"source":           source,
		"destination_path": path,
		"chmod":            os.FileMode(0o644),
		"chown":            "",
	})
}

// Rename plans a file-move invocation (git mv when possible).
func (p *planBuilder) Rename(source, path string) *op.Invocation {
	return p.add("file.move", map[string]any{
		"source":           source,
		"destination_path": path,
	})
}

// Remove plans a file/directory-removal invocation.
func (p *planBuilder) Remove(path string) *op.Invocation {
	return p.add("file.remove", map[string]any{
		"resource": path,
	})
}

// DependsOn records an ordering constraint: `from` must complete before `to` begins. A nil endpoint is a
// no-op (a prior planning error already invalidated the build).
func (p *planBuilder) DependsOn(from, to *op.Invocation) {
	if from == nil || to == nil {
		return
	}
	p.ordering = append(p.ordering, orderingEdge{from: from, to: to})
}

// Build topologically sorts the accumulated invocations by their ordering constraints and assembles them
// into an immutable graph whose [op.Origin] records the builder's project.
//
// Returns:
//   - *op.Graph: the assembled graph.
//   - `error`: non-nil on a prior planning error, a dependency cycle, or an assembly failure.
func (p *planBuilder) Build() (*op.Graph, error) {

	if p.err != nil {
		return nil, p.err
	}

	sorted, err := p.topologicalOrder()
	if err != nil {
		return nil, err
	}

	graph, err := p.planProvider.Assemble(sorted, nil, nil, nil, p.planProvider.Origin(p.project))
	if err != nil {
		return nil, fmt.Errorf("planBuilder.Build: assemble: %w", err)
	}

	return graph, nil
}

// add plans one invocation against the file-provider method named by `actionName` (e.g. "file.mkdir") with
// `slots` as its keyword arguments and accumulates it. On any failure it records the first error (surfaced by
// Build) and returns nil.
func (p *planBuilder) add(actionName string, slots map[string]any) *op.Invocation {

	if p.err != nil {
		return nil
	}

	invocation, err := p.planProvider.Plan(actionName, nil, slots)
	if err != nil {
		p.err = fmt.Errorf("planBuilder: plan %s: %w", actionName, err)
		return nil
	}

	p.invocations = append(p.invocations, invocation)
	return invocation
}

// topologicalOrder returns the accumulated invocations ordered so every DependsOn constraint is honored
// (Kahn's algorithm, seeding ready nodes in insertion order for stable output).
func (p *planBuilder) topologicalOrder() ([]*op.Invocation, error) {

	inDegree := make(map[*op.Invocation]int, len(p.invocations))
	for _, invocation := range p.invocations {
		inDegree[invocation] = 0
	}

	successors := make(map[*op.Invocation][]*op.Invocation)
	for _, edge := range p.ordering {
		successors[edge.from] = append(successors[edge.from], edge.to)
		inDegree[edge.to]++
	}

	var ready []*op.Invocation
	for _, invocation := range p.invocations {
		if inDegree[invocation] == 0 {
			ready = append(ready, invocation)
		}
	}

	sorted := make([]*op.Invocation, 0, len(p.invocations))
	for len(ready) > 0 {
		current := ready[0]
		ready = ready[1:]
		sorted = append(sorted, current)
		for _, next := range successors[current] {
			inDegree[next]--
			if inDegree[next] == 0 {
				ready = append(ready, next)
			}
		}
	}

	if len(sorted) != len(p.invocations) {
		return nil, fmt.Errorf("planBuilder: dependency cycle in migration graph")
	}

	return sorted, nil
}
