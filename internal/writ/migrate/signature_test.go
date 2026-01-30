// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package migrate

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/NobleFactor/devlore-cli/internal/lorepackage"
)

func TestLoadSignatures_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	if os.Getenv("SKIP_NETWORK_TESTS") != "" {
		t.Skip("skipping network test")
	}

	// Use temp directory for test cache
	tmpDir, err := os.MkdirTemp("", "signature-test-*")
	if err != nil {
		t.Fatalf("creating temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	provider := lorepackage.NewGitProvider(
		"https://github.com/NobleFactor/devlore-lorepackage.git",
		"develop",
	)
	client := lorepackage.New("test", provider, filepath.Join(tmpDir, "central"))

	// Sync registry
	ctx := context.Background()
	_, err = client.Sync(ctx, lorepackage.SyncOptions{})
	if err != nil {
		t.Fatalf("Sync() error: %v", err)
	}

	// Skip if index doesn't exist yet
	if !client.FileExists("knowledge/migration/index.yaml") {
		t.Skip("knowledge/migration/index.yaml not yet in registry")
	}

	// Test LoadSignatures
	signatures, err := LoadSignatures(client)
	if err != nil {
		t.Fatalf("LoadSignatures() error: %v", err)
	}

	if len(signatures) == 0 {
		t.Fatal("expected at least one signature")
	}

	// Verify signature structure
	for _, sig := range signatures {
		if sig.Name == "" {
			t.Error("signature has empty name")
		}
		if len(sig.Markers) == 0 {
			t.Errorf("signature %q has no markers", sig.Name)
		}
		for _, marker := range sig.Markers {
			if marker.Type == "" {
				t.Errorf("signature %q has marker with empty type", sig.Name)
			}
			if marker.Confidence <= 0 || marker.Confidence > 1 {
				t.Errorf("signature %q marker has invalid confidence: %v", sig.Name, marker.Confidence)
			}
		}
	}

	t.Logf("loaded %d signatures", len(signatures))
	for _, sig := range signatures {
		t.Logf("  %s: %d markers", sig.Name, len(sig.Markers))
	}
}

