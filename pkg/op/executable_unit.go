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
// All fields are unexported; callers read through the ID() / Parameters() / Parent() methods and write
// through the constructors (NewNode, NewSubgraph), plan-time hooks (Node.Bind), and the
// (s *Subgraph).AddChild method that stamps the parent pointer.
//
// The parent pointer is the forward-compatibility scaffolding for the frame-based execution model
// (plan-doc D11). It models structural ownership: each executable unit knows the Subgraph that
// contains it. The graph's Root subgraph has parent == nil; everything else has a non-nil parent
// after AddChild wires it in.
type executableUnit struct {
	id         string
	parameters []Parameter
	parent     *Subgraph
}

// ID returns the identifier.
func (e *executableUnit) ID() string { return e.id }

// Parameters returns the precomputed parameter surface.
//
// For Node, this is the full Go-method signature (populated by Node.Bind from method.Parameters());
// for Subgraph, this is shadowed by *Subgraph.Parameters() which computes the bubble-up variable
// surface dynamically via a graph-walk (plan-doc D3).
func (e *executableUnit) Parameters() []Parameter { return e.parameters }

// Parent returns the Subgraph that owns this executable unit, or nil for the graph root.
//
// Returns:
//   - *Subgraph: the owning subgraph, or nil if this is the graph's root or has not yet been added
//     to any parent.
func (e *executableUnit) Parent() *Subgraph { return e.parent }

// endregion
