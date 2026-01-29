// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

/*
Package model provides LLM provider configuration and client implementations.

# Configuration Reference

Model configuration can be specified via CLI flags, environment variables,
config file, or native keystore (for API keys only).

## Resolution Order

For each field, the first source with a value wins:

  1. CLI flags (highest priority)
  2. Environment variables
  3. Config file (~/.config/devlore/config.yaml)
  4. Native keystore (API key only, lowest priority)

## Configuration Fields

	| Field        | CLI Flag           | Environment Variable       | Config Key     | Keystore         |
	|--------------|--------------------|-----------------------------|----------------|------------------|
	| model        | --model            | DEVLORE_MODEL              | model.model    | —                |
	| api-key      | --model-api-key    | DEVLORE_MODEL_API_KEY      | model.api-key  | Account=provider |
	| endpoint     | --model-endpoint   | DEVLORE_MODEL_ENDPOINT     | model.endpoint | —                |
	| provider     | --model-provider   | DEVLORE_MODEL_PROVIDER     | model.provider | —                |

## Provider Field

The provider field determines:

  - API protocol: Anthropic API vs OpenAI-compatible API (different request/response formats)
  - Authentication method: "Authorization: Bearer" header vs "api-key" header (Azure)
  - Default endpoint: Each provider has a different default API URL
  - Keystore account: API keys are stored under Service=com.noblefactor.DevLore, Account=<provider>
  - Client implementation: Which Provider implementation to instantiate

Supported providers: anthropic, azure-openai, github, ollama, openai

## Native Keystore

API keys are stored in the native OS keystore:

  - macOS: Keychain (security command)
  - Linux: libsecret (secret-tool command)
  - Windows: Credential Manager (PowerShell)

Keystore entry format:

	Service: com.noblefactor.DevLore
	Account: <provider>  (e.g., "anthropic", "openai")
	Key:     <api-key>

## Config File Example

	# ~/.config/devlore/config.yaml
	model:
	  model: claude-sonnet-4-20250514
	  provider: anthropic

API keys should be stored in the native keystore, not the config file.

## CLI Usage Examples

	# Using environment variables
	DEVLORE_MODEL_PROVIDER=github DEVLORE_MODEL_API_KEY=$(gh auth token) writ migrate --dry-run ~/dotfiles

	# Using CLI flags
	writ --model-provider=github --model-api-key=$(gh auth token) migrate --dry-run ~/dotfiles
*/
package model

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/term"
	"gopkg.in/yaml.v3"

	"github.com/NobleFactor/devlore-cli/internal/cli"
	"github.com/NobleFactor/devlore-cli/internal/credentials"
)

// Environment variables for model configuration (sorted alphabetically).
const (
	EnvModel         = "DEVLORE_MODEL"
	EnvModelAPIKey   = "DEVLORE_MODEL_API_KEY"
	EnvModelEndpoint = "DEVLORE_MODEL_ENDPOINT"
	EnvModelProvider = "DEVLORE_MODEL_PROVIDER"
)

// CLIFlags holds model configuration from command-line flags.
// Fields are sorted alphabetically to match documentation.
type CLIFlags struct {
	APIKey   string // --model-api-key
	Endpoint string // --model-endpoint
	Model    string // --model
	Provider string // --model-provider
}

// ApplyTo applies CLI flags to a config. CLI flags take highest priority.
func (f CLIFlags) ApplyTo(cfg *Config) {
	if f.APIKey != "" {
		cfg.APIKey = f.APIKey
	}
	if f.Endpoint != "" {
		cfg.Endpoint = f.Endpoint
	}
	if f.Model != "" {
		cfg.Model = f.Model
	}
	if f.Provider != "" {
		cfg.Provider = f.Provider
	}
}

// devloreConfig is the full config file structure.
type devloreConfig struct {
	Model modelConfig `yaml:"model"`
}

// modelConfig holds model provider configuration in the config file.
// Fields are sorted alphabetically.
type modelConfig struct {
	APIKey   string `yaml:"api-key"`  // API key (prefer native keystore)
	Endpoint string `yaml:"endpoint"` // Optional endpoint URL override
	Model    string `yaml:"model"`    // Model name (e.g., claude-sonnet-4-20250514)
	Provider string `yaml:"provider"` // Provider: anthropic, azure-openai, github, ollama, openai
}

