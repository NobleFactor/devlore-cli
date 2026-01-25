// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

// Package ai provides AI provider abstraction and lazy configuration.
// All AI operations go through this package, which handles provider
// selection, configuration prompting, and LLM calls.
package ai

import (
	"context"
	"fmt"
)

// Provider is the interface for AI backends.
type Provider interface {
	// Chat sends messages and returns a response.
	Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)

	// Name returns the provider name (e.g., "ollama", "anthropic").
	Name() string

	// Available returns true if the provider is ready to use.
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

// Config holds AI provider configuration per ADR-017.
type Config struct {
	Provider string `yaml:"provider"` // ollama, anthropic, openai, azure-openai
	Model    string `yaml:"model"`    // Model name (e.g., "llama3.1:8b", "claude-sonnet-4-20250514")
	Endpoint string `yaml:"endpoint"` // API endpoint (optional, for custom/azure)
	APIKey   string `yaml:"api_key"`  // API key (for cloud providers)
}

// DefaultConfig returns the default configuration (Ollama).
func DefaultConfig() Config {
	return Config{
		Provider: "ollama",
		Model:    "llama3.1:8b",
		Endpoint: "http://localhost:11434",
	}
}

// NewProvider creates a provider from configuration.
func NewProvider(cfg Config) (Provider, error) {
	switch cfg.Provider {
	case "ollama":
		return NewOllamaProvider(cfg.Endpoint, cfg.Model), nil
	case "anthropic":
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("anthropic provider requires api_key")
		}
		return NewAnthropicProvider(cfg.APIKey, cfg.Model), nil
	case "openai":
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("openai provider requires api_key")
		}
		return NewOpenAIProvider(cfg.APIKey, cfg.Model, cfg.Endpoint), nil
	default:
		return nil, fmt.Errorf("unknown provider: %s", cfg.Provider)
	}
}
