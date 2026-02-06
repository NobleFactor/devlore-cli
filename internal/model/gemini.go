// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package model

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// GeminiProvider implements Provider for Google's Gemini API.
type GeminiProvider struct {
	apiKey   string
	model    string
	endpoint string
	client   *http.Client
}

// NewGeminiProvider creates a Gemini provider.
func NewGeminiProvider(apiKey, model string) *GeminiProvider {
	if model == "" {
		model = "gemini-2.5-flash"
	}
	return &GeminiProvider{
		apiKey:   apiKey,
		model:    model,
		endpoint: "https://generativelanguage.googleapis.com/v1beta",
		client: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}
}

// Name returns "gemini" for keystore lookup.
func (g *GeminiProvider) Name() string {
	return "gemini"
}

// Model returns the model identifier.
func (g *GeminiProvider) Model() string {
	return g.model
}

// Endpoint returns the API endpoint URL.
func (g *GeminiProvider) Endpoint() string {
	return g.endpoint
}

// Available checks if the API key is set.
func (g *GeminiProvider) Available(ctx context.Context) bool {
	return g.apiKey != ""
}

// Chat sends a chat completion request to Gemini.
func (g *GeminiProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	// Build Gemini request
	geminiReq := geminiRequest{
		GenerationConfig: geminiGenerationConfig{
			Temperature:     req.Temperature,
			MaxOutputTokens: req.MaxTokens,
		},
	}

	// Add system instruction if present
	if req.SystemPrompt != "" {
		geminiReq.SystemInstruction = &geminiContent{
			Parts: []geminiPart{{Text: req.SystemPrompt}},
		}
	}

	// Add messages as contents
	for _, msg := range req.Messages {
		role := "user"
		if msg.Role == RoleAssistant {
			role = "model"
		}
		geminiReq.Contents = append(geminiReq.Contents, geminiContent{
			Role:  role,
			Parts: []geminiPart{{Text: msg.Content}},
		})
	}

	// Request JSON output if specified
	if req.JSONMode {
		geminiReq.GenerationConfig.ResponseMimeType = "application/json"
	}

	body, err := json.Marshal(geminiReq)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	// Gemini uses API key as query parameter
	url := fmt.Sprintf("%s/models/%s:generateContent?key=%s", g.endpoint, g.model, g.apiKey)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gemini error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var geminiResp geminiResponse
	if err := json.NewDecoder(resp.Body).Decode(&geminiResp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	if len(geminiResp.Candidates) == 0 {
		return nil, fmt.Errorf("no candidates in response")
	}

	candidate := geminiResp.Candidates[0]
	content := ""
	if len(candidate.Content.Parts) > 0 {
		content = candidate.Content.Parts[0].Text
	}

	return &ChatResponse{
		Content:      content,
		FinishReason: candidate.FinishReason,
		TokensUsed:   geminiResp.UsageMetadata.TotalTokenCount,
	}, nil
}

// Gemini API types

type geminiRequest struct {
	Contents          []geminiContent        `json:"contents"`
	SystemInstruction *geminiContent         `json:"systemInstruction,omitempty"`
	GenerationConfig  geminiGenerationConfig `json:"generationConfig"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiGenerationConfig struct {
	Temperature      float64 `json:"temperature,omitempty"`
	MaxOutputTokens  int     `json:"maxOutputTokens,omitempty"`
	ResponseMimeType string  `json:"responseMimeType,omitempty"`
}

type geminiResponse struct {
	Candidates    []geminiCandidate   `json:"candidates"`
	UsageMetadata geminiUsageMetadata `json:"usageMetadata"`
}

type geminiCandidate struct {
	Content      geminiContent `json:"content"`
	FinishReason string        `json:"finishReason"`
}

type geminiUsageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}