// ConfigPath returns the path to the devlore config file.
func ConfigPath() (string, error) {
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		configHome = filepath.Join(home, ".config")
	}
	return filepath.Join(configHome, "devlore", "config.yaml"), nil
}

// credentialKey returns the keystore account name for an AI provider.
func credentialKey(provider string) string {
	return provider
}

// LoadConfig loads model configuration with consistent fallback:
//
//	CLI (handled by caller) → Environment → Config → Keystore
//
// First source with a value wins. If config has api-key, keystore is not checked.
func LoadConfig() (*Config, error) {
	// Start with environment variables
	provider := os.Getenv(EnvModelProvider)
	model := os.Getenv(EnvModel)
	endpoint := os.Getenv(EnvModelEndpoint)
	apiKey := os.Getenv(EnvModelAPIKey)

	// Load config file for any missing values
	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}

	var fileCfg devloreConfig
	data, err := os.ReadFile(path)
	if err == nil {
		if err := yaml.Unmarshal(data, &fileCfg); err != nil {
			return nil, fmt.Errorf("parsing config: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	// Fill missing values from config file
	if provider == "" {
		provider = fileCfg.Model.Provider
	}
	if model == "" {
		model = fileCfg.Model.Model
	}
	if endpoint == "" {
		endpoint = fileCfg.Model.Endpoint
	}
	if apiKey == "" {
		apiKey = fileCfg.Model.APIKey
	}

	// No provider configured
	if provider == "" {
		return nil, nil
	}

	// Keystore is last resort - only if nothing else provided api-key
	if apiKey == "" {
		apiKey, _ = credentials.Get(credentialKey(provider))
	}

	return &Config{
		Provider: provider,
		Model:    model,
		Endpoint: endpoint,
		APIKey:   apiKey,
	}, nil
}

// SaveConfig saves model configuration to the config file.
// API keys are stored in the native keystore, not the config file.
func SaveConfig(cfg *Config) error {
	// Store API key in native keystore (if provided and not Ollama)
	if cfg.APIKey != "" && cfg.Provider != "ollama" {
		if err := credentials.Set(credentialKey(cfg.Provider), cfg.APIKey); err != nil {
			cli.Warn("could not store API key in keystore: %v", err)
		}
	}

	// Save config (without API key - it's in keystore)
	path, err := ConfigPath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	// Load existing config to preserve other sections
	var existing devloreConfig
	data, err := os.ReadFile(path)
	if err == nil {
		_ = yaml.Unmarshal(data, &existing)
	}

	// Update model section
	existing.Model = modelConfig{
		Provider: cfg.Provider,
		Model:    cfg.Model,
		Endpoint: cfg.Endpoint,
	}

	data, err = yaml.Marshal(&existing)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// EnsureProvider returns a configured AI provider, prompting the user if needed.
// If noAI is true, returns nil without prompting.
//
// Lookup priority:
//   - CLI flags (passed via cliFlags parameter)
//   - Environment: DEVLORE_AI_PROVIDER, DEVLORE_AI_API_KEY, etc.
//   - Config file: ~/.config/devlore/config.yaml
//   - API key from native keystore
//   - Fallback to Ollama if available locally
//   - Interactive prompt
func EnsureProvider(ctx context.Context, noAI bool, cliFlags CLIFlags) (Provider, error) {
	if noAI {
		return nil, nil
	}

	// 1. Try loading config (env > config file, API key from env > config > keystore)
	cfg, err := LoadConfig()
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}

	// Initialize empty config if nil
	if cfg == nil {
		cfg = &Config{}
	}

	// 2. Apply CLI flags (highest priority)
	cliFlags.ApplyTo(cfg)

	if cfg.Provider != "" {
		// Config exists, create provider
		provider, err := NewProvider(*cfg)
		if err != nil {
			return nil, err
		}

		// Check if provider is available
		if provider.Available(ctx) {
			return provider, nil
		}

		// Provider configured but not available
		cli.Error("AI provider %q configured but not available.", cfg.Provider)
		if cfg.Provider == "ollama" {
			cli.Note("Is Ollama running? Start with: ollama serve")
			cli.Note("Is model pulled? Run: ollama pull %s", cfg.Model)
		} else if cfg.APIKey == "" {
			cli.Note("No API key found. Set DEVLORE_AI_API_KEY or store in keystore.")
		}
	}

	// 3. Fallback: Check if Ollama is available locally
	ollamaCfg := DefaultConfig()
	ollamaProvider := NewOllamaProvider(ollamaCfg.Endpoint, ollamaCfg.Model)
	if ollamaProvider.Available(ctx) {
		cli.Note("Using Ollama (detected locally)")
		return ollamaProvider, nil
	}

	// 4. No provider available - prompt user or fail
	return promptForProvider(ctx)
}

// promptForProvider interactively configures an AI provider.
// In non-interactive mode (no TTY), defaults to Ollama.
func promptForProvider(ctx context.Context) (Provider, error) {
	// Non-interactive: default to Ollama
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		cli.Note("No AI provider configured. Defaulting to Ollama.")
		cfg := DefaultConfig()
		provider := NewOllamaProvider(cfg.Endpoint, cfg.Model)
		if !provider.Available(ctx) {
			return nil, fmt.Errorf("Ollama not available; install from https://ollama.ai, run 'ollama serve', then 'ollama pull %s'", cfg.Model)
		}
		if err := SaveConfig(&cfg); err != nil {
			cli.Warn("could not save config: %v", err)
		}
		return provider, nil
	}

	reader := bufio.NewReader(os.Stdin)

	cli.Note("AI features require a provider. Options:")
	cli.Note("")
	cli.Note("  [1] Ollama (local, free, private) <- Recommended")
	cli.Note("      Install: https://ollama.ai")
	cli.Note("      Then: ollama pull llama3.1:8b")
	cli.Note("")
	cli.Note("  [2] Anthropic Claude (cloud, requires API key)")
	cli.Note("")
	cli.Note("  [3] OpenAI (cloud, requires API key)")
	cli.Note("")
	fmt.Fprint(os.Stderr, "Choice [1/2/3]: ")

	input, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	choice := strings.TrimSpace(input)

	var cfg Config

	switch choice {
	case "1", "":
		cfg = DefaultConfig() // Ollama

		// Check if Ollama is available
		provider := NewOllamaProvider(cfg.Endpoint, cfg.Model)
		if !provider.Available(ctx) {
			cli.Error("Ollama not detected. Please:")
			cli.Note("  1. Install Ollama: https://ollama.ai")
			cli.Note("  2. Start Ollama: ollama serve")
			cli.Note("  3. Pull model: ollama pull llama3.1:8b")
			cli.Note("  4. Re-run this command")
			return nil, fmt.Errorf("ollama not available")
		}

		// Save config
		if err := SaveConfig(&cfg); err != nil {
			cli.Warn("could not save config: %v", err)
		}
		return provider, nil

	case "2":
		fmt.Fprint(os.Stderr, "Anthropic API key: ")
		apiKey, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		apiKey = strings.TrimSpace(apiKey)

		cfg = Config{
			Provider: "anthropic",
			Model:    "claude-sonnet-4-20250514",
			APIKey:   apiKey,
		}

		if err := SaveConfig(&cfg); err != nil {
			cli.Warn("could not save config: %v", err)
		}

		provider, err := NewProvider(cfg)
		if err != nil {
			return nil, err
		}
		return provider, nil

	case "3":
		fmt.Fprint(os.Stderr, "OpenAI API key: ")
		apiKey, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		apiKey = strings.TrimSpace(apiKey)

		cfg = Config{
			Provider: "openai",
			Model:    "gpt-4-turbo",
			APIKey:   apiKey,
		}

		if err := SaveConfig(&cfg); err != nil {
			cli.Warn("could not save config: %v", err)
		}

		provider, err := NewProvider(cfg)
		if err != nil {
			return nil, err
		}
		return provider, nil

	case "4":
		cli.Note("Skipping AI setup.")
		cli.Note("Run 'lore config ai' later to configure a provider.")
		return nil, nil

	default:
		return nil, fmt.Errorf("invalid choice: %s", choice)
	}
}
