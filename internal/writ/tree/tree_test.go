// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

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
		{"foo.template", "foo", []Operation{OpExpand, OpCopy}},
		{"foo.age", "foo", []Operation{OpDecrypt, OpCopy}},
		{"foo.template.age", "foo", []Operation{OpDecrypt, OpExpand, OpCopy}},
		{".bashrc", ".bashrc", []Operation{OpLink}},
		{".bashrc.template", ".bashrc", []Operation{OpExpand, OpCopy}},
		{"config.yaml.template.age", "config.yaml", []Operation{OpDecrypt, OpExpand, OpCopy}},
		{"packages.manifest", "packages.manifest", []Operation{OpDelegate}},
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

	tree, err := Build(BuildConfig{
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
	if len(tree.MatchedDirs) != expectedDirs {
		t.Errorf("got %d matched dirs, want %d", len(tree.MatchedDirs), expectedDirs)
		for _, m := range tree.MatchedDirs {
			t.Logf("  matched: %s", filepath.Base(m.Path))
		}
	}

	// Verify files were found
	expectedFiles := 6 // all the files we created
	if len(tree.Nodes) != expectedFiles {
		t.Errorf("got %d nodes, want %d", len(tree.Nodes), expectedFiles)
		for _, n := range tree.Nodes {
			t.Logf("  node: %s -> %s %v", n.RelSource, n.RelTarget, n.Operations.Strings())
		}
	}

	// Verify template detection
	if tree.TemplateCount() != 1 {
		t.Errorf("template count = %d, want 1", tree.TemplateCount())
	}

	// Verify link count
	if tree.LinkCount() != 5 {
		t.Errorf("link count = %d, want 5", tree.LinkCount())
	}

	// Test output
	output := tree.String()
	if output == "" {
		t.Error("String() returned empty")
	}
	t.Logf("Tree output:\n%s", output)

	// Test JSON
	jsonBytes, err := tree.JSON()
	if err != nil {
		t.Errorf("JSON() failed: %v", err)
	}
	if len(jsonBytes) == 0 {
		t.Error("JSON() returned empty")
	}
}

func TestBuildWithCollisions(t *testing.T) {
	// Create temp directory with overlapping files
	tmpDir := t.TempDir()

	// Create directories with different specificity
	dirs := []string{
		"all",          // specificity 0
		"all.Darwin",   // specificity 1
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

	tree, err := Build(BuildConfig{
		SourceRoot: tmpDir,
		TargetRoot: targetDir,
		Projects:   []string{"all"},
		Segments:   darwinSegs,
	})

	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Should have exactly 1 node (collision resolved)
	if len(tree.Nodes) != 1 {
		t.Errorf("got %d nodes, want 1 (collision should resolve to single node)", len(tree.Nodes))
	}

	// Should have 1 collision recorded
	if len(tree.Collisions) != 1 {
		t.Errorf("got %d collisions, want 1", len(tree.Collisions))
	}

	// The winner should be the more specific one (all.Darwin)
	if len(tree.Nodes) > 0 {
		node := tree.Nodes[0]
		if len(node.Suffixes) != 1 || node.Suffixes[0] != "Darwin" {
			t.Errorf("winner should be from all.Darwin, got suffixes %v", node.Suffixes)
		}
	}

	// Verify collision details
	if len(tree.Collisions) > 0 {
		c := tree.Collisions[0]
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
	output := tree.String()
	if !strings.Contains(output, "Collisions (1)") {
		t.Error("output should contain collision warning")
	}

	t.Logf("Tree output:\n%s", output)
}

func TestNodeHelpers(t *testing.T) {
	tests := []struct {
		ops        []Operation
		isSecret   bool
		isTemplate bool
		isLink     bool
		isDelegate bool
	}{
		{[]Operation{OpLink}, false, false, true, false},
		{[]Operation{OpExpand, OpCopy}, false, true, false, false},
		{[]Operation{OpDecrypt, OpCopy}, true, false, false, false},
		{[]Operation{OpDecrypt, OpExpand, OpCopy}, true, true, false, false},
		{[]Operation{OpDelegate}, false, false, false, true},
	}

	for _, tt := range tests {
		n := &Node{Operations: tt.ops}
		if n.IsSecret() != tt.isSecret {
			t.Errorf("IsSecret() with %v = %v, want %v", tt.ops, n.IsSecret(), tt.isSecret)
		}
		if n.IsTemplate() != tt.isTemplate {
			t.Errorf("IsTemplate() with %v = %v, want %v", tt.ops, n.IsTemplate(), tt.isTemplate)
		}
		if n.IsLink() != tt.isLink {
			t.Errorf("IsLink() with %v = %v, want %v", tt.ops, n.IsLink(), tt.isLink)
		}
		if n.IsDelegate() != tt.isDelegate {
			t.Errorf("IsDelegate() with %v = %v, want %v", tt.ops, n.IsDelegate(), tt.isDelegate)
		}
	}
}
