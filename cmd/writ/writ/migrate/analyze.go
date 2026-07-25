// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package migrate

// ScriptAnalysis captures information extracted from a lifecycle script.
// In the LLM-first approach, this is populated by the LLM based on
// reading script contents directly.
type ScriptAnalysis struct {
	RelPath       string            `json:"rel_path" yaml:"rel_path"`
	Name          string            `json:"name" yaml:"name"`
	Phase         string            `json:"phase" yaml:"phase"`
	PlatformGuard string            `json:"platform_guard,omitempty" yaml:"platform_guard,omitempty"`
	LineCount     int               `json:"line_count" yaml:"line_count"`
	Resolved      []DetectedInstall `json:"resolved,omitempty" yaml:"resolved,omitempty"`
	Unresolved    []DetectedInstall `json:"unresolved,omitempty" yaml:"unresolved,omitempty"`
	Observations  []string          `json:"observations,omitempty" yaml:"observations,omitempty"`
}

// DetectedInstall represents a package installation detected in a script.
// The LLM may populate this when analyzing scripts that contain
// package manager commands.
type DetectedInstall struct {
	Line        int    `json:"line" yaml:"line"`
	Manager     string `json:"manager" yaml:"manager"`
	Name        string `json:"name" yaml:"name"`
	Command     string `json:"command" yaml:"command"`
	LorePackage string `json:"lore_package,omitempty" yaml:"lore_package,omitempty"`
}

// IsResolved returns true if this install maps to a known lore package.
func (d DetectedInstall) IsResolved() bool {
	return d.LorePackage != ""
}
