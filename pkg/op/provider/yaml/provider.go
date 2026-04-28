// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package yaml provides YAML encoding and decoding for the operation graph.
package yaml

import (
	"fmt"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"gopkg.in/yaml.v3"
)

// Provider provides YAML encoding and decoding operations.
// +devlore:access=both
type Provider struct {
	op.ProviderBase
}

// NewProvider creates a YAML provider bound to the given context.
func NewProvider(ctx *op.ExecutionContext) *Provider {
	return &Provider{ProviderBase: op.NewProviderBase(ctx)}
}

// Encode marshals a Go value to a YAML string.
func (p *Provider) Encode(value any) (result string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("yaml encode: %v", r)
		}
	}()
	data, err := yaml.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("yaml encode: %w", err)
	}
	return string(data), nil
}

// Decode parses a YAML string into a Go value.
func (p *Provider) Decode(data string) (any, error) {
	var result any
	if err := yaml.Unmarshal([]byte(data), &result); err != nil {
		return nil, fmt.Errorf("yaml decode: %w", err)
	}
	return result, nil
}

// Parse decodes a YAML string into a [Resource] that holds the parsed Go value.
//
// Unlike [Decode], which returns a bare Go value (marshaled to a Starlark dict), Parse returns a Resource whose
// internal representation can be validated against a JSON Schema or re-encoded without Starlark↔Go round-trips.
func (p *Provider) Parse(data string) (*Resource, error) {

	raw := []byte(data)

	var parsed any

	if err := yaml.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("yaml parse: %w", err)
	}

	ctx := p.ExecutionContext()

	candidate, err := NewResource(ctx, raw)

	if err != nil {
		return nil, err
	}

	candidate.parsed = parsed

	// Parse is content-keyed — two calls with the same input produce the same URI. Route through the catalog so
	// they share a single canonical *Resource (and thus a single parsed value) per ExecutionContext.
	got, err := ctx.Catalog.GetOrCreate(candidate.URI(), func() (op.Resource, error) {
		return candidate, nil
	})

	if err != nil {
		return nil, err
	}

	r, ok := got.(*Resource)
	if !ok {
		return nil, fmt.Errorf("yaml.Parse: catalog entry for %q is %T, want *yaml.Resource", candidate.URI(), got)
	}

	return r, nil
}
