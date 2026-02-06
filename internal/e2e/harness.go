// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package e2e provides end-to-end testing infrastructure for LLM-based operations.
//
// # Provider Configuration
//
// E2E tests use devlore's standard provider resolution chain:
//
//  1. CLI flags (not applicable in tests)
//  2. Environment variables: DEVLORE_MODEL_PROVIDER, DEVLORE_MODEL_API_KEY, etc.
//  3. Config file: ~/.config/devlore/config.yaml
//  4. Native keystore (API keys stored via 'lore config model')
//  5. Auto-detect from common env vars: GROQ_API_KEY, GEMINI_API_KEY, etc.
//  6. Ollama fallback if running locally
//
// To run E2E tests:
//
//	# Using devlore config (recommended)
//	lore config model   # one-time setup
//	E2E_TEST=1 go test ./internal/e2e/...
//
//	# Using environment variables
//	E2E_TEST=1 DEVLORE_MODEL_PROVIDER=groq DEVLORE_MODEL_API_KEY=... go test ./internal/e2e/...
//
//	# Auto-detect from existing API keys
//	E2E_TEST=1 go test ./internal/e2e/...  # picks up GROQ_API_KEY, ANTHROPIC_API_KEY, etc.
//
// # Package Contents
//
// This package includes:
//   - GetTestProvider: Returns a provider using devlore's standard resolution
//   - Metrics collection (latency, tokens, correctness)
//   - Test fixtures for migration and onboarding scenarios
//   - Result comparison and reporting
package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/NobleFactor/devlore-cli/internal/config"
	"github.com/NobleFactor/devlore-cli/internal/model"
)

// GetTestProvider returns a provider using devlore's standard configuration.
// Uses the full resolution chain: CLI flags → env vars → config → keystore → auto-detect → Ollama.
// Returns nil and an error message if no provider is available (tests should skip).
func GetTestProvider(ctx context.Context) (model.Provider, string) {
	provider, err := model.EnsureProvider(ctx, false, model.CLIFlags{})
	if err != nil {
		return nil, fmt.Sprintf("no provider available: %v", err)
	}
	if provider == nil {
		return nil, "no provider configured; run 'lore config model' or set DEVLORE_MODEL_PROVIDER"
	}
	return provider, ""
}

// ProviderConfig defines configuration for a test provider.
type ProviderConfig struct {
	Name     string `yaml:"name" json:"name"`
	Provider string `yaml:"provider" json:"provider"` // ollama, anthropic, openai, github, etc.
	Model    string `yaml:"model" json:"model"`
	Endpoint string `yaml:"endpoint,omitempty" json:"endpoint,omitempty"`
	EnvKey   string `yaml:"env_key,omitempty" json:"env_key,omitempty"` // Environment variable for API key
}

// TestConfig holds configuration for E2E tests.
type TestConfig struct {
	Providers []ProviderConfig `yaml:"providers" json:"providers"`
	Timeout   time.Duration    `yaml:"timeout" json:"timeout"`
}

// DefaultTestConfig returns the default test configuration.
func DefaultTestConfig() TestConfig {
	return TestConfig{
		Providers: []ProviderConfig{
			{Name: "ollama-llama3.1", Provider: "ollama", Model: "llama3.1:8b"},
		},
		Timeout: 5 * time.Minute,
	}
}

// LoadTestConfig loads test configuration from a file.
func LoadTestConfig(path string) (TestConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return TestConfig{}, fmt.Errorf("reading config: %w", err)
	}

	var cfg TestConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return TestConfig{}, fmt.Errorf("parsing config: %w", err)
	}

	return cfg, nil
}

// CreateProvider creates a model.Provider from a ProviderConfig.
func CreateProvider(cfg ProviderConfig) (model.Provider, error) {
	apiKey := ""
	if cfg.EnvKey != "" {
		apiKey = os.Getenv(cfg.EnvKey)
		if apiKey == "" && cfg.Provider != "ollama" {
			return nil, fmt.Errorf("environment variable %s not set for provider %s", cfg.EnvKey, cfg.Name)
		}
	}

	return model.NewProvider(config.ModelConfig{
		Provider: cfg.Provider,
		Name:     cfg.Model,
		Endpoint: cfg.Endpoint,
		APIKey:   apiKey,
	})
}

// PerformanceMetrics captures performance data for an LLM operation.
type PerformanceMetrics struct {
	LatencyMs    int64   `json:"latency_ms" yaml:"latency_ms"`
	InputTokens  int     `json:"input_tokens" yaml:"input_tokens"`
	OutputTokens int     `json:"output_tokens" yaml:"output_tokens"`
	TotalTokens  int     `json:"total_tokens" yaml:"total_tokens"`
	CostUSD      float64 `json:"cost_usd,omitempty" yaml:"cost_usd,omitempty"`
	Retries      int     `json:"retries" yaml:"retries"`
}

// CorrectnessMetrics captures correctness data for test validation.
type CorrectnessMetrics struct {
	// Generic metrics
	TotalExpected  int     `json:"total_expected" yaml:"total_expected"`
	TotalFound     int     `json:"total_found" yaml:"total_found"`
	TruePositives  int     `json:"true_positives" yaml:"true_positives"`
	FalsePositives int     `json:"false_positives" yaml:"false_positives"`
	FalseNegatives int     `json:"false_negatives" yaml:"false_negatives"`
	Precision      float64 `json:"precision" yaml:"precision"`
	Recall         float64 `json:"recall" yaml:"recall"`
	F1Score        float64 `json:"f1_score" yaml:"f1_score"`

	// Domain-specific flags
	SystemCorrect    bool `json:"system_correct,omitempty" yaml:"system_correct,omitempty"`
	ProductCorrect   bool `json:"product_correct,omitempty" yaml:"product_correct,omitempty"`
	PlatformsCorrect bool `json:"platforms_correct,omitempty" yaml:"platforms_correct,omitempty"`
}

