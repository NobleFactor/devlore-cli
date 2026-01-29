// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package receipt

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestNewReceipt(t *testing.T) {
	segments := map[string]string{"OS": "Darwin", "ARCH": "arm64"}
	rcpt := New("/home/user/environment", "/home/user", []string{"all", "noblefactor"}, segments)

	if rcpt.Version != CurrentVersion {
		t.Errorf("expected version %q, got %q", CurrentVersion, rcpt.Version)
	}
	if rcpt.Format != "graph" {
		t.Errorf("expected format 'graph', got %q", rcpt.Format)
	}
	if rcpt.Tool != "writ" {
		t.Errorf("expected tool 'writ', got %q", rcpt.Tool)
	}
	if rcpt.Context.SourceRoot != "/home/user/environment" {
		t.Errorf("expected source_root '/home/user/environment', got %q", rcpt.Context.SourceRoot)
	}
	if len(rcpt.Roots) != 2 {
		t.Errorf("expected 2 roots, got %d", len(rcpt.Roots))
	}
	if rcpt.Platform.OS == "" {
		t.Error("expected platform OS to be set")
	}
}

func TestComputeSummary(t *testing.T) {
	rcpt := &Receipt{
		Nodes: []Node{
			{ID: ".bashrc", Operation: "link", Status: "completed"},
			{ID: ".gitconfig", Operation: "expand", Status: "completed"},
			{ID: ".npmrc", Operation: "decrypt", Status: "completed"},
			{ID: ".config/packages.manifest", Operation: "delegate", Status: "completed"},
			{ID: ".conflicted", Status: "skipped"},
		},
	}

	rcpt.ComputeSummary()

	if rcpt.Summary.TotalFiles != 3 {
		t.Errorf("expected 3 total files, got %d", rcpt.Summary.TotalFiles)
	}
	if rcpt.Summary.Links != 1 {
		t.Errorf("expected 1 link, got %d", rcpt.Summary.Links)
	}
	if rcpt.Summary.Templates != 1 {
		t.Errorf("expected 1 template, got %d", rcpt.Summary.Templates)
	}
	if rcpt.Summary.Secrets != 1 {
		t.Errorf("expected 1 secret, got %d", rcpt.Summary.Secrets)
	}
	if rcpt.Summary.Delegated != 1 {
		t.Errorf("expected 1 delegated, got %d", rcpt.Summary.Delegated)
	}
	if rcpt.Summary.Skipped != 1 {
		t.Errorf("expected 1 skipped, got %d", rcpt.Summary.Skipped)
	}
}

