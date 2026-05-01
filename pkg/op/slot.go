// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

// region Properties

// Properties provides ambient runtime context for EnvironmentValue resolution.
type Properties interface {
	Property(key string) (any, bool)
}

// endregion

// region Slot

// Slot binds a Method parameter to its value in a Node.
type Slot struct {
	Parameter Parameter
	Value     SlotValue
}

// Immediate returns the unwrapped Go value if this slot holds an ImmediateValue, or nil otherwise.
// Nil-safe: returns nil on a nil receiver.
func (s *Slot) Immediate() any {

	if s == nil {
		return nil
	}
	if iv, ok := s.Value.(ImmediateValue); ok {
		return iv.Value
	}
	return nil
}

// endregion

// region SlotValue — sealed interface

// SlotValue is the value bound to a Slot. Sealed: only ImmediateValue,
// PromiseValue, and EnvironmentValue implement it. Extensibility is
// prevented — the set is closed at three.
type SlotValue interface {
	isSlotValue()
	Resolve(props Properties, results map[string]any) any
}

// endregion

// region ImmediateValue

// ImmediateValue is a Go value known at plan time.
type ImmediateValue struct {
	Value any
}

func (ImmediateValue) isSlotValue() {}

func (iv ImmediateValue) Resolve(_ Properties, _ map[string]any) any {
	return iv.Value
}

// endregion

// region PromiseValue

// PromiseValue references the output of another executable unit (node or subgraph),
// resolved to a Go value at execution time via the scope-chain results.
type PromiseValue struct {
	NodeRef string
	Slot    string
}

func (PromiseValue) isSlotValue() {}

func (pv PromiseValue) Resolve(_ Properties, results map[string]any) any {
	if results == nil {
		return nil
	}
	return results[pv.NodeRef]
}

// endregion

// region EnvironmentValue

// EnvironmentValue binds a slot to a Properties property,
// resolved at execution time. Authored at plan time via a starlark
// surface such as plan.env("target_root").
type EnvironmentValue struct {
	Key string
}

func (EnvironmentValue) isSlotValue() {}

func (ev EnvironmentValue) Resolve(props Properties, _ map[string]any) any {
	if props == nil {
		return nil
	}
	v, _ := props.Property(ev.Key)
	return v
}
	return v
}

// endregion