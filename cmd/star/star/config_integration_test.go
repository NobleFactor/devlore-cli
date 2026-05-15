// SPDX-License-Identifier: MIT
// Copyright Noble Factor. All rights reserved.

//go:build integration

package star

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	starlarklib "go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/cmd/star/config"
)

// TestConfigIntegration verifies that every extension declaring a config
// section in its extension.yaml can:
//
//  1. register its config spec and serve correct defaults via ConfigAccessor
//  2. Accept YAML overrides via LoadFromFiles and reflect them through ConfigAccessor
//  3. Expose the same values through the Starlark config.get() attribute chain
//
// The test uses extension.Discover to load real extension.yaml files, so it
// breaks if an extension.yaml declaration drifts from the config system's
// expectations. No specs are hardcoded — the extension.yaml files are the
// source of truth.
//
// Phase 1 (defaults) and Phase 3 (Starlark) run against a runtime loaded
// without any project config.yaml. Phase 2 (overrides) writes a temporary
// config.yaml with generated values and verifies they propagate.
func TestConfigIntegration(t *testing.T) {
	projectRoot, err := findProjectRoot()
	if err != nil {
		t.Fatalf("findProjectRoot: %v", err)
	}
	extDir := filepath.Join(projectRoot, "cmd", "star", "extensions")

	loader := &ExtensionLoader{searchPaths: []string{extDir}}
	allExts, err := loader.DiscoverAll()
	if err != nil {
		t.Fatalf("DiscoverAll(%s): %v", extDir, err)
	}

	var configExts []*Extension
	for _, ext := range allExts {
		if ext.HasConfig() {
			configExts = append(configExts, ext)
		}
	}
	if len(configExts) == 0 {
		t.Fatal("no extensions with config found — expected at least one")
	}

	for _, ext := range configExts {
		ext := ext // capture for parallel subtests

		t.Run(ext.ConfigPath(), func(t *testing.T) {
			configPath := ext.ConfigPath()
			configSpec := ext.ToConfigSpec()

			// --- Phase 1: Defaults via Go ConfigAccessor ---
			t.Run("defaults", func(t *testing.T) {
				config.ClearTypeCache()
				defer config.ClearTypeCache()

				// Isolate from real project config — empty temp dir
				config.SetGitWorkspaceRoot(t.TempDir())
				defer config.ResetGitWorkspaceRoot()

				r := NewApplication(&cobra.Command{Use: "star"})
				if err := r.LoadExtensionsFrom(extDir); err != nil {
					t.Fatalf("LoadExtensionsFrom: %v", err)
				}

				acc := r.Config().Accessor(configPath)
				if acc == nil || !acc.IsValid() {
					t.Fatalf("Accessor(%q) invalid", configPath)
				}

				for fieldName, fieldType := range configSpec.Fields {
					if !acc.Has(fieldName) {
						t.Errorf("field %q (%s) not found in accessor", fieldName, fieldType)
						continue
					}
					checkConfigDefault(t, acc, fieldName, fieldType, configSpec.Defaults)
				}
			})

			// --- Phase 2: Overrides via project config.yaml ---
			t.Run("overrides", func(t *testing.T) {
				overrides := generateConfigOverrides(configSpec)
				if len(overrides) == 0 {
					t.Skip("no overridable fields")
				}

				config.ClearTypeCache()
				defer config.ClearTypeCache()

				// Write override YAML to a temp project root
				tmpDir := t.TempDir()
				yamlContent := overrideYAML(configPath, overrides)
				starDir := filepath.Join(tmpDir, "star")
				if err := os.MkdirAll(starDir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(starDir, "config.yaml"), yamlContent, 0o644); err != nil {
					t.Fatal(err)
				}

				config.SetGitWorkspaceRoot(tmpDir)
				defer config.ResetGitWorkspaceRoot()

				r := NewApplication(&cobra.Command{Use: "star"})
				if err := r.LoadExtensionsFrom(extDir); err != nil {
					t.Fatalf("LoadExtensionsFrom: %v", err)
				}

				acc := r.Config().Accessor(configPath)
				if acc == nil || !acc.IsValid() {
					t.Fatalf("Accessor(%q) invalid after override", configPath)
				}

				for fieldName, expected := range overrides {
					checkConfigOverride(t, acc, fieldName, configSpec.Fields[fieldName], expected)
				}
			})

			// --- Phase 3: Starlark attribute chain ---
			t.Run("starlark", func(t *testing.T) {
				config.ClearTypeCache()
				defer config.ClearTypeCache()

				// Isolate from real project config — defaults only
				config.SetGitWorkspaceRoot(t.TempDir())
				defer config.ResetGitWorkspaceRoot()

				r := NewApplication(&cobra.Command{Use: "star"})
				if err := r.LoadExtensionsFrom(extDir); err != nil {
					t.Fatalf("LoadExtensionsFrom: %v", err)
				}

				starVal := r.Config().ToStarlark()
				leaf := walkConfigPath(t, starVal, configPath)
				if leaf == nil {
					t.Fatalf("Starlark walk to %q returned nil", configPath)
				}

				for fieldName, fieldType := range configSpec.Fields {
					checkStarlarkType(t, leaf, fieldName, fieldType)
				}
			})
		})
	}
}

