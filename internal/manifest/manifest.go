// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package manifest provides loading and validation for packages-manifest files.
package manifest

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/NobleFactor/devlore-cli/internal/document"
	"github.com/NobleFactor/devlore-cli/schema"
)

// PackagesManifest represents the parsed packages-manifest.{yaml,json} file.
type PackagesManifest struct {
	// Packages is the list of package entries.
	Packages []PackageEntry `json:"packages" yaml:"packages"`
}

// PackageEntry represents a single package in the manifest.
type PackageEntry struct {
	Name string   `json:"name" yaml:"name"`                     // package name (required)
	With []string `json:"with,omitempty" yaml:"with,omitempty"` // features to enable
}

// region EXPORTED METHODS

// region Behaviors

// Actions

// PackageNames returns the list of package names from the manifest.
//
// Returns:
//   - []string: names extracted from each package entry
func (m *PackagesManifest) PackageNames() []string {

	names := make([]string, len(m.Packages))
	for i, pkg := range m.Packages {
		names[i] = pkg.Name
	}
	return names
}

// String returns a human-readable representation of the package entry.
//
// Returns:
//   - string: name alone or name with --with flags
func (e *PackageEntry) String() string {

	if len(e.With) == 0 {
		return e.Name
	}
	return fmt.Sprintf("%s --with %s", e.Name, strings.Join(e.With, " --with "))
}

// endregion

// endregion

// region EXPORTED FUNCTIONS

// region Behaviors

// Fallible actions

// Load reads and parses a packages-manifest file from the given path. Supports both YAML and JSON formats based on
// file extension.
//
// Parameters:
//   - path: filesystem path to the manifest file
//
// Returns:
//   - *PackagesManifest: parsed manifest
//   - error: read or parse error
func Load(path string) (*PackagesManifest, error) {

	return document.ReadFile[PackagesManifest](path)
}

// Validate validates a packages-manifest file against structural rules. Returns nil if valid, or an error describing
// the validation failure.
//
// Parameters:
//   - path: filesystem path to the manifest file
//
// Returns:
//   - error: validation error or nil
func Validate(path string) error {

	doc, err := document.ReadFile[map[string]interface{}](path)
	if err != nil {
		return err
	}

	return validateDoc(*doc)
}

// Actions

// IsManifestFile returns true if the filename is a packages-manifest file.
//
// Parameters:
//   - filename: file path or name to check
//
// Returns:
//   - bool: true for packages-manifest.yaml or packages-manifest.json
func IsManifestFile(filename string) bool {

	base := filepath.Base(filename)
	return base == "packages-manifest.yaml" ||
		base == "packages-manifest.json"
}

// endregion

// endregion

// region HELPER FUNCTIONS

// region Behaviors

// Fallible actions

// validateDoc validates a parsed manifest document against structural rules.
//
// Parameters:
//   - doc: unmarshaled manifest as a generic map
//
// Returns:
//   - error: validation error or nil
func validateDoc(doc map[string]interface{}) error {

	// Parse the schema
	var schemaDoc map[string]interface{}
	if err := json.Unmarshal(schema.PackagesManifestSchema, &schemaDoc); err != nil {
		return fmt.Errorf("parse schema: %w", err)
	}

	// Check required fields
	if _, exists := doc["packages"]; !exists {
		return fmt.Errorf("missing required field: packages")
	}

	// Check packages is an array
	packages, ok := doc["packages"].([]interface{})
	if !ok {
		return fmt.Errorf("'packages' must be an array")
	}

	// Validate each package entry
	for i, pkg := range packages {
		if err := validatePackageEntry(pkg, i); err != nil {
			return err
		}
	}

	// Check for unknown top-level fields
	for key := range doc {
		if key != "packages" {
			return fmt.Errorf("unknown field: %s", key)
		}
	}

	return nil
}

// validatePackageEntry validates a single package entry object.
//
// Parameters:
//   - pkg: raw unmarshaled entry value
//   - index: position in the packages array (for error messages)
//
// Returns:
//   - error: validation error or nil
func validatePackageEntry(pkg interface{}, index int) error {

	entry, ok := pkg.(map[string]interface{})
	if !ok {
		return fmt.Errorf("packages[%d]: must be an object with 'name' field", index)
	}

	// Check required 'name' field
	nameRaw, exists := entry["name"]
	if !exists {
		return fmt.Errorf("packages[%d]: missing required field 'name'", index)
	}

	name, ok := nameRaw.(string)
	if !ok || name == "" {
		return fmt.Errorf("packages[%d]: 'name' must be a non-empty string", index)
	}

	// Validate 'with' if present
	if withRaw, exists := entry["with"]; exists {
		withArray, ok := withRaw.([]interface{})
		if !ok {
			return fmt.Errorf("packages[%d] (%s): 'with' must be an array", index, name)
		}

		for j, w := range withArray {
			if _, ok := w.(string); !ok {
				return fmt.Errorf("packages[%d] (%s): with[%d] must be a string", index, name, j)
			}
		}
	}

	// Check for unknown fields
	for key := range entry {
		if key != "name" && key != "with" {
			return fmt.Errorf("packages[%d] (%s): unknown field %q", index, name, key)
		}
	}

	return nil
}

// endregion

// endregion
