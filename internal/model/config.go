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
	| name         | --model            | DEVLORE_MODEL              | model.name     | —                |
	| api_key      | --model-api-key    | DEVLORE_MODEL_API_KEY      | model.api_key  | Account=provider |
	| endpoint     | --model-endpoint   | DEVLORE_MODEL_ENDPOINT     | model.endpoint | —                |
	| provider     | --model-provider   | DEVLORE_MODEL_PROVIDER     | model.provider | —                |

## Provider Field

The provider field determines:

  - API protocol: Anthropic API vs OpenAI-compatible API (different request/response formats)
  - Authentication method: "Authorization: Bearer" header vs "api-key" header (Azure)
  - Default endpoint: Each provider has a different default API URL
  - Keystore account: API keys are stored under Service=com.noblefactor.DevLore, Account=<provider>
  - Client implementation: Which Provider implementation to instantiate

Supported providers: anthropic, gemini, github, groq, ollama, openai

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
	  name: claude-sonnet-4-20250514
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
	"strings"

	"golang.org/x/term"

	"github.com/NobleFactor/devlore-cli/internal/cli"
	"github.com/NobleFactor/devlore-cli/internal/config"
	"github.com/NobleFactor/devlore-cli/internal/credentials"
)

// CLIFlags holds model configuration from command-line flags.
// Fields are sorted alphabetically to match documentation.
type CLIFlags struct {
	APIKey   string //nolint:gosec // G101: field name is intentional, not a credential
	Endpoint string // --model-endpoint
	Model    string // --model
	Provider string // --model-provider
}

// ApplyTo applies CLI flags to a config. CLI flags take highest priority.
func (f CLIFlags) ApplyTo(cfg *config.ModelConfig) {
	if f.APIKey != "" {
		cfg.APIKey = f.APIKey
	}
	if f.Endpoint != "" {
		cfg.Endpoint = f.Endpoint
	}
	if f.Model != "" {
		cfg.Name = f.Model
	}
	if f.Provider != "" {
		cfg.Provider = f.Provider
	}
}

// autoDetectProvider checks for common API key environment variables and keystore entries.
// Returns the first available provider, prioritizing by cost (cheapest first).
func autoDetectProvider(ctx context.Context) Provider {
	// Check providers in order of preference (cost/speed)
	providerChecks := []struct {
		name   string
		envVar string
		model  string
	}{
		{"groq", "GROQ_API_KEY", "llama-3.3-70b-versatile"},
		{"gemini", "GEMINI_API_KEY", "gemini-2.5-flash"},
		{"openai", "OPENAI_API_KEY", "gpt-4o"},
		{"anthropic", "ANTHROPIC_API_KEY", "claude-sonnet-4-20250514"},
	}

	for _, check := range providerChecks {
		// Check environment variable first
		apiKey := os.Getenv(check.envVar)

		// Fall back to keystore
		if apiKey == "" {
			apiKey, _ = credentials.Get(check.name) //nolint:errcheck // fallback: continue without value
		}

		if apiKey != "" {
			cfg := config.ModelConfig{
				Provider: check.name,
				Name:     check.model,
				APIKey:   apiKey,
			}
			provider, err := NewProvider(cfg)
			if err != nil {
				continue
			}
			if provider.Available(ctx) {
				cli.Note("Using %s (auto-detected from %s)", check.name, check.envVar)
				return provider
			}
		}
	}

	return nil
}

// EnsureProvider returns a configured AI provider, prompting the user if needed.
// If interactive is false, fails immediately if no provider is configured.
//
// Lookup priority:
//  1. CLI flags (passed via cliFlags parameter)
//  2. Environment: DEVLORE_MODEL_PROVIDER, DEVLORE_MODEL_API_KEY, etc.
//  3. Config file: ~/.config/devlore/config.yaml
//  4. API key from native keystore (for configured provider)
//  5. Auto-detect from common API key env vars (GROQ_API_KEY, GEMINI_API_KEY, etc.)
//  6. Auto-detect from native keystore entries
//  7. Fallback to Ollama if available locally
//  8. Interactive prompt (only if interactive=true)
func EnsureProvider(ctx context.Context, interactive bool, cliFlags CLIFlags) (Provider, error) {
	// Load config (handles env vars, config file, and keystore)
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}

	// Apply CLI flags (highest priority)
	cliFlags.ApplyTo(&cfg.Model)

	if cfg.Model.Provider != "" {
		// Config exists, create provider
		provider, err := NewProvider(cfg.Model)
		if err != nil {
			return nil, err
		}

		// Check if provider is available
		if provider.Available(ctx) {
			return provider, nil
		}

		// Provider configured but not available
		cli.Error("AI provider %q configured but not available.", cfg.Model.Provider)
		if cfg.Model.Provider == "ollama" {
			cli.Note("Is Ollama running? Start with: ollama serve")
			cli.Note("Is model pulled? Run: ollama pull %s", cfg.Model.Name)
		} else if cfg.Model.APIKey == "" {
			cli.Note("No API key found. Set DEVLORE_MODEL_API_KEY or store in keystore.")
		}
	}

	// Auto-detect from common environment variables
	if provider := autoDetectProvider(ctx); provider != nil {
		return provider, nil
	}

	// Fallback: Check if Ollama is available locally
	ollamaCfg := config.ModelConfig{}.WithDefaults()
	ollamaProvider := NewOllamaProvider(ollamaCfg.Endpoint, ollamaCfg.Name)
	if ollamaProvider.Available(ctx) {
		cli.Note("Using Ollama (detected locally)")
		return ollamaProvider, nil
	}

	// No provider available - fail in non-interactive mode, prompt otherwise
	if !interactive {
		return nil, fmt.Errorf("no AI provider configured; set DEVLORE_MODEL_PROVIDER and DEVLORE_MODEL_API_KEY, or use --model-provider and --model-api-key flags")
	}
	return promptForProvider(ctx)
}

