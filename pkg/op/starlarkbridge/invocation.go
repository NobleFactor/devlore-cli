// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlarkbridge

import (
	"fmt"
	"reflect"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"go.starlark.net/starlark"
)

var (
	_ starlark.Value    = (*Invocation)(nil) // Interface Guard: ensures *Invocation implements starlark.Value.
	_ starlark.HasAttrs = (*Invocation)(nil) // Interface Guard: ensures *Invocation implements starlark.HasAttrs.
)

// Invocation is the handle dispatch constructs for every plan.* call and the starlark value every plan.* call
// returns to the author. It carries both representations a binding site may need: Target is the op-level unit
// the invocation will dispatch (an [op.Node] or [op.Subgraph]); Result is the Promise to the invocation's
// value-side output.
//
// Per phase-8 D2, the binding layer picks which field to use based on the target parameter's type at the
// binding site — slots typed [op.ExecutableUnit] consume Target; value-typed slots consume Result and the
// resulting slot PromiseValue carries the producer's NodeRef for plan.run to materialize into an edge.
//
// Attribute access on an Invocation delegates to the wrapped Promise so the starlark surface matches what
// callers saw before dispatch switched its return type from *Promise to *Invocation — same node slots,
// node_id, slot, retry, and per-slot value lookups.
type Invocation struct {
	Label  string            // registry label under which this invocation is registered (user-supplied or auto-generated)
	Target op.ExecutableUnit // op-level unit that will dispatch when executed
	Result *Promise          // Promise for the invocation's value-side output
}

// region EXPORTED METHODS

// region State management

// Freeze implements starlark.Value.
//
// Invocations are effectively immutable from the starlark side; Freeze is a no-op.
func (i *Invocation) Freeze() {}

// Hash implements starlark.Value.
//
// Invocations are unhashable because the wrapped Promise references a Node whose slots may accumulate
// bindings as the script evaluates. Callers that need a map-keyable identity should use [Invocation.Label].
//
// Returns:
//   - uint32: unused, always 0.
//   - error: always non-nil ("unhashable: Invocation").
func (i *Invocation) Hash() (uint32, error) {

	return 0, fmt.Errorf("unhashable: Invocation")
}

// String implements starlark.Value.
//
// Returns:
//   - string: a diagnostic representation identifying the invocation by its registered label.
func (i *Invocation) String() string {

	return fmt.Sprintf("Invocation(%s)", i.Label)
}

// Truth implements starlark.Value.
//
// Returns:
//   - starlark.Bool: always true; invocations are opaque handles, never falsy.
func (i *Invocation) Truth() starlark.Bool {

	return true
}

// Type implements starlark.Value.
//
// Returns:
//   - string: the type name "Invocation".
func (i *Invocation) Type() string {

	return "Invocation"
}

// endregion

// region Behaviors

// Fallible actions

// Attr implements starlark.HasAttrs by delegating to the wrapped Promise's attribute surface.
//
// The forwarded surface matches what the starlark author saw before step 10 (when dispatch returned a
// *Promise directly): node_id, slot, retry, and per-slot-parameter value lookups.
//
// Parameters:
//   - name: the attribute name to look up.
//
// Returns:
//   - starlark.Value: the attribute value from the wrapped Promise.
//   - error: non-nil if the attribute does not exist on the Promise.
func (i *Invocation) Attr(name string) (starlark.Value, error) {

	return i.Result.Attr(name)
}

// Unmarshal projects this invocation onto a Go target.
//
// Accepted targets:
//   - *Invocation: stores this pointer directly.
//   - *Promise: stores the wrapped Result.
//   - [op.PromiseValue]: stores the slot-ref shape (NodeRef + Slot) for direct slot assignment.
//   - interface{}: stores this pointer directly.
//
// Every other target errors — invocations are not directly resolvable to Go scalar types at plan time.
//
// Parameters:
//   - target: the [reflect.Value] target to populate.
//
// Returns:
//   - error: non-nil if the target type is unsupported.
func (i *Invocation) Unmarshal(target reflect.Value) error {

	invType := reflect.TypeFor[*Invocation]()
	promiseType := reflect.TypeFor[*Promise]()
	promiseValueType := reflect.TypeFor[op.PromiseValue]()

	if target.Kind() == reflect.Interface {
		target.Set(reflect.ValueOf(i))
		return nil
	}

	if target.Type() == invType {
		target.Set(reflect.ValueOf(i))
		return nil
	}

	if target.Type() == promiseType {
		target.Set(reflect.ValueOf(i.Result))
		return nil
	}

	if target.Type() == promiseValueType {
		target.Set(reflect.ValueOf(op.PromiseValue{NodeRef: i.Result.node.ID(), Slot: i.Result.slot}))
		return nil
	}

	return fmt.Errorf("unmarshal: cannot assign Invocation to %s (invocations resolve at execute time)", target.Type())
}

// Actions

// AttrNames implements starlark.HasAttrs by delegating to the wrapped Promise.
//
// Returns:
//   - []string: the attribute names exposed by the wrapped Promise (node slot parameters + node_id / slot / retry).
func (i *Invocation) AttrNames() []string {

	return i.Result.AttrNames()
}

// FillSlot fills a slot on the consumer node with a [op.PromiseValue] referencing this invocation's producer
// node.
//
// Delegates to Result.FillSlot, preserving the detachment contract established by phase-8 D5 — only the slot
// PromiseValue is written; no edge struct accumulates during dispatch. plan.run materializes the
// producer→consumer edge at graph construction time from the consumer slot's NodeRef.
//
// Parameters:
//   - consumer: the node receiving this invocation's output.
//   - slot: the slot name to fill on the consumer.
func (i *Invocation) FillSlot(consumer *op.Node, slot string) {

	i.Result.FillSlot(consumer, slot)
}

// endregion

// endregion
