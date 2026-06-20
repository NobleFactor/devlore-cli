// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package json provides JSON encoding and decoding for the operation graph.
package json

import (
	"encoding/json"
	"fmt"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// Provider provides JSON encoding and decoding operations.
//
// +devlore:access=both
type Provider struct {
	op.ProviderBase
}

// NewProvider creates a JSON provider bound to the given context.
func NewProvider(runtimeEnvironment *op.RuntimeEnvironment) *Provider {
	return &Provider{ProviderBase: op.NewProviderBase(runtimeEnvironment)}
}

// region EXPORTED METHODS

// region Behaviors

// Decode parses a JSON string into a Go value.
//
// Parameters:
//   - `data`: the JSON text to parse.
//
// Returns:
//   - `any`: the decoded Go value (maps, slices, and scalars per encoding/json).
//   - `error`: non-nil when `data` is not valid JSON.
func (p *Provider) Decode(data string) (any, error) {

	var result any
	if err := json.Unmarshal([]byte(data), &result); err != nil {
		return nil, fmt.Errorf("json decode: %w", err)
	}
	return result, nil
}

// Encode marshals a Go value to a compact JSON string.
//
// Parameters:
//   - `value`: the Go value to marshal.
//
// Returns:
//   - `string`: the compact JSON encoding of `value`.
//   - `error`: non-nil when `value` cannot be marshaled to JSON.
func (p *Provider) Encode(value any) (string, error) {

	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("json encode: %w", err)
	}
	return string(data), nil
}

// EncodeIndent marshals a Go value to an indented JSON string.
//
// Parameters:
//   - `value`: the Go value to marshal.
//   - `indent`: the per-level indentation string (e.g., "  " or "\t").
//
// Returns:
//   - `string`: the indented JSON encoding of `value`.
//   - `error`: non-nil when `value` cannot be marshaled to JSON.
func (p *Provider) EncodeIndent(value any, indent string) (string, error) {

	data, err := json.MarshalIndent(value, "", indent)
	if err != nil {
		return "", fmt.Errorf("json encode_indent: %w", err)
	}
	return string(data), nil
}

// Parse decodes a JSON string into a [Resource] that holds the parsed Go value.
//
// Unlike [Decode], which returns a bare Go value (marshaled to a Starlark dict), Parse returns a Resource
// whose internal representation can be validated against a JSON Schema or re-encoded without Starlark↔Go
// round-trips.
//
// Parse is content-keyed — two calls with the same input produce the same URI and share a single canonical
// catalog entry. The first caller's `Unit.ID()` stamps producerID; subsequent same-content callers get the
// existing entry unchanged. [NewResource] handles the parse, hash, and catalog interning in one step.
//
// Parameters:
//   - `activationRecord`: the per-dispatch activation; its `Unit` stamps the produced Resource's producerID.
//   - `data`: the JSON text to parse.
//
// Returns:
//   - `*Resource`: the canonical catalog entry holding the parsed value.
//   - `error`: non-nil when `data` is not valid JSON or catalog interning fails.
func (p *Provider) Parse(activationRecord *op.ActivationRecord, data string) (*Resource, error) {

	return NewResource(p.RuntimeEnvironment(), activationRecord.Unit, []byte(data))
}

// endregion

// endregion