func TestDetectWithSignatures(t *testing.T) {
	// Create test fixtures
	tmpDir, err := os.MkdirTemp("", "detect-sig-test-*")
	if err != nil {
		t.Fatalf("creating temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Create a stow-like structure with .stow-local-ignore
	stowDir := filepath.Join(tmpDir, "stow-repo")
	if err := os.MkdirAll(filepath.Join(stowDir, "bash"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stowDir, ".stow-local-ignore"), []byte("*.bak\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stowDir, "bash", ".bashrc"), []byte("# bashrc"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a chezmoi-like structure
	chezmoiDir := filepath.Join(tmpDir, "chezmoi-repo")
	if err := os.MkdirAll(chezmoiDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(chezmoiDir, ".chezmoiroot"), []byte("home"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a tuckr-like structure
	tuckrDir := filepath.Join(tmpDir, "tuckr-repo")
	if err := os.MkdirAll(filepath.Join(tuckrDir, "Configs"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tuckrDir, "Hooks.toml"), []byte("[hooks]\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create empty directory for no-match test
	emptyDir := filepath.Join(tmpDir, "empty")
	if err := os.MkdirAll(emptyDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Define test signatures matching actual registry format
	signatures := []Signature{
		{
			Name: "stow",
			Markers: []Marker{
				{Type: "file", Path: ".stow-local-ignore", Confidence: 1.0},
				{Type: "file", Path: ".stowrc", Confidence: 0.95},
			},
		},
		{
			Name: "chezmoi",
			Markers: []Marker{
				{Type: "file", Path: ".chezmoiroot", Confidence: 0.95},
				{Type: "file", Path: ".chezmoi.toml.tmpl", Confidence: 0.9},
			},
		},
		{
			Name: "tuckr",
			Markers: []Marker{
				{Type: "file", Path: "Hooks.toml", Confidence: 0.95},
				{Type: "directory", Path: "Configs", Confidence: 0.8},
			},
		},
	}

	// Test stow detection
	results := DetectWithSignatures(stowDir, signatures)
	if len(results) == 0 {
		t.Error("expected to detect stow structure")
	} else if results[0].System != "stow" {
		t.Errorf("expected system 'stow', got %q", results[0].System)
	} else if results[0].Confidence != 1.0 {
		t.Errorf("expected confidence 1.0, got %v", results[0].Confidence)
	}

	// Test chezmoi detection
	results = DetectWithSignatures(chezmoiDir, signatures)
	if len(results) == 0 {
		t.Error("expected to detect chezmoi structure")
	} else if results[0].System != "chezmoi" {
		t.Errorf("expected system 'chezmoi', got %q", results[0].System)
	}

	// Test tuckr detection
	results = DetectWithSignatures(tuckrDir, signatures)
	if len(results) == 0 {
		t.Error("expected to detect tuckr structure")
	} else if results[0].System != "tuckr" {
		t.Errorf("expected system 'tuckr', got %q", results[0].System)
	}

	// Test no match
	results = DetectWithSignatures(emptyDir, signatures)
	if len(results) != 0 {
		t.Errorf("expected no matches for empty dir, got %d", len(results))
	}
}

func TestEvaluateMarkers(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "marker-test-*")
	if err != nil {
		t.Fatalf("creating temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Create test files
	if err := os.WriteFile(filepath.Join(tmpDir, "config.yaml"), []byte("key: value"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tmpDir, "subdir"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "marker.txt"), []byte("MAGIC_STRING_HERE"), 0644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		marker   Marker
		expected bool
	}{
		{
			name:     "file exists",
			marker:   Marker{Type: "file", Path: "config.yaml", Confidence: 0.8},
			expected: true,
		},
		{
			name:     "file not exists",
			marker:   Marker{Type: "file", Path: "missing.yaml", Confidence: 0.8},
			expected: false,
		},
		{
			name:     "directory exists",
			marker:   Marker{Type: "directory", Path: "subdir", Confidence: 0.8},
			expected: true,
		},
		{
			name:     "directory not exists",
			marker:   Marker{Type: "directory", Path: "missing", Confidence: 0.8},
			expected: false,
		},
		{
			name:     "file_contains match",
			marker:   Marker{Type: "file_contains", Path: "marker.txt", Pattern: "MAGIC_STRING", Confidence: 0.9},
			expected: true,
		},
		{
			name:     "file_contains no match",
			marker:   Marker{Type: "file_contains", Path: "marker.txt", Pattern: "NOT_FOUND", Confidence: 0.9},
			expected: false,
		},
		{
			name:     "file_pattern match",
			marker:   Marker{Type: "file_pattern", Pattern: "*.yaml", Confidence: 0.7},
			expected: true,
		},
		{
			name:     "file_pattern no match",
			marker:   Marker{Type: "file_pattern", Pattern: "*.json", Confidence: 0.7},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			match, ok := evaluateMarker(tmpDir, tt.marker)
			if ok != tt.expected {
				t.Errorf("evaluateMarker() = %v, want %v", ok, tt.expected)
			}
			if ok && match.Confidence != tt.marker.Confidence {
				t.Errorf("match.Confidence = %v, want %v", match.Confidence, tt.marker.Confidence)
			}
		})
	}
}

func TestMatchPattern(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		match   bool
	}{
		{"simple glob", "*.txt", true},
		{"no match", "*.yaml", false},
		{"alternation match", "test.{txt,md}", true},
		{"alternation no match", "test.{yaml,json}", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchPattern("test.txt", tt.pattern)
			if got != tt.match {
				t.Errorf("matchPattern(%q, %q) = %v, want %v", "test.txt", tt.pattern, got, tt.match)
			}
		})
	}
}
