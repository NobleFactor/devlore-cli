// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"fmt"
	"reflect"

	"go.starlark.net/starlark"
)

// Promise represents the promise of an output from a producing executable unit (Node or Subgraph).
//
// When passed to a plan function's slot, the consumer's slot is filled with a [PromiseValue] that references the
// producer by ID.
//
// Promise is detached. It holds no graph reference. The producer→consumer edge is implicit in the consumer slot's
// PromiseValue and is materialized by plan.assemble when it builds the [Graph] from the reachable invocation set.
type Promise struct {

	// unit is the executable unit that produces this output (Node or Subgraph)
	unit ExecutableUnit

	// slot identifies which output of the unit this represents (empty = default)
	slot string
}

// NewPromise creates a new Promise representing an executable unit's output.
//
// Parameters:
//   - `unit`: the producing executable unit ([*Node] or [*Subgraph]).
//   - `slot`: which output slot this represents (empty for default).
//
// Returns:
//   - `*Promise`: the new promise handle.
func NewPromise(unit ExecutableUnit, slot string) *Promise {

	return &Promise{
		unit: unit,
		slot: slot,
	}
}

// region EXPORTED METHODS

// region State management

// Path returns the producer's "path" slot value, or the empty string when absent.
//
// Resolves only when the producer is a [*Node] whose "path" slot carries an immediate string; Subgraph producers (no
// slot map) return the empty string.
//
// Returns:
//   - `string`: the path slot value, or empty string if not present or not a string.
func (p *Promise) Path() string {

	node, ok := p.unit.(*Node)
	if !ok {
		return ""
	}
	value, ok := node.Slots()["path"]
	if !ok {
		return ""
	}
	path, _ := ImmediateOf(value).(string) //nolint:errcheck // zero value (empty) is acceptable
	return path
}

// Slot returns which output slot this represents.
//
// Returns:
//   - `string`: the slot identifier.
func (p *Promise) Slot() string {

	return p.slot
}

// Unit returns the executable unit that produces this output.
//
// Returns:
//   - `ExecutableUnit`: the producing unit (Node or Subgraph).
func (p *Promise) Unit() ExecutableUnit {

	return p.unit
}

// endregion

// region Behaviors

// Attr implements starlark.HasAttrs.
//
// Slot-shaped attribute lookups (anything beyond `node_id` / `slot` / `retry`) only resolve when the producer is a
// [*Node]; Subgraph producers have no slot map yet (Layer B) and return NoSuchAttr for per-slot reads.
//
// Parameters:
//   - `name`: the attribute name to look up.
//
// Returns:
//   - `starlark.Value`: the attribute value.
//   - `error`: non-nil if the attribute does not exist.
func (p *Promise) Attr(name string) (starlark.Value, error) {

	switch name {
	case "node_id":
		return starlark.String(p.unit.ID()), nil
	case "slot":
		return starlark.String(p.slot), nil
	case "retry":
		return starlark.NewBuiltin("output.retry", p.retryBuiltin), nil
	default:
		// Get the value from the node's slots and convert to Starlark.

		node, ok := p.unit.(*Node)
		if !ok {
			return nil, starlark.NoSuchAttrError(fmt.Sprintf("Promise has no attribute %q", name))
		}

		value, ok := node.Slots()[name]
		if !ok {
			return nil, starlark.NoSuchAttrError(fmt.Sprintf("Promise has no attribute %q", name))
		}

		slotVal := ImmediateOf(value)

		if slotVal == nil {
			return nil, fmt.Errorf("slot %q: not an immediate value", name)
		}

		switch v := slotVal.(type) {
		case string:
			return starlark.String(v), nil
		case int:
			return starlark.MakeInt(v), nil
		case int64:
			return starlark.MakeInt64(v), nil
		case bool:
			return starlark.Bool(v), nil
		case float64:
			return starlark.Float(v), nil
		case []byte:
			return starlark.Bytes(v), nil
		case starlark.Value:
			return v, nil
		default:
			return nil, fmt.Errorf("slot %q: cannot represent %T as a starlark value", name, slotVal)
		}
	}
}

// AttrNames implements starlark.HasAttrs.
//
// Returns:
//   - `[]string`: all available attribute names. For Node producers the per-slot names are appended; for Subgraph
//     producers only the fixed `node_id` / `slot` / `retry` are returned.
func (p *Promise) AttrNames() []string {

	names := []string{"node_id", "retry", "slot"}
	if node, ok := p.unit.(*Node); ok {
		for name := range node.Slots() {
			names = append(names, name)
		}
	}
	return names
}

