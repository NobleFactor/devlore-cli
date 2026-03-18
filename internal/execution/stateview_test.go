// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package execution

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/NobleFactor/devlore-cli/pkg/op"

	"gopkg.in/yaml.v3"
)

// TestPackageEntryLastAction tests PackageEntry.LastAction.
func TestPackageEntryLastAction(t *testing.T) {
	t.Run("empty history", func(t *testing.T) {
		e := &PackageEntry{Name: "docker"}
		if e.LastAction() != nil {
			t.Error("expected nil for empty history")
		}
	})

	t.Run("single record", func(t *testing.T) {
		e := &PackageEntry{
			Name: "docker",
			History: []HistoryRecord{
				{Receipt: "a.yaml", Status: op.StatusCompleted},
			},
		}
		last := e.LastAction()
		if last == nil {
			t.Fatal("expected non-nil last action")
		}
		if last.Receipt != "a.yaml" {
			t.Errorf("expected receipt 'a.yaml', got %q", last.Receipt)
		}
	})

	t.Run("multiple records", func(t *testing.T) {
		e := &PackageEntry{
			Name: "docker",
			History: []HistoryRecord{
				{Receipt: "a.yaml", Status: op.StatusCompleted},
				{Receipt: "b.yaml", Status: op.StatusCompleted},
				{Receipt: "c.yaml", Status: op.StatusFailed},
			},
		}
		last := e.LastAction()
		if last == nil {
			t.Fatal("expected non-nil last action")
		}
		if last.Receipt != "c.yaml" {
			t.Errorf("expected receipt 'c.yaml', got %q", last.Receipt)
		}
	})
}

// TestFileEntryLastAction tests FileEntry.LastAction.
func TestFileEntryLastAction(t *testing.T) {
	t.Run("empty history", func(t *testing.T) {
		e := &FileEntry{Target: ".bashrc"}
		if e.LastAction() != nil {
			t.Error("expected nil for empty history")
		}
	})

	t.Run("with history", func(t *testing.T) {
		e := &FileEntry{
			Target: ".bashrc",
			History: []HistoryRecord{
				{Receipt: "old.yaml", Action: "link"},
				{Receipt: "new.yaml", Action: "copy"},
			},
		}
		last := e.LastAction()
		if last == nil {
			t.Fatal("expected non-nil last action")
		}
		if last.Receipt != "new.yaml" {
			t.Errorf("expected receipt 'new.yaml', got %q", last.Receipt)
		}
	})
}

