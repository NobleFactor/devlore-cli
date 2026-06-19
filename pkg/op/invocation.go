// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

// Invocation is the handle dispatch constructs for every plan.* call and the starlark value every plan.* call
// returns to the author. Target is the op-level unit the invocation will dispatch (a [*Node] or [*Subgraph]).
//
// Per phase-8 D2, the binding layer picks how to bind an invocation from the target parameter's type at the binding
// site: slots typed [ExecutableUnit] consume Target directly; value-typed slots consume the invocation's
// [PromiseBinding] (via [Invocation.Binding]), which carries the producer's ID for plan.run to materialize into an
// edge.
type Invocation struct {
	Target ExecutableUnit // workflow-level unit that will dispatch when executed
	Label  string         // registered (user-supplied or auto-generated)
}

// region EXPORTED METHODS

// region Behaviors

// Binding returns the [PromiseBinding] that binds a consumer slot to this invocation's producer.
//
// Preserves the detachment contract (phase-8 D5): the returned [PromiseBinding] carries the producer's ID and
// plan.run materializes the producer→consumer edge at graph construction. The caller places it into a spec slot via
// WithSlot — no node is mutated.
//
// Returns:
//   - `Binding`: a [PromiseBinding] referencing the producer by ID.
func (i *Invocation) Binding() Binding {
	return NewPromiseBinding(i.Target.ID())
}

// endregion

// endregion
