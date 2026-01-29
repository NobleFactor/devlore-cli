// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

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

// OpenAIProvider implements Provider for OpenAI and compatible APIs.
type OpenAIProvider struct {
	apiKey   string
	model    string
	endpoint string
	client   *http.Client
}

// NewOpenAIProvider creates an OpenAI provider.
// endpoint can be empty for api.openai.com, or set for Azure/compatible APIs.
func NewOpenAIProvider(apiKey, model, endpoint string) *OpenAIProvider {
	if model == "" {
		model = "gpt-4-turbo"
	}
	if endpoint == "" {
		endpoint = "https://api.openai.com/v1"
	}
	return &OpenAIProvider{
		apiKey:   apiKey,
		model:    model,
		endpoint: endpoint,
		client: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}
}

// Name returns "openai".
func (o *OpenAIProvider) Name() string {
	return "openai"
}

// Model returns the model identifier (e.g., "gpt-4-turbo").
func (o *OpenAIProvider) Model() string {
	return o.model
}

// Endpoint returns the API endpoint URL.
func (o *OpenAIProvider) Endpoint() string {
	return o.endpoint
}

// Available checks if the API key is set.
func (o *OpenAIProvider) Available(ctx context.Context) bool {
	return o.apiKey != ""
}

// Chat sends a chat completion request to OpenAI.
func (o *OpenAIProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	// Build OpenAI request
	openaiReq := openaiRequest{
		Model:       o.model,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
	}

	// Add system message if present
	if req.SystemPrompt != "" {
		openaiReq.Messages = append(openaiReq.Messages, openaiMessage{
			Role:    "system",
			Content: req.SystemPrompt,
		})
	}

	for _, msg := range req.Messages {
		openaiReq.Messages = append(openaiReq.Messages, openaiMessage{
			Role:    string(msg.Role),
			Content: msg.Content,
		})
	}

	if req.JSONMode {
		openaiReq.ResponseFormat = &openaiResponseFormat{Type: "json_object"}
	}

	body, err := json.Marshal(openaiReq)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", o.endpoint+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+o.apiKey)

	resp, err := o.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openai error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var openaiResp openaiResponse
	if err := json.NewDecoder(resp.Body).Decode(&openaiResp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	if len(openaiResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	choice := openaiResp.Choices[0]
	return &ChatResponse{
		Content:      choice.Message.Content,
		FinishReason: choice.FinishReason,
		TokensUsed:   openaiResp.Usage.TotalTokens,
	}, nil
}

// OpenAI API types

type openaiRequest struct {
	Model          string                `json:"model"`
	Messages       []openaiMessage       `json:"messages"`
	MaxTokens      int                   `json:"max_tokens,omitempty"`
	Temperature    float64               `json:"temperature,omitempty"`
	ResponseFormat *openaiResponseFormat `json:"response_format,omitempty"`
}

type openaiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openaiResponseFormat struct {
	Type string `json:"type"`
}

type openaiResponse struct {
	Choices []openaiChoice `json:"choices"`
	Usage   openaiUsage    `json:"usage"`
}

type openaiChoice struct {
	Message      openaiMessage `json:"message"`
	FinishReason string        `json:"finish_reason"`
}

type openaiUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}
