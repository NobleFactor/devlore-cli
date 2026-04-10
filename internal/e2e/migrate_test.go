// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

//go:build e2e

package e2e

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/NobleFactor/devlore-cli/internal/lorepackage"
	"github.com/NobleFactor/devlore-cli/internal/model"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/migrate"
	"github.com/NobleFactor/devlore-cli/pkg/op"
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

// TestMigrate_E2E runs end-to-end tests for writ migrate.
// Requires LLM providers. Opt in with: go test -tags e2e
// Provider configuration uses devlore's standard resolution chain:
// CLI flags → DEVLORE_MODEL_* env → config file → keystore → auto-detect → Ollama
func TestMigrate_E2E(t *testing.T) {

	ctx := context.Background()

	// Get provider using devlore's standard configuration
	provider, skipReason := GetTestProvider(ctx)
	if provider == nil {
		t.Skipf("E2E test environment not ready: %s", skipReason)
	}
	t.Logf("Using provider: %s (%s)", provider.Name(), provider.Model())

	fixtures := loadMigrateFixtures(t)
	if len(fixtures) == 0 {
		t.Fatal("no migration fixtures found")
	}

	timeout := 5 * time.Minute

	// Create report
	report := &TestReport{
		GeneratedAt: time.Now(),
	}

	// Run tests for each fixture
	for _, fixture := range fixtures {
		suite := TestSuite{
			Name:  "migrate/" + fixture.Name,
			RunAt: time.Now(),
		}

		result := runMigrateTestWithProvider(t, fixture, provider, timeout)
		suite.Results = append(suite.Results, result)

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

// runMigrateTestWithProvider runs a single migration test with a provider.
func runMigrateTestWithProvider(t *testing.T, fixture MigrateFixture, provider model.Provider, timeout time.Duration) TestResult {
	t.Helper()

	result := TestResult{
		TestName:  fixture.Name,
		Provider:  provider.Name(),
		Model:     provider.Model(),
		StartTime: time.Now(),
	}

	ctx := context.Background()

	// Load registry (required for prompts)
	regPath := os.Getenv("DEVLORE_REGISTRY_PATH")
	if regPath == "" {
		result.Error = "DEVLORE_REGISTRY_PATH not set"
		result.EndTime = time.Now()
		return result
	}

	// Create a registry pointing to the local path
	reg := lorepackage.New("test", nil, regPath)

	// Run migration analysis with timeout
	timer := NewTimer()

	var graph *migrate.LLMResult
	var analysisErr error

	err := RunWithTimeout(ctx, timeout, func(ctx context.Context) error {
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
		"rename_count":    len(graph.Graph.Nodes()),
	}

	return result
}

// evaluateMigrateCorrectness computes correctness metrics for migration.
func evaluateMigrateCorrectness(analysis *migrate.MigrationAnalysis, graph *op.Graph, expected MigrateExpected) CorrectnessMetrics {
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
		for _, node := range graph.Nodes() {
			// Extract relative paths from source/target slots
			src, _ := node.SlotByName("source").(string)
			tgt, _ := node.SlotByName("path").(string)
			source := filepath.Base(src)
			target := filepath.Base(tgt)
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
