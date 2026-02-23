// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// createTestGraph creates a minimal test graph for receipt testing.
func createTestGraph() *op.Graph {
	node := &op.Node{
		ID:     ".bashrc",
		Action: op.StubAction("file.link"),
		Status: op.StatusCompleted,
	}
	node.SetSlotImmediate("source", "/home/user/env/.bashrc")
	node.SetSlotImmediate("path", "/home/user/.bashrc")

	return &op.Graph{
		Version:   "5",
		Tool:      "test",
		Timestamp: time.Date(2026, 1, 21, 10, 30, 0, 0, time.UTC),
		State:     op.StateExecuted,
		Platform: op.Platform{
			OS:   "darwin",
			Arch: "arm64",
		},
		Nodes: []*op.Node{node},
	}
}

func TestWriteReceipt_WithGPGSigning(t *testing.T) {
	// This test requires GPG to be installed and configured
	// It creates a .sops.yaml with a test GPG fingerprint

	// Set up temp state directory
	tmpState := t.TempDir()
	origStateHome := os.Getenv("XDG_STATE_HOME")
	_ = os.Setenv("XDG_STATE_HOME", tmpState)
	defer func() { _ = os.Setenv("XDG_STATE_HOME", origStateHome) }()

	// Create devlore state directory
	devloreDir := filepath.Join(tmpState, "devlore")
	if err := os.MkdirAll(devloreDir, 0o755); err != nil {
		t.Fatalf("create devlore dir: %v", err)
	}

	// Create .sops.yaml with a non-existent GPG fingerprint
	// This will make signing fail gracefully (no matching key)
	sopsConfig := `creation_rules:
  - pgp: "0000000000000000000000000000000000000000"
`
	if err := os.WriteFile(filepath.Join(devloreDir, ".sops.yaml"), []byte(sopsConfig), 0o644); err != nil {
		t.Fatalf("write .sops.yaml: %v", err)
	}

	// Create and write receipt
	g := createTestGraph()
	path, err := WriteReceipt(g, "test")
	if err != nil {
		t.Fatalf("WriteReceipt: %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("receipt file not created: %v", err)
	}

	// Verify checksum is always set
	if g.Checksum == "" {
		t.Error("expected Checksum to be set")
	}
	if !strings.HasPrefix(g.Checksum, "sha256:") {
		t.Errorf("expected Checksum to start with 'sha256:', got %q", g.Checksum)
	}

	// Signature may or may not be set depending on GPG availability
	// This is expected - signing is optional
	t.Logf("Checksum: %s", g.Checksum)
	if g.Signature != nil {
		t.Logf("Signature: method=%s, keyID=%s", g.Signature.Method, g.Signature.KeyID)
	} else {
		t.Log("No signature (expected - no matching GPG key)")
	}
}

func TestWriteReceipt_NoSopsConfig(t *testing.T) {
	// Test with no .sops.yaml - signing should be skipped

	// Set up temp state directory
	tmpState := t.TempDir()
	origStateHome := os.Getenv("XDG_STATE_HOME")
	_ = os.Setenv("XDG_STATE_HOME", tmpState)
	defer func() { _ = os.Setenv("XDG_STATE_HOME", origStateHome) }()

	// Create devlore state directory (but no .sops.yaml)
	devloreDir := filepath.Join(tmpState, "devlore")
	if err := os.MkdirAll(devloreDir, 0o755); err != nil {
		t.Fatalf("create devlore dir: %v", err)
	}

	// Create and write receipt
	g := createTestGraph()
	path, err := WriteReceipt(g, "test")
	if err != nil {
		t.Fatalf("WriteReceipt: %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("receipt file not created: %v", err)
	}

	// Verify checksum is always set
	if g.Checksum == "" {
		t.Error("expected Checksum to be set")
	}

	// Verify signature is NOT set (no .sops.yaml)
	if g.Signature != nil {
		t.Errorf("expected Signature to be nil when no .sops.yaml, got %+v", g.Signature)
	}
}

func TestWriteReceipt_EmptySopsConfig(t *testing.T) {
	// Test with empty .sops.yaml - signing should be skipped

	// Set up temp state directory
	tmpState := t.TempDir()
	origStateHome := os.Getenv("XDG_STATE_HOME")
	_ = os.Setenv("XDG_STATE_HOME", tmpState)
	defer func() { _ = os.Setenv("XDG_STATE_HOME", origStateHome) }()

	// Create devlore state directory
	devloreDir := filepath.Join(tmpState, "devlore")
	if err := os.MkdirAll(devloreDir, 0o755); err != nil {
		t.Fatalf("create devlore dir: %v", err)
	}

	// Create empty .sops.yaml
	if err := os.WriteFile(filepath.Join(devloreDir, ".sops.yaml"), []byte("creation_rules: []\n"), 0o644); err != nil {
		t.Fatalf("write .sops.yaml: %v", err)
	}

	// Create and write receipt
	g := createTestGraph()
	path, err := WriteReceipt(g, "test")
	if err != nil {
		t.Fatalf("WriteReceipt: %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("receipt file not created: %v", err)
	}

	// Verify checksum is always set
	if g.Checksum == "" {
		t.Error("expected Checksum to be set")
	}

	// Verify signature is NOT set (empty .sops.yaml)
	if g.Signature != nil {
		t.Errorf("expected Signature to be nil when .sops.yaml is empty, got %+v", g.Signature)
	}
}

func TestLoadReceipt(t *testing.T) {
	// Create a temp receipt file
	tmpDir := t.TempDir()
	receiptPath := filepath.Join(tmpDir, "test-2026-01-21T10-30-00.yaml")

	receiptContent := `version: "5"
tool: test
timestamp: 2026-01-21T10:30:00Z
state: executed
platform:
  os: darwin
  arch: arm64
context: {}
nodes:
  - id: .bashrc
    action: file.link
    status: completed
checksum: "sha256:abc123"
`
	if err := os.WriteFile(receiptPath, []byte(receiptContent), 0o644); err != nil {
		t.Fatalf("write receipt: %v", err)
	}

	g, err := LoadReceipt(receiptPath)
	if err != nil {
		t.Fatalf("LoadReceipt: %v", err)
	}

	if g.Version != "5" {
		t.Errorf("expected Version '5', got %q", g.Version)
	}
	if g.Tool != "test" {
		t.Errorf("expected Tool 'test', got %q", g.Tool)
	}
	if g.Checksum != "sha256:abc123" {
		t.Errorf("expected Checksum 'sha256:abc123', got %q", g.Checksum)
	}
	if len(g.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(g.Nodes))
	}
	if g.Nodes[0].ID != ".bashrc" {
		t.Errorf("expected node ID '.bashrc', got %q", g.Nodes[0].ID)
	}
}

func TestReceiptsDir(t *testing.T) {
	// Test with XDG_STATE_HOME set
	tmpDir := t.TempDir()
	origStateHome := os.Getenv("XDG_STATE_HOME")
	_ = os.Setenv("XDG_STATE_HOME", tmpDir)
	defer func() { _ = os.Setenv("XDG_STATE_HOME", origStateHome) }()

	dir := ReceiptsDir()
	expected := filepath.Join(tmpDir, "devlore", "receipts")
	if dir != expected {
		t.Errorf("ReceiptsDir() = %q, want %q", dir, expected)
	}
}

func TestLatestReceiptPath(t *testing.T) {
	tmpDir := t.TempDir()
	origStateHome := os.Getenv("XDG_STATE_HOME")
	_ = os.Setenv("XDG_STATE_HOME", tmpDir)
	defer func() { _ = os.Setenv("XDG_STATE_HOME", origStateHome) }()

	path := LatestReceiptPath("writ")
	expected := filepath.Join(tmpDir, "devlore", "receipts", "writ-latest.yaml")
	if path != expected {
		t.Errorf("LatestReceiptPath('writ') = %q, want %q", path, expected)
	}
}
