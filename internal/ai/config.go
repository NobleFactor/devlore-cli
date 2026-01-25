// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

package ai

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// devloreConfig is the full config file structure.
// We only care about the ai-provider section.
type devloreConfig struct {
	Lore loreConfig `yaml:"lore"`
	Writ writConfig `yaml:"writ"`
}

type loreConfig struct {
	AIProvider aiProviderConfig `yaml:"ai-provider"`
}

type writConfig struct {
	AIProvider aiProviderConfig `yaml:"ai-provider"`
}

type aiProviderConfig struct {
	Model modelConfig `yaml:"model"`
}

type modelConfig struct {
	Name     string `yaml:"name"`
	Provider string `yaml:"provider"`
	Endpoint string `yaml:"endpoint"`
	APIKey   string `yaml:"api-key"`
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

// LoadConfig loads AI configuration from the config file.
// Returns nil config (not error) if no config exists.
func LoadConfig() (*Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No config yet
		}
		return nil, err
	}

	var cfg devloreConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	// Try writ config first, fall back to lore config
	model := cfg.Writ.AIProvider.Model
	if model.Provider == "" {
		model = cfg.Lore.AIProvider.Model
	}

	if model.Provider == "" {
		return nil, nil // AI not configured
	}

	// Expand environment variables in API key
	apiKey := model.APIKey
	if strings.HasPrefix(apiKey, "${") && strings.HasSuffix(apiKey, "}") {
		envVar := apiKey[2 : len(apiKey)-1]
		apiKey = os.Getenv(envVar)
	}

	return &Config{
		Provider: model.Provider,
		Model:    model.Name,
		Endpoint: model.Endpoint,
		APIKey:   apiKey,
	}, nil
}

// SaveConfig saves AI configuration to the config file.
func SaveConfig(cfg *Config) error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	// Load existing config or create new
	var existing devloreConfig
	data, err := os.ReadFile(path)
	if err == nil {
		_ = yaml.Unmarshal(data, &existing)
	}

	// Update AI provider section
	existing.Lore.AIProvider.Model = modelConfig{
		Name:     cfg.Model,
		Provider: cfg.Provider,
		Endpoint: cfg.Endpoint,
		APIKey:   cfg.APIKey,
	}

	data, err = yaml.Marshal(&existing)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600) // Restrictive permissions for API keys
}

// EnsureProvider returns a configured AI provider, prompting the user if needed.
// If noAI is true, returns nil without prompting.
func EnsureProvider(ctx context.Context, noAI bool) (Provider, error) {
	if noAI {
		return nil, nil
	}

	// Try loading existing config
	cfg, err := LoadConfig()
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}

	if cfg != nil {
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
		fmt.Fprintf(os.Stderr, "AI provider %q configured but not available.\n", cfg.Provider)
		if cfg.Provider == "ollama" {
			fmt.Fprintf(os.Stderr, "Is Ollama running? Start with: ollama serve\n")
			fmt.Fprintf(os.Stderr, "Is model pulled? Run: ollama pull %s\n\n", cfg.Model)
		}
	}

	// No config or unavailable - prompt user
	return promptForProvider(ctx)
}

// promptForProvider interactively configures an AI provider.
func promptForProvider(ctx context.Context) (Provider, error) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "AI features require a provider. Options:")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "  [1] Ollama (local, free, private) ← Recommended")
	fmt.Fprintln(os.Stderr, "      Install: https://ollama.ai")
	fmt.Fprintln(os.Stderr, "      Then: ollama pull llama3.1:8b")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "  [2] Anthropic Claude (cloud, requires API key)")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "  [3] OpenAI (cloud, requires API key)")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "  [4] Skip AI features (basic mode)")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprint(os.Stderr, "Choice [1/2/3/4]: ")

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
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintln(os.Stderr, "Ollama not detected. Please:")
			fmt.Fprintln(os.Stderr, "  1. Install Ollama: https://ollama.ai")
			fmt.Fprintln(os.Stderr, "  2. Start Ollama: ollama serve")
			fmt.Fprintln(os.Stderr, "  3. Pull model: ollama pull llama3.1:8b")
			fmt.Fprintln(os.Stderr, "  4. Re-run this command")
			fmt.Fprintln(os.Stderr, "")
			return nil, fmt.Errorf("ollama not available")
		}

		// Save config
		if err := SaveConfig(&cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not save config: %v\n", err)
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
			fmt.Fprintf(os.Stderr, "Warning: could not save config: %v\n", err)
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
			fmt.Fprintf(os.Stderr, "Warning: could not save config: %v\n", err)
		}

		provider, err := NewProvider(cfg)
		if err != nil {
			return nil, err
		}
		return provider, nil

	case "4":
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Running in basic mode (no AI features).")
		fmt.Fprintln(os.Stderr, "Re-run without --no-ai to configure AI later.")
		return nil, nil

	default:
		return nil, fmt.Errorf("invalid choice: %s", choice)
	}
}
