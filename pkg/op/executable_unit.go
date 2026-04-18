// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import "fmt"

// region ExecutableUnit interface

// ExecutableUnit is anything the executor can dispatch — a Node or a Subgraph.
//
// Nodes and subgraphs share one ID space and are interchangeable wherever a reference is valid (Phase 7 invariant). The
// parameter surface is precomputed at plan time and immutable afterward; Execute trusts the surface and does not
// revalidate.
type ExecutableUnit interface {
	ID() string
	Parameters() []Parameter
}

// endregion

// region executableUnit base

// executableUnit is the shared state embedded by Node and Subgraph.
//
// Both fields are unexported; callers read through the ID() and Parameters() methods and write through the constructors
// (NewNode, NewSubgraph) and plan-time hooks (Node.Bind, Subgraph.FinalizeParameters).
type executableUnit struct {
	id         string
	parameters []Parameter
}

// ID returns the identifier.
func (e *executableUnit) ID() string { return e.id }

// Parameters returns the precomputed parameter surface.
func (e *executableUnit) Parameters() []Parameter { return e.parameters }

// endregion

// region Subgraph — plan-time parameter finalization

// FinalizeParameters computes and stores the subgraph's parameter surface.
//
// Called once by the planner after children and edges are finalized.
//
// The surface is the union by name of every topological root's parameters, where a root is a child with no incoming
// edges from within this subgraph. Name collisions across roots are a plan-time error: if two roots declare the same
// parameter name with different types, FinalizeParameters returns an error and the plan is rejected.
//
// Recursive: if a root is itself a subgraph, its FinalizeParameters must have been called first (or be safe to call
// now; this method invokes it via Parameters()).
func (s *Subgraph) FinalizeParameters() error {

	roots := s.topologicalRoots()

	seen := make(map[string]Parameter, len(roots)*2)
	order := make([]string, 0, len(roots)*2)

	for _, child := range roots {
		for _, p := range childParameters(child) {
			if existing, ok := seen[p.Name]; ok {
				if existing.Type != p.Type {
					return fmt.Errorf(
						"subgraph %q: parameter %q collides across roots (types %v vs %v)",
						s.id, p.Name, existing.Type, p.Type)
				}
				continue
			}
			seen[p.Name] = p
			order = append(order, p.Name)
		}
	}

	params := make([]Parameter, len(order))
	for i, name := range order {
		params[i] = seen[name]
	}
	s.parameters = params
	return nil
}

// endregion

// region UNEXPORTED HELPERS

// topologicalRoots returns children with no incoming edges from within s.Edges.
func (s *Subgraph) topologicalRoots() []SubgraphChild {

	hasIncoming := make(map[string]bool, len(s.Edges))
	for _, e := range s.Edges {
		hasIncoming[e.To] = true
	}

	roots := make([]SubgraphChild, 0, len(s.Children))
	for _, c := range s.Children {
		if !hasIncoming[c.ChildID()] {
			roots = append(roots, c)
		}
	}
	return roots
}

// childParameters returns the parameter surface of a SubgraphChild by delegating to whichever field is populated.
//
// A child subgraph must have been finalized (Parameters() returns non-nil if it has roots, nil otherwise — an empty
// subgraph legitimately exposes no parameters).
func childParameters(c SubgraphChild) []Parameter {

	if c.Node != nil {
		return c.Node.Parameters()
	}
	if c.Subgraph != nil {
		return c.Subgraph.Parameters()
	}
	return nil
}

// endregion