// ComputePrecisionRecall calculates precision, recall, and F1 from TP/FP/FN.
func (c *CorrectnessMetrics) ComputePrecisionRecall() {
	if c.TruePositives+c.FalsePositives > 0 {
		c.Precision = float64(c.TruePositives) / float64(c.TruePositives+c.FalsePositives)
	}
	if c.TruePositives+c.FalseNegatives > 0 {
		c.Recall = float64(c.TruePositives) / float64(c.TruePositives+c.FalseNegatives)
	}
	if c.Precision+c.Recall > 0 {
		c.F1Score = 2 * c.Precision * c.Recall / (c.Precision + c.Recall)
	}
}

// TestResult captures the complete result of a single test run.
type TestResult struct {
	TestName    string             `json:"test_name" yaml:"test_name"`
	Provider    string             `json:"provider" yaml:"provider"`
	Model       string             `json:"model" yaml:"model"`
	StartTime   time.Time          `json:"start_time" yaml:"start_time"`
	EndTime     time.Time          `json:"end_time" yaml:"end_time"`
	Success     bool               `json:"success" yaml:"success"`
	Error       string             `json:"error,omitempty" yaml:"error,omitempty"`
	Performance PerformanceMetrics `json:"performance" yaml:"performance"`
	Correctness CorrectnessMetrics `json:"correctness" yaml:"correctness"`
	Details     map[string]any     `json:"details,omitempty" yaml:"details,omitempty"`
}

// TestSuite holds results for all providers on a single test.
type TestSuite struct {
	Name    string       `json:"name" yaml:"name"`
	RunAt   time.Time    `json:"run_at" yaml:"run_at"`
	Results []TestResult `json:"results" yaml:"results"`
}

// TestReport aggregates results across all tests and providers.
type TestReport struct {
	GeneratedAt time.Time   `json:"generated_at" yaml:"generated_at"`
	Suites      []TestSuite `json:"suites" yaml:"suites"`
}

// WriteReport writes the test report to a directory.
func (r *TestReport) WriteReport(outDir string) error {
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	// Write JSON report
	jsonPath := filepath.Join(outDir, "results.json")
	jsonData, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling JSON: %w", err)
	}
	if err := os.WriteFile(jsonPath, jsonData, 0644); err != nil {
		return fmt.Errorf("writing JSON report: %w", err)
	}

	// Write YAML report
	yamlPath := filepath.Join(outDir, "results.yaml")
	yamlData, err := yaml.Marshal(r)
	if err != nil {
		return fmt.Errorf("marshaling YAML: %w", err)
	}
	if err := os.WriteFile(yamlPath, yamlData, 0644); err != nil {
		return fmt.Errorf("writing YAML report: %w", err)
	}

	// Write summary markdown
	summaryPath := filepath.Join(outDir, "summary.md")
	summary := r.GenerateSummary()
	if err := os.WriteFile(summaryPath, []byte(summary), 0644); err != nil {
		return fmt.Errorf("writing summary: %w", err)
	}

	return nil
}

// GenerateSummary creates a markdown summary of test results.
func (r *TestReport) GenerateSummary() string {
	var sb stringBuilder
	sb.WriteString("# E2E Test Report\n\n")
	sb.WriteString(fmt.Sprintf("Generated: %s\n\n", r.GeneratedAt.Format(time.RFC3339)))

	for _, suite := range r.Suites {
		sb.WriteString(fmt.Sprintf("## %s\n\n", suite.Name))
		sb.WriteString("| Provider | Model | Success | Latency (ms) | Tokens | F1 Score |\n")
		sb.WriteString("|----------|-------|---------|--------------|--------|----------|\n")

		for _, result := range suite.Results {
			status := "✓"
			if !result.Success {
				status = "✗"
			}
			sb.WriteString(fmt.Sprintf("| %s | %s | %s | %d | %d | %.2f |\n",
				result.Provider,
				result.Model,
				status,
				result.Performance.LatencyMs,
				result.Performance.TotalTokens,
				result.Correctness.F1Score,
			))
		}
		sb.WriteString("\n")

		// Add failure details
		for _, result := range suite.Results {
			if !result.Success && result.Error != "" {
				sb.WriteString(fmt.Sprintf("### %s Failure\n\n", result.Provider))
				sb.WriteString(fmt.Sprintf("```\n%s\n```\n\n", result.Error))
			}
		}
	}

	return sb.String()
}

// stringBuilder is a simple string builder for summary generation.
type stringBuilder struct {
	data []byte
}

func (sb *stringBuilder) WriteString(s string) {
	sb.data = append(sb.data, s...)
}

func (sb *stringBuilder) String() string {
	return string(sb.data)
}

// Timer helps measure operation latency.
type Timer struct {
	start time.Time
}

// NewTimer creates and starts a new timer.
func NewTimer() *Timer {
	return &Timer{start: time.Now()}
}

// ElapsedMs returns elapsed milliseconds since timer start.
func (t *Timer) ElapsedMs() int64 {
	return time.Since(t.start).Milliseconds()
}

// RunWithTimeout executes a function with a timeout.
func RunWithTimeout(ctx context.Context, timeout time.Duration, fn func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return fn(ctx)
}
