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
	"strings"
	"time"
)

// AzureOpenAIProvider implements Provider for Azure OpenAI Service.
type AzureOpenAIProvider struct {
	apiKey     string
	endpoint   string // e.g., https://{resource}.openai.azure.com
	deployment string // model deployment name
	apiVersion string
	client     *http.Client
}

// NewAzureOpenAIProvider creates an Azure OpenAI provider.
// endpoint should be the base URL like https://myresource.openai.azure.com
// deployment is the name of your model deployment (e.g., "gpt-4")
func NewAzureOpenAIProvider(apiKey, endpoint, deployment string) *AzureOpenAIProvider {
	// Clean up endpoint - remove trailing slash
	endpoint = strings.TrimSuffix(endpoint, "/")

	return &AzureOpenAIProvider{
		apiKey:     apiKey,
		endpoint:   endpoint,
		deployment: deployment,
		apiVersion: "2024-02-01",
		client: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}
}

// Name returns "azure-openai".
func (a *AzureOpenAIProvider) Name() string {
	return "azure-openai"
}

// Model returns the deployment name (Azure's equivalent of model).
func (a *AzureOpenAIProvider) Model() string {
	return a.deployment
}

// Endpoint returns the Azure OpenAI endpoint URL.
func (a *AzureOpenAIProvider) Endpoint() string {
	return a.endpoint
}

// Available checks if the required configuration is present.
func (a *AzureOpenAIProvider) Available(ctx context.Context) bool {
	return a.apiKey != "" && a.endpoint != "" && a.deployment != ""
}

// Chat sends a chat completion request to Azure OpenAI.
func (a *AzureOpenAIProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	// Build request (same format as OpenAI)
	azureReq := openaiRequest{
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
	}

	// Add system message if present
	if req.SystemPrompt != "" {
		azureReq.Messages = append(azureReq.Messages, openaiMessage{
			Role:    "system",
			Content: req.SystemPrompt,
		})
	}

	for _, msg := range req.Messages {
		azureReq.Messages = append(azureReq.Messages, openaiMessage{
			Role:    string(msg.Role),
			Content: msg.Content,
		})
	}

	if req.JSONMode {
		azureReq.ResponseFormat = &openaiResponseFormat{Type: "json_object"}
	}

	body, err := json.Marshal(azureReq)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	// Azure endpoint format: {endpoint}/openai/deployments/{deployment}/chat/completions?api-version={version}
	url := fmt.Sprintf("%s/openai/deployments/%s/chat/completions?api-version=%s",
		a.endpoint, a.deployment, a.apiVersion)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("api-key", a.apiKey) // Azure uses api-key header, not Bearer token

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("azure openai error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var azureResp openaiResponse
	if err := json.NewDecoder(resp.Body).Decode(&azureResp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	if len(azureResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	choice := azureResp.Choices[0]
	return &ChatResponse{
		Content:      choice.Message.Content,
		FinishReason: choice.FinishReason,
		TokensUsed:   azureResp.Usage.TotalTokens,
	}, nil
}
