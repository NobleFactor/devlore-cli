// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package e2e

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/NobleFactor/devlore-cli/internal/execution"
	"github.com/NobleFactor/devlore-cli/internal/lorepackage"
	"github.com/NobleFactor/devlore-cli/internal/writ/migrate"
)

// MigrateExpected represents the expected output for a migration test.
type MigrateExpected struct {
	System          string           `yaml:"system"`
	Groups          []string         `yaml:"groups"`
	Platforms       []string         `yaml:"platforms"`
	ExpectedRenames []ExpectedRename `yaml:"expected_renames"`
	Notes           string           `yaml:"notes"`
}

// ExpectedRename represents an expected rename operation.
type ExpectedRename struct {
	Source string `yaml:"source"`
	Target string `yaml:"target"`
}

// MigrateFixture represents a test fixture for migration.
type MigrateFixture struct {
	Name     string
	Path     string
	Expected MigrateExpected
}

// loadMigrateFixtures loads all migration test fixtures.
func loadMigrateFixtures(t *testing.T) []MigrateFixture {
	t.Helper()

	testdataDir := filepath.Join("testdata", "migrate")
	entries, err := os.ReadDir(testdataDir)
	if err != nil {
		t.Fatalf("reading testdata/migrate: %v", err)
	}

	var fixtures []MigrateFixture
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		fixturePath := filepath.Join(testdataDir, entry.Name())
		expectedPath := filepath.Join(fixturePath, "expected.yaml")

		data, err := os.ReadFile(expectedPath)
		if err != nil {
			t.Logf("skipping %s: no expected.yaml", entry.Name())
			continue
		}

		var expected MigrateExpected
		if err := yaml.Unmarshal(data, &expected); err != nil {
			t.Fatalf("parsing %s/expected.yaml: %v", entry.Name(), err)
		}

		absPath, err := filepath.Abs(fixturePath)
		if err != nil {
			t.Fatalf("abs path for %s: %v", entry.Name(), err)
		}

		fixtures = append(fixtures, MigrateFixture{
			Name:     entry.Name(),
			Path:     absPath,
			Expected: expected,
		})
	}

	return fixtures
}

// TestMigrate_E2E runs end-to-end tests for writ migrate across providers.
// This test is skipped unless E2E_TEST=1 is set, as it requires LLM providers.
func TestMigrate_E2E(t *testing.T) {
	if os.Getenv("E2E_TEST") != "1" {
		t.Skip("E2E_TEST=1 not set, skipping E2E tests")
	}

	// Load test configuration
	cfg := loadE2EConfig(t)
	fixtures := loadMigrateFixtures(t)

	if len(fixtures) == 0 {
		t.Fatal("no migration fixtures found")
	}

	// Create report
	report := &TestReport{
		GeneratedAt: time.Now(),
	}

	// Run tests for each fixture and provider combination
	for _, fixture := range fixtures {
		suite := TestSuite{
			Name:  "migrate/" + fixture.Name,
			RunAt: time.Now(),
		}

		for _, provCfg := range cfg.Providers {
			result := runMigrateTest(t, fixture, provCfg, cfg.Timeout)
			suite.Results = append(suite.Results, result)
		}

		report.Suites = append(report.Suites, suite)
	}

	// Write report if output directory is specified
	if outDir := os.Getenv("E2E_OUTPUT_DIR"); outDir != "" {
		if err := report.WriteReport(outDir); err != nil {
			t.Errorf("writing report: %v", err)
		}
	}

	// Log summary
	t.Log(report.GenerateSummary())
}

