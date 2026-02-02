// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package e2e

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/NobleFactor/devlore-cli/internal/lore/onboard"
	"github.com/NobleFactor/devlore-cli/internal/lorepackage"
	"github.com/NobleFactor/devlore-cli/internal/model"
)

// OnboardExpected represents the expected output for an onboarding test.
type OnboardExpected struct {
	Product       OnboardProduct       `yaml:"product"`
	Sources       OnboardSources       `yaml:"sources,omitempty"`
	Platforms     OnboardPlatforms     `yaml:"platforms,omitempty"`
	Complexity    OnboardComplexity    `yaml:"complexity,omitempty"`
	ExpectedSlots []OnboardExpectedSlot `yaml:"expected_slots,omitempty"`
}

type OnboardProduct struct {
	Name          string `yaml:"name"`
	CanonicalName string `yaml:"canonical_name"`
	Category      string `yaml:"category"`
	Vendor        string `yaml:"vendor"`
	Version       string `yaml:"version,omitempty"`
	License       string `yaml:"license,omitempty"`
}

type OnboardSources struct {
	Homepage   string `yaml:"homepage,omitempty"`
	Repository string `yaml:"repository,omitempty"`
}

type OnboardPlatforms struct {
	Darwin  *OnboardPlatformSupport `yaml:"darwin,omitempty"`
	Linux   *OnboardPlatformSupport `yaml:"linux,omitempty"`
	Windows *OnboardPlatformSupport `yaml:"windows,omitempty"`
}

type OnboardPlatformSupport struct {
	Supported      bool     `yaml:"supported"`
	Architectures  []string `yaml:"architectures,omitempty"`
	Distributions  []string `yaml:"distributions,omitempty"`
	InstallMethods []string `yaml:"install_methods,omitempty"`
}

type OnboardComplexity struct {
	Rating   string   `yaml:"rating"`
	Concerns []string `yaml:"concerns,omitempty"`
}

type OnboardExpectedSlot struct {
	Name     string `yaml:"name"`
	Platform string `yaml:"platform"`
	Value    string `yaml:"value"`
}

// OnboardFixture represents a test fixture for onboarding.
type OnboardFixture struct {
	Name        string
	HTMLPath    string
	HTMLContent string
	Expected    OnboardExpected
}

// loadOnboardFixtures loads all onboarding test fixtures.
func loadOnboardFixtures(t *testing.T) []OnboardFixture {
	t.Helper()

	testdataDir := filepath.Join("testdata", "onboard")
	entries, err := os.ReadDir(testdataDir)
	if err != nil {
		t.Fatalf("reading testdata/onboard: %v", err)
	}

	var fixtures []OnboardFixture
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".html") {
			continue
		}

		baseName := strings.TrimSuffix(entry.Name(), ".html")
		htmlPath := filepath.Join(testdataDir, entry.Name())
		expectedPath := filepath.Join(testdataDir, baseName+"-expected.yaml")

		// Read HTML content
		htmlContent, err := os.ReadFile(htmlPath)
		if err != nil {
			t.Logf("skipping %s: cannot read HTML", entry.Name())
			continue
		}

		// Read expected output
		expectedData, err := os.ReadFile(expectedPath)
		if err != nil {
			t.Logf("skipping %s: no expected.yaml", baseName)
			continue
		}

		var expected OnboardExpected
		if err := yaml.Unmarshal(expectedData, &expected); err != nil {
			t.Fatalf("parsing %s-expected.yaml: %v", baseName, err)
		}

		fixtures = append(fixtures, OnboardFixture{
			Name:        baseName,
			HTMLPath:    htmlPath,
			HTMLContent: string(htmlContent),
			Expected:    expected,
		})
	}

	return fixtures
}