func TestLegacyToGraph(t *testing.T) {
	lr := &LegacyReceipt{
		Version:    "2",
		Timestamp:  time.Date(2026, 1, 21, 10, 30, 0, 0, time.UTC),
		SourceRoot: "/home/user/environment",
		TargetRoot: "/home/user",
		Projects:   []string{"all", "noblefactor"},
		Segments:   map[string]string{"OS": "Darwin"},
		Entries: []LegacyEntry{
			{
				Source:     "/home/user/environment/all/.bashrc",
				Target:     "/home/user/.bashrc",
				RelTarget:  ".bashrc",
				Operations: []string{"link"},
				Project:    "all",
			},
			{
				Source:          "/home/user/environment/noblefactor/.gitconfig.template",
				Target:          "/home/user/.gitconfig",
				RelTarget:       ".gitconfig",
				Operations:      []string{"expand", "copy"},
				Project:         "noblefactor",
				AlreadyDeployed: false,
				SourceChecksum:  "sha256:abc123",
				TargetChecksum:  "sha256:def456",
			},
			{
				Source:          "/home/user/environment/all/.zshrc",
				Target:          "/home/user/.zshrc",
				RelTarget:       ".zshrc",
				Operations:      []string{"link"},
				Project:         "all",
				AlreadyDeployed: true,
			},
		},
		Delegated: []string{"/home/user/environment/noblefactor/.config/packages.manifest"},
		Skipped:   []string{".conflicted"},
	}

	rcpt := lr.ToGraph()

	if rcpt.Version != CurrentVersion {
		t.Errorf("expected version %q, got %q", CurrentVersion, rcpt.Version)
	}
	if rcpt.Format != "graph" {
		t.Errorf("expected format 'graph', got %q", rcpt.Format)
	}
	if rcpt.Tool != "writ" {
		t.Errorf("expected tool 'writ', got %q", rcpt.Tool)
	}
	if rcpt.Context.SourceRoot != "/home/user/environment" {
		t.Errorf("expected source root, got %q", rcpt.Context.SourceRoot)
	}

	// 3 entries + 1 delegated + 1 skipped = 5 nodes
	if len(rcpt.Nodes) != 5 {
		t.Fatalf("expected 5 nodes, got %d", len(rcpt.Nodes))
	}

	// Check first node
	if rcpt.Nodes[0].ID != ".bashrc" {
		t.Errorf("expected node ID '.bashrc', got %q", rcpt.Nodes[0].ID)
	}
	if rcpt.Nodes[0].Operation != "link" {
		t.Errorf("expected operation 'link', got %q", rcpt.Nodes[0].Operation)
	}

	// Check template node uses primary operation
	if rcpt.Nodes[1].Operation != "expand" {
		t.Errorf("expected operation 'expand', got %q", rcpt.Nodes[1].Operation)
	}
	if rcpt.Nodes[1].SourceChecksum != "sha256:abc123" {
		t.Errorf("expected source checksum preserved")
	}

	// Check already_deployed annotation
	if rcpt.Nodes[2].Annotations == nil || rcpt.Nodes[2].Annotations["already_deployed"] != "true" {
		t.Error("expected already_deployed annotation on third node")
	}

	// Check delegated node
	if rcpt.Nodes[3].Operation != "delegate" {
		t.Errorf("expected delegate operation, got %q", rcpt.Nodes[3].Operation)
	}
	if rcpt.Nodes[3].DelegateTo != "lore" {
		t.Errorf("expected delegate_to 'lore', got %q", rcpt.Nodes[3].DelegateTo)
	}

	// Check skipped node
	if rcpt.Nodes[4].Status != "skipped" {
		t.Errorf("expected status 'skipped', got %q", rcpt.Nodes[4].Status)
	}
}

func TestLoadLegacyReceipt(t *testing.T) {
	tmpDir := t.TempDir()

	// Write a v2 legacy receipt
	legacy := LegacyReceipt{
		Version:    "2",
		Timestamp:  time.Date(2026, 1, 21, 10, 30, 0, 0, time.UTC),
		SourceRoot: "/home/user/environment",
		TargetRoot: "/home/user",
		Projects:   []string{"all"},
		Segments:   map[string]string{},
		Entries: []LegacyEntry{
			{
				Source:     "/home/user/environment/all/.bashrc",
				Target:     "/home/user/.bashrc",
				RelTarget:  ".bashrc",
				Operations: []string{"link"},
				Project:    "all",
			},
		},
	}

	data, err := yaml.Marshal(legacy)
	if err != nil {
		t.Fatalf("marshal legacy receipt: %v", err)
	}

	path := filepath.Join(tmpDir, "legacy.yaml")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write legacy receipt: %v", err)
	}

	// Load should detect version and convert
	rcpt, err := Load(path)
	if err != nil {
		t.Fatalf("load legacy receipt: %v", err)
	}

	if rcpt.Version != CurrentVersion {
		t.Errorf("expected converted version %q, got %q", CurrentVersion, rcpt.Version)
	}
	if rcpt.Format != "graph" {
		t.Errorf("expected format 'graph', got %q", rcpt.Format)
	}
	if len(rcpt.Nodes) != 1 {
		t.Errorf("expected 1 node, got %d", len(rcpt.Nodes))
	}
}

func TestLoadV4Receipt(t *testing.T) {
	tmpDir := t.TempDir()

	// Write a v4 receipt
	v4 := Receipt{
		Version:   "4",
		Format:    "graph",
		Timestamp: time.Date(2026, 1, 23, 14, 30, 0, 0, time.UTC),
		Tool:      "writ",
		Platform:  Platform{OS: "darwin", Arch: "arm64"},
		Context: WritContext{
			SourceRoot: "/home/user/environment",
			TargetRoot: "/home/user",
			Projects:   []string{"all"},
			Segments:   map[string]string{},
		},
		Roots: []string{"all"},
		Nodes: []Node{
			{
				ID:        ".bashrc",
				Operation: "link",
				Status:    "completed",
				Source:    "/home/user/environment/all/.bashrc",
				Target:    "/home/user/.bashrc",
				Project:   "all",
			},
		},
	}

	data, err := yaml.Marshal(v4)
	if err != nil {
		t.Fatalf("marshal v4 receipt: %v", err)
	}

	path := filepath.Join(tmpDir, "v4.yaml")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write v4 receipt: %v", err)
	}

	// Load should read directly as v4
	rcpt, err := Load(path)
	if err != nil {
		t.Fatalf("load v4 receipt: %v", err)
	}

	if rcpt.Version != "4" {
		t.Errorf("expected version '4', got %q", rcpt.Version)
	}
	if len(rcpt.Nodes) != 1 {
		t.Errorf("expected 1 node, got %d", len(rcpt.Nodes))
	}
	if rcpt.Nodes[0].ID != ".bashrc" {
		t.Errorf("expected node ID '.bashrc', got %q", rcpt.Nodes[0].ID)
	}
}

