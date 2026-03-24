// SPDX-License-Identifier: MIT
// Copyright Noble Factor. All rights reserved.

package config

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestNewExtensionsConfig(t *testing.T) {
	cfg := newExtensionsConfig("star/config.yaml")

	if cfg.Source() != "star/config.yaml" {
		t.Errorf("Source() = %q, want 'star/config.yaml'", cfg.Source())
	}
	if cfg.Path() != "" {
		t.Errorf("Path() = %q, want ''", cfg.Path())
	}
	if cfg.IsDirty() {
		t.Error("new config should not be dirty")
	}
}

func TestExtensionsConfig_SetDirty(t *testing.T) {
	cfg := newExtensionsConfig("star/config.yaml")

	cfg.SetDirty(true)
	if !cfg.IsDirty() {
		t.Error("IsDirty() should be true after SetDirty(true)")
	}

	cfg.SetDirty(false)
	if cfg.IsDirty() {
		t.Error("IsDirty() should be false after SetDirty(false)")
	}
}

func TestExtensionsConfig_RegisterExtension_Simple(t *testing.T) {
	ClearTypeCache()
	defer ClearTypeCache()

	cfg := newExtensionsConfig("star/config.yaml")

	spec := ConfigSpec{
		Fields: map[string]string{
			"enabled": "bool",
			"name":    "string",
		},
		Defaults: map[string]interface{}{
			"enabled": true,
			"name":    "default",
		},
	}

	err := cfg.registerExtension("test", spec)
	if err != nil {
		t.Fatalf("registerExtension error: %v", err)
	}

	// Should be able to navigate to it
	elem := cfg.Navigate("test")
	if elem == nil {
		t.Fatal("Navigate('test') should return element")
	}

	// Should have default values
	acc := cfg.accessor("test")
	if !acc.Bool("enabled") {
		t.Error("enabled should be true")
	}
	if acc.String("name") != "default" {
		t.Errorf("name = %q, want 'default'", acc.String("name"))
	}
}

func TestExtensionsConfig_RegisterExtension_Nested(t *testing.T) {
	ClearTypeCache()
	defer ClearTypeCache()

	cfg := newExtensionsConfig("star/config.yaml")

	spec := ConfigSpec{
		Fields: map[string]string{
			"enabled": "bool",
			"license": "string",
		},
		Defaults: map[string]interface{}{
			"enabled": false,
			"license": "MIT",
		},
	}

	err := cfg.registerExtension("lint.copyright", spec)
	if err != nil {
		t.Fatalf("registerExtension error: %v", err)
	}

	// Should create intermediate "lint" element
	lint := cfg.Navigate("lint")
	if lint == nil {
		t.Fatal("Navigate('lint') should return intermediate element")
	}

	// Should be able to navigate to copyright
	copyright := cfg.Navigate("lint.copyright")
	if copyright == nil {
		t.Fatal("Navigate('lint.copyright') should return element")
	}

	// Check defaults
	acc := cfg.accessor("lint.copyright")
	if acc.Bool("enabled") {
		t.Error("enabled should be false")
	}
	if acc.String("license") != "MIT" {
		t.Errorf("license = %q, want 'MIT'", acc.String("license"))
	}
}

func TestExtensionsConfig_RegisterExtension_EmptyPath(t *testing.T) {
	cfg := newExtensionsConfig("star/config.yaml")

	err := cfg.registerExtension("", ConfigSpec{})
	if err == nil {
		t.Error("registerExtension with empty path should error")
	}
}

func TestExtensionsConfig_GetSpec(t *testing.T) {
	ClearTypeCache()
	defer ClearTypeCache()

	cfg := newExtensionsConfig("star/config.yaml")

	spec := ConfigSpec{
		Type: "TestConfig",
		Fields: map[string]string{
			"enabled": "bool",
		},
	}

	cfg.registerExtension("test", spec)

	got, ok := cfg.getSpec("test")
	if !ok {
		t.Fatal("getSpec('test') should return true")
	}
	if got.Type != "TestConfig" {
		t.Errorf("spec.Type = %q, want 'TestConfig'", got.Type)
	}

	_, ok = cfg.getSpec("missing")
	if ok {
		t.Error("getSpec('missing') should return false")
	}
}

func TestExtensionsConfig_Accessor_Invalid(t *testing.T) {
	cfg := newExtensionsConfig("star/config.yaml")

	acc := cfg.accessor("missing.path")
	if acc.IsValid() {
		t.Error("accessor for missing path should be invalid")
	}
}

func TestLoadExtensions_FileNotExist(t *testing.T) {
	cfg, err := loadExtensions("/nonexistent/path/star/config.yaml")
	if err != nil {
		t.Fatalf("loadExtensions should not error for missing file: %v", err)
	}
	if cfg == nil {
		t.Fatal("loadExtensions should return config even for missing file")
	}
}

