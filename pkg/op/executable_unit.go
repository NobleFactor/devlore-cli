// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"fmt"

	"github.com/NobleFactor/devlore-cli/pkg/assert"
)

// region ExecutableUnit interface

// ExecutableUnit is anything the executor can dispatch — a Node or a Subgraph.
//
// Nodes and subgraphs share one ID space and are interchangeable wherever a reference is valid (Phase 7
// invariant). Parameters reports the unit's input surface and is consumed by flow combinators (e.g.,
// gather) to discover the body's expected input.
//
// stampParent is package-internal — exposed on the interface so [Subgraph.AddChild] and
// [Subgraph.SetErrorAction] can stamp ownership without a *Node / *Subgraph type-switch. Because the
// method is unexported, the interface is closed to same-package implementations — only [*Node] and
// [*Subgraph] satisfy it.
type ExecutableUnit interface {
	ID() string
	Parameters() []Parameter
	stampParent(parentID string)
}

// endregion

// region executableUnit base

// executableUnit is the shared state embedded by Node and Subgraph.
//
// All fields are unexported; callers read through the ID() / Parameters() / ParentID() / RetryPolicy() /
// ErrorAction() methods and write through the constructors (NewNode, NewSubgraph), plan-time hooks
// (Node.Bind), and the (s *Subgraph).AddChild method that stamps the parent ID.
//
// parentID identifies the enclosing Subgraph by ID rather than pointer (plan-doc D11). By-ID rather than
// by-pointer because containment must round-trip through plan.save / plan.load — pointers don't
// serialize. The graph's Root subgraph has parentID == "" (the only unit with an empty parentID after
// it has been wired into a Graph). Cross-Graph reuse works because graph.Root.ID is the constant "root"
// across all Graphs, so an Invocation that's a root child in two different Graphs stamps the same
// parentID value both times — idempotent.
//
// retryPolicy and errorAction are plan-doc D5 / D11 fields — every executable unit can carry them. Nil
// retryPolicy means no retry; nil errorAction defaults to the flow.Provider.Failed sentinel at dispatch
// time.
type executableUnit struct {
	id          string
	parameters  []Parameter
	parentID    string
	retryPolicy *RetryPolicy
	errorAction ExecutableUnit
}

// ID returns the identifier.
func (e *executableUnit) ID() string { return e.id }

// Parameters returns the precomputed parameter surface.
//
// For Node, this is the full Go-method signature (populated by Node.Bind from method.Parameters());
// for Subgraph, this is shadowed by *Subgraph.Parameters() which computes the bubble-up variable
// surface dynamically via a graph-walk (plan-doc D3).
func (e *executableUnit) Parameters() []Parameter { return e.parameters }

// ParentID returns the ID of the enclosing Subgraph, or the empty string for the graph root (or for a
// unit that has not yet been added to any parent).
//
// Returns:
//   - string: the parent Subgraph's ID, or "".
func (e *executableUnit) ParentID() string { return e.parentID }

// RetryPolicy returns this unit's retry policy, or nil when no policy is configured.
//
// Returns:
//   - *RetryPolicy: the configured retry policy, or nil.
func (e *executableUnit) RetryPolicy() *RetryPolicy { return e.retryPolicy }

// SetRetryPolicy sets this unit's retry policy.
//
// Parameters:
//   - p: the retry policy to set. Pass nil to disable retry.
func (e *executableUnit) SetRetryPolicy(p *RetryPolicy) { e.retryPolicy = p }

// ErrorAction returns the failure handler for this unit, or nil when no error action is configured.
//
// Returns:
//   - ExecutableUnit: the configured error-action unit, or nil. Nil defaults to the flow.Provider.Failed
//     sentinel at dispatch time.
func (e *executableUnit) ErrorAction() ExecutableUnit { return e.errorAction }

// SetErrorAction sets the failure-handler unit.
//
// Parameters:
//   - ea: the failure-handling executable unit. Pass nil to use the default flow.Provider.Failed sentinel.
func (e *executableUnit) SetErrorAction(ea ExecutableUnit) { e.errorAction = ea }

// stampParent sets this unit's parentID with idempotency.
//
// Calling again with the same parentID succeeds silently; calling with a different non-empty parentID
// panics — within a single Graph context, a unit can be a child of only one Subgraph at a time. Cross-
// Graph reuse via the constant "root" ID for graph.Root is the use case the idempotency permits.
//
// Parameters:
//   - newParentID: the parent Subgraph's ID to stamp. Must not be empty (asserted).
func (e *executableUnit) stampParent(newParentID string) {

	assert.True("newParentID not empty", newParentID != "")

	if e.parentID != "" && e.parentID != newParentID {
		panic(fmt.Sprintf(
			"executableUnit %q already has parentID %q; cannot re-parent to %q",
			e.id, e.parentID, newParentID,
		))
	}

	e.parentID = newParentID
}

// endregion