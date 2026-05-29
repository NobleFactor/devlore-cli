// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package migrate

import (
	"fmt"
	"os"
	"reflect"
	"strings"

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
	registry     *op.ReceiverRegistry
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

// newPlanBuilder creates a plan builder bound to the planning provider sourced from `env`.
//
// Parameters:
//   - `env`: the planning runtime environment; supplies the cached [plan.Provider] and the receiver registry.
//   - `project`: the default project stamped onto each planned node's [op.Node.Origin].
//
// Returns:
//   - *planBuilder: the ready builder.
//   - `error`: non-nil when the plan provider cannot be resolved from the runtime environment.
func newPlanBuilder(env *op.RuntimeEnvironment, project string) (*planBuilder, error) {

	raw, err := env.ProviderByType(reflect.TypeFor[plan.Provider]())
	if err != nil {
		return nil, fmt.Errorf("newPlanBuilder: resolve plan provider: %w", err)
	}

	planProvider, ok := raw.(*plan.Provider)
	if !ok {
		return nil, fmt.Errorf("newPlanBuilder: plan provider is %T, want *plan.Provider", raw)
	}

	return &planBuilder{
		planProvider: planProvider,
		registry:     env.ReceiverRegistry,
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
// into an immutable graph with `origin`.
//
// Returns:
//   - *op.Graph: the assembled graph.
//   - `error`: non-nil on a prior planning error, a dependency cycle, or an assembly failure.
func (p *planBuilder) Build(origin op.Origin) (*op.Graph, error) {

	if p.err != nil {
		return nil, p.err
	}

	sorted, err := p.topologicalOrder()
	if err != nil {
		return nil, err
	}

	graph, err := p.planProvider.Assemble(sorted, nil, nil, nil, origin)
	if err != nil {
		return nil, fmt.Errorf("planBuilder.Build: assemble: %w", err)
	}

	return graph, nil
}

// add plans one invocation against the file-provider method named by `actionName` (e.g. "file.mkdir") with
// `slots` as its keyword arguments, registers it, and accumulates it. On any failure it records the first
// error (surfaced by Build) and returns nil.
func (p *planBuilder) add(actionName string, slots map[string]any) *op.Invocation {

	if p.err != nil {
		return nil
	}

	receiverType, method, err := p.resolveAction(actionName)
	if err != nil {
		p.err = err
		return nil
	}

	unit, err := method.Planner().Plan(p.planProvider, receiverType, method, nil, slots, nil, nil)
	if err != nil {
		p.err = fmt.Errorf("planBuilder: plan %s: %w", actionName, err)
		return nil
	}

	if node, ok := unit.(*op.Node); ok {
		node.Origin = p.project
	}

	label := p.planProvider.InvocationRegistry().AutoLabel(receiverType.Name() + "." + op.CamelToSnake(method.Name()))
	invocation := &op.Invocation{
		Target: unit,
		Result: op.NewPromise(unit, ""),
		Label:  label,
	}

	if err := p.planProvider.InvocationRegistry().Register(label, invocation); err != nil {
		p.err = fmt.Errorf("planBuilder: register %s: %w", actionName, err)
		return nil
	}

	p.invocations = append(p.invocations, invocation)
	return invocation
}

// resolveAction resolves a dotted action name (e.g. "file.mkdir") to its provider receiver type and method.
func (p *planBuilder) resolveAction(name string) (op.ProviderReceiverType, *op.Method, error) {

	dot := strings.LastIndex(name, ".")
	if dot < 0 {
		return nil, nil, fmt.Errorf("planBuilder: invalid action name %q: no dot", name)
	}

	receiverName, methodSnake := name[:dot], name[dot+1:]

	receiverType, ok := p.registry.ActionByName(receiverName)
	if !ok {
		return nil, nil, fmt.Errorf("planBuilder: unknown action provider %q", receiverName)
	}

	for method := range receiverType.Methods() {
		if op.CamelToSnake(method.Name()) == methodSnake {
			return receiverType, method, nil
		}
	}

	return nil, nil, fmt.Errorf("planBuilder: method %q not found on %q", methodSnake, receiverName)
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