// Freeze implements starlark.Value.
func (p *Promise) Freeze() {}

// Hash implements starlark.Value.
//
// Returns:
//   - `uint32`: unused, always 0.
//   - `error`: always non-nil (Promise is unhashable).
func (p *Promise) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: Promise") }

// Project renders this Promise for the given target type.
//
// For a [*Promise] or interface target the Promise itself is returned; for a [PromiseValue] target the slot-ref shape
// is returned; for any other target it errors — promises are not directly resolvable to Go scalar types at plan time.
//
// Parameters:
//   - `target`: the requested Go [reflect.Type].
//
// Returns:
//   - `any`: the Promise, or its [PromiseValue] slot-ref shape, projected to `target`.
//   - `error`: non-nil when `target` is a concrete non-promise type.
func (p *Promise) Project(target reflect.Type) (any, error) {

	promiseType := reflect.TypeFor[*Promise]()
	promiseValueType := reflect.TypeFor[PromiseValue]()

	if target.Kind() == reflect.Interface {
		return p, nil
	}
	if target == promiseType {
		return p, nil
	}
	if target == promiseValueType {
		return PromiseValue{UnitRef: p.unit.ID(), Slot: p.slot}, nil
	}

	return nil, fmt.Errorf("cannot project Promise to %s (promises resolve at execute time)", target)
}

// SlotValue returns the [PromiseValue] that binds a consumer slot to this promise's producer.
//
// The returned value references the producer by ID; the producer→consumer edge is implicit in that reference and
// materialized by plan.assemble when it walks the reachable invocation set and builds the [Graph] (phase-8 D5). The
// caller places the result into a [NodeSpec] / [SubgraphSpec] slot via WithSlot — no node is mutated here.
//
// Returns:
//   - `SlotValue`: a [PromiseValue] referencing the producer by ID.
func (p *Promise) SlotValue() SlotValue {

	return PromiseValue{UnitRef: p.unit.ID(), Slot: p.slot}
}

// String implements starlark.Value.
//
// Returns:
//   - `string`: human-readable representation.
func (p *Promise) String() string { return fmt.Sprintf("Promise(%s)", p.unit.ID()) }

// Truth implements starlark.Value.
//
// Returns:
//   - `starlark.Bool`: always true.
func (p *Promise) Truth() starlark.Bool { return true }

// Type implements starlark.Value.
//
// Returns:
//   - `string`: the type name "Promise".
func (p *Promise) Type() string { return "Promise" }

// endregion

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// retryBuiltin sets the retry policy on this output's node.
//
// Usage: node = plan.appnet.download(...); node.retry(max_attempts=5, backoff="linear")
//
// Parameters:
//   - `thread`: the Starlark thread (unused).
//   - `b`: the builtin (unused).
//   - `args`: positional arguments.
//   - `kwargs`: keyword arguments (max_attempts, backoff?, initial_delay?, max_delay?).
//
// Returns:
//   - `starlark.Value`: this Promise (for chaining).
//   - `error`: non-nil if arguments are invalid.
func (p *Promise) retryBuiltin(
	_ *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {

	var maxAttempts int
	var backoff, initialDelay, maxDelay string

	if err := starlark.UnpackArgs("retry", args, kwargs,
		"max_attempts", &maxAttempts,
		"backoff?", &backoff,
		"initial_delay?", &initialDelay,
		"max_delay?", &maxDelay,
	); err != nil {
		return nil, err
	}

	if maxAttempts < 0 {
		return nil, fmt.Errorf("retry(): max_attempts must be non-negative, got %d", maxAttempts)
	}

	policy := &RetryPolicy{
		MaxAttempts: maxAttempts,
	}

	if backoff != "" {
		switch backoff {
		case "none":
			policy.Backoff = BackoffNone
		case "linear":
			policy.Backoff = BackoffLinear
		case "exponential":
			policy.Backoff = BackoffExponential
		default:
			return nil, fmt.Errorf("retry(): unknown backoff %q (use none, linear, or exponential)", backoff)
		}
	}

	if initialDelay != "" {
		policy.InitialDelay = initialDelay
	}
	if maxDelay != "" {
		policy.MaxDelay = maxDelay
	}

	p.unit.setRetryPolicy(policy)
	return p, nil
}

// endregion

// endregion