// =============================================================================
// Helpers: Go accessor verification
// =============================================================================

// checkConfigDefault verifies an accessor returns the declared default.
// Complex types (maps, untyped slices) are only checked for presence.
func checkConfigDefault(t *testing.T, acc *config.ConfigAccessor, field, fieldType string, defaults map[string]interface{}) {
	t.Helper()

	def, hasDef := defaults[field]
	if !hasDef {
		return
	}

	switch fieldType {
	case "string":
		if want, ok := def.(string); ok {
			if got := acc.String(field); got != want {
				t.Errorf("default %s = %q, want %q", field, got, want)
			}
		}
	case "bool":
		if want, ok := def.(bool); ok {
			if got := acc.Bool(field); got != want {
				t.Errorf("default %s = %v, want %v", field, got, want)
			}
		}
	case "int":
		var want int
		switch v := def.(type) {
		case int:
			want = v
		case float64:
			want = int(v) // YAML unmarshals integers as float64
		default:
			return
		}
		if got := acc.Int(field); got != want {
			t.Errorf("default %s = %d, want %d", field, got, want)
		}
	case "[]string":
		wantSlice, ok := def.([]interface{})
		if !ok {
			return
		}
		got := acc.StringSlice(field)
		if len(got) != len(wantSlice) {
			t.Errorf("default %s len = %d, want %d", field, len(got), len(wantSlice))
		}
	}
}

// =============================================================================
// Helpers: override generation and verification
// =============================================================================

// generateConfigOverrides creates test override values for simple field types.
// Complex types (map[string]interface{}, []interface{}) are skipped because
// they cannot be meaningfully auto-generated.
func generateConfigOverrides(spec config.ConfigSpec) map[string]interface{} {
	overrides := make(map[string]interface{})

	for field, fieldType := range spec.Fields {
		switch fieldType {
		case "string":
			overrides[field] = "override_" + field
		case "bool":
			if def, ok := spec.Defaults[field].(bool); ok {
				overrides[field] = !def
			} else {
				overrides[field] = true
			}
		case "int":
			overrides[field] = 99
		case "[]string":
			overrides[field] = []string{"test_value"}
		}
	}

	return overrides
}

// overrideYAML generates a star/config.yaml from a dotted path and field values.
//
//	overrideYAML("lint.go", {"path":"x"}) → "lint:\n  go:\n    path: x\n"
func overrideYAML(configPath string, values map[string]interface{}) []byte {
	parts := strings.Split(configPath, ".")

	var b strings.Builder
	for i, part := range parts {
		indent := strings.Repeat("  ", i)
		b.WriteString(indent + part + ":\n")
	}

	indent := strings.Repeat("  ", len(parts))
	for field, val := range values {
		switch v := val.(type) {
		case string:
			b.WriteString(indent + field + ": " + quoteYAML(v) + "\n")
		case bool:
			if v {
				b.WriteString(indent + field + ": true\n")
			} else {
				b.WriteString(indent + field + ": false\n")
			}
		case int:
			b.WriteString(indent + field + ": " + fmt.Sprintf("%d", v) + "\n")
		case []string:
			b.WriteString(indent + field + ":\n")
			for _, item := range v {
				b.WriteString(indent + "  - " + quoteYAML(item) + "\n")
			}
		}
	}

	return []byte(b.String())
}

