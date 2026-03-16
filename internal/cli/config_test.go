// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSharedConfigPath(t *testing.T) {
	// SharedConfigPath should return the same path regardless of which tool calls it
	path := SharedConfigPath()

	// Verify it's an absolute path
	if !filepath.IsAbs(path) {
		t.Errorf("expected absolute path, got %q", path)
	}

	// Verify it's in the devlore directory (shared by both lore and writ)
	if !strings.Contains(path, "devlore") {
		t.Errorf("expected path to contain 'devlore', got %q", path)
	}

	// Verify it ends with config.yaml
	if filepath.Base(path) != "config.yaml" {
		t.Errorf("expected path ending in 'config.yaml', got %q", path)
	}
}

func TestUnifiedConfig_BothToolsSeeSharedSettings(t *testing.T) {
	// This test verifies that lore and writ both read from the same config file
	// and see the same shared settings (model, registry, verbosity)

	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Create shared config with settings relevant to both tools
	configDir := filepath.Join(tmpDir, "devlore")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("creating config dir: %v", err)
	}

	configContent := `
verbosity: verbose
dry_run: true
model:
  provider: anthropic
  name: claude-sonnet-4-20250514
registry:
  url: https://example.com/registry.git
  branch: develop
lore:
  preferences:
    shell: zsh
writ:
  segments:
    - ROLE
    - SITE
  vars:
    USER_NAME: "Test User"
`
	configPath := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
		t.Fatalf("writing config file: %v", err)
	}

	// Load config (simulates what both lore and writ do via cli.loadConfig)
	config, err := loadConfig(SharedConfigPath())
	if err != nil {
		t.Fatalf("loadConfig() error: %v", err)
	}

	// Verify shared settings are visible
	// These settings should be the same regardless of whether accessed from lore or writ

	// Check verbosity (shared)
	verbosity, ok := getNestedValue(config, "verbosity")
	if !ok {
		t.Error("expected to find 'verbosity' in config")
	} else if verbosity != "verbose" {
		t.Errorf("expected verbosity 'verbose', got %q", verbosity)
	}

	// Check dry_run (shared)
	dryRun, ok := getNestedValue(config, "dry_run")
	if !ok {
		t.Error("expected to find 'dry_run' in config")
	} else if dryRun != true {
		t.Errorf("expected dry_run=true, got %v", dryRun)
	}

	// Check model settings (shared)
	provider, ok := getNestedValue(config, "model.provider")
	if !ok {
		t.Error("expected to find 'model.provider' in config")
	} else if provider != "anthropic" {
		t.Errorf("expected model.provider 'anthropic', got %q", provider)
	}

	modelName, ok := getNestedValue(config, "model.name")
	if !ok {
		t.Error("expected to find 'model.name' in config")
	} else if modelName != "claude-sonnet-4-20250514" {
		t.Errorf("expected model.name 'claude-sonnet-4-20250514', got %q", modelName)
	}

	// Check registry settings (shared)
	regURL, ok := getNestedValue(config, "registry.url")
	if !ok {
		t.Error("expected to find 'registry.url' in config")
	} else if regURL != "https://example.com/registry.git" {
		t.Errorf("expected registry.url, got %q", regURL)
	}
}

func TestUnifiedConfig_ToolSpecificSettingsVisible(t *testing.T) {
	// Both tools should be able to read tool-specific settings from the shared config

	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	configDir := filepath.Join(tmpDir, "devlore")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("creating config dir: %v", err)
	}

	configContent := `
lore:
  preferences:
    shell: bash
    editor: vim
writ:
  vars:
    USER_NAME: "Jane Doe"
    USER_EMAIL: "jane@example.com"
`
	configPath := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
		t.Fatalf("writing config file: %v", err)
	}

	config, err := loadConfig(SharedConfigPath())
	if err != nil {
		t.Fatalf("loadConfig() error: %v", err)
	}

	// Both tools can see lore-specific settings
	loreShell, ok := getNestedValue(config, "lore.preferences.shell")
	if !ok {
		t.Error("expected to find 'lore.preferences.shell'")
	} else if loreShell != "bash" {
		t.Errorf("expected lore.preferences.shell 'bash', got %q", loreShell)
	}

	// Both tools can see writ-specific settings
	writUserName, ok := getNestedValue(config, "writ.vars.USER_NAME")
	if !ok {
		t.Error("expected to find 'writ.vars.USER_NAME'")
	} else if writUserName != "Jane Doe" {
		t.Errorf("expected writ.vars.USER_NAME 'Jane Doe', got %q", writUserName)
	}
}

func TestPrintFlattened_ShowsAllSettings(t *testing.T) {
	// printFlattened should output all settings from the unified config
	// This is what 'config list' uses

	config := map[string]interface{}{
		"verbosity": "verbose",
		"dry_run":   true,
		"model": map[string]interface{}{
			"provider": "anthropic",
			"name":     "claude-sonnet-4-20250514",
		},
		"registry": map[string]interface{}{
			"url":    "https://example.com/registry.git",
			"branch": "develop",
		},
		"lore": map[string]interface{}{
			"preferences": map[string]interface{}{
				"shell": "zsh",
			},
		},
		"writ": map[string]interface{}{
			"vars": map[string]interface{}{
				"USER_NAME": "Test User",
			},
		},
	}

	// Capture output
	var buf bytes.Buffer
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	printFlattened("", config)

	_ = w.Close()
	os.Stdout = old
	_, _ = buf.ReadFrom(r)

	output := buf.String()

	// Verify shared settings are printed
	expectedKeys := []string{
		"verbosity=verbose",
		"dry_run=true",
		"model.provider=anthropic",
		"model.name=claude-sonnet-4-20250514",
		"registry.url=https://example.com/registry.git",
		"registry.branch=develop",
		"lore.preferences.shell=zsh",
		"writ.vars.USER_NAME=Test User",
	}

	for _, expected := range expectedKeys {
		if !strings.Contains(output, expected) {
			t.Errorf("expected output to contain %q, got:\n%s", expected, output)
		}
	}
}

