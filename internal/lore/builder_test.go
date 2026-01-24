// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

package lore

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/NobleFactor/devlore-cli/internal/engine"
)

func TestBuilderBuildGraph(t *testing.T) {
	tmpDir := t.TempDir()
	manifest := filepath.Join(tmpDir, "packages.manifest")

	content := `# Development tools
docker --with rootless --with compose
kubectl
gh
neovim --with lsp
`
	if err := os.WriteFile(manifest, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	builder := &Builder{}
	graph, err := builder.BuildGraph(context.Background(), manifest, engine.BuildOptions{})
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}

	if len(graph.Nodes) != 4 {
		t.Fatalf("expected 4 nodes, got %d", len(graph.Nodes))
	}

	// Check docker node
	docker := graph.Nodes[0]
	if docker.ID != "docker" {
		t.Errorf("expected ID 'docker', got %q", docker.ID)
	}
	if docker.Operations[0] != "install" {
		t.Errorf("expected operation 'install', got %q", docker.Operations[0])
	}
	if docker.Metadata["features"] != "rootless,compose" {
		t.Errorf("expected features 'rootless,compose', got %q", docker.Metadata["features"])
	}
	if docker.Metadata["tool"] != "lore" {
		t.Errorf("expected tool 'lore', got %q", docker.Metadata["tool"])
	}

	// Check kubectl node (no features)
	kubectl := graph.Nodes[1]
	if kubectl.ID != "kubectl" {
		t.Errorf("expected ID 'kubectl', got %q", kubectl.ID)
	}
	if _, ok := kubectl.Metadata["features"]; ok {
		t.Error("expected no features on kubectl")
	}
}

func TestBuilderBuildGraphWithGlobalFeatures(t *testing.T) {
	tmpDir := t.TempDir()
	manifest := filepath.Join(tmpDir, "packages.manifest")

	content := `docker --with compose
kubectl
`
	if err := os.WriteFile(manifest, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	builder := &Builder{}
	graph, err := builder.BuildGraph(context.Background(), manifest, engine.BuildOptions{
		Features: []string{"debug"},
	})
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}

	// Docker should have both package-level and global features
	docker := graph.Nodes[0]
	if docker.Metadata["features"] != "compose,debug" {
		t.Errorf("expected features 'compose,debug', got %q", docker.Metadata["features"])
	}

	// kubectl should have only global features
	kubectl := graph.Nodes[1]
	if kubectl.Metadata["features"] != "debug" {
		t.Errorf("expected features 'debug', got %q", kubectl.Metadata["features"])
	}
}

func TestBuilderBuildGraphEmptyManifest(t *testing.T) {
	tmpDir := t.TempDir()
	manifest := filepath.Join(tmpDir, "packages.manifest")

	content := `# Only comments
# and blank lines

`
	if err := os.WriteFile(manifest, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	builder := &Builder{}
	graph, err := builder.BuildGraph(context.Background(), manifest, engine.BuildOptions{})
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}

	if len(graph.Nodes) != 0 {
		t.Errorf("expected 0 nodes for empty manifest, got %d", len(graph.Nodes))
	}
}

func TestBuilderBuildGraphMissingFile(t *testing.T) {
	builder := &Builder{}
	_, err := builder.BuildGraph(context.Background(), "/nonexistent/packages.manifest", engine.BuildOptions{})
	if err == nil {
		t.Error("expected error for missing manifest")
	}
}

func TestLoadManifest(t *testing.T) {
	tmpDir := t.TempDir()
	manifest := filepath.Join(tmpDir, "test.manifest")

	content := `# comment
docker --with rootless
kubectl

gh
`
	os.WriteFile(manifest, []byte(content), 0644)

	entries, err := loadManifest(manifest)
	if err != nil {
		t.Fatalf("loadManifest: %v", err)
	}

	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	if entries[0].Name != "docker" {
		t.Errorf("expected 'docker', got %q", entries[0].Name)
	}
	if len(entries[0].Features) != 1 || entries[0].Features[0] != "rootless" {
		t.Errorf("expected features ['rootless'], got %v", entries[0].Features)
	}
	if entries[1].Name != "kubectl" {
		t.Errorf("expected 'kubectl', got %q", entries[1].Name)
	}
}

func TestParseLine(t *testing.T) {
	tests := []struct {
		line     string
		name     string
		features []string
	}{
		{"docker", "docker", nil},
		{"docker --with rootless", "docker", []string{"rootless"}},
		{"docker --with rootless --with compose", "docker", []string{"rootless", "compose"}},
		{"neovim --with lsp --with treesitter", "neovim", []string{"lsp", "treesitter"}},
	}

	for _, tt := range tests {
		entry := parseLine(tt.line)
		if entry.Name != tt.name {
			t.Errorf("parseLine(%q): name = %q, want %q", tt.line, entry.Name, tt.name)
		}
		if len(entry.Features) != len(tt.features) {
			t.Errorf("parseLine(%q): features = %v, want %v", tt.line, entry.Features, tt.features)
			continue
		}
		for i, f := range entry.Features {
			if f != tt.features[i] {
				t.Errorf("parseLine(%q): feature[%d] = %q, want %q", tt.line, i, f, tt.features[i])
			}
		}
	}
}

// Verify Builder implements engine.GraphBuilder interface.
var _ engine.GraphBuilder = (*Builder)(nil)
