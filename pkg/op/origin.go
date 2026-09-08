// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package op

import (
	"encoding/json"
	"fmt"
)

// Origin is tool-stamped graph metadata: the contract the framework reads and round-trips.
//
// The framework consults exactly one thing — [Origin.Scope] — to derive the graph filename. [Origin.Tool] is the
// trace-identity, and [Origin.Annotations] is an open bag of tool-specific metadata the framework stores and
// round-trips but never inspects. Produced at plan-time, immutable thereafter (matches the graph seal).
//
// [OriginBase] is the single concrete carrier: tools build one via [NewOriginBase], and read graph.Origin() back as
// this interface, wrapping it in their own typed view (e.g. lore.Origin / writ.Origin) projected over the annotations.
type Origin interface {

	// Tool identifies which program produced the graph ("lore", "writ") — the trace-identity and filename key.
	Tool() string

	// Scope identifies the planning scope (writ: "system"/"home"; lore: package cache scope).
	//
	// The only field the framework reads; it derives the graph filename from it.
	Scope() string

	// Annotations is the open bag of tool-specific metadata the framework round-trips but never inspects.
	Annotations() AnnotationMap
}

// OriginBase is the single concrete [Origin] carrier and the consumer-facing embedded base.
//
// Its fields are unexported and set once at construction (via [NewOriginBase]); there are no mutators, matching the
// graph seal. It (un)marshals to a flat {tool, scope, annotations} document through the unexported [originData] DTO.
// Tools do not extend OriginBase with typed fields — they project typed read-only views over [OriginBase.Annotations]
// — so every concrete Origin serializes to the same shape and decodes back into this one type.
type OriginBase struct {
	annotations AnnotationMap
	scope       string
	tool        string
}

// NewOriginBase returns an [OriginBase] stamped with the given tool, scope, and annotations.
//
// Parameters:
//   - `tool`: the producing program's name ("lore", "writ"); becomes the trace-identity.
//   - `scope`: the planning scope; the framework derives the graph filename from it.
//   - `annotations`: the tool-specific metadata bag; tools project typed views over it on the read side.
//
// Returns:
//   - `OriginBase`: the stamped origin.
func NewOriginBase(tool, scope string, annotations AnnotationMap) OriginBase {
	return OriginBase{tool: tool, scope: scope, annotations: annotations}
}

// region State Management

// Annotations returns the tool-specific metadata bag.
//
// Returns:
//   - `AnnotationMap`: the annotation map; the zero value when none were stamped.
func (o OriginBase) Annotations() AnnotationMap { return o.annotations }

// Scope returns the planning scope the framework uses to derive the graph filename.
//
// Returns:
//   - `string`: the scope; "" when unset.
func (o OriginBase) Scope() string { return o.scope }

// Tool returns the producing program's name.
//
// Returns:
//   - `string`: the tool name; "" when unset.
func (o OriginBase) Tool() string { return o.tool }

// endregion

// region Behaviors

// MarshalJSON encodes the origin to its flat {tool, scope, annotations} JSON document via [originData], each
// annotation value enveloped (#712 phase 3, item 3).
//
// Returns:
//   - `[]byte`: the JSON encoding.
//   - `error`: an annotation value the document has no name for, or any error from [json.Marshal].
func (o OriginBase) MarshalJSON() ([]byte, error) {
	annotations, err := envelopeAnnotations(o.annotations.values)
	if err != nil {
		return nil, fmt.Errorf("op.OriginBase.MarshalJSON: %w", err)
	}
	return json.Marshal(originData{Tool: o.tool, Scope: o.scope, Annotations: annotations})
}

// MarshalYAML returns the origin's flat [originData] shape for the YAML encoder, each annotation value enveloped
// (#712 phase 3, item 3).
//
// Returns:
//   - `any`: the [originData] value.
//   - `error`: an annotation value the document has no name for.
func (o OriginBase) MarshalYAML() (any, error) {
	annotations, err := envelopeAnnotations(o.annotations.values)
	if err != nil {
		return nil, fmt.Errorf("op.OriginBase.MarshalYAML: %w", err)
	}
	return originData{Tool: o.tool, Scope: o.scope, Annotations: annotations}, nil
}

// UnmarshalJSON decodes a flat {tool, scope, annotations} JSON document into the receiver via [originData].
//
// Parameters:
//   - `data`: the JSON document.
//
// Returns:
//   - `error`: any error from [json.Unmarshal], or an annotation value that does not carry its type.
func (o *OriginBase) UnmarshalJSON(data []byte) error {

	var d originData
	if err := json.Unmarshal(data, &d); err != nil {
		return err
	}

	annotations, err := unwrapAnnotations(d.Annotations)
	if err != nil {
		return fmt.Errorf("op.OriginBase.UnmarshalJSON: %w", err)
	}
	o.tool, o.scope, o.annotations = d.Tool, d.Scope, NewAnnotationMap(annotations)
	return nil
}

// UnmarshalYAML decodes a flat {tool, scope, annotations} YAML node into the receiver via [originData].
//
// Parameters:
//   - `unmarshal`: the yaml.v3 node-decoding callback.
//
// Returns:
//   - `error`: any error from `unmarshal`, or an annotation value that does not carry its type.
func (o *OriginBase) UnmarshalYAML(unmarshal func(any) error) error {

	var d originData
	if err := unmarshal(&d); err != nil {
		return err
	}

	annotations, err := unwrapAnnotations(d.Annotations)
	if err != nil {
		return fmt.Errorf("op.OriginBase.UnmarshalYAML: %w", err)
	}
	o.tool, o.scope, o.annotations = d.Tool, d.Scope, NewAnnotationMap(annotations)
	return nil
}

// endregion

// region SUPPORTING TYPES

// originData is the unexported document DTO for [OriginBase] — the flat {tool, scope, annotations} shape with
// exported, tagged fields. It exists only to (de)serialize OriginBase (JSON + YAML; no text form — an Origin is a
// composite, never a scalar). Annotations holds one type envelope per value (#712): [OriginBase] envelopes its
// [AnnotationMap]'s values into this map for marshal and unwraps them back through [NewAnnotationMap] on decode.
type originData struct {
	Tool        string         `json:"tool,omitempty"        yaml:"tool,omitempty"`
	Scope       string         `json:"scope,omitempty"       yaml:"scope,omitempty"`
	Annotations map[string]any `json:"annotations,omitempty" yaml:"annotations,omitempty"`
}

// endregion
