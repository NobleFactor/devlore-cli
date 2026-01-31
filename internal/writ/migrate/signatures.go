// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package migrate

// SignatureIndex maps manager → package_name → lore_package.
// This is the inverted index built from all lifecycle.yaml signatures.
// Load via Registry.SignatureIndex() from the lorepackage client.
type SignatureIndex map[string]map[string]string

// Resolve looks up a package name for a given manager.
// Returns the lore package name if found, empty string otherwise.
func (idx SignatureIndex) Resolve(manager, name string) string {
	if idx == nil {
		return ""
	}
	managerMap, ok := idx[manager]
	if !ok {
		return ""
	}
	return managerMap[name]
}

// HasPackages returns true if the index has any entries.
func (idx SignatureIndex) HasPackages() bool {
	for _, m := range idx {
		if len(m) > 0 {
			return true
		}
	}
	return false
}

// DetectedInstall represents a package installation detected in a script.
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
