// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package template

import (
	"bytes"
	"fmt"
	"text/template"
)

// Provider provides template expansion actions. It takes input content
// and produces output content through Go template expansion — no filesystem access.
//
//devlore:plannable
type Provider struct{}

// Render processes content as a Go text/template. Returns the rendered bytes.
//
// Parameters:
//   - templateData: Key-value pairs available as template variables
//   - source: Source file path (available as .Source in the template)
//   - path: Target file path (available as .Target in the template)
//   - project: Project name (available as .Project in the template)
func (p *Provider) Render(templateData map[string]any, source, path, project string, content []byte) ([]byte, error) {
	tmpl, err := template.New("render").Parse(string(content))
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}

	data := make(map[string]any)
	for k, v := range templateData {
		data[k] = v
	}
	data["Source"] = source
	data["Target"] = path
	data["Project"] = project

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("execute template: %w", err)
	}

	return buf.Bytes(), nil
}
