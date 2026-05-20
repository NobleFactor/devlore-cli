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

// region State management

// FillSlot fills a slot on the consumer node with a [PromiseValue] referencing this invocation's producer node.
//
// Delegates to Result.FillSlot, preserving the detachment contract established by phase-8 D5 — only the slot
// PromiseValue is written; no edge struct accumulates during dispatch. plan.run materializes the
// producer→consumer edge at graph construction time from the consumer slot's UnitRef.
//
// Parameters:
//   - consumer: the node receiving this invocation's output.
//   - slot: the slot name to fill on the consumer.
func (i *Invocation) FillSlot(consumer *Node, slot string) {
	i.Result.FillSlot(consumer, slot)
}

// endregion

// endregion
