// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package model

import (
	"context"
	"testing"

	"github.com/NobleFactor/devlore-cli/internal/config"
)

func TestModelConfigDefaults(t *testing.T) {
	cfg := config.ModelConfig{}.WithDefaults()

	if cfg.Provider != "ollama" {
		t.Errorf("expected provider 'ollama', got %q", cfg.Provider)
	}
	if cfg.Name != "llama3.1:8b" {
		t.Errorf("expected model 'llama3.1:8b', got %q", cfg.Name)
	}
	if cfg.Endpoint != "http://localhost:11434" {
		t.Errorf("expected endpoint 'http://localhost:11434', got %q", cfg.Endpoint)
	}
}

func TestNewProvider_Ollama(t *testing.T) {
	cfg := config.ModelConfig{}.WithDefaults()
	provider, err := NewProvider(cfg)
	if err != nil {
		t.Fatalf("NewProvider() error: %v", err)
	}
	if provider.Name() != "ollama" {
		t.Errorf("expected name 'ollama', got %q", provider.Name())
	}
}

func TestNewProvider_Anthropic(t *testing.T) {
	cfg := config.ModelConfig{
		Provider: "anthropic",
		Name:     "claude-sonnet-4-20250514",
		APIKey:   "test-key",
	}
	provider, err := NewProvider(cfg)
	if err != nil {
		t.Fatalf("NewProvider() error: %v", err)
	}
	if provider.Name() != "anthropic" {
		t.Errorf("expected name 'anthropic', got %q", provider.Name())
	}
}

func TestNewProvider_Anthropic_NoKey(t *testing.T) {
	cfg := config.ModelConfig{
		Provider: "anthropic",
		Name:     "claude-sonnet-4-20250514",
	}
	_, err := NewProvider(cfg)
	if err == nil {
		t.Error("expected error for missing API key")
	}
}

func TestNewProvider_OpenAI(t *testing.T) {
	cfg := config.ModelConfig{
		Provider: "openai",
		Name:     "gpt-4-turbo",
		APIKey:   "test-key",
	}
	provider, err := NewProvider(cfg)
	if err != nil {
		t.Fatalf("NewProvider() error: %v", err)
	}
	if provider.Name() != "openai" {
		t.Errorf("expected name 'openai', got %q", provider.Name())
	}
}

func TestNewProvider_Unknown(t *testing.T) {
	cfg := config.ModelConfig{
		Provider: "unknown",
	}
	_, err := NewProvider(cfg)
	if err == nil {
		t.Error("expected error for unknown provider")
	}
}

func TestOllamaProvider_Available_NotRunning(t *testing.T) {
	// Ollama not running on a random port
	provider := NewOllamaProvider("http://localhost:19999", "test")
	ctx := context.Background()

	if provider.Available(ctx) {
		t.Error("expected Available() = false when Ollama not running")
	}
}