// TestFileEntryIsCopied tests FileEntry.IsCopied.
func TestFileEntryIsCopied(t *testing.T) {
	tests := []struct {
		name      string
		operation string
		want      bool
	}{
		{"no history", "", false},
		{"link only", "file.link", false},
		{"copy operation", "file.copy", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &FileEntry{Target: "test"}
			if tt.operation != "" {
				e.History = []HistoryRecord{
					{Action: tt.operation},
				}
			}
			if got := e.IsCopied(); got != tt.want {
				t.Errorf("IsCopied() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestFileEntryIsLinked tests FileEntry.IsLinked.
func TestFileEntryIsLinked(t *testing.T) {
	tests := []struct {
		name      string
		operation string
		want      bool
	}{
		{"no history", "", false},
		{"link only", "file.link", true},
		{"copy operation", "file.copy", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &FileEntry{Target: "test"}
			if tt.operation != "" {
				e.History = []HistoryRecord{
					{Action: tt.operation},
				}
			}
			if got := e.IsLinked(); got != tt.want {
				t.Errorf("IsLinked() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestFileTreeForProject tests FileTree.ForProject.
func TestFileTreeForProject(t *testing.T) {
	tree := &FileTree{
		Entries: map[string]*FileEntry{
			".bashrc":       {Target: ".bashrc", Project: "shell"},
			".zshrc":        {Target: ".zshrc", Project: "shell"},
			".gitconfig":    {Target: ".gitconfig", Project: "git"},
			".vimrc":        {Target: ".vimrc", Project: "vim"},
			".config/nvim":  {Target: ".config/nvim", Project: "vim"},
			".editorconfig": {Target: ".editorconfig", Project: ""},
		},
	}

	t.Run("shell project", func(t *testing.T) {
		files := tree.ForProject("shell")
		if len(files) != 2 {
			t.Errorf("expected 2 shell files, got %d", len(files))
		}
		if files[".bashrc"] == nil || files[".zshrc"] == nil {
			t.Error("expected both .bashrc and .zshrc")
		}
	})

	t.Run("vim project", func(t *testing.T) {
		files := tree.ForProject("vim")
		if len(files) != 2 {
			t.Errorf("expected 2 vim files, got %d", len(files))
		}
	})

	t.Run("nonexistent project", func(t *testing.T) {
		files := tree.ForProject("nonexistent")
		if len(files) != 0 {
			t.Errorf("expected 0 files, got %d", len(files))
		}
	})
}

// TestFileTreeCopiedLinkedFiles tests FileTree.CopiedFiles and LinkedFiles.
func TestFileTreeCopiedLinkedFiles(t *testing.T) {
	tree := &FileTree{
		Entries: map[string]*FileEntry{
			".bashrc": {
				Target:  ".bashrc",
				History: []HistoryRecord{{Action: "file.link"}},
			},
			".zshrc": {
				Target:  ".zshrc",
				History: []HistoryRecord{{Action: "file.link"}},
			},
			".gitconfig": {
				Target:  ".gitconfig",
				History: []HistoryRecord{{Action: "file.copy"}},
			},
			".ssh/config": {
				Target:  ".ssh/config",
				History: []HistoryRecord{{Action: "file.copy"}},
			},
		},
	}

	linked := tree.LinkedFiles()
	if len(linked) != 2 {
		t.Errorf("expected 2 linked files, got %d", len(linked))
	}

	copied := tree.CopiedFiles()
	if len(copied) != 2 {
		t.Errorf("expected 2 copied files, got %d", len(copied))
	}
}

// TestFileTreeProjects tests FileTree.Projects.
func TestFileTreeProjects(t *testing.T) {
	tree := &FileTree{
		Entries: map[string]*FileEntry{
			"a": {Target: "a", Project: "shell"},
			"b": {Target: "b", Project: "git"},
			"c": {Target: "c", Project: "shell"},
			"d": {Target: "d", Project: ""},
			"e": {Target: "e", Project: "git"},
			"f": {Target: "f", Project: "vim"},
		},
	}

	projects := tree.Projects()
	if len(projects) != 3 {
		t.Errorf("expected 3 projects, got %d: %v", len(projects), projects)
	}

	// Should be sorted
	expected := []string{"git", "shell", "vim"}
	for i, p := range expected {
		if projects[i] != p {
			t.Errorf("projects[%d] = %q, want %q", i, projects[i], p)
		}
	}
}

// TestFileTreeBuildTree tests the hierarchical tree building.
func TestFileTreeBuildTree(t *testing.T) {
	tree := &FileTree{
		Root: "/home/user",
		Entries: map[string]*FileEntry{
			".bashrc":                      {Target: ".bashrc"},
			".config/nvim/init.lua":        {Target: ".config/nvim/init.lua"},
			".config/nvim/lua/plugins.lua": {Target: ".config/nvim/lua/plugins.lua"},
			".ssh/config":                  {Target: ".ssh/config"},
		},
	}
	tree.buildTree()

	if tree.Tree == nil {
		t.Fatal("expected tree to be built")
	}
	if tree.Tree.Name != "user" {
		t.Errorf("expected root name 'user', got %q", tree.Tree.Name)
	}
	if !tree.Tree.IsDir {
		t.Error("expected root to be a directory")
	}

	// Check .bashrc exists at root level
	if tree.Tree.Children[".bashrc"] == nil {
		t.Error("expected .bashrc at root level")
	}
	if tree.Tree.Children[".bashrc"].IsDir {
		t.Error("expected .bashrc to be a file")
	}

	// Check .config directory
	configDir := tree.Tree.Children[".config"]
	if configDir == nil {
		t.Fatal("expected .config directory")
	}
	if !configDir.IsDir {
		t.Error("expected .config to be a directory")
	}

	// Check nested structure
	nvimDir := configDir.Children["nvim"]
	if nvimDir == nil {
		t.Fatal("expected nvim directory")
	}
	if nvimDir.Children["init.lua"] == nil {
		t.Error("expected init.lua in nvim directory")
	}
	if nvimDir.Children["lua"] == nil {
		t.Error("expected lua directory in nvim")
	}
	if nvimDir.Children["lua"].Children["plugins.lua"] == nil {
		t.Error("expected plugins.lua in lua directory")
	}
}

// TestStateViewSummary tests StateView.Summary.
func TestStateViewSummary(t *testing.T) {
	view := &StateView{
		Packages: map[string]*PackageEntry{
			"docker": {Name: "docker"},
			"golang": {Name: "golang"},
		},
		Files: &FileTree{
			Entries: map[string]*FileEntry{
				".bashrc": {
					Target:  ".bashrc",
					History: []HistoryRecord{{Action: "file.link"}},
				},
				".zshrc": {
					Target:  ".zshrc",
					History: []HistoryRecord{{Action: "file.link"}},
				},
				".gitconfig": {
					Target:  ".gitconfig",
					History: []HistoryRecord{{Action: "file.copy"}},
				},
			},
		},
	}

	packages, links, copied := view.Summary()
	if packages != 2 {
		t.Errorf("expected 2 packages, got %d", packages)
	}
	if links != 2 {
		t.Errorf("expected 2 links, got %d", links)
	}
	if copied != 1 {
		t.Errorf("expected 1 copied, got %d", copied)
	}
}

// TestStateViewBuilderBuildFrom tests building a view from graphs.
func TestStateViewBuilderBuildFrom(t *testing.T) {
	now := time.Now()
	earlier := now.Add(-time.Hour)

	// Helper to create nodes with source slot
	makeNode := func(id string, action string, source, project, layer string) *op.Node {
		n := &op.Node{
			ID:      id,
			Action:  op.StubAction(action),
			Project: project,
			Layer:   layer,
			Status:  op.StatusCompleted,
		}
		n.SetSlotImmediate("source", source)
		return n
	}

	graphs := []*op.Graph{
		{
			Tool:      "writ",
			Timestamp: earlier,
			Context:   op.GraphContext{TargetRoot: "/home/user"},
			Nodes: []*op.Node{
				makeNode(".bashrc", "file.link", "/repo/.bashrc", "shell", "base"),
			},
		},
		{
			Tool:      "writ",
			Timestamp: now,
			Context:   op.GraphContext{TargetRoot: "/home/user"},
			Nodes: []*op.Node{
				makeNode(".bashrc", "file.copy", "/repo/.bashrc", "shell", "personal"),
				makeNode(".gitconfig", "file.link", "/repo/.gitconfig", "git", ""),
			},
		},
	}

	builder := NewStateViewBuilder(ViewOptions{})
	view := builder.BuildFrom(graphs)

	if view.ReceiptCount != 2 {
		t.Errorf("expected 2 receipts, got %d", view.ReceiptCount)
	}

	// Check files
	if len(view.Files.Entries) != 2 {
		t.Errorf("expected 2 file entries, got %d", len(view.Files.Entries))
	}

	// Check .bashrc has 2 history records
	bashrc := view.Files.Entries[".bashrc"]
	if bashrc == nil {
		t.Fatal("expected .bashrc entry")
	}
	if len(bashrc.History) != 2 {
		t.Errorf("expected 2 history records for .bashrc, got %d", len(bashrc.History))
	}
	// Latest record should be personal layer with render+copy
	if bashrc.Layer != "personal" {
		t.Errorf("expected layer 'personal', got %q", bashrc.Layer)
	}
	if !bashrc.IsCopied() {
		t.Error("expected .bashrc to be marked as copied")
	}

	// Check target root
	if view.Files.Root != "/home/user" {
		t.Errorf("expected root '/home/user', got %q", view.Files.Root)
	}

	// Check tree was built
	if view.Files.Tree == nil {
		t.Error("expected file tree to be built")
	}
}

// TestStateViewBuilderPackageNodes tests package lifecycle node handling.
func TestStateViewBuilderPackageNodes(t *testing.T) {
	now := time.Now()

	graphs := []*op.Graph{
		{
			Tool:      "lore",
			Timestamp: now.Add(-2 * time.Hour),
			Nodes: []*op.Node{
				{
					ID:     "docker",
					Action: op.StubAction("pkg.install"),
					Status: op.StatusCompleted,
				},
			},
		},
		{
			Tool:      "lore",
			Timestamp: now.Add(-time.Hour),
			Nodes: []*op.Node{
				{
					ID:     "docker",
					Action: op.StubAction("pkg.verify"),
					Status: op.StatusCompleted,
				},
				{
					ID:     "golang",
					Action: op.StubAction("pkg.install"),
					Status: op.StatusCompleted,
				},
			},
		},
		{
			Tool:      "lore",
			Timestamp: now,
			Nodes: []*op.Node{
				{
					ID:     "docker",
					Action: op.StubAction("pkg.upgrade"),
					Status: op.StatusCompleted,
				},
			},
		},
	}

	builder := NewStateViewBuilder(ViewOptions{})
	view := builder.BuildFrom(graphs)

	// Check packages
	if len(view.Packages) != 2 {
		t.Errorf("expected 2 packages, got %d", len(view.Packages))
	}

	docker := view.Packages["docker"]
	if docker == nil {
		t.Fatal("expected docker package")
	}
	if len(docker.History) != 3 {
		t.Errorf("expected 3 history records for docker, got %d", len(docker.History))
	}

	// History should be ordered by time
	if docker.History[0].Action != "pkg.install" {
		t.Errorf("expected first operation 'pkg.install', got %q", docker.History[0].Action)
	}
	if docker.History[1].Action != "pkg.verify" {
		t.Errorf("expected second operation 'pkg.verify', got %q", docker.History[1].Action)
	}
	if docker.History[2].Action != "pkg.upgrade" {
		t.Errorf("expected third operation 'pkg.upgrade', got %q", docker.History[2].Action)
	}

	// golang should have 1 history record
	golang := view.Packages["golang"]
	if golang == nil {
		t.Fatal("expected golang package")
	}
	if len(golang.History) != 1 {
		t.Errorf("expected 1 history record for golang, got %d", len(golang.History))
	}
}

// TestStateViewBuilderTimeFilter tests time-based filtering.
func TestStateViewBuilderTimeFilter(t *testing.T) {
	now := time.Now()

	graphs := []*op.Graph{
		{Tool: "writ", Timestamp: now.Add(-3 * time.Hour), Nodes: []*op.Node{{ID: "a", Action: op.StubAction("file.link"), Status: op.StatusCompleted}}},
		{Tool: "writ", Timestamp: now.Add(-2 * time.Hour), Nodes: []*op.Node{{ID: "b", Action: op.StubAction("file.link"), Status: op.StatusCompleted}}},
		{Tool: "writ", Timestamp: now.Add(-1 * time.Hour), Nodes: []*op.Node{{ID: "c", Action: op.StubAction("file.link"), Status: op.StatusCompleted}}},
		{Tool: "writ", Timestamp: now, Nodes: []*op.Node{{ID: "d", Action: op.StubAction("file.link"), Status: op.StatusCompleted}}},
	}

	// Filter to middle two
	builder := NewStateViewBuilder(ViewOptions{
		Since: now.Add(-2*time.Hour - time.Minute),
		Until: now.Add(-time.Hour + time.Minute),
	})
	view := builder.BuildFrom(graphs)

	if view.ReceiptCount != 2 {
		t.Errorf("expected 2 receipts, got %d", view.ReceiptCount)
	}
	if len(view.Files.Entries) != 2 {
		t.Errorf("expected 2 files, got %d", len(view.Files.Entries))
	}
	if view.Files.Entries["b"] == nil || view.Files.Entries["c"] == nil {
		t.Error("expected files b and c")
	}
}

// TestStateViewBuilderToolFilter tests tool-based filtering.
func TestStateViewBuilderToolFilter(t *testing.T) {
	now := time.Now()

	graphs := []*op.Graph{
		{Tool: "writ", Timestamp: now.Add(-2 * time.Hour), Nodes: []*op.Node{{ID: ".bashrc", Action: op.StubAction("file.link"), Status: op.StatusCompleted}}},
		{Tool: "lore", Timestamp: now.Add(-1 * time.Hour), Nodes: []*op.Node{{ID: "docker", Action: op.StubAction("pkg.install"), Status: op.StatusCompleted}}},
		{Tool: "writ", Timestamp: now, Nodes: []*op.Node{{ID: ".zshrc", Action: op.StubAction("file.link"), Status: op.StatusCompleted}}},
	}

	t.Run("writ only", func(t *testing.T) {
		builder := NewStateViewBuilder(ViewOptions{Tools: []string{"writ"}})
		view := builder.BuildFrom(graphs)

		if view.ReceiptCount != 2 {
			t.Errorf("expected 2 receipts, got %d", view.ReceiptCount)
		}
		if len(view.Files.Entries) != 2 {
			t.Errorf("expected 2 files, got %d", len(view.Files.Entries))
		}
		if len(view.Packages) != 0 {
			t.Errorf("expected 0 packages, got %d", len(view.Packages))
		}
	})

	t.Run("lore only", func(t *testing.T) {
		builder := NewStateViewBuilder(ViewOptions{Tools: []string{"lore"}})
		view := builder.BuildFrom(graphs)

		if view.ReceiptCount != 1 {
			t.Errorf("expected 1 receipt, got %d", view.ReceiptCount)
		}
		if len(view.Files.Entries) != 0 {
			t.Errorf("expected 0 files, got %d", len(view.Files.Entries))
		}
		if len(view.Packages) != 1 {
			t.Errorf("expected 1 package, got %d", len(view.Packages))
		}
	})
}

// TestStateViewBuilderSkipsSkipped tests that skipped nodes are not included.
func TestStateViewBuilderSkipsSkipped(t *testing.T) {
	now := time.Now()

	graphs := []*op.Graph{
		{
			Tool:      "writ",
			Timestamp: now,
			Nodes: []*op.Node{
				{ID: "a", Action: op.StubAction("file.link"), Status: op.StatusCompleted},
				{ID: "b", Action: op.StubAction("file.link"), Status: op.StatusSkipped},
				{ID: "c", Action: op.StubAction("file.link"), Status: op.StatusFailed},
			},
		},
	}

	builder := NewStateViewBuilder(ViewOptions{})
	view := builder.BuildFrom(graphs)

	// Should include completed and failed, but not skipped
	if len(view.Files.Entries) != 2 {
		t.Errorf("expected 2 files, got %d", len(view.Files.Entries))
	}
	if view.Files.Entries["a"] == nil {
		t.Error("expected file a")
	}
	if view.Files.Entries["b"] != nil {
		t.Error("expected file b to be excluded (skipped)")
	}
	if view.Files.Entries["c"] == nil {
		t.Error("expected file c")
	}
}

// TestStateViewBuilderLoadReceipts tests loading from a receipts directory.
func TestStateViewBuilderLoadReceipts(t *testing.T) {
	tmpDir := t.TempDir()

	now := time.Now()

	// Create test receipt files
	receipts := []struct {
		name  string
		graph *op.Graph
	}{
		{
			name: "writ-2025-01-01T10-00-00.yaml",
			graph: &op.Graph{
				Version:   "1",
				Tool:      "writ",
				Timestamp: now.Add(-time.Hour),
				State:     op.StateExecuted,
				Context:   op.GraphContext{TargetRoot: "/home/user"},
				Nodes: []*op.Node{
					{ID: ".bashrc", Action: op.StubAction("file.link"), Status: op.StatusCompleted},
				},
			},
		},
		{
			name: "writ-2025-01-01T11-00-00.yaml",
			graph: &op.Graph{
				Version:   "1",
				Tool:      "writ",
				Timestamp: now,
				State:     op.StateExecuted,
				Context:   op.GraphContext{TargetRoot: "/home/user"},
				Nodes: []*op.Node{
					{ID: ".gitconfig", Action: op.StubAction("file.link"), Status: op.StatusCompleted},
				},
			},
		},
	}

	for _, r := range receipts {
		path := filepath.Join(tmpDir, r.name)
		data, err := yaml.Marshal(r.graph)
		if err != nil {
			t.Fatalf("marshal %s: %v", r.name, err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatalf("write %s: %v", r.name, err)
		}
	}

	// Create a symlink that should be skipped
	if err := os.Symlink(receipts[1].name, filepath.Join(tmpDir, "writ-latest.yaml")); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	// Create a non-yaml file that should be skipped
	if err := os.WriteFile(filepath.Join(tmpDir, "README.md"), []byte("test"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}

	builder := NewStateViewBuilder(ViewOptions{})
	view, err := builder.Build(tmpDir)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if view.ReceiptCount != 2 {
		t.Errorf("expected 2 receipts, got %d", view.ReceiptCount)
	}
	if len(view.Files.Entries) != 2 {
		t.Errorf("expected 2 files, got %d", len(view.Files.Entries))
	}
}

// TestStateViewBuilderNonexistentDir tests that a nonexistent directory is OK.
func TestStateViewBuilderNonexistentDir(t *testing.T) {
	builder := NewStateViewBuilder(ViewOptions{})
	view, err := builder.Build("/nonexistent/path/to/receipts")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if view.ReceiptCount != 0 {
		t.Errorf("expected 0 receipts, got %d", view.ReceiptCount)
	}
}

// TestIsPackageNode tests the package node detection logic.
func TestIsPackageNode(t *testing.T) {
	builder := &StateViewBuilder{}

	tests := []struct {
		operation string
		want      bool
	}{
		{"file.link", false},
		{"file.copy", false},
		{"template.render_text", false},
		{"template.render_bytes", false},
		{"encryption.decrypt", false},
		{"pkg.install", true},
		{"pkg.prepare", true},
		{"pkg.verify", true},
		{"pkg.upgrade", true},
		{"pkg.uninstall", true},
		{"pkg.cleanup", true},
		{"pkg.remove", true},
	}

	for _, tt := range tests {
		node := &op.Node{Action: op.StubAction(tt.operation)}
		got := builder.isPackageNode(node)
		if got != tt.want {
			t.Errorf("isPackageNode(%q) = %v, want %v", tt.operation, got, tt.want)
		}
	}
}

// TestStateViewBuilderScopeFilter tests scope-based filtering.
func TestStateViewBuilderScopeFilter(t *testing.T) {
	now := time.Now()

	graphs := []*op.Graph{
		{
			Tool:      "writ",
			Timestamp: now.Add(-2 * time.Hour),
			Context:   op.GraphContext{Scope: "system", TargetRoot: "/"},
			Nodes: []*op.Node{
				{ID: "etc/devlore.conf", Action: op.StubAction("file.copy"), Status: op.StatusCompleted},
			},
		},
		{
			Tool:      "writ",
			Timestamp: now.Add(-time.Hour),
			Context:   op.GraphContext{Scope: "home", TargetRoot: "/home/user"},
			Nodes: []*op.Node{
				{ID: ".bashrc", Action: op.StubAction("file.link"), Status: op.StatusCompleted},
				{ID: ".gitconfig", Action: op.StubAction("file.link"), Status: op.StatusCompleted},
			},
		},
		{
			Tool:      "writ",
			Timestamp: now,
			Context:   op.GraphContext{Scope: "home", TargetRoot: "/home/user"},
			Nodes: []*op.Node{
				{ID: ".vimrc", Action: op.StubAction("file.link"), Status: op.StatusCompleted},
			},
		},
	}

	t.Run("home only", func(t *testing.T) {
		builder := NewStateViewBuilder(ViewOptions{Scope: "home"})
		view := builder.BuildFrom(graphs)

		if view.ReceiptCount != 2 {
			t.Errorf("expected 2 receipts, got %d", view.ReceiptCount)
		}
		if len(view.Files.Entries) != 3 {
			t.Errorf("expected 3 files, got %d", len(view.Files.Entries))
		}
		if view.Files.Root != "/home/user" {
			t.Errorf("expected root '/home/user', got %q", view.Files.Root)
		}
	})

	t.Run("system only", func(t *testing.T) {
		builder := NewStateViewBuilder(ViewOptions{Scope: "system"})
		view := builder.BuildFrom(graphs)

		if view.ReceiptCount != 1 {
			t.Errorf("expected 1 receipt, got %d", view.ReceiptCount)
		}
		if len(view.Files.Entries) != 1 {
			t.Errorf("expected 1 file, got %d", len(view.Files.Entries))
		}
		if view.Files.Root != "/" {
			t.Errorf("expected root '/', got %q", view.Files.Root)
		}
	})

	t.Run("all scopes", func(t *testing.T) {
		builder := NewStateViewBuilder(ViewOptions{})
		view := builder.BuildFrom(graphs)

		if view.ReceiptCount != 3 {
			t.Errorf("expected 3 receipts, got %d", view.ReceiptCount)
		}
		if len(view.Files.Entries) != 4 {
			t.Errorf("expected 4 files, got %d", len(view.Files.Entries))
		}
	})

	t.Run("nonexistent scope", func(t *testing.T) {
		builder := NewStateViewBuilder(ViewOptions{Scope: "other"})
		view := builder.BuildFrom(graphs)

		if view.ReceiptCount != 0 {
			t.Errorf("expected 0 receipts, got %d", view.ReceiptCount)
		}
		if len(view.Files.Entries) != 0 {
			t.Errorf("expected 0 files, got %d", len(view.Files.Entries))
		}
	})
}

// TestStateViewBuilderScopeFilterWithToolFilter tests scope + tool combined filters.
func TestStateViewBuilderScopeFilterWithToolFilter(t *testing.T) {
	now := time.Now()

	graphs := []*op.Graph{
		{
			Tool:      "writ",
			Timestamp: now.Add(-time.Hour),
			Context:   op.GraphContext{Scope: "home", TargetRoot: "/home/user"},
			Nodes: []*op.Node{
				{ID: ".bashrc", Action: op.StubAction("file.link"), Status: op.StatusCompleted},
			},
		},
		{
			Tool:      "lore",
			Timestamp: now,
			Context:   op.GraphContext{Scope: "home"},
			Nodes: []*op.Node{
				{ID: "docker", Action: op.StubAction("pkg.install"), Status: op.StatusCompleted},
			},
		},
	}

	builder := NewStateViewBuilder(ViewOptions{Scope: "home", Tools: []string{"writ"}})
	view := builder.BuildFrom(graphs)

	if view.ReceiptCount != 1 {
		t.Errorf("expected 1 receipt, got %d", view.ReceiptCount)
	}
	if len(view.Files.Entries) != 1 {
		t.Errorf("expected 1 file, got %d", len(view.Files.Entries))
	}
	if len(view.Packages) != 0 {
		t.Errorf("expected 0 packages, got %d", len(view.Packages))
	}
}

// TestDistinctScopes tests scope discovery from receipt files.
func TestDistinctScopes(t *testing.T) {
	tmpDir := t.TempDir()
	now := time.Now()

	receipts := []struct {
		name  string
		graph *op.Graph
	}{
		{
			name: "writ-system-2025-01-01T10-00-00.yaml",
			graph: &op.Graph{
				Version:   "1",
				Tool:      "writ",
				Timestamp: now.Add(-2 * time.Hour),
				State:     op.StateExecuted,
				Context:   op.GraphContext{Scope: "system", TargetRoot: "/"},
				Nodes:     []*op.Node{{ID: "etc/conf", Action: op.StubAction("file.copy"), Status: op.StatusCompleted}},
			},
		},
		{
			name: "writ-home-2025-01-01T10-00-00.yaml",
			graph: &op.Graph{
				Version:   "1",
				Tool:      "writ",
				Timestamp: now.Add(-time.Hour),
				State:     op.StateExecuted,
				Context:   op.GraphContext{Scope: "home", TargetRoot: "/home/user"},
				Nodes:     []*op.Node{{ID: ".bashrc", Action: op.StubAction("file.link"), Status: op.StatusCompleted}},
			},
		},
		{
			name: "writ-home-2025-01-01T11-00-00.yaml",
			graph: &op.Graph{
				Version:   "1",
				Tool:      "writ",
				Timestamp: now,
				State:     op.StateExecuted,
				Context:   op.GraphContext{Scope: "home", TargetRoot: "/home/user"},
				Nodes:     []*op.Node{{ID: ".vimrc", Action: op.StubAction("file.link"), Status: op.StatusCompleted}},
			},
		},
	}

	for _, r := range receipts {
		path := filepath.Join(tmpDir, r.name)
		data, err := yaml.Marshal(r.graph)
		if err != nil {
			t.Fatalf("marshal %s: %v", r.name, err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatalf("write %s: %v", r.name, err)
		}
	}

	builder := NewStateViewBuilder(ViewOptions{Tools: []string{"writ"}})
	scopes, err := builder.DistinctScopes(tmpDir)
	if err != nil {
		t.Fatalf("DistinctScopes: %v", err)
	}

	if len(scopes) != 2 {
		t.Fatalf("expected 2 scopes, got %d: %v", len(scopes), scopes)
	}
	// Sorted: "home" before "system"
	if scopes[0] != "home" {
		t.Errorf("expected scopes[0]='home', got %q", scopes[0])
	}
	if scopes[1] != "system" {
		t.Errorf("expected scopes[1]='system', got %q", scopes[1])
	}
}

// TestDistinctScopesWithUnscoped tests scope discovery including unscoped receipts.
func TestDistinctScopesWithUnscoped(t *testing.T) {
	tmpDir := t.TempDir()
	now := time.Now()

	receipt := &op.Graph{
		Version:   "1",
		Tool:      "writ",
		Timestamp: now,
		State:     op.StateExecuted,
		Context:   op.GraphContext{TargetRoot: "/home/user"},
		Nodes:     []*op.Node{{ID: ".bashrc", Action: op.StubAction("file.link"), Status: op.StatusCompleted}},
	}

	data, err := yaml.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "writ-2025-01-01T10-00-00.yaml"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	builder := NewStateViewBuilder(ViewOptions{Tools: []string{"writ"}})
	scopes, err := builder.DistinctScopes(tmpDir)
	if err != nil {
		t.Fatalf("DistinctScopes: %v", err)
	}

	if len(scopes) != 1 {
		t.Fatalf("expected 1 scope, got %d: %v", len(scopes), scopes)
	}
	if scopes[0] != "" {
		t.Errorf("expected empty scope for unscoped receipt, got %q", scopes[0])
	}
}

// TestStateViewBuilderScopeFilterFromDisk tests scope filtering with receipts loaded from disk.
func TestStateViewBuilderScopeFilterFromDisk(t *testing.T) {
	tmpDir := t.TempDir()
	now := time.Now()

	receipts := []struct {
		name  string
		graph *op.Graph
	}{
		{
			name: "writ-system-2025-01-01T10-00-00.yaml",
			graph: &op.Graph{
				Version:   "1",
				Tool:      "writ",
				Timestamp: now.Add(-time.Hour),
				State:     op.StateExecuted,
				Context:   op.GraphContext{Scope: "system", TargetRoot: "/"},
				Nodes:     []*op.Node{{ID: "etc/devlore.conf", Action: op.StubAction("file.copy"), Status: op.StatusCompleted}},
			},
		},
		{
			name: "writ-home-2025-01-01T10-00-00.yaml",
			graph: &op.Graph{
				Version:   "1",
				Tool:      "writ",
				Timestamp: now,
				State:     op.StateExecuted,
				Context:   op.GraphContext{Scope: "home", TargetRoot: "/home/user"},
				Nodes:     []*op.Node{{ID: ".bashrc", Action: op.StubAction("file.link"), Status: op.StatusCompleted}},
			},
		},
	}

	for _, r := range receipts {
		path := filepath.Join(tmpDir, r.name)
		data, err := yaml.Marshal(r.graph)
		if err != nil {
			t.Fatalf("marshal %s: %v", r.name, err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatalf("write %s: %v", r.name, err)
		}
	}

	// Create latest symlinks (should be skipped)
	_ = os.Symlink("writ-system-2025-01-01T10-00-00.yaml", filepath.Join(tmpDir, "writ-system-latest.yaml"))
	_ = os.Symlink("writ-home-2025-01-01T10-00-00.yaml", filepath.Join(tmpDir, "writ-home-latest.yaml"))

	t.Run("home scope from disk", func(t *testing.T) {
		builder := NewStateViewBuilder(ViewOptions{Scope: "home"})
		view, err := builder.Build(tmpDir)
		if err != nil {
			t.Fatalf("Build: %v", err)
		}

		if view.ReceiptCount != 1 {
			t.Errorf("expected 1 receipt, got %d", view.ReceiptCount)
		}
		if len(view.Files.Entries) != 1 {
			t.Errorf("expected 1 file, got %d", len(view.Files.Entries))
		}
		if view.Files.Entries[".bashrc"] == nil {
			t.Error("expected .bashrc")
		}
		if view.Files.Root != "/home/user" {
			t.Errorf("expected root '/home/user', got %q", view.Files.Root)
		}
	})

	t.Run("system scope from disk", func(t *testing.T) {
		builder := NewStateViewBuilder(ViewOptions{Scope: "system"})
		view, err := builder.Build(tmpDir)
		if err != nil {
			t.Fatalf("Build: %v", err)
		}

		if view.ReceiptCount != 1 {
			t.Errorf("expected 1 receipt, got %d", view.ReceiptCount)
		}
		if len(view.Files.Entries) != 1 {
			t.Errorf("expected 1 file, got %d", len(view.Files.Entries))
		}
		if view.Files.Entries["etc/devlore.conf"] == nil {
			t.Error("expected etc/devlore.conf")
		}
		if view.Files.Root != "/" {
			t.Errorf("expected root '/', got %q", view.Files.Root)
		}
	})
}
