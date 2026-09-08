// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package op

import (
	"encoding/json"
	"fmt"
)

// VariableSourceKind identifies a variable-value source category. Numeric values ascend with precedence —
// higher beats lower. Callers can compare kinds directly to determine which source would win in a cascade.
type VariableSourceKind int

const (
	// VariableSourceKindUnknown is the zero value; should not appear on a resolved Variable.
	VariableSourceKindUnknown VariableSourceKind = iota

	// VariableSourceKindDefault — the parameter's declared default; lowest non-unknown precedence.
	VariableSourceKindDefault

	// VariableSourceKindConfig — starlark or YAML config files.
	VariableSourceKindConfig

	// VariableSourceKindEnv — process environment variables, derived prefix from ProgramName.
	VariableSourceKindEnv

	// VariableSourceKindFlag — command-line arguments parsed by the program's flag layer.
	VariableSourceKindFlag

	// VariableSourceKindOverride — programmatic force; highest precedence.
	VariableSourceKindOverride
)

// region EXPORTED METHODS

// region Behaviors

// String returns the canonical lowercase name of the [VariableSourceKind].
//
// Returns:
//   - `string`: the canonical name ("unknown", "default", "config", "env", "flag", or "override").
func (k VariableSourceKind) String() string {

	return [...]string{"unknown", "default", "config", "env", "flag", "override"}[k]
}

// endregion

// endregion

// VariableSource records where a resolved [Variable]'s value came from.
type VariableSource struct {

	// Kind identifies the source category. See [VariableSourceKind] for the enum.
	Kind VariableSourceKind `json:"kind" yaml:"kind"`

	// Name is the literal lookup key that matched. Examples: "WRIT_TARGET_ROOT" for an env hit;
	// "target_root" for a flag/config/default hit.
	Name string `json:"name" yaml:"name"`
}

// region EXPORTED METHODS

// region Behaviors

// String formats as "<kind>:<name>". [VariableSourceKindUnknown] renders as "unknown" alone since no name is
// meaningful in that case.
//
// Returns:
//   - `string`: the canonical "<kind>:<name>" form, or "unknown" for the zero value.
func (s VariableSource) String() string {

	if s.Kind == VariableSourceKindUnknown {
		return "unknown"
	}
	return s.Kind.String() + ":" + s.Name
}

// endregion

// endregion

// Variable pairs a resolved value with its name and source. Produced by [VariableResolver.Resolve] and
// consumed by the executor at slot-fill time for [VariableBinding] slots.
type Variable struct {

	// Name is the parameter name the variable satisfies. Matches the parameter declared via plan.variable(name).
	Name string `json:"name" yaml:"name"`

	// Field optionally projects one field out of a record-valued variable at resolve time — authored via
	// plan.item(field) or plan.variable(name, field=...). Empty means the whole value (phase-8 step 45).
	Field string `json:"field,omitempty" yaml:"field,omitempty"`

	// Value is the resolved value, already parsed to the parameter's declared Go type by the resolver.
	// Env-sourced strings are parsed; other sources supply already-typed values.
	Value any `json:"value" yaml:"value"`

	// Source records the source kind and lookup key that produced this value.
	Source VariableSource `json:"source" yaml:"source"`
}

// region EXPORTED METHODS

// region Behaviors

// MarshalJSON writes the variable with its value enveloped. A variable's value is an `any` position, and a paused
// run reads it back from the trace, so the document names its type (#712 phase 3, item 4).
//
// Returns:
//   - `[]byte`: the JSON encoding.
//   - `error`: a value the document has no name for, or any error from [json.Marshal].
func (v Variable) MarshalJSON() ([]byte, error) {

	value, err := encodeTypeWrapper(v.Value)
	if err != nil {
		return nil, fmt.Errorf("op.Variable.MarshalJSON: %s: %w", v.Name, err)
	}
	return json.Marshal(variableDocument{Name: v.Name, Field: v.Field, Value: value, Source: v.Source})
}

// MarshalYAML returns the variable's document shape for the YAML encoder, its value enveloped (#712 phase 3, item 4).
//
// Returns:
//   - `any`: the [variableDocument] value.
//   - `error`: a value the document has no name for.
func (v Variable) MarshalYAML() (any, error) {

	value, err := encodeTypeWrapper(v.Value)
	if err != nil {
		return nil, fmt.Errorf("op.Variable.MarshalYAML: %s: %w", v.Name, err)
	}
	return variableDocument{Name: v.Name, Field: v.Field, Value: value, Source: v.Source}, nil
}

// String formats as "<name> = <value> [<source>]". The bracketed source keeps the boundary between value
// and source unambiguous even when the value contains spaces.
//
// Returns:
//   - `string`: the canonical "<name> = <value> [<source>]" form.
func (v Variable) String() string {

	return fmt.Sprintf("%s = %v [%s]", v.Name, v.Value, v.Source)
}

// UnmarshalJSON reads a variable back, decoding its enveloped value with no catalog: a `$resource` becomes a
// [recordedResourceID], which graph dispatch resolves against the run catalog by id.
//
// Parameters:
//   - `data`: the JSON document.
//
// Returns:
//   - `error`: any error from [json.Unmarshal], or a value that does not carry its type.
func (v *Variable) UnmarshalJSON(data []byte) error {

	var document variableDocument
	if err := json.Unmarshal(data, &document); err != nil {
		return err
	}
	value, err := decodeTypeWrapper(document.Value, nil)
	if err != nil {
		return fmt.Errorf("op.Variable.UnmarshalJSON: %s: %w", document.Name, err)
	}
	*v = Variable{Name: document.Name, Field: document.Field, Value: value, Source: document.Source}
	return nil
}

// UnmarshalYAML reads a variable back from a YAML node, as [Variable.UnmarshalJSON] does from JSON.
//
// Parameters:
//   - `unmarshal`: the yaml.v3 node-decoding callback.
//
// Returns:
//   - `error`: any error from `unmarshal`, or a value that does not carry its type.
func (v *Variable) UnmarshalYAML(unmarshal func(any) error) error {

	var document variableDocument
	if err := unmarshal(&document); err != nil {
		return err
	}
	value, err := decodeTypeWrapper(document.Value, nil)
	if err != nil {
		return fmt.Errorf("op.Variable.UnmarshalYAML: %s: %w", document.Name, err)
	}
	*v = Variable{Name: document.Name, Field: document.Field, Value: value, Source: document.Source}
	return nil
}

// endregion

// endregion

// region SUPPORTING TYPES

// variableDocument is [Variable]'s document shape: the same fields, with `Value` carrying its type envelope (#712).
// It exists only to (de)serialize a Variable through the two codecs.
type variableDocument struct {
	Name   string         `json:"name"            yaml:"name"`
	Field  string         `json:"field,omitempty" yaml:"field,omitempty"`
	Value  any            `json:"value"           yaml:"value"`
	Source VariableSource `json:"source"          yaml:"source"`
}

// endregion
