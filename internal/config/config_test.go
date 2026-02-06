// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPath(t *testing.T) {
	path := Path()
	if path == "" {
		t.Error("expected non-empty path")
	}
	// Should end with config.yaml
	if !filepath.IsAbs(path) {
		t.Errorf("expected absolute path, got %q", path)
	}
	if filepath.Base(path) != "config.yaml" {
		t.Errorf("expected path ending in 'config.yaml', got %q", path)
	}
}

func TestLoad_NoFile(t *testing.T) {
	// Use temp dir to ensure no config file exists
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}

	// Should return empty config
	if cfg.Model.Provider != "" {
		t.Errorf("expected empty provider, got %q", cfg.Model.Provider)
	}
	if cfg.Registry.URL != "" {
		t.Errorf("expected empty registry URL, got %q", cfg.Registry.URL)
	}
}

func TestLoad_WithEnvOverrides(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Set environment variables
	t.Setenv("DEVLORE_VERBOSITY", "verbose")
	t.Setenv("DEVLORE_DRY_RUN", "1")
	t.Setenv("DEVLORE_MODEL_PROVIDER", "anthropic")
	t.Setenv("DEVLORE_MODEL_NAME", "claude-sonnet-4-20250514")
	t.Setenv("DEVLORE_MODEL_API_KEY", "test-api-key")
	t.Setenv("DEVLORE_MODEL_ENDPOINT", "https://custom-endpoint.example.com")
	t.Setenv("DEVLORE_REGISTRY_URL", "https://custom-registry.example.com")
	t.Setenv("DEVLORE_REGISTRY_BRANCH", "main")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Verbosity != "verbose" {
		t.Errorf("expected verbosity 'verbose', got %q", cfg.Verbosity)
	}
	if !cfg.DryRun {
		t.Error("expected dry_run=true")
	}
	if cfg.Model.Provider != "anthropic" {
		t.Errorf("expected provider 'anthropic', got %q", cfg.Model.Provider)
	}
	if cfg.Model.Name != "claude-sonnet-4-20250514" {
		t.Errorf("expected model 'claude-sonnet-4-20250514', got %q", cfg.Model.Name)
	}
	if cfg.Model.APIKey != "test-api-key" {
		t.Errorf("expected api_key 'test-api-key', got %q", cfg.Model.APIKey)
	}
	if cfg.Model.Endpoint != "https://custom-endpoint.example.com" {
		t.Errorf("expected endpoint 'https://custom-endpoint.example.com', got %q", cfg.Model.Endpoint)
	}
	if cfg.Registry.URL != "https://custom-registry.example.com" {
		t.Errorf("expected registry URL 'https://custom-registry.example.com', got %q", cfg.Registry.URL)
	}
	if cfg.Registry.Branch != "main" {
		t.Errorf("expected registry branch 'main', got %q", cfg.Registry.Branch)
	}
}

func TestLoad_FromFile(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Create config file
	configDir := filepath.Join(tmpDir, "devlore")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("creating config dir: %v", err)
	}

	configContent := `
verbosity: quiet
dry_run: true
model:
  provider: groq
  name: llama-3.3-70b-versatile
registry:
  url: https://example.com/registry.git
  branch: main
writ:
  segments:
    - ROLE
    - SITE
`
	configPath := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("writing config file: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Verbosity != "quiet" {
		t.Errorf("expected verbosity 'quiet', got %q", cfg.Verbosity)
	}
	if !cfg.DryRun {
		t.Error("expected dry_run=true")
	}
	if cfg.Model.Provider != "groq" {
		t.Errorf("expected provider 'groq', got %q", cfg.Model.Provider)
	}
	if cfg.Model.Name != "llama-3.3-70b-versatile" {
		t.Errorf("expected model 'llama-3.3-70b-versatile', got %q", cfg.Model.Name)
	}
	if cfg.Registry.URL != "https://example.com/registry.git" {
		t.Errorf("expected registry URL 'https://example.com/registry.git', got %q", cfg.Registry.URL)
	}
	if cfg.Registry.Branch != "main" {
		t.Errorf("expected branch 'main', got %q", cfg.Registry.Branch)
	}
	if len(cfg.Writ.Segments) != 2 {
		t.Errorf("expected 2 writ segments, got %d", len(cfg.Writ.Segments))
	}
}

func TestLoad_EnvOverridesFile(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Create config file with one provider
	configDir := filepath.Join(tmpDir, "devlore")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("creating config dir: %v", err)
	}

	configContent := `
model:
  provider: ollama
  name: llama3.1:8b
`
	configPath := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("writing config file: %v", err)
	}

	// Set env to override
	t.Setenv("DEVLORE_MODEL_PROVIDER", "anthropic")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	// Env should override file
	if cfg.Model.Provider != "anthropic" {
		t.Errorf("expected provider 'anthropic' (from env), got %q", cfg.Model.Provider)
	}
	// Name should still come from file
	if cfg.Model.Name != "llama3.1:8b" {
		t.Errorf("expected model 'llama3.1:8b' (from file), got %q", cfg.Model.Name)
	}
}

