// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

// Invocation is the handle dispatch constructs for every plan.* call and the starlark value every plan.* call
// returns to the author. It carries both representations a binding site may need: Target is the op-level unit
// the invocation will dispatch (a [*Node] or [*Subgraph]); Promise is the Promise to the invocation's
// value-side output.
//
// Per phase-8 D2, the binding layer picks which field to use based on the target parameter's type at the
// binding site — slots typed [ExecutableUnit] consume Target; value-typed slots consume Promise and the
// resulting slot PromiseValue carries the producer's UnitRef for plan.run to materialize into an edge.
//
// Attribute access on an Invocation delegates to the wrapped Promise so the starlark surface matches what
// callers saw before dispatch switched its return type from *Promise to *Invocation — same node slots,
// node_id, slot, retry, and per-slot value lookups.
type Invocation struct {
	Target ExecutableUnit // workflow-level unit that will dispatch when executed
	Result *Promise       // promise for the invocation's value-side output
	Label  string         // registered (user-supplied or auto-generated)
}

// region EXPORTED METHODS

// region Behaviors

// SlotValue returns the [PromiseValue] that binds a consumer slot to this invocation's producer.
//
// Delegates to Result.SlotValue, preserving the detachment contract (phase-8 D5): the returned [PromiseValue] carries
// the producer's UnitRef and plan.run materializes the producer→consumer edge at graph construction. The caller places
// it into a spec slot via WithSlot — no node is mutated.
//
// Returns:
//   - `SlotValue`: a [PromiseValue] referencing the producer by ID.
func (i *Invocation) SlotValue() SlotValue {
	return i.Result.SlotValue()
}

// endregion

// endregion
