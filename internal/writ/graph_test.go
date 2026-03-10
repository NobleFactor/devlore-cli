// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package writ

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/NobleFactor/devlore-cli/internal/cli"
	"github.com/NobleFactor/devlore-cli/internal/execution"
	"github.com/NobleFactor/devlore-cli/internal/writ/segment"
	"github.com/NobleFactor/devlore-cli/internal/writ/tree"
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

func TestGraphStates(t *testing.T) {
	tests := []struct {
		state    op.GraphState
		expected string
	}{
		{op.StatePending, "pending"},
		{op.StateExecuted, "executed"},
		{op.StateFailed, "failed"},
	}

	for _, tt := range tests {
		if string(tt.state) != tt.expected {
			t.Errorf("expected state %q, got %q", tt.expected, tt.state)
		}
	}
}

func TestNodeStatus(t *testing.T) {
	tests := []struct {
		status   op.NodeStatus
		expected string
	}{
		{op.StatusPending, "pending"},
		{op.StatusCompleted, "completed"},
		{op.StatusSkipped, "skipped"},
		{op.StatusFailed, "failed"},
	}

	for _, tt := range tests {
		if string(tt.status) != tt.expected {
			t.Errorf("expected status %q, got %q", tt.expected, tt.status)
		}
	}
}

func TestNewGraph(t *testing.T) {
	cfg := &Config{
		Tool:       "writ",
		SourceRoot: "/home/user/env",
		TargetRoot: "/home/user",
		Projects:   []string{"all"},
	}

	g := NewGraph(cfg)

	if g.Version != CurrentVersion {
		t.Errorf("expected version %q, got %q", CurrentVersion, g.Version)
	}
	if g.Tool != "writ" {
		t.Errorf("expected tool 'writ', got %q", g.Tool)
	}
	if g.State != op.StatePending {
		t.Errorf("expected state 'pending', got %q", g.State)
	}
	if g.Context.SourceRoot != "/home/user/env" {
		t.Errorf("expected SourceRoot '/home/user/env', got %q", g.Context.SourceRoot)
	}
}

func TestNode(t *testing.T) {
	node := &op.Node{
		ID:      ".bashrc",
		Action:  op.StubAction("file.link"),
		Status:  op.StatusPending,
		Project: "all",
	}
	node.SetSlotImmediate("source", "/home/user/env/all/.bashrc")
	node.SetSlotImmediate("path", "/home/user/.bashrc")

	if node.ID != ".bashrc" {
		t.Errorf("expected ID '.bashrc', got %q", node.ID)
	}
	if node.ActionName() != "file.link" {
		t.Errorf("expected operation 'file.link', got %q", node.ActionName())
	}
	if node.Status != op.StatusPending {
		t.Errorf("expected status 'pending', got %q", node.Status)
	}
}

func TestEdge(t *testing.T) {
	edge := op.Edge{
		From: "nodeA",
		To:   "nodeB",
	}

	if edge.From != "nodeA" {
		t.Errorf("expected From 'nodeA', got %q", edge.From)
	}
	if edge.To != "nodeB" {
		t.Errorf("expected To 'nodeB', got %q", edge.To)
	}
}

func TestCollision(t *testing.T) {
	collision := op.Collision{
		Target:            ".gitconfig",
		Winner:            "/home/user/personal/.gitconfig",
		WinnerLayer:       "personal",
		WinnerSpecificity: 3,
		Loser:             "/home/user/team/.gitconfig",
		LoserLayer:        "team",
		LoserSpecificity:  2,
	}

	if collision.Target != ".gitconfig" {
		t.Errorf("expected Target '.gitconfig', got %q", collision.Target)
	}
	if collision.WinnerLayer != "personal" {
		t.Errorf("expected WinnerLayer 'personal', got %q", collision.WinnerLayer)
	}
}

