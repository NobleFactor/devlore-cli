// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package tree

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/internal/writ/segment"
)

func TestProcessingPipeline(t *testing.T) {
	tests := []struct {
		filename   string
		targetName string
		ops        []Operation
	}{
		{"foo", "foo", []Operation{OpLink}},
		{"foo.template", "foo", []Operation{OpRender, OpCopy}},
		{"foo.age", "foo", []Operation{OpDecrypt, OpCopy}},
		{"foo.sops", "foo", []Operation{OpDecrypt, OpCopy}},
		{"foo.template.age", "foo", []Operation{OpDecrypt, OpRender, OpCopy}},
		{"foo.template.sops", "foo", []Operation{OpDecrypt, OpRender, OpCopy}},
		{".bashrc", ".bashrc", []Operation{OpLink}},
		{".bashrc.template", ".bashrc", []Operation{OpRender, OpCopy}},
		{"config.yaml.template.age", "config.yaml", []Operation{OpDecrypt, OpRender, OpCopy}},
		{"packages.manifest", "packages.manifest", []Operation{OpPackages}},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			name, ops := ProcessingPipeline(tt.filename)
			if name != tt.targetName {
				t.Errorf("name = %q, want %q", name, tt.targetName)
			}
			if len(ops) != len(tt.ops) {
				t.Errorf("ops = %v, want %v", ops.Strings(), Operations(tt.ops).Strings())
				return
			}
			for i := range ops {
				if ops[i] != tt.ops[i] {
					t.Errorf("ops[%d] = %v, want %v", i, ops[i], tt.ops[i])
				}
			}
		})
	}
}

func TestBuild(t *testing.T) {
	// Create temp directory with test structure
	tmpDir := t.TempDir()

	// Create project directories
	dirs := []string{
		"all",
		"all.Darwin",
		"all.Unix",
		"noblefactor",
		"noblefactor.Unix",
	}

	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(tmpDir, d), 0755); err != nil {
			t.Fatalf("failed to create dir %s: %v", d, err)
		}
	}

	// Create test files
	files := map[string]string{
		"all/.bashrc":                      "bashrc content",
		"all/.config/test.yaml":            "test config",
		"all.Darwin/.config/darwin.yaml":   "darwin config",
		"all.Unix/.config/unix.yaml":       "unix config",
		"noblefactor/.ssh/config":          "ssh config",
		"noblefactor.Unix/script.template": "template content",
	}

	for path, content := range files {
		fullPath := filepath.Join(tmpDir, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			t.Fatalf("failed to create dir for %s: %v", path, err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write %s: %v", path, err)
		}
	}

	// Build tree for Darwin
	darwinSegs := segment.Segments{
		{Name: "OS", Value: "Darwin"},
		{Name: "DISTRO", Value: ""},
		{Name: "ARCH", Value: "arm64"},
	}

	targetDir := t.TempDir()

	result, err := Build(BuildConfig{
		SourceRoot: tmpDir,
		TargetRoot: targetDir,
		Projects:   []string{"all", "noblefactor"},
		Segments:   darwinSegs,
	})

	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Verify matched directories
	expectedDirs := 5 // all, all.Darwin, all.Unix, noblefactor, noblefactor.Unix
	if len(result.MatchedDirs) != expectedDirs {
		t.Errorf("got %d matched dirs, want %d", len(result.MatchedDirs), expectedDirs)
		for _, m := range result.MatchedDirs {
			t.Logf("  matched: %s", filepath.Base(m.Path))
		}
	}

	// Verify files were found
	expectedFiles := 6 // all the files we created
	if len(result.Files) != expectedFiles {
		t.Errorf("got %d nodes, want %d", len(result.Files), expectedFiles)
		for _, n := range result.Files {
			t.Logf("  node: %s ops=%v", n.ID, n.Operations)
		}
	}

	// Verify template detection
	if result.TemplateCount() != 1 {
		t.Errorf("template count = %d, want 1", result.TemplateCount())
	}

	// Verify link count
	if result.LinkCount() != 5 {
		t.Errorf("link count = %d, want 5", result.LinkCount())
	}

	// Test output
	output := result.String()
	if output == "" {
		t.Error("String() returned empty")
	}
	t.Logf("Tree output:\n%s", output)
}

