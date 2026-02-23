// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package onboard implements the multi-phase onboarding pipeline for lore.
// It parses wiki pages or setup scripts and generates packages-manifest.yaml files.
package onboard

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/NobleFactor/devlore-cli/internal/lorepackage"
	"github.com/NobleFactor/devlore-cli/internal/model"
)

// Options controls onboarding behavior.
type Options struct {
	Source     string                // URL or file path
	OutputDir  string                // Output directory (default: current)
	Format     string                // Manifest format: "plain" or "yaml"
	Verbose    bool                  // Show AI reasoning
	Explain    bool                  // Show detailed confidence decisions
	Provider   model.Provider        // AI provider
	RegClient  *lorepackage.Registry // Registry for prompts and matching
	MaxFetches int                   // Max additional URLs to fetch (default: 5)
}

// Result contains the onboarding output.
type Result struct {
	Product    *ProductInfo    `json:"product"`
	Sources    *SourceInfo     `json:"sources"`
	Platforms  *PlatformInfo   `json:"platforms"`
	Complexity *ComplexityInfo `json:"complexity"`
	Slots      []ExtractedSlot `json:"slots"`
	Manifest   string          `json:"manifest"` // Generated packages-manifest.yaml content
	Warnings   []string        `json:"warnings"`
}

// ProductInfo contains identified product information.
type ProductInfo struct {
	Name          string `json:"name"`
	CanonicalName string `json:"canonical_name"`
	Category      string `json:"category"`
	Vendor        string `json:"vendor"`
	Version       string `json:"version,omitempty"`
	License       string `json:"license,omitempty"`
}

// SourceInfo contains discovered documentation URLs.
type SourceInfo struct {
	Homepage        string   `json:"homepage"`
	Installation    []string `json:"installation"`
	Upgrade         []string `json:"upgrade"`
	Uninstall       []string `json:"uninstall"`
	Troubleshooting []string `json:"troubleshooting"`
	Repository      string   `json:"repository,omitempty"`
	Releases        string   `json:"releases,omitempty"`
}

// PlatformSupport describes support for a specific platform.
type PlatformSupport struct {
	Supported     bool     `json:"supported"`
	Architectures []string `json:"architectures,omitempty"`
	Distributions []string `json:"distributions,omitempty"`
	MinVersion    string   `json:"min_version,omitempty"`
	Notes         string   `json:"notes,omitempty"`
}

// PlatformInfo contains platform support information.
type PlatformInfo struct {
	Darwin  *PlatformSupport `json:"darwin,omitempty"`
	Linux   *PlatformSupport `json:"linux,omitempty"`
	Windows *PlatformSupport `json:"windows,omitempty"`
}

// ComplexityInfo describes installation complexity.
type ComplexityInfo struct {
	Rating   string   `json:"rating"` // simple, moderate, complex
	Concerns []string `json:"concerns"`
}

// ExtractedSlot represents a piece of extracted information.
type ExtractedSlot struct {
	Name        string   `json:"name"`
	Value       string   `json:"value"`
	Confidence  float64  `json:"confidence"`
	Source      string   `json:"source"`
	Platform    string   `json:"platform"`
	Annotations []string `json:"annotations,omitempty"`
}

