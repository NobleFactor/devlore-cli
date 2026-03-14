// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package staranalysis

import (
	"path/filepath"
	"sort"
	"testing"
)

func testdataDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs("../starcode/testdata")
	if err != nil {
		t.Fatalf("abs testdata: %v", err)
	}
	return dir
}

func captureFiles(t *testing.T, root, pattern string) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(root, pattern))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	sort.Strings(matches)
	return matches
}

func TestAnalyzeBasic(t *testing.T) {
	root := testdataDir(t)
	files := captureFiles(t, root, "*.star")

	report, err := (&Provider{Root: root}).Analyze(files, AnalysisConfig{
		Hotspots:            true,
		CyclomaticThreshold: 10,
		CognitiveThreshold:  15,
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	if report.Stats == nil {
		t.Fatal("Stats should not be nil")
	}
	if report.Complexity == nil {
		t.Fatal("Complexity should not be nil")
	}
	if report.Index != nil {
		t.Error("Index should be nil when WithIndex=false")
	}
}

func TestAnalyzeWithIndex(t *testing.T) {
	root := testdataDir(t)
	files := captureFiles(t, root, "*.star")

	report, err := (&Provider{Root: root}).Analyze(files, AnalysisConfig{
		Hotspots:  true,
		WithIndex: true,
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	if report.Index == nil {
		t.Error("Index should not be nil when WithIndex=true")
	}
}

func TestAnalyzeHotspots(t *testing.T) {
	root := testdataDir(t)
	files := captureFiles(t, root, "complex.star")

	// Set very low threshold so complex_logic (cyclomatic=8) becomes a hotspot
	report, err := (&Provider{Root: root}).Analyze(files, AnalysisConfig{
		Hotspots:            true,
		CyclomaticThreshold: 5,
		CognitiveThreshold:  5,
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	if len(report.Hotspots) == 0 {
		t.Error("expected at least one hotspot with threshold=5")
	}

	// Verify hotspot fields are populated
	for _, h := range report.Hotspots {
		if h.File == "" {
			t.Error("hotspot File should not be empty")
		}
		if h.Name == "" {
			t.Error("hotspot ReceiverName should not be empty")
		}
		if h.Line == 0 {
			t.Error("hotspot Line should not be 0")
		}
	}
}

func TestAnalyzeNoHotspots(t *testing.T) {
	root := testdataDir(t)
	files := captureFiles(t, root, "simple.star")

	// High thresholds — simple.star should have no hotspots
	report, err := (&Provider{Root: root}).Analyze(files, AnalysisConfig{
		Hotspots:            true,
		CyclomaticThreshold: 10,
		CognitiveThreshold:  15,
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	if len(report.Hotspots) != 0 {
		t.Errorf("expected 0 hotspots, got %d", len(report.Hotspots))
	}
}

func TestAnalyzeHotspotsDisabled(t *testing.T) {
	root := testdataDir(t)
	files := captureFiles(t, root, "complex.star")

	report, err := (&Provider{Root: root}).Analyze(files, AnalysisConfig{
		Hotspots:            false,
		CyclomaticThreshold: 1,
		CognitiveThreshold:  1,
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	if report.Hotspots != nil {
		t.Errorf("Hotspots should be nil when disabled, got %d", len(report.Hotspots))
	}
}

func TestAnalyzeDefaultThresholds(t *testing.T) {
	root := testdataDir(t)
	files := captureFiles(t, root, "complex.star")

	// Zero thresholds should default to 10/15
	report, err := (&Provider{Root: root}).Analyze(files, AnalysisConfig{Hotspots: true})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	// complex_logic has cyclomatic=8, below default 10
	// So there should be no hotspots at default thresholds
	if len(report.Hotspots) != 0 {
		t.Errorf("expected 0 hotspots at default thresholds, got %d", len(report.Hotspots))
	}
}
