// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package writ

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/NobleFactor/devlore-cli/internal/cli"
	"github.com/NobleFactor/devlore-cli/internal/execution"
	"github.com/NobleFactor/devlore-cli/pkg/projection"
)

func TestGraphStates(t *testing.T) {
	tests := []struct {
		state    projection.GraphState
		expected string
	}{
		{projection.StatePending, "pending"},
		{projection.StateExecuted, "executed"},
		{projection.StateFailed, "failed"},
	}

	for _, tt := range tests {
		if string(tt.state) != tt.expected {
			t.Errorf("expected state %q, got %q", tt.expected, tt.state)
		}
	}
}

func TestNodeStatus(t *testing.T) {
	tests := []struct {
		status   projection.NodeStatus
		expected string
	}{
		{projection.StatusPending, "pending"},
		{projection.StatusCompleted, "completed"},
		{projection.StatusSkipped, "skipped"},
		{projection.StatusFailed, "failed"},
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
	if g.State != projection.StatePending {
		t.Errorf("expected state 'pending', got %q", g.State)
	}
	if g.Context.SourceRoot != "/home/user/env" {
		t.Errorf("expected SourceRoot '/home/user/env', got %q", g.Context.SourceRoot)
	}
}

func TestNode(t *testing.T) {
	node := &projection.Node{
		ID:         ".bashrc",
		Action: projection.StubAction("file.link"),
		Status:    projection.StatusPending,
		Project:   "all",
	}
	node.SetSlotImmediate("source", "/home/user/env/all/.bashrc")
	node.SetSlotImmediate("path", "/home/user/.bashrc")

	if node.ID != ".bashrc" {
		t.Errorf("expected ID '.bashrc', got %q", node.ID)
	}
	if node.ActionName() != "file.link" {
		t.Errorf("expected operation 'file.link', got %q", node.ActionName())
	}
	if node.Status != projection.StatusPending {
		t.Errorf("expected status 'pending', got %q", node.Status)
	}
}

func TestEdge(t *testing.T) {
	edge := projection.Edge{
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
	collision := projection.Collision{
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
		summary  projection.Summary
		contains []string
	}{
		{
			name: "basic",
			summary: projection.Summary{
				TotalFiles: 10,
				Links:      5,
				Templates:  3,
				Secrets:    2,
			},
			contains: []string{"10 files", "5 links", "3 templates", "2 secrets"},
		},
		{
			name: "with skipped",
			summary: projection.Summary{
				TotalFiles: 5,
				Links:      5,
				Skipped:    2,
			},
			contains: []string{"5 files", "2 skipped"},
		},
		{
			name: "with failed",
			summary: projection.Summary{
				TotalFiles: 5,
				Links:      4,
				Failed:     1,
			},
			contains: []string{"5 files", "1 failed"},
		},
		{
			name: "with backups",
			summary: projection.Summary{
				TotalFiles: 5,
				Links:      5,
				BackedUp:   3,
			},
			contains: []string{"3 backed up"},
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
	sig := &projection.Signature{
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
	p := projection.Platform{
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
	ctx := projection.GraphContext{
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
	g := &projection.Graph{
		Version:   "1",
		Tool:      "writ",
		Timestamp: time.Date(2026, 1, 29, 10, 0, 0, 0, time.UTC),
		State:     projection.StatePending,
		Platform:  projection.Platform{OS: "darwin", Arch: "arm64"},
		Context: projection.GraphContext{
			SourceRoot: "/home/user/env",
			TargetRoot: "/home/user",
		},
		Nodes: []*projection.Node{
			{
				ID:         ".bashrc",
				Action: projection.StubAction("file.link"),
				Status:     projection.StatusPending,
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
	g := &projection.Graph{
		Tool:      "writ",
		Timestamp: time.Date(2026, 1, 29, 14, 30, 45, 0, time.UTC),
	}

	filename := g.Filename()
	expected := "writ-2026-01-29T14-30-45.yaml"
	if filename != expected {
		t.Errorf("expected filename %q, got %q", expected, filename)
	}
}

func TestGitStyleChecksum(t *testing.T) {
	content := []byte("test content")
	basename := "test.yaml"

	checksum := projection.GitStyleChecksum("graph", basename, content)

	// Should have sha256: prefix
	if !strings.HasPrefix(checksum, "sha256:") {
		t.Errorf("expected checksum to start with 'sha256:', got %q", checksum)
	}

	// Should be deterministic
	checksum2 := projection.GitStyleChecksum("graph", basename, content)
	if checksum != checksum2 {
		t.Error("expected checksum to be deterministic")
	}

	// Different content should produce different checksum
	checksum3 := projection.GitStyleChecksum("graph", basename, []byte("different"))
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
	g := &projection.Graph{
		State: projection.StateExecuted,
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
	g := &projection.Graph{
		Nodes: []*projection.Node{
			{ID: "1", Action: projection.StubAction("file.link"), Status: projection.StatusCompleted},
			{ID: "2", Action: projection.StubAction("file.link"), Status: projection.StatusCompleted},
			{ID: "3", Action: projection.StubAction("template.render"), Status: projection.StatusCompleted},
			{ID: "4", Action: projection.StubAction("encryption.decrypt"), Status: projection.StatusCompleted},
			{ID: "5", Action: projection.StubAction("file.copy"), Status: projection.StatusCompleted},
			{ID: "6", Status: projection.StatusSkipped},
			{ID: "7", Action: projection.StubAction("file.link"), Status: projection.StatusFailed},
			{ID: "8", Action: projection.StubAction("file.link"), Status: projection.StatusCompleted, Annotations: map[string]string{"backup": "/path/to/backup"}},
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
	if g.Summary.BackedUp != 1 {
		t.Errorf("expected BackedUp 1, got %d", g.Summary.BackedUp)
	}
}

func TestNodeAnnotations(t *testing.T) {
	node := &projection.Node{
		ID:          ".bashrc",
		Annotations: map[string]string{"backup": "/backup/path"},
	}

	if node.Annotations["backup"] != "/backup/path" {
		t.Errorf("expected backup annotation, got %v", node.Annotations)
	}
}

func TestNodeSlots(t *testing.T) {
	node := &projection.Node{
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
	node := &projection.Node{
		ID: ".ssh/config",
	}
	node.SetSlotImmediate("mode", os.FileMode(0600))

	got := node.GetSlot("mode")
	mode, ok := got.(os.FileMode)
	if !ok {
		t.Fatalf("expected mode slot to be os.FileMode, got %T", got)
	}
	if mode != 0600 {
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
