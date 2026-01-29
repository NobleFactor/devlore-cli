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

// OllamaProvider implements Provider for local Ollama.
type OllamaProvider struct {
	endpoint string
	model    string
	client   *http.Client
}

// NewOllamaProvider creates an Ollama provider.
func NewOllamaProvider(endpoint, model string) *OllamaProvider {
	if endpoint == "" {
		endpoint = "http://localhost:11434"
	}
	if model == "" {
		model = "llama3.1:8b"
	}
	return &OllamaProvider{
		endpoint: endpoint,
		model:    model,
		client: &http.Client{
			Timeout: 5 * time.Minute, // LLM calls can be slow
		},
	}
}

// Name returns "ollama".
func (o *OllamaProvider) Name() string {
	return "ollama"
}

// Model returns the model identifier (e.g., "llama3.1:8b").
func (o *OllamaProvider) Model() string {
	return o.model
}

// Endpoint returns the Ollama API endpoint URL.
func (o *OllamaProvider) Endpoint() string {
	return o.endpoint
}

// Available checks if Ollama is running and the model is available.
func (o *OllamaProvider) Available(ctx context.Context) bool {
	// Check if Ollama is running
	req, err := http.NewRequestWithContext(ctx, "GET", o.endpoint+"/api/tags", nil)
	if err != nil {
		return false
	}

	resp, err := o.client.Do(req)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return false
	}

	// Check if model is available
	var tags ollamaTagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		return false
	}

	for _, m := range tags.Models {
		if m.Name == o.model {
			return true
		}
	}

	return false
}

// Chat sends a chat completion request to Ollama.
func (o *OllamaProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	// Build Ollama request
	ollamaReq := ollamaChatRequest{
		Model:  o.model,
		Stream: false,
		Options: ollamaOptions{
			Temperature: req.Temperature,
			NumPredict:  req.MaxTokens,
		},
	}

	// Add system message if present
	if req.SystemPrompt != "" {
		ollamaReq.Messages = append(ollamaReq.Messages, ollamaMessage{
			Role:    "system",
			Content: req.SystemPrompt,
		})
	}

	// Add conversation messages
	for _, msg := range req.Messages {
		ollamaReq.Messages = append(ollamaReq.Messages, ollamaMessage{
			Role:    string(msg.Role),
			Content: msg.Content,
		})
	}

	// JSON mode hint in system prompt if requested
	if req.JSONMode && req.SystemPrompt != "" {
		// Ollama doesn't have native JSON mode, but we can hint
		ollamaReq.Format = "json"
	}

	// Send request
	body, err := json.Marshal(ollamaReq)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", o.endpoint+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var ollamaResp ollamaChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &ChatResponse{
		Content:      ollamaResp.Message.Content,
		FinishReason: ollamaResp.DoneReason,
		TokensUsed:   ollamaResp.PromptEvalCount + ollamaResp.EvalCount,
	}, nil
}

// Ollama API types

type ollamaTagsResponse struct {
	Models []ollamaModel `json:"models"`
}

type ollamaModel struct {
	Name string `json:"name"`
}

type ollamaChatRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Stream   bool            `json:"stream"`
	Format   string          `json:"format,omitempty"`
	Options  ollamaOptions   `json:"options,omitempty"`
}

type ollamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaOptions struct {
	Temperature float64 `json:"temperature,omitempty"`
	NumPredict  int     `json:"num_predict,omitempty"`
}

type ollamaChatResponse struct {
	Message         ollamaMessage `json:"message"`
	Done            bool          `json:"done"`
	DoneReason      string        `json:"done_reason"`
	PromptEvalCount int           `json:"prompt_eval_count"`
	EvalCount       int           `json:"eval_count"`
}