func TestPrimaryOperation(t *testing.T) {
	tests := []struct {
		ops      []string
		expected string
	}{
		{[]string{"link"}, "link"},
		{[]string{"expand", "copy"}, "expand"},
		{[]string{"decrypt", "copy"}, "decrypt"},
		{[]string{"decrypt", "expand", "copy"}, "decrypt"},
		{[]string{"copy"}, "copy"},
		{nil, "link"},
	}

	for _, tt := range tests {
		got := primaryOperation(tt.ops)
		if got != tt.expected {
			t.Errorf("primaryOperation(%v) = %q, want %q", tt.ops, got, tt.expected)
		}
	}
}

func TestToDependencyGraph(t *testing.T) {
	rcpt := &Receipt{
		Tool: "writ",
		Nodes: []Node{
			{ID: ".bashrc", Operation: "link", Project: "all"},
			{ID: ".zshrc", Operation: "link", Project: "all"},
			{ID: ".gitconfig", Operation: "expand", Project: "noblefactor"},
			{
				ID:         ".config/packages.manifest",
				Operation:  "delegate",
				Project:    "noblefactor",
				DelegateTo: "lore",
			},
		},
		Edges: []Edge{
			{From: ".config/packages.manifest", To: "lore:docker", Relation: "delegates"},
		},
	}

	dg := rcpt.ToDependencyGraph()

	// Should have: all, noblefactor (writ), lore:.config/packages.manifest
	if len(dg.Nodes) != 3 {
		t.Errorf("expected 3 dependency nodes, got %d", len(dg.Nodes))
	}

	// Should have explicit edge + implicit delegate edge
	if len(dg.Edges) != 2 {
		t.Errorf("expected 2 dependency edges, got %d", len(dg.Edges))
	}

	// Check the implicit delegate edge
	foundDelegate := false
	for _, e := range dg.Edges {
		if e.From == "noblefactor" && e.Relation == "delegates" {
			foundDelegate = true
		}
	}
	if !foundDelegate {
		t.Error("expected implicit delegate edge from noblefactor")
	}
}

func TestAddBackupAnnotation(t *testing.T) {
	rcpt := &Receipt{
		Nodes: []Node{
			{ID: ".bashrc", Operation: "link", Target: "/home/user/.bashrc"},
		},
	}

	rcpt.AddBackup("/home/user/.bashrc", "/home/user/.bashrc.writ-backup.20260123")

	if rcpt.Nodes[0].Annotations == nil {
		t.Fatal("expected annotations on node")
	}
	if rcpt.Nodes[0].Annotations["backup"] != "/home/user/.bashrc.writ-backup.20260123" {
		t.Error("expected backup annotation")
	}
}

func TestAddSkipped(t *testing.T) {
	rcpt := &Receipt{Nodes: make([]Node, 0)}

	rcpt.AddSkipped(".conflicted")

	if len(rcpt.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(rcpt.Nodes))
	}
	if rcpt.Nodes[0].ID != ".conflicted" {
		t.Errorf("expected ID '.conflicted', got %q", rcpt.Nodes[0].ID)
	}
	if rcpt.Nodes[0].Status != "skipped" {
		t.Errorf("expected status 'skipped', got %q", rcpt.Nodes[0].Status)
	}
}

func TestChecksum(t *testing.T) {
	content := []byte("hello world")
	sum := Checksum(content)

	if sum == "" {
		t.Error("expected non-empty checksum")
	}
	if len(sum) < 10 {
		t.Error("checksum too short")
	}
	if sum[:7] != "sha256:" {
		t.Errorf("expected sha256: prefix, got %q", sum[:7])
	}
}