func TestSave_RoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Create initial config
	cfg := &Config{
		Verbosity: "verbose",
		DryRun:    true,
		Model: ModelConfig{
			Provider: "groq",
			Name:     "llama-3.3-70b-versatile",
		},
		Registry: RegistryConfig{
			URL:    "https://example.com/registry.git",
			Branch: "develop",
		},
		Writ: WritConfig{
			Segments: []string{"ROLE"},
			Vars:     map[string]string{"ROLE": "desktop"},
		},
	}

	// Save
	if err := Save(cfg); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	// Load back
	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	// Verify
	if loaded.Verbosity != cfg.Verbosity {
		t.Errorf("verbosity mismatch: got %q, want %q", loaded.Verbosity, cfg.Verbosity)
	}
	if loaded.DryRun != cfg.DryRun {
		t.Errorf("dry_run mismatch: got %v, want %v", loaded.DryRun, cfg.DryRun)
	}
	if loaded.Model.Provider != cfg.Model.Provider {
		t.Errorf("provider mismatch: got %q, want %q", loaded.Model.Provider, cfg.Model.Provider)
	}
	if loaded.Model.Name != cfg.Model.Name {
		t.Errorf("model name mismatch: got %q, want %q", loaded.Model.Name, cfg.Model.Name)
	}
	if loaded.Registry.URL != cfg.Registry.URL {
		t.Errorf("registry URL mismatch: got %q, want %q", loaded.Registry.URL, cfg.Registry.URL)
	}
	if len(loaded.Writ.Segments) != len(cfg.Writ.Segments) {
		t.Errorf("writ.segments length mismatch: got %d, want %d", len(loaded.Writ.Segments), len(cfg.Writ.Segments))
	}
}

func TestSave_APIKeyNotWrittenToFile(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	cfg := &Config{
		Model: ModelConfig{
			Provider: "anthropic",
			Name:     "claude-sonnet-4-20250514",
			APIKey:   "secret-api-key",
		},
	}

	if err := Save(cfg); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	// Read raw file content
	configPath := filepath.Join(tmpDir, "devlore", "config.yaml")
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("reading config file: %v", err)
	}

	// API key should NOT be in the file
	if string(content) != "" && contains(string(content), "secret-api-key") {
		t.Error("API key should not be written to config file")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && (s[:len(substr)] == substr || contains(s[1:], substr)))
}

func TestModelConfig_WithDefaults(t *testing.T) {
	tests := []struct {
		name     string
		input    ModelConfig
		expected ModelConfig
	}{
		{
			name:  "empty config gets all defaults",
			input: ModelConfig{},
			expected: ModelConfig{
				Provider: DefaultModelProvider,
				Name:     DefaultModelName,
				Endpoint: DefaultModelEndpoint,
			},
		},
		{
			name: "provider set keeps provider, gets other defaults",
			input: ModelConfig{
				Provider: "anthropic",
			},
			expected: ModelConfig{
				Provider: "anthropic",
				Name:     DefaultModelName,
				Endpoint: "", // No default endpoint for non-ollama
			},
		},
		{
			name: "ollama with custom model keeps model",
			input: ModelConfig{
				Provider: "ollama",
				Name:     "codellama:7b",
			},
			expected: ModelConfig{
				Provider: "ollama",
				Name:     "codellama:7b",
				Endpoint: DefaultModelEndpoint,
			},
		},
		{
			name: "fully specified config unchanged",
			input: ModelConfig{
				Provider: "openai",
				Name:     "gpt-4",
				Endpoint: "https://custom.openai.com",
				APIKey:   "key",
			},
			expected: ModelConfig{
				Provider: "openai",
				Name:     "gpt-4",
				Endpoint: "https://custom.openai.com",
				APIKey:   "key",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.input.WithDefaults()
			if got.Provider != tt.expected.Provider {
				t.Errorf("Provider: got %q, want %q", got.Provider, tt.expected.Provider)
			}
			if got.Name != tt.expected.Name {
				t.Errorf("Name: got %q, want %q", got.Name, tt.expected.Name)
			}
			if got.Endpoint != tt.expected.Endpoint {
				t.Errorf("Endpoint: got %q, want %q", got.Endpoint, tt.expected.Endpoint)
			}
			if got.APIKey != tt.expected.APIKey {
				t.Errorf("APIKey: got %q, want %q", got.APIKey, tt.expected.APIKey)
			}
		})
	}
}

func TestRegistryConfig_WithDefaults(t *testing.T) {
	tests := []struct {
		name     string
		input    RegistryConfig
		expected RegistryConfig
	}{
		{
			name:  "empty config gets all defaults",
			input: RegistryConfig{},
			expected: RegistryConfig{
				URL:    DefaultRegistryURL,
				Branch: DefaultRegistryBranch,
			},
		},
		{
			name: "URL set keeps URL, gets branch default",
			input: RegistryConfig{
				URL: "https://custom.example.com/registry.git",
			},
			expected: RegistryConfig{
				URL:    "https://custom.example.com/registry.git",
				Branch: DefaultRegistryBranch,
			},
		},
		{
			name: "fully specified config unchanged",
			input: RegistryConfig{
				URL:       "https://custom.example.com/registry.git",
				Branch:    "main",
				ForceTags: true,
			},
			expected: RegistryConfig{
				URL:       "https://custom.example.com/registry.git",
				Branch:    "main",
				ForceTags: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.input.WithDefaults()
			if got.URL != tt.expected.URL {
				t.Errorf("URL: got %q, want %q", got.URL, tt.expected.URL)
			}
			if got.Branch != tt.expected.Branch {
				t.Errorf("Branch: got %q, want %q", got.Branch, tt.expected.Branch)
			}
			if got.ForceTags != tt.expected.ForceTags {
				t.Errorf("ForceTags: got %v, want %v", got.ForceTags, tt.expected.ForceTags)
			}
		})
	}
}
