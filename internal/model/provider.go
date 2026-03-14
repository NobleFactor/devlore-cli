// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

/*
Package model provides an opaque LLM provider interface and configuration.

# Provider Interface

The Provider interface is the single abstraction for all AI/LLM backends.
Callers interact only through this interface—they don't need to know whether
the underlying provider is Anthropic, OpenAI, Ollama, etc.

	type Provider interface {
	    Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
	    ReceiverName() string       // Provider name (also keystore account)
	    Model() string      // Model identifier
	    Endpoint() string   // API endpoint (empty = default)
	    Available(ctx context.Context) bool
	}

The interface exposes queryable attributes (provider, model, endpoint) but
never exposes the API key for security.

# Provider Field Uses

The provider field in configuration determines:

 1. API Protocol: Anthropic API vs OpenAI-compatible API (different request/response formats)
 2. Authentication Method: "Authorization: Bearer" header vs "api-key" header (Azure)
 3. Default Endpoint: Each provider has a different default API URL
 4. Keystore Account: API keys stored under Service=com.noblefactor.DevLore, Account=<provider>
 5. Client Implementation: Which Provider implementation to instantiate

# Supported Providers

	| Provider     | Default Endpoint                                | Auth Header               |
	|--------------|------------------------------------------------|---------------------------|
	| anthropic    | https://api.anthropic.com/v1/messages          | x-api-key                 |
	| gemini       | https://generativelanguage.googleapis.com/v1beta| ?key= query param        |
	| github       | https://models.inference.ai.azure.com          | Authorization: Bearer     |
	| groq         | https://api.groq.com/openai/v1                 | Authorization: Bearer     |
	| ollama       | http://localhost:11434                         | (none)                    |
	| openai       | https://api.openai.com/v1                      | Authorization: Bearer     |

See config.go for full configuration reference including CLI flags,
environment variables, config file format, and resolution order.
*/
package model

import (
	"context"
	"fmt"

	"github.com/NobleFactor/devlore-cli/internal/config"
)

// Provider is the opaque interface for AI/LLM backends.
//
// Callers interact only through this interface—they don't need to know
// whether the underlying provider is Anthropic, OpenAI, Ollama, etc.
// The interface exposes queryable attributes (provider, model, endpoint)
// but never exposes the API key for security.
type Provider interface {
	// Chat sends messages and returns a response.
	Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)

	// Name returns the provider name (e.g., "ollama", "anthropic", "openai").
	// This is the same value used for keystore account lookup.
	Name() string

	// Model returns the model identifier (e.g., "llama3.1:8b", "claude-sonnet-4-20250514").
	Model() string

	// Endpoint returns the API endpoint URL.
	// Returns empty string if using provider's default endpoint.
	Endpoint() string

	// Available returns true if the provider is ready to use.
	// For cloud providers, this typically means the API key is configured.
	// For local providers (Ollama), this checks if the service is running.
	Available(ctx context.Context) bool
}

// ChatRequest represents a chat completion request.
type ChatRequest struct {
	SystemPrompt string    // System prompt (instructions)
	Messages     []Message // Conversation messages
	Temperature  float64   // 0.0 for deterministic, higher for creativity
	MaxTokens    int       // Maximum response tokens (0 = provider default)
	JSONMode     bool      // Request JSON output if supported
}

// Message represents a conversation turn.
type Message struct {
	Role    Role   // user, assistant
	Content string // Message content
}

// Role identifies the message sender.
type Role string

// Chat role constants.
const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// ChatResponse contains the AI response.
type ChatResponse struct {
	Content      string // Response text
	FinishReason string // "stop", "length", etc.
	TokensUsed   int    // Total tokens consumed
}

// NewProvider creates a provider from configuration.
func NewProvider(cfg config.ModelConfig) (Provider, error) {
	switch cfg.Provider {
	case "ollama":
		return NewOllamaProvider(cfg.Endpoint, cfg.Name), nil

	case "anthropic":
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("anthropic provider requires api_key")
		}
		return NewAnthropicProvider(cfg.APIKey, cfg.Name), nil

	case "openai":
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("openai provider requires api_key")
		}
		return NewOpenAIProvider(cfg.APIKey, cfg.Name, cfg.Endpoint), nil

	case "github":
		// GitHub Models uses OpenAI-compatible API
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("github provider requires api_key (use gh auth token or GITHUB_TOKEN)")
		}
		endpoint := cfg.Endpoint
		if endpoint == "" {
			endpoint = "https://models.inference.ai.azure.com"
		}
		model := cfg.Name
		if model == "" {
			model = "gpt-4o"
		}
		return NewOpenAIProvider(cfg.APIKey, model, endpoint), nil

	case "groq":
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("groq provider requires api_key")
		}
		return NewGroqProvider(cfg.APIKey, cfg.Name), nil

	case "gemini":
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("gemini provider requires api_key")
		}
		return NewGeminiProvider(cfg.APIKey, cfg.Name), nil

	default:
		return nil, fmt.Errorf("unknown provider: %s", cfg.Provider)
	}
}