// TestOnboard_E2E runs end-to-end tests for lore onboard.
// This test is skipped unless E2E_TEST=1 is set, as it requires LLM providers.
// Provider configuration uses devlore's standard resolution chain:
// CLI flags → DEVLORE_MODEL_* env → config file → keystore → auto-detect → Ollama
func TestOnboard_E2E(t *testing.T) {
	if os.Getenv("E2E_TEST") != "1" {
		t.Skip("E2E_TEST=1 not set, skipping E2E tests")
	}

	ctx := context.Background()

	// Get provider using devlore's standard configuration
	provider, skipReason := GetTestProvider(ctx)
	if provider == nil {
		t.Skipf("skipping E2E tests: %s", skipReason)
	}
	t.Logf("Using provider: %s (%s)", provider.Name(), provider.Model())

	fixtures := loadOnboardFixtures(t)
	if len(fixtures) == 0 {
		t.Fatal("no onboarding fixtures found")
	}

	timeout := 5 * time.Minute

	// Create report
	report := &TestReport{
		GeneratedAt: time.Now(),
	}

	// Run tests for each fixture
	for _, fixture := range fixtures {
		suite := TestSuite{
			Name:  "onboard/" + fixture.Name,
			RunAt: time.Now(),
		}

		result := runOnboardTestWithProvider(t, fixture, provider, timeout)
		suite.Results = append(suite.Results, result)

		report.Suites = append(report.Suites, suite)
	}

	// Write report if output directory is specified
	if outDir := os.Getenv("E2E_OUTPUT_DIR"); outDir != "" {
		if err := report.WriteReport(filepath.Join(outDir, "onboard")); err != nil {
			t.Errorf("writing report: %v", err)
		}
	}

	// Log summary
	t.Log(report.GenerateSummary())
}

// runOnboardTestWithProvider runs a single onboarding test with a provider.
func runOnboardTestWithProvider(t *testing.T, fixture OnboardFixture, provider model.Provider, timeout time.Duration) TestResult {
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

	// Start a local test server to serve the HTML
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(fixture.HTMLContent))
	}))
	defer server.Close()

	// Run onboarding with timeout
	timer := NewTimer()

	var onboardResult *onboard.Result
	var onboardErr error

	err := RunWithTimeout(ctx, timeout, func(ctx context.Context) error {
		onboardResult, onboardErr = onboard.Run(ctx, onboard.Options{
			Source:    server.URL,
			Provider:  provider,
			RegClient: reg,
		})
		return onboardErr
	})

	result.Performance.LatencyMs = timer.ElapsedMs()
	result.EndTime = time.Now()

	if err != nil {
		result.Error = err.Error()
		return result
	}

	if onboardResult == nil {
		result.Error = "nil onboard result"
		return result
	}

	// Evaluate correctness
	result.Correctness = evaluateOnboardCorrectness(onboardResult, fixture.Expected)
	result.Success = result.Correctness.ProductCorrect && result.Correctness.F1Score >= 0.6

	// Store details
	result.Details = map[string]any{
		"detected_product": onboardResult.Product,
		"expected_product": fixture.Expected.Product,
		"slot_count":       len(onboardResult.Slots),
	}

	return result
}

// evaluateOnboardCorrectness computes correctness metrics for onboarding.
func evaluateOnboardCorrectness(result *onboard.Result, expected OnboardExpected) CorrectnessMetrics {
	metrics := CorrectnessMetrics{}

	// Check product identification
	if result.Product != nil {
		metrics.ProductCorrect = strings.EqualFold(result.Product.Name, expected.Product.Name) ||
			strings.EqualFold(result.Product.CanonicalName, expected.Product.CanonicalName)
	}

	// Check slots
	expectedSlots := make(map[string]string) // key: "name:platform"
	for _, slot := range expected.ExpectedSlots {
		key := slot.Name + ":" + slot.Platform
		expectedSlots[key] = slot.Value
	}

	actualSlots := make(map[string]string)
	for _, slot := range result.Slots {
		key := slot.Name + ":" + slot.Platform
		actualSlots[key] = slot.Value
	}

	metrics.TotalExpected = len(expectedSlots)
	metrics.TotalFound = len(actualSlots)

	// Calculate TP, FP, FN
	for key, expectedVal := range expectedSlots {
		if actualVal, found := actualSlots[key]; found {
			// Check if values match (allowing for some flexibility)
			if containsIgnoreCase(actualVal, expectedVal) || containsIgnoreCase(expectedVal, actualVal) {
				metrics.TruePositives++
			} else {
				metrics.FalseNegatives++
			}
		} else {
			metrics.FalseNegatives++
		}
	}

	for key := range actualSlots {
		if _, expected := expectedSlots[key]; !expected {
			metrics.FalsePositives++
		}
	}

	metrics.ComputePrecisionRecall()

	// Check platform support
	if expected.Platforms.Darwin != nil && result.Platforms != nil && result.Platforms.Darwin != nil {
		metrics.PlatformsCorrect = result.Platforms.Darwin.Supported == expected.Platforms.Darwin.Supported
	}

	return metrics
}

// containsIgnoreCase checks if s contains substr (case-insensitive).
func containsIgnoreCase(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