func TestSummaryString(t *testing.T) {
	tests := []struct {
		name     string
		summary  op.Summary
		contains []string
	}{
		{
			name: "basic",
			summary: op.Summary{
				TotalFiles: 10,
				Links:      5,
				Templates:  3,
				Secrets:    2,
			},
			contains: []string{"10 files", "5 links", "3 templates", "2 secrets"},
		},
		{
			name: "with skipped",
			summary: op.Summary{
				TotalFiles: 5,
				Links:      5,
				Skipped:    2,
			},
			contains: []string{"5 files", "2 skipped"},
		},
		{
			name: "with failed",
			summary: op.Summary{
				TotalFiles: 5,
				Links:      4,
				Failed:     1,
			},
			contains: []string{"5 files", "1 failed"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.summary.String()
			for _, c := range tt.contains {
				if !strings.Contains(result, c) {
					t.Errorf("expected summary to contain %q, got %q", c, result)
				}
			}
		})
	}
}

func TestSignature(t *testing.T) {
	sig := &op.Signature{
		Method: "gpg",
		Value:  "base64-encoded-signature",
		KeyID:  "ABC123DEF456",
	}

	if sig.Method != "gpg" {
		t.Errorf("expected Method 'gpg', got %q", sig.Method)
	}
	if sig.Value == "" {
		t.Error("expected Value to be non-empty")
	}
	if sig.KeyID == "" {
		t.Error("expected KeyID to be non-empty")
	}
}

func TestPlatform(t *testing.T) {
	p := op.Platform{
		OS:   "darwin",
		Arch: "arm64",
	}

	if p.OS != "darwin" {
		t.Errorf("expected OS 'darwin', got %q", p.OS)
	}
	if p.Arch != "arm64" {
		t.Errorf("expected Arch 'arm64', got %q", p.Arch)
	}
}

func TestGraphContext(t *testing.T) {
	ctx := op.GraphContext{
		SourceRoot: "/home/user/env",
		TargetRoot: "/home/user",
		Projects:   []string{"all", "work"},
		Segments:   map[string]string{"OS": "darwin", "ARCH": "arm64"},
		Layers:     []string{"base", "personal"},
	}

	if ctx.SourceRoot != "/home/user/env" {
		t.Errorf("expected SourceRoot '/home/user/env', got %q", ctx.SourceRoot)
	}
	if len(ctx.Projects) != 2 {
		t.Errorf("expected 2 projects, got %d", len(ctx.Projects))
	}
	if len(ctx.Segments) != 2 {
		t.Errorf("expected 2 segments, got %d", len(ctx.Segments))
	}
	if len(ctx.Layers) != 2 {
		t.Errorf("expected 2 layers, got %d", len(ctx.Layers))
	}
}