// Run executes the onboarding pipeline.
func Run(ctx context.Context, opts Options) (*Result, error) {
	if opts.Provider == nil {
		return nil, fmt.Errorf("AI provider required for onboarding")
	}
	if opts.RegClient == nil {
		return nil, fmt.Errorf("registry required for onboarding prompts")
	}
	if opts.Source == "" {
		return nil, fmt.Errorf("source URL or file path required")
	}
	if opts.MaxFetches <= 0 {
		opts.MaxFetches = 5
	}
	if opts.OutputDir == "" {
		opts.OutputDir = "."
	}

	// Phase 1: Fetch initial content
	content, sourceURL, err := fetchContent(ctx, opts.Source)
	if err != nil {
		return nil, fmt.Errorf("fetch source: %w", err)
	}

	// Phase 2: Discover product
	discovery, err := discoverProduct(ctx, opts, content, sourceURL)
	if err != nil {
		return nil, fmt.Errorf("discover product: %w", err)
	}

	// Phase 3: Fetch additional URLs and parse documents
	allContent := []documentContent{{URL: sourceURL, Content: content}}

	if len(discovery.NextURLs) > 0 && opts.MaxFetches > 0 {
		fetched := fetchAdditionalURLs(ctx, discovery.NextURLs, opts.MaxFetches)
		allContent = append(allContent, fetched...)
	}

	// Phase 4: Parse documents to extract slots
	slots, err := parseDocuments(ctx, opts, allContent)
	if err != nil {
		return nil, fmt.Errorf("parse documents: %w", err)
	}

	// Phase 5: Generate manifest
	manifest := generateManifest(discovery, slots, opts.Format)

	result := &Result{
		Product:    discovery.Product,
		Sources:    discovery.Sources,
		Platforms:  discovery.Platforms,
		Complexity: discovery.Complexity,
		Slots:      slots,
		Manifest:   manifest,
	}

	return result, nil
}

// documentContent holds fetched content with its source URL.
type documentContent struct {
	URL     string
	Content string
}

// discoveryResult holds the result of product discovery.
type discoveryResult struct {
	Product    *ProductInfo    `json:"product"`
	Sources    *SourceInfo     `json:"sources"`
	Platforms  *PlatformInfo   `json:"platforms"`
	Complexity *ComplexityInfo `json:"complexity"`
	NextURLs   []string        `json:"next_urls_to_fetch"`
}

// fetchContent fetches content from a URL or file.
func fetchContent(ctx context.Context, source string) (content, sourceURL string, err error) {
	// Check if it's a URL
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, http.NoBody) //nolint:gosec // G107: URL from config
		if err != nil {
			return "", "", err
		}
		resp, err := http.DefaultClient.Do(req) //nolint:gosec // G704: URL from user-provided source input
		if err != nil {
			return "", "", err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return "", "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", "", err
		}
		return string(body), source, nil
	}

	// It's a file path
	absPath, err := filepath.Abs(source)
	if err != nil {
		return "", "", err
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return "", "", err
	}

	return string(data), "file://" + absPath, nil
}

// discoverProduct uses the AI to identify the product from content.
func discoverProduct(ctx context.Context, opts Options, content, sourceURL string) (*discoveryResult, error) {
	prompt, err := opts.RegClient.Knowledge("onboarding").Prompt("discover-product.txt")
	if err != nil {
		return nil, fmt.Errorf("loading discover-product prompt: %w", err)
	}

	userMessage := fmt.Sprintf("Source URL: %s\n\nContent:\n%s", sourceURL, truncateContent(content, 50000))

	resp, err := opts.Provider.Chat(ctx, model.ChatRequest{
		SystemPrompt: prompt,
		Messages: []model.Message{
			{Role: model.RoleUser, Content: userMessage},
		},
		Temperature: 0,
		JSONMode:    true,
	})
	if err != nil {
		return nil, fmt.Errorf("AI discovery failed: %w", err)
	}

	var result discoveryResult
	if err := json.Unmarshal([]byte(resp.Content), &result); err != nil {
		return nil, fmt.Errorf("parse discovery response: %w\nResponse: %s", err, resp.Content)
	}

	return &result, nil
}

// fetchAdditionalURLs fetches content from additional URLs.
func fetchAdditionalURLs(ctx context.Context, urls []string, maxFetches int) []documentContent {
	var result []documentContent

	for i, u := range urls {
		if i >= maxFetches {
			break
		}

		// Validate URL
		if _, err := url.Parse(u); err != nil {
			continue
		}

		content, sourceURL, err := fetchContent(ctx, u)
		if err != nil {
			continue // Skip failed fetches
		}

		result = append(result, documentContent{URL: sourceURL, Content: content})
	}

	return result
}