// runMigrateTest runs a single migration test with a specific provider.
func runMigrateTest(t *testing.T, fixture MigrateFixture, provCfg ProviderConfig, timeout time.Duration) TestResult {
	t.Helper()

	result := TestResult{
		TestName:  fixture.Name,
		Provider:  provCfg.Name,
		Model:     provCfg.Model,
		StartTime: time.Now(),
	}

	// Create provider
	provider, err := CreateProvider(provCfg)
	if err != nil {
		result.Error = err.Error()
		result.EndTime = time.Now()
		return result
	}

	// Check availability
	ctx := context.Background()
	if !provider.Available(ctx) {
		result.Error = "provider not available"
		result.EndTime = time.Now()
		return result
	}

	// Load registry (required for prompts)
	regPath := os.Getenv("DEVLORE_REGISTRY_PATH")
	if regPath == "" {
		result.Error = "DEVLORE_REGISTRY_PATH not set"
		result.EndTime = time.Now()
		return result
	}

	// Create a registry pointing to the local path
	// For testing, we use the local path directly as the cache directory
	reg := lorepackage.New("test", nil, regPath)

	// Run migration analysis with timeout
	timer := NewTimer()

	var graph *migrate.LLMResult
	var analysisErr error

	err = RunWithTimeout(ctx, timeout, func(ctx context.Context) error {
		// Gather inputs
		input, err := migrate.GatherInputs(fixture.Path, 10, 100*1024)
		if err != nil {
			return err
		}

		// Run LLM analysis
		graph, analysisErr = migrate.AnalyzeWithLLMFromRegistry(ctx, provider, reg, input)
		return analysisErr
	})

	result.Performance.LatencyMs = timer.ElapsedMs()
	result.EndTime = time.Now()

	if err != nil {
		result.Error = err.Error()
		return result
	}

	if graph == nil || graph.Analysis == nil {
		result.Error = "nil analysis result"
		return result
	}

	// Evaluate correctness
	result.Correctness = evaluateMigrateCorrectness(graph.Analysis, graph.Graph, fixture.Expected)
	result.Success = result.Correctness.SystemCorrect && result.Correctness.F1Score >= 0.8

	// Store details
	result.Details = map[string]any{
		"detected_system": string(graph.Analysis.System),
		"expected_system": fixture.Expected.System,
		"projects":        graph.Analysis.Projects,
		"rename_count":    len(graph.Graph.Nodes),
	}

	return result
}

// evaluateMigrateCorrectness computes correctness metrics for migration.
func evaluateMigrateCorrectness(analysis *migrate.MigrationAnalysis, graph *execution.Graph, expected MigrateExpected) CorrectnessMetrics {
	metrics := CorrectnessMetrics{}

	// Check system detection
	metrics.SystemCorrect = strings.EqualFold(string(analysis.System), expected.System)

	// Check rename operations
	expectedRenames := make(map[string]string)
	for _, r := range expected.ExpectedRenames {
		expectedRenames[r.Source] = r.Target
	}

	actualRenames := make(map[string]string)
	if graph != nil {
		for _, node := range graph.Nodes {
			// Extract relative paths from source/target
			source := filepath.Base(node.Source)
			target := filepath.Base(node.Target)
			actualRenames[source] = target
		}
	}

	metrics.TotalExpected = len(expectedRenames)
	metrics.TotalFound = len(actualRenames)

	// Calculate TP, FP, FN
	for src, tgt := range expectedRenames {
		if actualTgt, found := actualRenames[src]; found && actualTgt == tgt {
			metrics.TruePositives++
		} else {
			metrics.FalseNegatives++
		}
	}

	for src := range actualRenames {
		if _, expected := expectedRenames[src]; !expected {
			metrics.FalsePositives++
		}
	}

	metrics.ComputePrecisionRecall()

	// Check platforms (if expected)
	if len(expected.Platforms) > 0 && analysis.Structure != nil {
		platformsMatch := true
		for _, p := range expected.Platforms {
			found := false
			for _, ap := range analysis.Structure.Platforms {
				if strings.EqualFold(ap, p) {
					found = true
					break
				}
			}
			if !found {
				platformsMatch = false
				break
			}
		}
		metrics.PlatformsCorrect = platformsMatch
	}

	return metrics
}

// loadE2EConfig loads the E2E test configuration.
func loadE2EConfig(t *testing.T) TestConfig {
	t.Helper()

	configPath := os.Getenv("E2E_CONFIG")
	if configPath != "" {
		cfg, err := LoadTestConfig(configPath)
		if err != nil {
			t.Fatalf("loading E2E config: %v", err)
		}
		return cfg
	}

	// Build config from environment variables
	cfg := DefaultTestConfig()

	// Check for provider-specific env vars
	if apiKey := os.Getenv("ANTHROPIC_API_KEY"); apiKey != "" {
		cfg.Providers = append(cfg.Providers, ProviderConfig{
			Name:     "anthropic-claude-sonnet",
			Provider: "anthropic",
			Model:    "claude-sonnet-4-20250514",
			EnvKey:   "ANTHROPIC_API_KEY",
		})
	}

	if apiKey := os.Getenv("GITHUB_TOKEN"); apiKey != "" {
		cfg.Providers = append(cfg.Providers, ProviderConfig{
			Name:     "github-gpt4o",
			Provider: "github",
			Model:    "gpt-4o",
			EnvKey:   "GITHUB_TOKEN",
		})
	}

	if apiKey := os.Getenv("OPENAI_API_KEY"); apiKey != "" {
		cfg.Providers = append(cfg.Providers, ProviderConfig{
			Name:     "openai-gpt4o",
			Provider: "openai",
			Model:    "gpt-4o",
			EnvKey:   "OPENAI_API_KEY",
		})
	}

	return cfg
}
