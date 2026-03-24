// SPDX-License-Identifier: MIT
// Copyright Noble Factor. All rights reserved.

package config

import (
	"os"
	"path/filepath"
	"testing"

	"go.starlark.net/starlark"
)

func TestLoad(t *testing.T) {
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg == nil {
		t.Fatal("Load() returned nil")
	}
	if cfg.extensions == nil {
		t.Error("Load() extensions should not be nil")
	}
}

func TestNew(t *testing.T) {
	cfg := New()
	if cfg == nil {
		t.Fatal("New() returned nil")
	}
	if cfg.extensions == nil {
		t.Error("New() extensions should not be nil")
	}
}

func TestLoadWithSources(t *testing.T) {
	cfg, sources, err := LoadWithSources()
	if err != nil {
		t.Fatalf("LoadWithSources() error = %v", err)
	}
	if cfg == nil {
		t.Fatal("LoadWithSources() returned nil config")
	}
	if len(sources) == 0 {
		t.Error("LoadWithSources() should return at least one source")
	}

	// Should have defaults source
	hasDefaults := false
	for _, s := range sources {
		if s.Path == "<defaults>" {
			hasDefaults = true
			break
		}
	}
	if !hasDefaults {
		t.Error("LoadWithSources() should include <defaults> source")
	}
}

func TestConfig_RegisterExtension(t *testing.T) {
	ClearTypeCache()
	defer ClearTypeCache()

	cfg := New()

	spec := ConfigSpec{
		Fields: map[string]string{
			"enabled": "bool",
			"name":    "string",
		},
		Defaults: map[string]interface{}{
			"enabled": true,
			"name":    "test",
		},
	}

	err := cfg.RegisterExtension("test.extension", spec)
	if err != nil {
		t.Fatalf("RegisterExtension() error = %v", err)
	}

	// Verify it was registered
	gotSpec, ok := cfg.GetSpec("test.extension")
	if !ok {
		t.Error("GetSpec() should return true after registration")
	}
	if gotSpec.Fields["enabled"] != "bool" {
		t.Errorf("GetSpec() fields mismatch, got %v", gotSpec.Fields)
	}
}

func TestConfig_GetSpec_NotFound(t *testing.T) {
	cfg := New()

	_, ok := cfg.GetSpec("nonexistent.path")
	if ok {
		t.Error("GetSpec() should return false for nonexistent path")
	}
}

func TestConfig_Sync(t *testing.T) {
	ClearTypeCache()
	defer ClearTypeCache()

	// Change to temp dir for test
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origDir)

	cfg := New()

	// Register precommit config with hooks
	cfg.RegisterExtension("precommit", ConfigSpec{
		Fields: map[string]string{
			"hooks": "[]interface{}",
		},
		Defaults: map[string]interface{}{},
	})

	// Set hooks via mergeRaw (simulating YAML load)
	cfg.extensions.mergeRaw(map[string]interface{}{
		"precommit": map[string]interface{}{
			"hooks": []interface{}{
				map[string]interface{}{
					"id":    "test-hook",
					"name":  "Test",
					"entry": "echo test",
				},
			},
		},
	})

	result, err := cfg.Sync()
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}

	if result.FilesGenerated != 1 {
		t.Errorf("Sync() FilesGenerated = %d, want 1", result.FilesGenerated)
	}
	if result.PrecommitConfig == "" {
		t.Error("Sync() should have generated pre-commit config")
	}
}

func TestConfig_ToStarlark(t *testing.T) {
	cfg := New()

	val := cfg.ToStarlark()
	if val == nil {
		t.Fatal("ToStarlark() returned nil")
	}

	// Should be a ConfigValue
	cv, ok := val.(*ConfigValue)
	if !ok {
		t.Fatalf("ToStarlark() type = %T, want *ConfigValue", val)
	}
	if cv.Type() != "config" {
		t.Errorf("ToStarlark().Type() = %q, want %q", cv.Type(), "config")
	}
}

func TestConfig_Accessor(t *testing.T) {
	ClearTypeCache()
	defer ClearTypeCache()

	cfg := New()

	// Register an extension
	cfg.RegisterExtension("lint.go", ConfigSpec{
		Fields: map[string]string{
			"path":          "string",
			"skip_mod_tidy": "bool",
		},
		Defaults: map[string]interface{}{
			"path":          "./...",
			"skip_mod_tidy": false,
		},
	})

	acc := cfg.Accessor("lint.go")
	if acc == nil {
		t.Fatal("Accessor() returned nil")
	}

	path := acc.String("path")
	if path != "./..." {
		t.Errorf("Accessor().String('path') = %q, want %q", path, "./...")
	}

	skipModTidy := acc.Bool("skip_mod_tidy")
	if skipModTidy {
		t.Error("Accessor().Bool('skip_mod_tidy') = true, want false")
	}
}

