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
// +devlore:access=both
type Provider struct {
	op.ProviderBase
}

// NewProvider creates a JSON provider bound to the given context.
func NewProvider(ctx *op.RuntimeEnvironment) *Provider {
	return &Provider{ProviderBase: op.NewProviderBase(ctx)}
}

// Encode marshals a Go value to a compact JSON string.
func (p *Provider) Encode(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("json encode: %w", err)
	}
	return string(data), nil
}

// EncodeIndent marshals a Go value to an indented JSON string.
func (p *Provider) EncodeIndent(value any, indent string) (string, error) {
	data, err := json.MarshalIndent(value, "", indent)
	if err != nil {
		return "", fmt.Errorf("json encode_indent: %w", err)
	}
	return string(data), nil
}

// Decode parses a JSON string into a Go value.
func (p *Provider) Decode(data string) (any, error) {
	var result any
	if err := json.Unmarshal([]byte(data), &result); err != nil {
		return nil, fmt.Errorf("json decode: %w", err)
	}
	return result, nil
}

// Parse decodes a JSON string into a [Resource] that holds the parsed Go value.
//
// Unlike [Decode], which returns a bare Go value (marshaled to a Starlark dict), Parse returns a Resource whose
// internal representation can be validated against a JSON Schema or re-encoded without Starlark↔Go round-trips.
func (p *Provider) Parse(data string) (*Resource, error) {

	raw := []byte(data)

	var parsed any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("json parse: %w", err)
	}

	ctx := p.RuntimeEnvironment()
	candidate, err := NewResource(ctx, raw)
	if err != nil {
		return nil, err
	}
	candidate.parsed = parsed

	// Parse is content-keyed — two calls with the same input produce the same URI. Route through the catalog so
	// they share a single canonical *Resource (and thus a single parsed value) per RuntimeEnvironment.
	got, err := ctx.Catalog.Discover(candidate.URI(), func() (op.Resource, error) {
		return candidate, nil
	})
	if err != nil {
		return nil, err
	}

	r, ok := got.(*Resource)
	if !ok {
		return nil, fmt.Errorf("json.Parse: catalog entry for %q is %T, want *json.Resource", candidate.URI(), got)
	}
	return r, nil
}