func TestGraphSerialize(t *testing.T) {
	g := &op.Graph{
		Version:   "1",
		Tool:      "writ",
		Timestamp: time.Date(2026, 1, 29, 10, 0, 0, 0, time.UTC),
		State:     op.StatePending,
		Platform:  op.Platform{OS: "darwin", Arch: "arm64"},
		Context: op.GraphContext{
			SourceRoot: "/home/user/env",
			TargetRoot: "/home/user",
		},
		Nodes: []*op.Node{
			{
				ID:     ".bashrc",
				Action: op.StubAction("file.link"),
				Status: op.StatusPending,
			},
		},
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	err := g.Serialize(enc)
	_ = enc.Close()
	if err != nil {
		t.Fatalf("Serialize failed: %v", err)
	}

	output := buf.String()
	// Check YAML output contains expected fields
	if !strings.Contains(output, "version: \"1\"") {
		t.Errorf("expected YAML to contain version, got %q", output)
	}
	if !strings.Contains(output, "tool: writ") {
		t.Errorf("expected YAML to contain tool, got %q", output)
	}
	if !strings.Contains(output, ".bashrc") {
		t.Errorf("expected YAML to contain node ID, got %q", output)
	}
	// Note: checksum is computed by WriteReceipt, not Serialize
}

func TestGraphFilename(t *testing.T) {
	t.Run("unscoped", func(t *testing.T) {
		g := &op.Graph{
			Tool:      "writ",
			Timestamp: time.Date(2026, 1, 29, 14, 30, 45, 0, time.UTC),
		}
		filename := g.Filename()
		expected := "writ-2026-01-29T14-30-45.yaml"
		if filename != expected {
			t.Errorf("expected filename %q, got %q", expected, filename)
		}
	})

	t.Run("scoped", func(t *testing.T) {
		g := &op.Graph{
			Tool:      "writ",
			Timestamp: time.Date(2026, 1, 29, 14, 30, 45, 0, time.UTC),
			Context:   op.GraphContext{Scope: "system"},
		}
		filename := g.Filename()
		expected := "writ-system-2026-01-29T14-30-45.yaml"
		if filename != expected {
			t.Errorf("expected filename %q, got %q", expected, filename)
		}
	})
}

func TestGitStyleChecksum(t *testing.T) {
	content := []byte("test content")
	basename := "test.yaml"

	checksum := op.GitStyleChecksum("graph", basename, content)

	// Should have sha256: prefix
	if !strings.HasPrefix(checksum, "sha256:") {
		t.Errorf("expected checksum to start with 'sha256:', got %q", checksum)
	}

	// Should be deterministic
	checksum2 := op.GitStyleChecksum("graph", basename, content)
	if checksum != checksum2 {
		t.Error("expected checksum to be deterministic")
	}

	// Different content should produce different checksum
	checksum3 := op.GitStyleChecksum("graph", basename, []byte("different"))
	if checksum == checksum3 {
		t.Error("expected different content to produce different checksum")
	}
}

func TestReceiptsDir(t *testing.T) {
	dir := cli.ReceiptsDir()
	if dir == "" {
		t.Error("expected ReceiptsDir to return non-empty path")
	}
	if !strings.Contains(dir, "receipts") {
		t.Errorf("expected path to contain 'receipts', got %q", dir)
	}
}

func TestRunGraphAlreadyExecuted(t *testing.T) {
	g := &op.Graph{
		State: op.StateExecuted,
	}

	// Create a minimal executor for the test
	executor := execution.NewGraphExecutor(execution.ExecutorOptions{DryRun: true})

	err := executor.Run(context.Background(), g)
	if err == nil {
		t.Fatal("expected executor.Run to fail when already executed")
	}
	if !strings.Contains(err.Error(), "already executed") {
		t.Errorf("expected error about already executed, got %v", err)
	}
}

func TestComputeSummary(t *testing.T) {
	g := &op.Graph{
		Nodes: []*op.Node{
			{ID: "1", Action: op.StubAction("file.link"), Status: op.StatusCompleted},
			{ID: "2", Action: op.StubAction("file.link"), Status: op.StatusCompleted},
			{ID: "3", Action: op.StubAction("template.render"), Status: op.StatusCompleted},
			{ID: "4", Action: op.StubAction("encryption.decrypt"), Status: op.StatusCompleted},
			{ID: "5", Action: op.StubAction("file.copy"), Status: op.StatusCompleted},
			{ID: "6", Status: op.StatusSkipped},
			{ID: "7", Action: op.StubAction("file.link"), Status: op.StatusFailed},
			{ID: "8", Action: op.StubAction("file.link"), Status: op.StatusCompleted},
		},
	}

	g.ComputeSummary()

	if g.Summary.TotalFiles != 6 {
		t.Errorf("expected TotalFiles 6, got %d", g.Summary.TotalFiles)
	}
	if g.Summary.Links != 3 {
		t.Errorf("expected Links 3, got %d", g.Summary.Links)
	}
	if g.Summary.Templates != 1 {
		t.Errorf("expected Templates 1, got %d", g.Summary.Templates)
	}
	if g.Summary.Secrets != 1 {
		t.Errorf("expected Secrets 1, got %d", g.Summary.Secrets)
	}
	if g.Summary.Copies != 1 {
		t.Errorf("expected Copies 1, got %d", g.Summary.Copies)
	}
	if g.Summary.Skipped != 1 {
		t.Errorf("expected Skipped 1, got %d", g.Summary.Skipped)
	}
	if g.Summary.Failed != 1 {
		t.Errorf("expected Failed 1, got %d", g.Summary.Failed)
	}
}

func TestNodeAnnotations(t *testing.T) {
	node := &op.Node{
		ID:          ".bashrc",
		Annotations: map[string]string{"provider": "file"},
	}

	if node.Annotations["provider"] != "file" {
		t.Errorf("expected provider annotation, got %v", node.Annotations)
	}
}

func TestNodeSlots(t *testing.T) {
	node := &op.Node{
		ID: "install-curl",
	}
	node.SetSlotImmediate("packages", "curl,wget")
	node.SetSlotImmediate("manager", "brew")

	if node.GetSlot("packages") != "curl,wget" {
		t.Errorf("expected packages slot, got %v", node.GetSlot("packages"))
	}
	if node.GetSlot("manager") != "brew" {
		t.Errorf("expected manager slot, got %v", node.GetSlot("manager"))
	}
}

func TestNodeMode(t *testing.T) {
	node := &op.Node{
		ID: ".ssh/config",
	}
	node.SetSlotImmediate("mode", os.FileMode(0o600))

	got := node.GetSlot("mode")
	mode, ok := got.(os.FileMode)
	if !ok {
		t.Fatalf("expected mode slot to be os.FileMode, got %T", got)
	}
	if mode != 0o600 {
		t.Errorf("expected mode 0600, got %o", mode)
	}
}

func TestVerifyResult(t *testing.T) {
	tests := []struct {
		result   VerifyResult
		expected string
	}{
		{VerifyOK, "valid"},
		{VerifyUnsigned, "unsigned"},
		{VerifyInvalid, "invalid"},
		{VerifyMissing, "missing"},
	}

	for _, tt := range tests {
		if tt.result.String() != tt.expected {
			t.Errorf("expected %q, got %q", tt.expected, tt.result.String())
		}
	}
}

// --- Multi-Scope Graph Building ---

// stubRegistry creates an action registry with stub actions for testing.
func stubRegistry(names ...string) *op.ActionRegistry {
	t := op.NewActionRegistry()
	for _, name := range names {
		t.Register(op.StubAction(name))
	}
	return t
}

// createLayerTree creates a temp dir with an "all" project containing the given files.
func createLayerTree(t *testing.T, files map[string]string) string {
	t.Helper()

	dir := t.TempDir()
	for path, content := range files {
		full := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
	}
	return dir
}

func TestDeployGraphBuilder_MultiScopeBuild(t *testing.T) {
	// Create base layer with System and Home files
	baseSystem := createLayerTree(t, map[string]string{
		"all/etc/profile": "system profile",
	})
	baseHome := createLayerTree(t, map[string]string{
		"all/.bashrc": "base bashrc",
	})

	// Create personal layer with Home file
	personalHome := createLayerTree(t, map[string]string{
		"all/.zshrc": "personal zshrc",
	})

	targetHome := t.TempDir()
	segs := segment.Segments{{Name: "OS", Value: "Darwin"}}

	sources := []tree.LayerSource{
		{Layer: "base", Path: baseSystem, Order: 0, SourceRoot: baseSystem, TargetRoot: "/", TargetName: "System"},
		{Layer: "base", Path: baseHome, Order: 0, SourceRoot: baseHome, TargetRoot: targetHome, TargetName: "Home"},
		{Layer: "personal", Path: personalHome, Order: 2, SourceRoot: personalHome, TargetRoot: targetHome, TargetName: "Home"},
	}

	reg := stubRegistry("file.link")
	cfg := &DeployConfig{
		Config: Config{
			Tool:         "writ",
			LayerSources: sources,
			Projects:     []string{"all"},
			Segments:     segs,
		},
	}

	builder := NewDeployGraphBuilder(cfg, reg)
	graphs, err := builder.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Should produce 2 graphs: Home and System (alphabetical order)
	if len(graphs) != 2 {
		t.Fatalf("got %d graphs, want 2", len(graphs))
	}

	// First graph: Home (alphabetical)
	home := graphs[0]
	if home.Context.Scope != "home" {
		t.Errorf("graphs[0].Scope = %q, want %q", home.Context.Scope, "home")
	}
	if home.Context.TargetRoot != targetHome {
		t.Errorf("graphs[0].TargetRoot = %q, want %q", home.Context.TargetRoot, targetHome)
	}
	// Home graph: .bashrc (base) + .zshrc (personal) = 2 nodes
	if len(home.Nodes) != 2 {
		t.Errorf("Home graph has %d nodes, want 2", len(home.Nodes))
		for _, n := range home.Nodes {
			t.Logf("  node: %s", n.ID)
		}
	}

	// Second graph: System
	sys := graphs[1]
	if sys.Context.Scope != "system" {
		t.Errorf("graphs[1].Scope = %q, want %q", sys.Context.Scope, "system")
	}
	if sys.Context.TargetRoot != "/" {
		t.Errorf("graphs[1].TargetRoot = %q, want %q", sys.Context.TargetRoot, "/")
	}
	// System graph: etc/profile (base) = 1 node
	if len(sys.Nodes) != 1 {
		t.Errorf("System graph has %d nodes, want 1", len(sys.Nodes))
	}

	// Verify layers are recorded per scope
	if len(home.Context.Layers) != 2 {
		t.Errorf("Home layers = %v, want [base personal]", home.Context.Layers)
	}
	if len(sys.Context.Layers) != 1 || sys.Context.Layers[0] != "base" {
		t.Errorf("System layers = %v, want [base]", sys.Context.Layers)
	}
}

func TestDeployGraphBuilder_SingleSourceReturnsSlice(t *testing.T) {
	srcDir := createLayerTree(t, map[string]string{
		"all/.bashrc": "content",
	})
	targetDir := t.TempDir()
	segs := segment.Segments{{Name: "OS", Value: "Darwin"}}

	reg := stubRegistry("file.link")
	cfg := &DeployConfig{
		Config: Config{
			Tool:       "writ",
			SourceRoot: srcDir,
			TargetRoot: targetDir,
			Projects:   []string{"all"},
			Segments:   segs,
		},
	}

	builder := NewDeployGraphBuilder(cfg, reg)
	graphs, err := builder.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Single-source mode: one graph, no scope
	if len(graphs) != 1 {
		t.Fatalf("got %d graphs, want 1", len(graphs))
	}
	if graphs[0].Context.Scope != "" {
		t.Errorf("single-source graph should have empty scope, got %q", graphs[0].Context.Scope)
	}
	if len(graphs[0].Nodes) != 1 {
		t.Errorf("got %d nodes, want 1", len(graphs[0].Nodes))
	}
}

func TestDeployGraphBuilder_CrossScopeCollisionDetection(t *testing.T) {
	// Same relative path in System and Home — Home shadows System per architecture
	systemDir := createLayerTree(t, map[string]string{
		"all/.config/app.conf": "system version",
	})
	homeDir := createLayerTree(t, map[string]string{
		"all/.config/app.conf": "home version",
	})

	targetHome := t.TempDir()
	segs := segment.Segments{{Name: "OS", Value: "Darwin"}}

	sources := []tree.LayerSource{
		{Layer: "base", Path: systemDir, Order: 0, SourceRoot: systemDir, TargetRoot: "/", TargetName: "System"},
		{Layer: "base", Path: homeDir, Order: 0, SourceRoot: homeDir, TargetRoot: targetHome, TargetName: "Home"},
	}

	reg := stubRegistry("file.link")
	cfg := &DeployConfig{
		Config: Config{
			Tool:         "writ",
			LayerSources: sources,
			Projects:     []string{"all"},
			Segments:     segs,
		},
	}

	builder := NewDeployGraphBuilder(cfg, reg)
	graphs, err := builder.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Cross-scope collision: Home shadows System for same ID.
	// Only Home graph should have the file; System graph should be absent
	// (no winning System entries means no System graph created).
	if len(graphs) != 1 {
		t.Fatalf("got %d graphs, want 1 (System entry shadowed)", len(graphs))
	}
	if graphs[0].Context.Scope != "home" {
		t.Errorf("surviving graph scope = %q, want %q", graphs[0].Context.Scope, "home")
	}

	// Should have collision recorded
	if len(graphs[0].Collisions) != 1 {
		t.Errorf("got %d collisions, want 1", len(graphs[0].Collisions))
	}
}

func TestNewScopedGraph(t *testing.T) {
	cfg := &Config{
		Tool:     "writ",
		Projects: []string{"all", "noblefactor"},
	}

	g := NewScopedGraph(cfg, "home", "/home/user")

	if g.Context.Scope != "home" {
		t.Errorf("Scope = %q, want %q", g.Context.Scope, "home")
	}
	if g.Context.TargetRoot != "/home/user" {
		t.Errorf("TargetRoot = %q, want %q", g.Context.TargetRoot, "/home/user")
	}
	if len(g.Context.Projects) != 2 {
		t.Errorf("Projects = %v, want [all noblefactor]", g.Context.Projects)
	}
}

// --- Scope Ordering ---

func TestSortGraphsByScope_SystemBeforeHome(t *testing.T) {
	graphs := []*op.Graph{
		{Context: op.GraphContext{Scope: "home"}},
		{Context: op.GraphContext{Scope: "system"}},
	}

	sortGraphsByScope(graphs)

	if graphs[0].Context.Scope != "system" {
		t.Errorf("graphs[0].Scope = %q, want %q", graphs[0].Context.Scope, "system")
	}
	if graphs[1].Context.Scope != "home" {
		t.Errorf("graphs[1].Scope = %q, want %q", graphs[1].Context.Scope, "home")
	}
}

func TestSortGraphsByScope_AlreadyOrdered(t *testing.T) {
	graphs := []*op.Graph{
		{Context: op.GraphContext{Scope: "system"}},
		{Context: op.GraphContext{Scope: "home"}},
	}

	sortGraphsByScope(graphs)

	if graphs[0].Context.Scope != "system" {
		t.Errorf("graphs[0].Scope = %q, want %q", graphs[0].Context.Scope, "system")
	}
	if graphs[1].Context.Scope != "home" {
		t.Errorf("graphs[1].Scope = %q, want %q", graphs[1].Context.Scope, "home")
	}
}

func TestSortGraphsByScope_UnscopedLast(t *testing.T) {
	graphs := []*op.Graph{
		{Context: op.GraphContext{Scope: ""}},
		{Context: op.GraphContext{Scope: "home"}},
		{Context: op.GraphContext{Scope: "system"}},
	}

	sortGraphsByScope(graphs)

	if graphs[0].Context.Scope != "system" {
		t.Errorf("graphs[0].Scope = %q, want %q", graphs[0].Context.Scope, "system")
	}
	if graphs[1].Context.Scope != "home" {
		t.Errorf("graphs[1].Scope = %q, want %q", graphs[1].Context.Scope, "home")
	}
	if graphs[2].Context.Scope != "" {
		t.Errorf("graphs[2].Scope = %q, want empty", graphs[2].Context.Scope)
	}
}
