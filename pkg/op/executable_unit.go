// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

// region ExecutableUnit interface

// ExecutableUnit is anything the executor can dispatch — a Node or a Subgraph.
//
// Nodes and subgraphs share one ID space and are interchangeable wherever a reference is valid (Phase 7
// invariant). Parameters reports the unit's input surface and is consumed by flow combinators (e.g., gather)
// to discover the body's expected input.
type ExecutableUnit interface {
	ID() string
	Parameters() []Parameter
}

// endregion

// region executableUnit base

// executableUnit is the shared state embedded by Node and Subgraph.
//
// Both fields are unexported; callers read through the ID() and Parameters() methods and write through the
// constructors (NewNode, NewSubgraph) and plan-time hooks (Node.Bind).
type executableUnit struct {
	id         string
	parameters []Parameter
}

// ID returns the identifier.
func (e *executableUnit) ID() string { return e.id }

// Parameters returns the precomputed parameter surface.
func (e *executableUnit) Parameters() []Parameter { return e.parameters }

// endregion