func TestLoadExtensions_ValidYAML(t *testing.T) {
	ClearTypeCache()
	defer ClearTypeCache()

	tmpDir := t.TempDir()
	starDir := filepath.Join(tmpDir, "star")
	if err := os.MkdirAll(starDir, 0755); err != nil {
		t.Fatal(err)
	}
	yamlPath := filepath.Join(starDir, "config.yaml")

	yamlContent := `
lint:
  copyright:
    enabled: true
    license: Apache-2.0
`
	if err := os.WriteFile(yamlPath, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	// First register the extension
	cfg := newExtensionsConfig(yamlPath)
	spec := ConfigSpec{
		Fields: map[string]string{
			"enabled": "bool",
			"license": "string",
		},
		Defaults: map[string]interface{}{
			"enabled": false,
			"license": "MIT",
		},
	}
	cfg.registerExtension("lint.copyright", spec)

	// Now load and merge values
	data, _ := os.ReadFile(yamlPath)
	var raw map[string]interface{}
	if err := parseYAML(data, &raw); err != nil {
		t.Fatal(err)
	}
	cfg.mergeRaw(raw)

	// Check merged values
	acc := cfg.accessor("lint.copyright")
	if !acc.Bool("enabled") {
		t.Error("enabled should be true after merge")
	}
	if acc.String("license") != "Apache-2.0" {
		t.Errorf("license = %q, want 'Apache-2.0'", acc.String("license"))
	}
}

func TestLoadExtensionsWithSpecs(t *testing.T) {
	ClearTypeCache()
	defer ClearTypeCache()

	tmpDir := t.TempDir()
	starDir := filepath.Join(tmpDir, "star")
	if err := os.MkdirAll(starDir, 0755); err != nil {
		t.Fatal(err)
	}
	yamlPath := filepath.Join(starDir, "config.yaml")

	yamlContent := `
lint:
  go:
    enabled: true
`
	if err := os.WriteFile(yamlPath, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	specs := map[string]ConfigSpec{
		"lint.go": {
			Fields: map[string]string{
				"enabled": "bool",
				"path":    "string",
			},
			Defaults: map[string]interface{}{
				"enabled": false,
				"path":    "./...",
			},
		},
	}

	cfg, err := loadExtensionsWithSpecs(yamlPath, specs)
	if err != nil {
		t.Fatalf("loadExtensionsWithSpecs error: %v", err)
	}

	acc := cfg.accessor("lint.go")
	if !acc.Bool("enabled") {
		t.Error("enabled should be true (from YAML)")
	}
	if acc.String("path") != "./..." {
		t.Errorf("path = %q, want './...' (from default)", acc.String("path"))
	}
}

func TestExtensionsConfig_Save(t *testing.T) {
	ClearTypeCache()
	defer ClearTypeCache()

	tmpDir := t.TempDir()
	starDir := filepath.Join(tmpDir, "star")
	if err := os.MkdirAll(starDir, 0755); err != nil {
		t.Fatal(err)
	}
	yamlPath := filepath.Join(starDir, "config.yaml")

	cfg := newExtensionsConfig(yamlPath)
	spec := ConfigSpec{
		Fields: map[string]string{
			"enabled": "bool",
		},
		Defaults: map[string]interface{}{
			"enabled": true,
		},
	}
	cfg.registerExtension("test", spec)
	cfg.SetDirty(true)

	if err := cfg.save(); err != nil {
		t.Fatalf("save error: %v", err)
	}

	if cfg.IsDirty() {
		t.Error("IsDirty should be false after save")
	}

	// File should exist
	if _, err := os.Stat(yamlPath); err != nil {
		t.Errorf("saved file should exist: %v", err)
	}
}

func TestExtensionsConfig_RegisterExtension_WithNestedTypes(t *testing.T) {
	ClearTypeCache()
	defer ClearTypeCache()

	cfg := newExtensionsConfig("star/config.yaml")

	spec := ConfigSpec{
		Fields: map[string]string{
			"patterns": "map[string]Pattern",
		},
		Nested: map[string]ConfigSpec{
			"Pattern": {
				Fields: map[string]string{
					"match":   "string",
					"replace": "string",
				},
			},
		},
		Defaults: map[string]interface{}{
			"patterns": map[string]interface{}{
				"*.go": map[string]interface{}{
					"match":   "// Copyright",
					"replace": "// SPDX",
				},
			},
		},
	}

	err := cfg.registerExtension("lint.copyright", spec)
	if err != nil {
		t.Fatalf("registerExtension error: %v", err)
	}

	// Verify the nested structure
	acc := cfg.accessor("lint.copyright")
	patterns := acc.Map("patterns")
	if patterns == nil {
		t.Fatal("patterns should not be nil")
	}
	if len(patterns) != 1 {
		t.Errorf("patterns len = %d, want 1", len(patterns))
	}
}

// parseYAML is a helper for tests
func parseYAML(data []byte, v interface{}) error {
	return yaml.Unmarshal(data, v)
}
