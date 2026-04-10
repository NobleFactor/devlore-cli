// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package writ

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/NobleFactor/devlore-cli/internal/cli"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/segment"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/tree"
	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/sops"
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
	if g.Provenance.Tool != "writ" {
		t.Errorf("expected tool 'writ', got %q", g.Provenance.Tool)
	}
	if g.State != op.StatePending {
		t.Errorf("expected state 'pending', got %q", g.State)
	}
	if g.Provenance.SourceRoot != "/home/user/env" {
		t.Errorf("expected SourceRoot '/home/user/env', got %q", g.Provenance.SourceRoot)
	}
}

func TestNode(t *testing.T) {
	node := &op.Node{
		ID:       ".bashrc",
		Receiver: "file.link",
		Status:   op.StatusPending,
		Origin:   "all",
	}
	node.SetSlotImmediate("source", "/home/user/env/all/.bashrc")
	node.SetSlotImmediate("path", "/home/user/.bashrc")

	if node.ID != ".bashrc" {
		t.Errorf("expected ID '.bashrc', got %q", node.ID)
	}
	if node.Receiver != "file.link" {
		t.Errorf("expected operation 'file.link', got %q", node.Receiver)
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

func TestFormatGraphSummary(t *testing.T) {
	tests := []struct {
		name     string
		nodes    []op.SubgraphChild
		contains []string
	}{
		{
			name: "basic files",
			nodes: []op.SubgraphChild{
				{Node: &op.Node{ID: "1", Receiver: "file.link", Status: op.StatusCompleted}},
				{Node: &op.Node{ID: "2", Receiver: "file.link", Status: op.StatusCompleted}},
				{Node: &op.Node{ID: "3", Receiver: "file.link", Status: op.StatusCompleted}},
				{Node: &op.Node{ID: "4", Receiver: "file.link", Status: op.StatusCompleted}},
				{Node: &op.Node{ID: "5", Receiver: "file.link", Status: op.StatusCompleted}},
				{Node: &op.Node{ID: "6", Receiver: "template.render_text", Status: op.StatusCompleted}},
				{Node: &op.Node{ID: "7", Receiver: "template.render_text", Status: op.StatusCompleted}},
				{Node: &op.Node{ID: "8", Receiver: "template.render_text", Status: op.StatusCompleted}},
				{Node: &op.Node{ID: "9", Receiver: "encryption.decrypt", Status: op.StatusCompleted}},
				{Node: &op.Node{ID: "10", Receiver: "encryption.decrypt", Status: op.StatusCompleted}},
			},
			contains: []string{"10 files", "5 links", "3 templates", "2 secrets"},
		},
		{
			name: "with skipped",
			nodes: []op.SubgraphChild{
				{Node: &op.Node{ID: "1", Receiver: "file.link", Status: op.StatusCompleted}},
				{Node: &op.Node{ID: "2", Receiver: "file.link", Status: op.StatusCompleted}},
				{Node: &op.Node{ID: "3", Receiver: "file.link", Status: op.StatusCompleted}},
				{Node: &op.Node{ID: "4", Receiver: "file.link", Status: op.StatusCompleted}},
				{Node: &op.Node{ID: "5", Receiver: "file.link", Status: op.StatusCompleted}},
				{Node: &op.Node{ID: "6", Receiver: "file.link", Status: op.StatusSkipped}},
				{Node: &op.Node{ID: "7", Receiver: "file.link", Status: op.StatusSkipped}},
			},
			contains: []string{"5 files", "2 skipped"},
		},
		{
			name: "with failed",
			nodes: []op.SubgraphChild{
				{Node: &op.Node{ID: "1", Receiver: "file.link", Status: op.StatusCompleted}},
				{Node: &op.Node{ID: "2", Receiver: "file.link", Status: op.StatusCompleted}},
				{Node: &op.Node{ID: "3", Receiver: "file.link", Status: op.StatusCompleted}},
				{Node: &op.Node{ID: "4", Receiver: "file.link", Status: op.StatusCompleted}},
				{Node: &op.Node{ID: "5", Receiver: "file.link", Status: op.StatusFailed}},
			},
			contains: []string{"4 files", "1 failed"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &op.Graph{Children: tt.nodes}
			result := formatGraphSummary(g.Summary())
			for _, c := range tt.contains {
				if !strings.Contains(result, c) {
					t.Errorf("expected summary to contain %q, got %q", c, result)
				}
			}
		})
	}
}

func TestSignature(t *testing.T) {
	sig := &sops.Signature{
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
	ctx := op.Provenance{
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
		Timestamp: time.Date(2026, 1, 29, 10, 0, 0, 0, time.UTC),
		State:     op.StatePending,
		Provenance: op.Provenance{
			Tool:       "writ",
			SourceRoot: "/home/user/env",
			TargetRoot: "/home/user",
		},
		Children: []op.SubgraphChild{
			{Node: &op.Node{
				ID:       ".bashrc",
				Receiver: "file.link",
				Status:   op.StatusPending,
			}},
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
		t.Errorf("expected YAML to contain tool in provenance, got %q", output)
	}
	if !strings.Contains(output, ".bashrc") {
		t.Errorf("expected YAML to contain node ID, got %q", output)
	}
	// Note: checksum is computed by WriteReceipt, not Serialize
}

func TestGraphFilename(t *testing.T) {
	t.Run("unscoped", func(t *testing.T) {
		g := &op.Graph{
			Timestamp: time.Date(2026, 1, 29, 14, 30, 45, 0, time.UTC),
		}
		filename := g.Filename()
		expected := "2026-01-29T14-30-45.yaml"
		if filename != expected {
			t.Errorf("expected filename %q, got %q", expected, filename)
		}
	})

	t.Run("scoped", func(t *testing.T) {
		g := &op.Graph{
			Timestamp:  time.Date(2026, 1, 29, 14, 30, 45, 0, time.UTC),
			Provenance: op.Provenance{Scope: "system"},
		}
		filename := g.Filename()
		expected := "system-2026-01-29T14-30-45.yaml"
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
	executor, err := op.NewGraphExecutor("writ", op.Options{})
	if err != nil {
		t.Fatalf("unexpected error creating executor: %v", err)
	}

	_, err = executor.Run(g)
	if err == nil {
		t.Fatal("expected executor.Run to fail when already executed")
	}
	if !strings.Contains(err.Error(), "already executed") {
		t.Errorf("expected error about already executed, got %v", err)
	}
}

func TestGraphSummary(t *testing.T) {
	g := &op.Graph{
		Children: []op.SubgraphChild{
			{Node: &op.Node{ID: "1", Receiver: "file.link", Status: op.StatusCompleted}},
			{Node: &op.Node{ID: "2", Receiver: "file.link", Status: op.StatusCompleted}},
			{Node: &op.Node{ID: "3", Receiver: "template.render_bytes", Status: op.StatusCompleted}},
			{Node: &op.Node{ID: "4", Receiver: "encryption.decrypt", Status: op.StatusCompleted}},
			{Node: &op.Node{ID: "5", Receiver: "file.copy", Status: op.StatusCompleted}},
			{Node: &op.Node{ID: "6", Status: op.StatusSkipped}},
			{Node: &op.Node{ID: "7", Receiver: "file.link", Status: op.StatusFailed}},
			{Node: &op.Node{ID: "8", Receiver: "file.link", Status: op.StatusCompleted}},
		},
	}

	s := g.Summary()
	byAction := s.ByAction()

	if s.Completed() != 6 {
		t.Errorf("expected Completed 6, got %d", s.Completed())
	}
	if byAction["file.link"].Completed() != 3 {
		t.Errorf("expected file.link.Completed 3, got %d", byAction["file.link"].Completed())
	}
	if byAction["template.render_bytes"].Completed() != 1 {
		t.Errorf("expected template.render_bytes.Completed 1, got %d", byAction["template.render_bytes"].Completed())
	}
	if byAction["encryption.decrypt"].Completed() != 1 {
		t.Errorf("expected encryption.decrypt.Completed 1, got %d", byAction["encryption.decrypt"].Completed())
	}
	if byAction["file.copy"].Completed() != 1 {
		t.Errorf("expected file.copy.Completed 1, got %d", byAction["file.copy"].Completed())
	}
	if s.Skipped() != 1 {
		t.Errorf("expected Skipped 1, got %d", s.Skipped())
	}
	if s.Failed() != 1 {
		t.Errorf("expected Failed 1, got %d", s.Failed())
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

	if node.SlotByName("packages") != "curl,wget" {
		t.Errorf("expected packages slot, got %v", node.SlotByName("packages"))
	}
	if node.SlotByName("manager") != "brew" {
		t.Errorf("expected manager slot, got %v", node.SlotByName("manager"))
	}
}

func TestNodeMode(t *testing.T) {
	node := &op.Node{
		ID: ".ssh/config",
	}
	node.SetSlotImmediate("mode", os.FileMode(0o600))

	got := node.SlotByName("mode")
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

// stubRegistry creates a receiver registry for testing.
func stubRegistry(_ ...string) *op.ReceiverRegistry {
	return op.NewReceiverRegistry()
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
	if home.Provenance.Scope != "home" {
		t.Errorf("graphs[0].Scope = %q, want %q", home.Provenance.Scope, "home")
	}
	if home.Provenance.TargetRoot != targetHome {
		t.Errorf("graphs[0].TargetRoot = %q, want %q", home.Provenance.TargetRoot, targetHome)
	}
	// Home graph: .bashrc (base) + .zshrc (personal) = 2 nodes
	if len(home.Nodes()) != 2 {
		t.Errorf("Home graph has %d nodes, want 2", len(home.Nodes()))
		for _, n := range home.Nodes() {
			t.Logf("  node: %s", n.ID)
		}
	}

	// Second graph: System
	sys := graphs[1]
	if sys.Provenance.Scope != "system" {
		t.Errorf("graphs[1].Scope = %q, want %q", sys.Provenance.Scope, "system")
	}
	if sys.Provenance.TargetRoot != "/" {
		t.Errorf("graphs[1].TargetRoot = %q, want %q", sys.Provenance.TargetRoot, "/")
	}
	// System graph: etc/profile (base) = 1 node
	if len(sys.Nodes()) != 1 {
		t.Errorf("System graph has %d nodes, want 1", len(sys.Nodes()))
	}

	// Verify layers are recorded per scope
	if len(home.Provenance.Layers) != 2 {
		t.Errorf("Home layers = %v, want [base personal]", home.Provenance.Layers)
	}
	if len(sys.Provenance.Layers) != 1 || sys.Provenance.Layers[0] != "base" {
		t.Errorf("System layers = %v, want [base]", sys.Provenance.Layers)
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
	if graphs[0].Provenance.Scope != "" {
		t.Errorf("single-source graph should have empty scope, got %q", graphs[0].Provenance.Scope)
	}
	if len(graphs[0].Nodes()) != 1 {
		t.Errorf("got %d nodes, want 1", len(graphs[0].Nodes()))
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
	if graphs[0].Provenance.Scope != "home" {
		t.Errorf("surviving graph scope = %q, want %q", graphs[0].Provenance.Scope, "home")
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

	if g.Provenance.Scope != "home" {
		t.Errorf("Scope = %q, want %q", g.Provenance.Scope, "home")
	}
	if g.Provenance.TargetRoot != "/home/user" {
		t.Errorf("TargetRoot = %q, want %q", g.Provenance.TargetRoot, "/home/user")
	}
	if len(g.Provenance.Projects) != 2 {
		t.Errorf("Projects = %v, want [all noblefactor]", g.Provenance.Projects)
	}
}

// --- Scope Ordering ---

func TestSortGraphsByScope_SystemBeforeHome(t *testing.T) {
	graphs := []*op.Graph{
		{Provenance: op.Provenance{Scope: "home"}},
		{Provenance: op.Provenance{Scope: "system"}},
	}

	sortGraphsByScope(graphs)

	if graphs[0].Provenance.Scope != "system" {
		t.Errorf("graphs[0].Scope = %q, want %q", graphs[0].Provenance.Scope, "system")
	}
	if graphs[1].Provenance.Scope != "home" {
		t.Errorf("graphs[1].Scope = %q, want %q", graphs[1].Provenance.Scope, "home")
	}
}

func TestSortGraphsByScope_AlreadyOrdered(t *testing.T) {
	graphs := []*op.Graph{
		{Provenance: op.Provenance{Scope: "system"}},
		{Provenance: op.Provenance{Scope: "home"}},
	}

	sortGraphsByScope(graphs)

	if graphs[0].Provenance.Scope != "system" {
		t.Errorf("graphs[0].Scope = %q, want %q", graphs[0].Provenance.Scope, "system")
	}
	if graphs[1].Provenance.Scope != "home" {
		t.Errorf("graphs[1].Scope = %q, want %q", graphs[1].Provenance.Scope, "home")
	}
}

func TestSortGraphsByScope_UnscopedLast(t *testing.T) {
	graphs := []*op.Graph{
		{Provenance: op.Provenance{Scope: ""}},
		{Provenance: op.Provenance{Scope: "home"}},
		{Provenance: op.Provenance{Scope: "system"}},
	}

	sortGraphsByScope(graphs)

	if graphs[0].Provenance.Scope != "system" {
		t.Errorf("graphs[0].Scope = %q, want %q", graphs[0].Provenance.Scope, "system")
	}
	if graphs[1].Provenance.Scope != "home" {
		t.Errorf("graphs[1].Scope = %q, want %q", graphs[1].Provenance.Scope, "home")
	}
	if graphs[2].Provenance.Scope != "" {
		t.Errorf("graphs[2].Scope = %q, want empty", graphs[2].Provenance.Scope)
	}
}
