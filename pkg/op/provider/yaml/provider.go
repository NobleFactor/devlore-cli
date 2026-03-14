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

func NewProvider(ctx op.Context) *Provider {
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