// quoteYAML wraps a string in double quotes for safe YAML embedding.
func quoteYAML(s string) string {
	return "\"" + strings.ReplaceAll(s, "\"", "\\\"") + "\""
}

// checkConfigOverride verifies the accessor returns the expected override.
func checkConfigOverride(t *testing.T, acc *config.ConfigAccessor, field, fieldType string, expected interface{}) {
	t.Helper()

	switch fieldType {
	case "string":
		want, _ := expected.(string)
		if got := acc.String(field); got != want {
			t.Errorf("override %s = %q, want %q", field, got, want)
		}
	case "bool":
		want, _ := expected.(bool)
		if got := acc.Bool(field); got != want {
			t.Errorf("override %s = %v, want %v", field, got, want)
		}
	case "int":
		want, _ := expected.(int)
		if got := acc.Int(field); got != want {
			t.Errorf("override %s = %d, want %d", field, got, want)
		}
	case "[]string":
		got := acc.StringSlice(field)
		want, _ := expected.([]string)
		if len(got) != len(want) {
			t.Errorf("override %s len = %d, want %d", field, len(got), len(want))
		} else {
			for i := range want {
				if got[i] != want[i] {
					t.Errorf("override %s[%d] = %q, want %q", field, i, got[i], want[i])
				}
			}
		}
	}
}

// =============================================================================
// Helpers: Starlark attribute chain
// =============================================================================

// walkConfigPath splits a dotted config path and calls Attr at each step.
// Returns the leaf ConfigValue or fails the test.
func walkConfigPath(t *testing.T, root starlarklib.Value, path string) *config.ConfigValue {
	t.Helper()

	parts := strings.Split(path, ".")
	current := root

	for _, part := range parts {
		attrs, ok := current.(starlarklib.HasAttrs)
		if !ok {
			t.Fatalf("Starlark value at %q is %T, not HasAttrs", part, current)
			return nil
		}
		val, err := attrs.Attr(part)
		if err != nil {
			t.Fatalf("Starlark Attr(%q): %v", part, err)
			return nil
		}
		current = val
	}

	cv, ok := current.(*config.ConfigValue)
	if !ok {
		t.Fatalf("Starlark leaf at %q is %T, want *config.ConfigValue", path, current)
		return nil
	}
	return cv
}

// checkStarlarkType verifies that a field is accessible from the ConfigValue
// and returns the expected Starlark type.
func checkStarlarkType(t *testing.T, cv *config.ConfigValue, field, fieldType string) {
	t.Helper()

	val, err := cv.Attr(field)
	if err != nil {
		t.Errorf("Starlark Attr(%q): %v", field, err)
		return
	}

	switch fieldType {
	case "string":
		if _, ok := val.(starlarklib.String); !ok {
			t.Errorf("Starlark %s is %T, want String", field, val)
		}
	case "bool":
		if _, ok := val.(starlarklib.Bool); !ok {
			t.Errorf("Starlark %s is %T, want Bool", field, val)
		}
	case "int":
		if _, ok := val.(starlarklib.Int); !ok {
			t.Errorf("Starlark %s is %T, want Int", field, val)
		}
	case "[]string":
		if _, ok := val.(*starlarklib.List); !ok {
			t.Errorf("Starlark %s is %T, want *List", field, val)
		}
	case "[]interface{}":
		if _, ok := val.(*starlarklib.List); !ok {
			t.Errorf("Starlark %s is %T, want *List", field, val)
		}
	case "map[string]interface{}":
		// May be *Dict or None depending on content; just verify accessible
	}
}
