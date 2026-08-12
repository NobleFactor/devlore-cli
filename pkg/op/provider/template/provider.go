// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

// Package template provides template expansion actions for the operation graph.
package template //nolint:revive // package name is domain-specific

import (
	"bytes"
	"fmt"
	"os"
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

// NewProvider creates a template provider bound to the given runtime environment.
func NewProvider(runtimeEnvironment *op.RuntimeEnvironment) *Provider {
	return &Provider{ProviderBase: op.NewProviderBase(runtimeEnvironment)}
}

// region EXPORTED METHODS

// region Behaviors

// RenderBytes processes content as a Go text/template and returns the rendered bytes.
//
// Parameters:
//   - `content`: the template source bytes.
//   - `data`: key-value pairs available as template variables.
//
// Returns:
//   - `[]byte`: the rendered output bytes.
//   - `error`: non-nil when the template fails to parse or execute.
func (p *Provider) RenderBytes(content []byte, data map[string]any) ([]byte, error) {
	result, err := p.render(string(content), data)
	if err != nil {
		return nil, err
	}
	return []byte(result), nil
}

// RenderText processes content as a Go text/template and returns the rendered string.
//
// Parameters:
//   - `content`: the template source text.
//   - `data`: key-value pairs available as template variables.
//
// Returns:
//   - `string`: the rendered output text.
//   - `error`: non-nil when the template fails to parse or execute.
func (p *Provider) RenderText(content string, data map[string]any) (string, error) {
	result, err := p.render(content, data)
	if err != nil {
		return "", err
	}
	return result, nil
}

// endregion

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// render is the shared implementation for RenderText and RenderBytes.
//
// Parameters:
//   - `content`: the template source text.
//   - `data`: key-value pairs available as template variables.
//
// Returns:
//   - `string`: the rendered output text.
//   - `error`: non-nil when the template fails to parse or execute.
func (p *Provider) render(content string, data map[string]any) (string, error) {
	tmpl, err := template.New("render").Funcs(renderFuncs).Parse(content)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}

	return buf.String(), nil
}

// endregion

// endregion

// region SUPPORTING TYPES

// renderFuncs is the render-time function map available to every template.
//
// `Env` resolves at dispatch time on the machine rendering the template — deliberately: graphs are
// transportable, and the same plan renders differently under different environments by declaration. Resolving
// environment values at plan time would instead embed them in the persisted graph document (the store keeps
// every plan), leaking environmental state — potentially secrets — into durable artifacts.
var renderFuncs = template.FuncMap{
	"Env": os.Getenv,
}

// endregion