func TestBuildWithCollisions(t *testing.T) {
	// Create temp directory with overlapping files
	tmpDir := t.TempDir()

	// Create directories with different specificity
	dirs := []string{
		"all",        // specificity 0
		"all.Darwin", // specificity 1
	}

	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(tmpDir, d), 0755); err != nil {
			t.Fatalf("failed to create dir %s: %v", d, err)
		}
	}

	// Create the same file in both directories
	// all/.bashrc (less specific)
	if err := os.WriteFile(filepath.Join(tmpDir, "all", ".bashrc"), []byte("generic"), 0644); err != nil {
		t.Fatal(err)
	}
	// all.Darwin/.bashrc (more specific - should win)
	if err := os.WriteFile(filepath.Join(tmpDir, "all.Darwin", ".bashrc"), []byte("darwin specific"), 0644); err != nil {
		t.Fatal(err)
	}

	darwinSegs := segment.Segments{
		{Name: "OS", Value: "Darwin"},
		{Name: "DISTRO", Value: ""},
		{Name: "ARCH", Value: "arm64"},
	}

	targetDir := t.TempDir()

	result, err := Build(BuildConfig{
		SourceRoot: tmpDir,
		TargetRoot: targetDir,
		Projects:   []string{"all"},
		Segments:   darwinSegs,
	})

	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Should have exactly 1 node (collision resolved)
	if len(result.Files) != 1 {
		t.Errorf("got %d nodes, want 1 (collision should resolve to single node)", len(result.Files))
	}

	// Should have 1 collision recorded
	if len(result.Collisions) != 1 {
		t.Errorf("got %d collisions, want 1", len(result.Collisions))
	}

	// The winner should be the more specific one (all.Darwin)
	if len(result.Files) > 0 {
		node := result.Files[0]
		// The winner source should contain "all.Darwin"
		if !strings.Contains(node.Source, "all.Darwin") {
			t.Errorf("winner should be from all.Darwin, got source %s", node.Source)
		}
	}

	// Verify collision details
	if len(result.Collisions) > 0 {
		c := result.Collisions[0]
		if c.Target != ".bashrc" {
			t.Errorf("collision target = %q, want %q", c.Target, ".bashrc")
		}
		if c.WinnerSpecificity != 1 {
			t.Errorf("winner specificity = %d, want 1", c.WinnerSpecificity)
		}
		if c.LoserSpecificity != 0 {
			t.Errorf("loser specificity = %d, want 0", c.LoserSpecificity)
		}
	}

	// Verify output includes collision warning
	output := result.String()
	if !strings.Contains(output, "Collisions (1)") {
		t.Error("output should contain collision warning")
	}

	t.Logf("Tree output:\n%s", output)
}

func TestOperationHelpers(t *testing.T) {
	tests := []struct {
		ops         Operations
		hasCopy     bool
		hasPackages bool
	}{
		{Operations{OpLink}, false, false},
		{Operations{OpRender, OpCopy}, true, false},
		{Operations{OpDecrypt, OpCopy}, true, false},
		{Operations{OpDecrypt, OpRender, OpCopy}, true, false},
		{Operations{OpPackages}, false, true},
	}

	for _, tt := range tests {
		if got := tt.ops.HasCopy(); got != tt.hasCopy {
			t.Errorf("HasCopy() with %v = %v, want %v", tt.ops.Strings(), got, tt.hasCopy)
		}
		if got := tt.ops.HasPackages(); got != tt.hasPackages {
			t.Errorf("HasPackages() with %v = %v, want %v", tt.ops.Strings(), got, tt.hasPackages)
		}
	}
}

