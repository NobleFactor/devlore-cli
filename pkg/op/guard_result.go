// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import "fmt"

// GuardResult keys a conditional edge to an outcome of the guard.
//
// The guard is the truthiness evaluation of the From node's result: a decision node's out-edges each carry the
// [GuardResult] they are followed on, so the graph's topology — not any method body — routes execution (phase-8
// step 10, the choose decision tree). Serialized over [guardResultNames] ("none" / "truthy" / "falsy") in both
// document formats — [GuardResult.MarshalText] for JSON, [GuardResult.MarshalYAML] for gopkg.in/yaml.v3, which does
// not honor [encoding.TextMarshaler].
type GuardResult uint8

const (
	// GuardNone means the edge is unguarded: a plain ordering edge, always followed. As the zero value it is the
	// default, and with omitempty it never appears in serialized output, so existing traces are unchanged.
	GuardNone GuardResult = iota
	// GuardTruthy is followed when the From node's result is truthy in the Python sense: non-nil, non-false,
	// non-zero, non-empty. See isTruthy() for the exact rules.
	GuardTruthy
	// GuardFalsy is followed when the From node's result is falsy.
	GuardFalsy
)

// guardResultNames maps each [GuardResult] to its serialized name.
var guardResultNames = [...]string{
	GuardNone:   "none",
	GuardTruthy: "truthy",
	GuardFalsy:  "falsy",
}

// region EXPORTED METHODS

// region Behaviors

// MarshalText encodes this guard result as its serialized name.
//
// Satisfies [encoding.TextMarshaler], so JSON documents carry "truthy" / "falsy" rather than a bare integer.
//
// Returns:
//   - `[]byte`: the name from [guardResultNames].
//   - `error`: non-nil when the value is out of range.
func (g GuardResult) MarshalText() ([]byte, error) {

	if int(g) >= len(guardResultNames) {
		return nil, fmt.Errorf("op.GuardResult: unknown value %d", uint8(g))
	}

	return []byte(guardResultNames[g]), nil
}

// MarshalYAML encodes this guard result as its serialized name.
//
// gopkg.in/yaml.v3 does not honor [encoding.TextMarshaler], so YAML documents need this companion to carry
// "truthy" / "falsy" rather than a bare integer.
//
// Returns:
//   - `any`: the name from [guardResultNames], as a string.
//   - `error`: non-nil when the value is out of range.
func (g GuardResult) MarshalYAML() (any, error) {

	text, err := g.MarshalText()
	if err != nil {
		return nil, err
	}

	return string(text), nil
}

// String returns this guard result's serialized name.
//
// Returns:
//   - `string`: the name from [guardResultNames], or "GuardResult(<n>)" for an out-of-range value.
func (g GuardResult) String() string {

	if int(g) < len(guardResultNames) {
		return guardResultNames[g]
	}

	return fmt.Sprintf("GuardResult(%d)", uint8(g))
}

// UnmarshalText decodes a guard result from its serialized name.
//
// Satisfies [encoding.TextUnmarshaler] for JSON documents.
//
// Parameters:
//   - `text`: one of the [guardResultNames] entries.
//
// Returns:
//   - `error`: non-nil when `text` names no guard result.
func (g *GuardResult) UnmarshalText(text []byte) error {

	name := string(text)

	for value, candidate := range guardResultNames {
		if candidate == name {
			*g = GuardResult(value)
			return nil
		}
	}

	return fmt.Errorf("op.GuardResult: unknown name %q", name)
}

// UnmarshalYAML decodes a guard result from its serialized name.
//
// gopkg.in/yaml.v3 does not honor [encoding.TextUnmarshaler], so YAML documents need this companion.
//
// Parameters:
//   - `unmarshal`: the YAML node decoder supplied by the yaml package.
//
// Returns:
//   - `error`: non-nil when the node is not a string or names no guard result.
func (g *GuardResult) UnmarshalYAML(unmarshal func(any) error) error {

	var name string
	if err := unmarshal(&name); err != nil {
		return err
	}

	return g.UnmarshalText([]byte(name))
}

// endregion

// endregion