// =============================================================================
// ConfigValue (Starlark wrapper) tests
// =============================================================================

func TestConfigValue_Attr_Extension(t *testing.T) {
	ClearTypeCache()
	defer ClearTypeCache()

	cfg := New()

	spec := ConfigSpec{
		Fields: map[string]string{
			"enabled": "bool",
		},
		Defaults: map[string]interface{}{
			"enabled": true,
		},
	}
	cfg.RegisterExtension("myext", spec)

	val := cfg.ToStarlark()

	// Access extension attribute
	extVal, err := val.(starlark.HasAttrs).Attr("myext")
	if err != nil {
		t.Fatalf("Attr('myext') error = %v", err)
	}
	if extVal == nil {
		t.Error("Attr('myext') returned nil")
	}
}

func TestConfigValue_Attr_NotFound_ViaConfig(t *testing.T) {
	cfg := New()
	val := cfg.ToStarlark()

	_, err := val.(starlark.HasAttrs).Attr("nonexistent")
	if err == nil {
		t.Error("Attr('nonexistent') should return error")
	}
	if _, ok := err.(starlark.NoSuchAttrError); !ok {
		t.Errorf("Attr() error type = %T, want NoSuchAttrError", err)
	}
}

func TestConfigValue_Attr_NestedLint(t *testing.T) {
	ClearTypeCache()
	defer ClearTypeCache()

	cfg := New()

	// Register lint.go at the correct nested path
	cfg.RegisterExtension("lint.go", ConfigSpec{
		Fields: map[string]string{
			"path": "string",
		},
		Defaults: map[string]interface{}{
			"path": "./...",
		},
	})

	val := cfg.ToStarlark()

	// Access "lint" should return a ConfigValue for the intermediate element
	lintVal, err := val.(starlark.HasAttrs).Attr("lint")
	if err != nil {
		t.Fatalf("Attr('lint') error = %v", err)
	}
	if lintVal == nil {
		t.Error("Attr('lint') returned nil")
	}

	// Access "go" from lint
	lintAttrs, ok := lintVal.(starlark.HasAttrs)
	if !ok {
		t.Fatalf("lint value should implement HasAttrs, got %T", lintVal)
	}

	goVal, err := lintAttrs.Attr("go")
	if err != nil {
		t.Fatalf("Attr('go') error = %v", err)
	}
	if goVal == nil {
		t.Error("Attr('go') returned nil")
	}
}

func TestConfigValue_AttrNames_ViaConfig(t *testing.T) {
	ClearTypeCache()
	defer ClearTypeCache()

	cfg := New()
	cfg.RegisterExtension("lint.go", ConfigSpec{
		Fields: map[string]string{"path": "string"},
	})
	cfg.RegisterExtension("lint.shell", ConfigSpec{
		Fields: map[string]string{"path": "string"},
	})

	val := cfg.ToStarlark()

	names := val.(starlark.HasAttrs).AttrNames()
	hasLint := false
	for _, name := range names {
		if name == "lint" {
			hasLint = true
		}
	}
	if !hasLint {
		t.Error("AttrNames() should contain 'lint'")
	}
}

func TestLoad_WithProjectConfig(t *testing.T) {
	ClearTypeCache()
	defer ClearTypeCache()

	// Create temp dir with star/config.yaml
	tmpDir := t.TempDir()

	SetGitWorkspaceRoot(tmpDir)
	defer ResetGitWorkspaceRoot()

	// Create star/ directory
	starDir := filepath.Join(tmpDir, "star")
	if err := os.MkdirAll(starDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write a project config
	yamlContent := `
lint:
  go:
    path: "./custom/..."
`
	if err := os.WriteFile(filepath.Join(starDir, "config.yaml"), []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := New()

	// Register lint.go so the extension hierarchy knows about it
	cfg.RegisterExtension("lint.go", ConfigSpec{
		Fields: map[string]string{
			"path":          "string",
			"skip_mod_tidy": "bool",
		},
		Defaults: map[string]interface{}{
			"path":          "./...",
			"skip_mod_tidy": false,
		},
	})

	// Load from files
	if err := cfg.LoadFromFiles(); err != nil {
		t.Fatalf("LoadFromFiles() error = %v", err)
	}

	// Verify the project config path was merged
	acc := cfg.Accessor("lint.go")
	path := acc.String("path")
	if path != "./custom/..." {
		t.Errorf("LoadFromFiles() did not merge project config, got path = %q", path)
	}
}
