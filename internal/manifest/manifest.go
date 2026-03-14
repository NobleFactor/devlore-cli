// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package manifest provides loading and validation for packages-manifest files.
package manifest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/NobleFactor/devlore-cli/schema"
)

// PackagesManifest represents the parsed packages-manifest.{yaml,json} file.
type PackagesManifest struct {
	// Packages is the list of package entries.
	Packages []PackageEntry `json:"packages" yaml:"packages"`
}

// PackageEntry represents a single package in the manifest.
// It can be either a simple string (package name) or an object with options.
type PackageEntry struct {
	// Name is the package name.
	Name string

	// With is a list of features to enable.
	With []string
}

// PackageOptions holds the options for a package (used during parsing).
type PackageOptions struct {
	With []string `json:"with" yaml:"with"`
}

// Load reads and parses a packages-manifest file from the given path.
// Supports both YAML and JSON formats based on file extension.
func Load(path string) (*PackagesManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	return Parse(data, path)
}

// Parse parses packages-manifest content from bytes.
// The path is used to determine the format (yaml/json).
func Parse(data []byte, path string) (*PackagesManifest, error) {
	ext := strings.ToLower(filepath.Ext(path))

	// Parse into intermediate structure for flexible handling
	var raw struct {
		Packages []interface{} `json:"packages" yaml:"packages"`
	}

	switch ext {
	case ".json":
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, fmt.Errorf("invalid JSON: %w", err)
		}
	default: // .yaml, .yml
		if err := yaml.Unmarshal(data, &raw); err != nil {
			return nil, fmt.Errorf("invalid YAML: %w", err)
		}
	}

	manifest := &PackagesManifest{}

	for i, item := range raw.Packages {
		entry, err := parsePackageEntry(item)
		if err != nil {
			return nil, fmt.Errorf("packages[%d]: %w", i, err)
		}
		manifest.Packages = append(manifest.Packages, entry)
	}

	return manifest, nil
}

// parsePackageEntry parses a single package entry from the raw interface value.
// Supports both string format ("gh") and object format ({"neovim": {"with": [...]}}).
func parsePackageEntry(item interface{}) (PackageEntry, error) { //nolint:gocognit
	switch v := item.(type) {
	case string:
		// Simple string: "gh"
		if v == "" {
			return PackageEntry{}, fmt.Errorf("empty package name")
		}
		return PackageEntry{Name: v}, nil

	case map[string]interface{}:
		// Object format: {"neovim": {"with": [...]}}
		if len(v) != 1 {
			return PackageEntry{}, fmt.Errorf("expected single-key map, got %d keys", len(v))
		}

		for name, opts := range v {
			if name == "" {
				return PackageEntry{}, fmt.Errorf("empty package name")
			}

			entry := PackageEntry{Name: name}

			if opts == nil {
				return entry, nil
			}

			optsMap, ok := opts.(map[string]interface{})
			if !ok {
				return PackageEntry{}, fmt.Errorf("invalid options for %q: expected object", name)
			}

			// Parse "with" array
			if withRaw, exists := optsMap["with"]; exists {
				withArray, ok := withRaw.([]interface{})
				if !ok {
					return PackageEntry{}, fmt.Errorf("invalid 'with' for %q: expected array", name)
				}

				for _, w := range withArray {
					feature, ok := w.(string)
					if !ok {
						return PackageEntry{}, fmt.Errorf("invalid feature in 'with' for %q: expected string", name)
					}
					entry.With = append(entry.With, feature)
				}
			}

			// Check for unknown keys
			for key := range optsMap {
				if key != "with" {
					return PackageEntry{}, fmt.Errorf("unknown option %q for package %q", key, name)
				}
			}

			return entry, nil
		}
	}

	return PackageEntry{}, fmt.Errorf("invalid package entry: expected string or object")
}

// Validate validates a packages-manifest file against the embedded JSON schema.
// Returns nil if valid, or an error describing the validation failure.
func Validate(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read manifest: %w", err)
	}

	return ValidateBytes(data, path)
}

// ValidateBytes validates packages-manifest content against the embedded JSON schema.
func ValidateBytes(data []byte, path string) error {
	// Parse the schema
	var schemaDoc map[string]interface{}
	if err := json.Unmarshal(schema.PackagesManifestSchema, &schemaDoc); err != nil {
		return fmt.Errorf("parse schema: %w", err)
	}

	// Parse the manifest
	ext := strings.ToLower(filepath.Ext(path))
	var doc map[string]interface{}

	switch ext {
	case ".json":
		if err := json.Unmarshal(data, &doc); err != nil {
			return fmt.Errorf("invalid JSON: %w", err)
		}
	default:
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return fmt.Errorf("invalid YAML: %w", err)
		}
	}

	// Basic validation: check required fields
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

// validatePackageEntry validates a single package entry.
func validatePackageEntry(pkg interface{}, index int) error { //nolint:gocognit,gocyclo
	switch v := pkg.(type) {
	case string:
		if v == "" {
			return fmt.Errorf("packages[%d]: empty package name", index)
		}
		return nil

	case map[string]interface{}:
		if len(v) == 0 {
			return fmt.Errorf("packages[%d]: empty object", index)
		}
		if len(v) > 1 {
			return fmt.Errorf("packages[%d]: expected single-key map (package name), got %d keys", index, len(v))
		}

		for name, opts := range v {
			if name == "" {
				return fmt.Errorf("packages[%d]: empty package name", index)
			}

			if opts == nil {
				return nil
			}

			optsMap, ok := opts.(map[string]interface{})
			if !ok {
				return fmt.Errorf("packages[%d] (%s): options must be an object", index, name)
			}

			// Validate "with" if present
			if withRaw, exists := optsMap["with"]; exists {
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

			// Check for unknown keys
			for key := range optsMap {
				if key != "with" {
					return fmt.Errorf("packages[%d] (%s): unknown option %q", index, name, key)
				}
			}
		}
		return nil

	default:
		return fmt.Errorf("packages[%d]: must be a string or object", index)
	}
}

// IsManifestFile returns true if the filename is a packages-manifest file.
func IsManifestFile(filename string) bool {
	base := filepath.Base(filename)
	return base == "packages-manifest.yaml" ||
		base == "packages-manifest.json"
}

// PackageNames returns the list of package names from the manifest.
func (m *PackagesManifest) PackageNames() []string {
	names := make([]string, len(m.Packages))
	for i, pkg := range m.Packages {
		names[i] = pkg.Name
	}
	return names
}

// String returns a human-readable representation of the package entry.
func (e *PackageEntry) String() string {
	if len(e.With) == 0 {
		return e.Name
	}
	return fmt.Sprintf("%s --with %s", e.Name, strings.Join(e.With, " --with "))
}