func TestBuildMultiSource(t *testing.T) {
	// Create base and personal layer directories
	baseDir := t.TempDir()
	personalDir := t.TempDir()
	targetDir := t.TempDir()

	// Create project in base layer
	if err := os.MkdirAll(filepath.Join(baseDir, "all"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "all", ".bashrc"), []byte("base bashrc"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create project in personal layer with different file
	if err := os.MkdirAll(filepath.Join(personalDir, "all"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(personalDir, "all", ".zshrc"), []byte("personal zshrc"), 0644); err != nil {
		t.Fatal(err)
	}

	segs := segment.Segments{
		{Name: "OS", Value: "Darwin"},
	}

	sources := []LayerSource{
		{Layer: "base", Path: baseDir, Order: 0, SourceRoot: baseDir, TargetRoot: targetDir},
		{Layer: "personal", Path: personalDir, Order: 2, SourceRoot: personalDir, TargetRoot: targetDir},
	}

	result, err := Build(BuildConfig{
		Sources:  sources,
		Projects: []string{"all"},
		Segments: segs,
	})

	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Should have 2 nodes (one from each layer)
	if len(result.Files) != 2 {
		t.Errorf("got %d nodes, want 2", len(result.Files))
		for _, f := range result.Files {
			t.Logf("  file: %s layer=%s", f.ID, f.Layer)
		}
	}

	// Verify layer is set
	for _, f := range result.Files {
		if f.Layer == "" {
			t.Errorf("file %s missing layer", f.ID)
		}
	}

	// No collisions (different files)
	if len(result.Collisions) != 0 {
		t.Errorf("got %d collisions, want 0", len(result.Collisions))
	}
}

func TestBuildMultiSourceLayerPrecedence(t *testing.T) {
	// Create base and personal layer directories
	baseDir := t.TempDir()
	personalDir := t.TempDir()
	targetDir := t.TempDir()

	// Create same file in both layers
	if err := os.MkdirAll(filepath.Join(baseDir, "all"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "all", ".bashrc"), []byte("base bashrc"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(filepath.Join(personalDir, "all"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(personalDir, "all", ".bashrc"), []byte("personal bashrc"), 0644); err != nil {
		t.Fatal(err)
	}

	segs := segment.Segments{
		{Name: "OS", Value: "Darwin"},
	}

	sources := []LayerSource{
		{Layer: "base", Path: baseDir, Order: 0, SourceRoot: baseDir, TargetRoot: targetDir},
		{Layer: "personal", Path: personalDir, Order: 2, SourceRoot: personalDir, TargetRoot: targetDir},
	}

	result, err := Build(BuildConfig{
		Sources:  sources,
		Projects: []string{"all"},
		Segments: segs,
	})

	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Should have 1 node (collision resolved)
	if len(result.Files) != 1 {
		t.Errorf("got %d nodes, want 1", len(result.Files))
	}

	// Should have 1 collision
	if len(result.Collisions) != 1 {
		t.Errorf("got %d collisions, want 1", len(result.Collisions))
	}

	// Personal layer should win
	if len(result.Files) > 0 {
		f := result.Files[0]
		if f.Layer != "personal" {
			t.Errorf("winner layer = %s, want personal", f.Layer)
		}
		if !strings.Contains(f.Source, personalDir) {
			t.Errorf("winner source should be from personal layer, got %s", f.Source)
		}
	}

	// Verify collision details
	if len(result.Collisions) > 0 {
		c := result.Collisions[0]
		if c.WinnerLayer != "personal" {
			t.Errorf("collision winner layer = %s, want personal", c.WinnerLayer)
		}
		if c.LoserLayer != "base" {
			t.Errorf("collision loser layer = %s, want base", c.LoserLayer)
		}
	}
}

func TestBuildMultiSourceSpecificityWithinLayer(t *testing.T) {
	// Create single layer with different specificities
	layerDir := t.TempDir()
	targetDir := t.TempDir()

	// Create all (specificity 0) and all.Darwin (specificity 1)
	if err := os.MkdirAll(filepath.Join(layerDir, "all"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(layerDir, "all.Darwin"), 0755); err != nil {
		t.Fatal(err)
	}

	// Same file in both
	if err := os.WriteFile(filepath.Join(layerDir, "all", ".bashrc"), []byte("generic"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(layerDir, "all.Darwin", ".bashrc"), []byte("darwin"), 0644); err != nil {
		t.Fatal(err)
	}

	segs := segment.Segments{
		{Name: "OS", Value: "Darwin"},
	}

	sources := []LayerSource{
		{Layer: "personal", Path: layerDir, Order: 2, SourceRoot: layerDir, TargetRoot: targetDir},
	}

	result, err := Build(BuildConfig{
		Sources:  sources,
		Projects: []string{"all"},
		Segments: segs,
	})

	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Should have 1 node (collision resolved)
	if len(result.Files) != 1 {
		t.Errorf("got %d nodes, want 1", len(result.Files))
	}

	// More specific (all.Darwin) should win
	if len(result.Files) > 0 {
		node := result.Files[0]
		if !strings.Contains(node.Source, "all.Darwin") {
			t.Errorf("winner should be from all.Darwin, got %s", node.Source)
		}
	}

	// Verify collision details
	if len(result.Collisions) != 1 {
		t.Errorf("got %d collisions, want 1", len(result.Collisions))
	} else {
		c := result.Collisions[0]
		if c.WinnerSpecificity != 1 {
			t.Errorf("winner specificity = %d, want 1", c.WinnerSpecificity)
		}
		if c.LoserSpecificity != 0 {
			t.Errorf("loser specificity = %d, want 0", c.LoserSpecificity)
		}
	}
}

func TestBuildMultiSourceLayerBeatsSpecificity(t *testing.T) {
	// Layer precedence should beat specificity
	baseDir := t.TempDir()
	personalDir := t.TempDir()
	targetDir := t.TempDir()

	// Base layer with high specificity (all.Darwin)
	if err := os.MkdirAll(filepath.Join(baseDir, "all.Darwin"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "all.Darwin", ".bashrc"), []byte("base darwin"), 0644); err != nil {
		t.Fatal(err)
	}

	// Personal layer with low specificity (all)
	if err := os.MkdirAll(filepath.Join(personalDir, "all"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(personalDir, "all", ".bashrc"), []byte("personal generic"), 0644); err != nil {
		t.Fatal(err)
	}

	segs := segment.Segments{
		{Name: "OS", Value: "Darwin"},
	}

	sources := []LayerSource{
		{Layer: "base", Path: baseDir, Order: 0, SourceRoot: baseDir, TargetRoot: targetDir},
		{Layer: "personal", Path: personalDir, Order: 2, SourceRoot: personalDir, TargetRoot: targetDir},
	}

	result, err := Build(BuildConfig{
		Sources:  sources,
		Projects: []string{"all"},
		Segments: segs,
	})

	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Should have 1 node
	if len(result.Files) != 1 {
		t.Errorf("got %d nodes, want 1", len(result.Files))
	}

	// Personal layer should win despite lower specificity
	if len(result.Files) > 0 {
		f := result.Files[0]
		if f.Layer != "personal" {
			t.Errorf("winner layer = %s, want personal (layer beats specificity)", f.Layer)
		}
	}

	// Verify collision
	if len(result.Collisions) != 1 {
		t.Errorf("got %d collisions, want 1", len(result.Collisions))
	} else {
		c := result.Collisions[0]
		// Personal wins with specificity 0
		if c.WinnerSpecificity != 0 {
			t.Errorf("winner specificity = %d, want 0 (personal/all)", c.WinnerSpecificity)
		}
		// Base loses with specificity 1
		if c.LoserSpecificity != 1 {
			t.Errorf("loser specificity = %d, want 1 (base/all.Darwin)", c.LoserSpecificity)
		}
		if c.WinnerLayer != "personal" {
			t.Errorf("winner layer = %s, want personal", c.WinnerLayer)
		}
		if c.LoserLayer != "base" {
			t.Errorf("loser layer = %s, want base", c.LoserLayer)
		}
	}
}