// promptForProvider interactively configures an AI provider.
//
// Without a terminal on stdin the prompt cannot run, and configuration falls through to the
// Ollama defaults via [nonInteractiveOllama]. Choice "5" skips setup, returning a nil provider
// with a nil error.
//
// Parameters:
//   - `ctx`: the context governing the availability probe.
//
// Returns:
//   - `Provider`: the configured provider; nil when setup is skipped.
//   - `error`: non-nil on a failed read, an unrecognized choice, or a construction failure.
func promptForProvider(ctx context.Context) (Provider, error) {

	if !term.IsTerminal(int(os.Stdin.Fd())) { //nolint:gosec // G115: terminal width fits in int
		return nonInteractiveOllama(ctx)
	}

	reader := bufio.NewReader(os.Stdin)
	printProviderMenu()

	input, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	choice := strings.TrimSpace(input)

	if choice == "5" {
		cli.Note("Skipping AI setup.")
		cli.Note("Run 'lore config ai' later to configure a provider.")
		cli.Note("For local inference, install Ollama: https://ollama.ai")
		return nil, nil
	}

	option, known := providerChoices[choice]
	if !known {
		return nil, fmt.Errorf("invalid choice: %s", choice)
	}

	apiKey, err := readAPIKey(reader, option.KeyPrompt)
	if err != nil {
		return nil, err
	}

	modelCfg := config.ModelConfig{Provider: option.Provider, Name: option.Model, APIKey: apiKey}
	saveModelConfig(modelCfg)

	return NewProvider(modelCfg)
}

// providerChoice describes one provider on the interactive menu.
type providerChoice struct {
	KeyPrompt string
	Provider  string
	Model     string
}

// providerChoices maps a menu selection to the provider it configures. The empty selection
// aliases "1", so pressing Enter takes the recommended entry.
var providerChoices = map[string]providerChoice{
	"":  {KeyPrompt: "Groq API key: ", Provider: "groq", Model: "llama-3.3-70b-versatile"},
	"1": {KeyPrompt: "Groq API key: ", Provider: "groq", Model: "llama-3.3-70b-versatile"},
	"2": {KeyPrompt: "Gemini API key: ", Provider: "gemini", Model: "gemini-2.5-flash"},
	"3": {KeyPrompt: "Anthropic API key: ", Provider: "anthropic", Model: "claude-sonnet-4-20250514"},
	"4": {KeyPrompt: "OpenAI API key: ", Provider: "openai", Model: "gpt-4o-mini"},
}

// nonInteractiveOllama configures the Ollama defaults when no terminal is attached.
//
// Parameters:
//   - `ctx`: the context governing the availability probe.
//
// Returns:
//   - `Provider`: the Ollama provider, verified reachable.
//   - `error`: non-nil when Ollama is not serving.
func nonInteractiveOllama(ctx context.Context) (Provider, error) {

	cli.Note("No AI provider configured. Defaulting to Ollama.")
	modelCfg := config.ModelConfig{}.WithDefaults()
	provider := NewOllamaProvider(modelCfg.Endpoint, modelCfg.Name)
	if !provider.Available(ctx) {
		return nil, fmt.Errorf("ollama not available; install from https://ollama.ai, run 'ollama serve', then 'ollama pull %s'", modelCfg.Name)
	}

	saveModelConfig(modelCfg)

	return provider, nil
}

// printProviderMenu prints the provider menu and the choice prompt.
func printProviderMenu() {

	cli.Note("AI features require a provider. Options:")
	cli.Note("")
	cli.Note("  [1] Groq (cloud, fast, free tier) <- Recommended")
	cli.Note("      Get API key: https://console.groq.com")
	cli.Note("")
	cli.Note("  [2] Gemini (cloud, cheap)")
	cli.Note("      Get API key: https://aistudio.google.com/apikey")
	cli.Note("")
	cli.Note("  [3] Anthropic Claude (cloud, high quality)")
	cli.Note("      Get API key: https://console.anthropic.com")
	cli.Note("")
	cli.Note("  [4] OpenAI (cloud, GPT models)")
	cli.Note("      Get API key: https://platform.openai.com")
	cli.Note("")
	_, _ = fmt.Fprint(os.Stderr, "Choice [1/2/3/4]: ")
}

// readAPIKey prompts for and reads one API key.
//
// Parameters:
//   - `reader`: the input source.
//   - `prompt`: the prompt text, written to standard error.
//
// Returns:
//   - `string`: the key, whitespace-trimmed.
//   - `error`: non-nil when the read fails.
func readAPIKey(reader *bufio.Reader, prompt string) (string, error) {

	_, _ = fmt.Fprint(os.Stderr, prompt)
	apiKey, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(apiKey), nil
}

// saveModelConfig persists the model configuration, warning rather than failing when the
// surrounding config cannot be loaded or saved.
//
// Parameters:
//   - `modelCfg`: the model configuration to persist.
func saveModelConfig(modelCfg config.ModelConfig) {

	cfg, _ := config.Load() //nolint:errcheck // fallback: continue without value
	cfg.Model = modelCfg
	if err := config.Save(cfg); err != nil {
		cli.Warn("could not save config: %v", err)
	}
}
