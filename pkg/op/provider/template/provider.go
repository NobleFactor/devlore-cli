// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package template provides template expansion actions for the operation graph.
package template //nolint:revive // package name is domain-specific

import (
	"bytes"
	"fmt"
	"text/template"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// Provider provides template expansion actions.
//
// It takes input content and produces output content through Go template expansion — no filesystem access.
//
// +devlore:access=both
type Provider struct {
	op.ProviderBase
}

func NewProvider(ctx op.Context) *Provider {
	return &Provider{ProviderBase: op.NewProviderBase(ctx)}
}

// RenderText processes content as a Go text/template and returns the rendered string.
//
// Parameters:
//   - content: the template source text.
//   - data: key-value pairs available as template variables.
func (p *Provider) RenderText(content string, data map[string]any) (string, error) {
	result, err := p.render(content, data)
	if err != nil {
		return "", err
	}
	return result, nil
}

// RenderBytes processes content as a Go text/template and returns the rendered bytes.
//
// Parameters:
//   - content: the template source bytes.
//   - data: key-value pairs available as template variables.
func (p *Provider) RenderBytes(content []byte, data map[string]any) ([]byte, error) {
	result, err := p.render(string(content), data)
	if err != nil {
		return nil, err
	}
	return []byte(result), nil
}

// render is the shared implementation for RenderText and RenderBytes.
func (p *Provider) render(content string, data map[string]any) (string, error) {
	tmpl, err := template.New("render").Parse(content)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}

	return buf.String(), nil
}
