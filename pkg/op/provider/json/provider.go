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
