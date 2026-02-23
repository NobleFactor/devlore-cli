// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package config

// Default model settings (Ollama local inference).
const (
	DefaultModelProvider = "ollama"
	DefaultModelName     = "llama3.1:8b"
	DefaultModelEndpoint = "http://localhost:11434"
)

// ModelConfig configures the AI/LLM provider.
// This is shared across lore and writ for AI-assisted features.
type ModelConfig struct {
	Provider string `yaml:"provider" json:"provider"`                     // ollama, anthropic, openai, groq, gemini, github
	Name     string `yaml:"name" json:"name"`                             // Model identifier (e.g., claude-sonnet-4-20250514)
	Endpoint string `yaml:"endpoint,omitempty" json:"endpoint,omitempty"` // Custom endpoint URL
	APIKey   string `yaml:"api_key,omitempty" json:"api_key,omitempty"` //nolint:gosec // G117: field name is intentional
}

// WithDefaults returns a copy of the config with defaults applied.
func (c ModelConfig) WithDefaults() ModelConfig {
	cfg := c
	if cfg.Provider == "" {
		cfg.Provider = DefaultModelProvider
	}
	if cfg.Name == "" {
		cfg.Name = DefaultModelName
	}
	if cfg.Endpoint == "" && cfg.Provider == "ollama" {
		cfg.Endpoint = DefaultModelEndpoint
	}
	return cfg
}