// parseDocuments extracts slots from all fetched documents.
func parseDocuments(ctx context.Context, opts Options, docs []documentContent) ([]ExtractedSlot, error) {
	prompt, err := opts.RegClient.Knowledge("package-authoring").Prompt("parse-document.txt")
	if err != nil {
		return nil, fmt.Errorf("loading parse-document prompt: %w", err)
	}

	// Build combined document content
	var sb strings.Builder
	for i, doc := range docs {
		fmt.Fprintf(&sb, "## Document %d: %s\n\n", i+1, doc.URL)
		sb.WriteString(truncateContent(doc.Content, 30000))
		sb.WriteString("\n\n---\n\n")
	}

	resp, err := opts.Provider.Chat(ctx, model.ChatRequest{
		SystemPrompt: prompt,
		Messages: []model.Message{
			{Role: model.RoleUser, Content: sb.String()},
		},
		Temperature: 0,
		JSONMode:    true,
	})
	if err != nil {
		return nil, fmt.Errorf("AI parsing failed: %w", err)
	}

	// Parse the response - expect a slots array
	var parseResult struct {
		Slots []ExtractedSlot `json:"slots"`
	}
	if err := json.Unmarshal([]byte(resp.Content), &parseResult); err != nil {
		// Try parsing as a plain array
		var slots []ExtractedSlot
		if err2 := json.Unmarshal([]byte(resp.Content), &slots); err2 != nil {
			return nil, fmt.Errorf("parse slots response: %w\nResponse: %s", err, resp.Content)
		}
		return slots, nil
	}

	return parseResult.Slots, nil
}

// generateManifest creates a packages-manifest.yaml from discovery and slots.
func generateManifest(discovery *discoveryResult, slots []ExtractedSlot, _ string) string { //nolint:gocognit,gocyclo
	var sb strings.Builder

	if discovery.Product != nil {
		fmt.Fprintf(&sb, "# Package: %s\n", discovery.Product.Name)
		if discovery.Product.Vendor != "" {
			fmt.Fprintf(&sb, "# Vendor: %s\n", discovery.Product.Vendor)
		}
		if discovery.Product.Version != "" {
			fmt.Fprintf(&sb, "# Version: %s\n", discovery.Product.Version)
		}
		sb.WriteString("#\n")
	}

	// Add complexity warning if complex
	if discovery.Complexity != nil && discovery.Complexity.Rating == "complex" {
		sb.WriteString("# WARNING: Complex installation\n")
		for _, concern := range discovery.Complexity.Concerns {
			fmt.Fprintf(&sb, "#   - %s\n", concern)
		}
		sb.WriteString("#\n")
	}

	sb.WriteString("\n")

	// Extract package manager commands from slots
	for _, slot := range slots {
		if slot.Name == "install_command" || slot.Name == "package_manager" {
			if slot.Platform != "" && slot.Platform != "all" {
				fmt.Fprintf(&sb, "# Platform: %s\n", slot.Platform)
			}
			fmt.Fprintf(&sb, "%s\n", slot.Value)
			if len(slot.Annotations) > 0 {
				for _, ann := range slot.Annotations {
					fmt.Fprintf(&sb, "  # %s\n", ann)
				}
			}
			sb.WriteString("\n")
		}
	}

	// If no install commands found, use canonical name as placeholder
	if discovery.Product != nil && !strings.Contains(sb.String(), discovery.Product.CanonicalName) {
		fmt.Fprintf(&sb, "# TODO: Add installation method for %s\n", discovery.Product.CanonicalName)
		fmt.Fprintf(&sb, "# %s\n", discovery.Product.CanonicalName)
	}

	return sb.String()
}

// truncateContent limits content to maxLen characters.
func truncateContent(content string, maxLen int) string {
	if len(content) <= maxLen {
		return content
	}
	return content[:maxLen] + "\n\n[Content truncated...]"
}

// WriteManifest writes the manifest to a file.
func WriteManifest(result *Result, outputDir string) error {
	path := filepath.Join(outputDir, "packages-manifest.yaml")
	return os.WriteFile(path, []byte(result.Manifest), 0o600)
}