func TestConfigListOutput_SameForBothTools(t *testing.T) {
	// The config list command should show identical output for both lore and writ
	// because they read from the same file

	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	configDir := filepath.Join(tmpDir, "devlore")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("creating config dir: %v", err)
	}

	configContent := `
model:
  provider: groq
  name: llama-3.3-70b-versatile
registry:
  url: https://example.com/registry.git
`
	configPath := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
		t.Fatalf("writing config file: %v", err)
	}

	// Simulate loading config from lore perspective
	loreConfig, err := loadConfig(SharedConfigPath())
	if err != nil {
		t.Fatalf("loadConfig() for lore error: %v", err)
	}

	// Simulate loading config from writ perspective
	writConfig, err := loadConfig(SharedConfigPath())
	if err != nil {
		t.Fatalf("loadConfig() for writ error: %v", err)
	}

	// Both should have identical content
	loreProvider, _ := getNestedValue(loreConfig, "model.provider")
	writProvider, _ := getNestedValue(writConfig, "model.provider")

	if loreProvider != writProvider {
		t.Errorf("model.provider differs: lore=%q, writ=%q", loreProvider, writProvider)
	}

	loreRegistry, _ := getNestedValue(loreConfig, "registry.url")
	writRegistry, _ := getNestedValue(writConfig, "registry.url")

	if loreRegistry != writRegistry {
		t.Errorf("registry.url differs: lore=%q, writ=%q", loreRegistry, writRegistry)
	}
}

func TestSetNestedValue_WorksForSharedAndToolSpecific(t *testing.T) {
	config := make(map[string]interface{})

	// Set shared value
	setNestedValue(config, "model.provider", "anthropic")

	// Set lore-specific value
	setNestedValue(config, "lore.preferences.shell", "zsh")

	// Set writ-specific value
	setNestedValue(config, "writ.vars.USER_NAME", "Test User")

	// Verify all values are set correctly
	provider, ok := getNestedValue(config, "model.provider")
	if !ok || provider != "anthropic" {
		t.Errorf("expected model.provider='anthropic', got %v", provider)
	}

	shell, ok := getNestedValue(config, "lore.preferences.shell")
	if !ok || shell != "zsh" {
		t.Errorf("expected lore.preferences.shell='zsh', got %v", shell)
	}

	userName, ok := getNestedValue(config, "writ.vars.USER_NAME")
	if !ok || userName != "Test User" {
		t.Errorf("expected writ.vars.USER_NAME='Test User', got %v", userName)
	}
}

func TestDeleteNestedValue_WorksForSharedAndToolSpecific(t *testing.T) {
	config := map[string]interface{}{
		"model": map[string]interface{}{
			"provider": "anthropic",
			"name":     "claude-sonnet-4-20250514",
		},
		"lore": map[string]interface{}{
			"preferences": map[string]interface{}{
				"shell": "zsh",
			},
		},
	}

	// Delete shared value
	if !deleteNestedValue(config, "model.name") {
		t.Error("expected to delete model.name")
	}

	_, exists := getNestedValue(config, "model.name")
	if exists {
		t.Error("model.name should have been deleted")
	}

	// model.provider should still exist
	_, exists = getNestedValue(config, "model.provider")
	if !exists {
		t.Error("model.provider should still exist")
	}

	// Delete tool-specific value
	if !deleteNestedValue(config, "lore.preferences.shell") {
		t.Error("expected to delete lore.preferences.shell")
	}

	_, exists = getNestedValue(config, "lore.preferences.shell")
	if exists {
		t.Error("lore.preferences.shell should have been deleted")
	}
}

func TestLoadConfig_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	configDir := filepath.Join(tmpDir, "devlore")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("creating config dir: %v", err)
	}

	// Create empty config file
	configPath := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(""), 0o644); err != nil {
		t.Fatalf("writing config file: %v", err)
	}

	config, err := loadConfig(SharedConfigPath())
	if err != nil {
		t.Fatalf("loadConfig() error: %v", err)
	}

	// Should return empty map, not nil
	if config == nil {
		t.Error("expected non-nil config")
	}

	if len(config) != 0 {
		t.Errorf("expected empty config, got %v", config)
	}
}

func TestSaveConfig_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Directory doesn't exist yet
	configPath := SharedConfigPath()

	config := map[string]interface{}{
		"model": map[string]interface{}{
			"provider": "ollama",
		},
	}

	err := saveConfig(configPath, config)
	if err != nil {
		t.Fatalf("saveConfig() error: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("config file should have been created")
	}

	// Verify content
	loaded, err := loadConfig(configPath)
	if err != nil {
		t.Fatalf("loadConfig() error: %v", err)
	}

	provider, ok := getNestedValue(loaded, "model.provider")
	if !ok || provider != "ollama" {
		t.Errorf("expected model.provider='ollama', got %v", provider)
	}
}

func TestFormatValue_VariousTypes(t *testing.T) {
	tests := []struct {
		input    interface{}
		expected string
	}{
		{"hello", "hello"},
		{42, "42"},
		{3.14, "3.14"},
		{true, "true"},
		{false, "false"},
		{nil, ""},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%T(%v)", tt.input, tt.input), func(t *testing.T) {
			got := formatValue(tt.input)
			if got != tt.expected {
				t.Errorf("formatValue(%v) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
