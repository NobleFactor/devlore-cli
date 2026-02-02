// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package model

import (
	"context"
	"net/http"
	"time"
)

// GroqProvider implements Provider for Groq's OpenAI-compatible API.
// Groq provides extremely fast inference using custom LPU hardware.
type GroqProvider struct {
	*OpenAIProvider
	name string
}

// NewGroqProvider creates a Groq provider.
// Groq uses OpenAI-compatible API at api.groq.com.
func NewGroqProvider(apiKey, model string) *GroqProvider {
	if model == "" {
		model = "llama-3.3-70b-versatile"
	}
	return &GroqProvider{
		OpenAIProvider: &OpenAIProvider{
			apiKey:   apiKey,
			model:    model,
			endpoint: "https://api.groq.com/openai/v1",
			client: &http.Client{
				Timeout: 5 * time.Minute,
			},
		},
		name: "groq",
	}
}

// Name returns "groq" for keystore lookup.
func (g *GroqProvider) Name() string {
	return g.name
}

// Available checks if the API key is set.
func (g *GroqProvider) Available(ctx context.Context) bool {
	return g.apiKey != ""
}
