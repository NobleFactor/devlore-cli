// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRegistryRegisterAndGet(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&LinkOp{})
	reg.Register(&CopyOp{})

	op, ok := reg.Get("link")
	if !ok {
		t.Fatal("expected link operation to be registered")
	}
	if op.Name() != "link" {
		t.Errorf("expected name 'link', got %q", op.Name())
	}

	_, ok = reg.Get("nonexistent")
	if ok {
		t.Error("expected nonexistent operation to not be found")
	}
}

func TestRegistryNames(t *testing.T) {
	reg := NewRegistry()
	for _, op := range FileOps() {
		reg.Register(op)
	}

	names := reg.Names()
	if len(names) != 8 {
		t.Errorf("expected 8 operations, got %d", len(names))
	}
}

func TestLinkOperation(t *testing.T) {
	tmpDir := t.TempDir()
	source := filepath.Join(tmpDir, "source.txt")
	target := filepath.Join(tmpDir, "target.txt")

	if err := os.WriteFile(source, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	op := &LinkOp{}
	ctx := &Context{Context: context.Background()}
	node := &Node{ID: "test", Source: source, Target: target}
	state := &PipelineState{Metadata: make(map[string]string)}

	if err := op.Execute(ctx, node, state); err != nil {
		t.Fatalf("link: %v", err)
	}

	linkTarget, err := os.Readlink(target)
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if linkTarget != source {
		t.Errorf("expected symlink to %s, got %s", source, linkTarget)
	}
}

func TestLinkOperationIdempotent(t *testing.T) {
	tmpDir := t.TempDir()
	source := filepath.Join(tmpDir, "source.txt")
	target := filepath.Join(tmpDir, "target.txt")

	if err := os.WriteFile(source, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(source, target); err != nil {
		t.Fatal(err)
	}

	op := &LinkOp{}
	ctx := &Context{Context: context.Background()}
	node := &Node{ID: "test", Source: source, Target: target}
	state := &PipelineState{Metadata: make(map[string]string)}

	if err := op.Execute(ctx, node, state); err != nil {
		t.Fatalf("idempotent link: %v", err)
	}
}

func TestCopyOperation(t *testing.T) {
	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "output.txt")

	op := &CopyOp{}
	ctx := &Context{Context: context.Background()}
	node := &Node{ID: "test", Target: target}
	state := &PipelineState{
		Content:  []byte("file content"),
		Metadata: make(map[string]string),
	}

	if err := op.Execute(ctx, node, state); err != nil {
		t.Fatalf("copy: %v", err)
	}

	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(content) != "file content" {
		t.Errorf("expected 'file content', got %q", string(content))
	}

	if state.TargetChecksum == "" {
		t.Error("expected target checksum to be set")
	}
	if !strings.HasPrefix(state.TargetChecksum, "sha256:") {
		t.Errorf("expected sha256: prefix, got %q", state.TargetChecksum)
	}
}

func TestCopyOperationCreatesParentDirs(t *testing.T) {
	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "deep", "nested", "output.txt")

	op := &CopyOp{}
	ctx := &Context{Context: context.Background()}
	node := &Node{ID: "test", Target: target}
	state := &PipelineState{
		Content:  []byte("nested content"),
		Metadata: make(map[string]string),
	}

	if err := op.Execute(ctx, node, state); err != nil {
		t.Fatalf("copy with nested dirs: %v", err)
	}

	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(content) != "nested content" {
		t.Errorf("expected 'nested content', got %q", string(content))
	}
}

func TestExpandOperation(t *testing.T) {
	op := &ExpandOp{}
	ctx := &Context{
		Context: context.Background(),
		Data:    map[string]any{"Username": "testuser", "Shell": "/bin/zsh"},
	}
	node := &Node{ID: ".bashrc", Source: "/dotfiles/all/.bashrc", Project: "all"}
	state := &PipelineState{
		Content:  []byte("# Shell: {{.Shell}}\n# User: {{.Username}}\n# Project: {{.Project}}"),
		Metadata: make(map[string]string),
	}

	if err := op.Execute(ctx, node, state); err != nil {
		t.Fatalf("expand: %v", err)
	}

	expected := "# Shell: /bin/zsh\n# User: testuser\n# Project: all"
	if string(state.Content) != expected {
		t.Errorf("expected %q, got %q", expected, string(state.Content))
	}
}

func TestDecryptOperation(t *testing.T) {
	op := &DecryptOp{}

	// Provide a mock decryptor
	mockDecrypt := func(ciphertext []byte) ([]byte, error) {
		return []byte("decrypted:" + string(ciphertext)), nil
	}

	ctx := &Context{
		Context: context.Background(),
		Data:    map[string]any{"decryptor": mockDecrypt},
	}
	node := &Node{ID: "secret.txt"}
	state := &PipelineState{
		Content:  []byte("encrypted-data"),
		Metadata: make(map[string]string),
	}

	if err := op.Execute(ctx, node, state); err != nil {
		t.Fatalf("decrypt: %v", err)
	}

	if string(state.Content) != "decrypted:encrypted-data" {
		t.Errorf("unexpected content: %q", string(state.Content))
	}
}

func TestDecryptOperationNoDecryptor(t *testing.T) {
	op := &DecryptOp{}
	ctx := &Context{Context: context.Background(), Data: map[string]any{}}
	node := &Node{ID: "secret.txt"}
	state := &PipelineState{Content: []byte("data"), Metadata: make(map[string]string)}

	if err := op.Execute(ctx, node, state); err == nil {
		t.Error("expected error when no decryptor configured")
	}
}

func TestDelegateOperation(t *testing.T) {
	op := &DelegateOp{}
	ctx := &Context{Context: context.Background()}
	node := &Node{ID: ".config/packages.manifest", DelegateTo: "lore"}
	state := &PipelineState{Metadata: make(map[string]string)}

	if err := op.Execute(ctx, node, state); err != nil {
		t.Fatalf("delegate: %v", err)
	}

	if state.Metadata["delegate_to"] != "lore" {
		t.Errorf("expected delegate_to 'lore', got %q", state.Metadata["delegate_to"])
	}
}

func TestUnlinkOperation(t *testing.T) {
	tmpDir := t.TempDir()
	source := filepath.Join(tmpDir, "source.txt")
	target := filepath.Join(tmpDir, "link.txt")

	if err := os.WriteFile(source, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(source, target); err != nil {
		t.Fatal(err)
	}

	op := &UnlinkOp{}
	ctx := &Context{Context: context.Background()}
	node := &Node{ID: "test", Target: target}
	state := &PipelineState{Metadata: make(map[string]string)}

	if err := op.Execute(ctx, node, state); err != nil {
		t.Fatalf("unlink: %v", err)
	}

	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Error("expected symlink to be removed")
	}
}

func TestRemoveOperation(t *testing.T) {
	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "file.txt")
	if err := os.WriteFile(target, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	op := &RemoveOp{}
	ctx := &Context{Context: context.Background()}
	node := &Node{ID: "test", Target: target}
	state := &PipelineState{Metadata: make(map[string]string)}

	if err := op.Execute(ctx, node, state); err != nil {
		t.Fatalf("remove: %v", err)
	}

	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Error("expected file to be removed")
	}
}

func TestEngineRunLinkPipeline(t *testing.T) {
	tmpDir := t.TempDir()
	source := filepath.Join(tmpDir, "source.txt")
	target := filepath.Join(tmpDir, "target.txt")
	if err := os.WriteFile(source, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	reg := NewRegistry()
	reg.Register(&LinkOp{})

	engine := New(reg, Options{})
	graph := &Graph{
		Nodes: []*Node{
			{ID: ".bashrc", Operations: []string{"link"}, Source: source, Target: target},
		},
	}

	results, err := engine.Run(context.Background(), graph)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != StatusCompleted {
		t.Errorf("expected completed, got %s", results[0].Status)
	}

	linkTarget, _ := os.Readlink(target)
	if linkTarget != source {
		t.Errorf("expected symlink to %s, got %s", source, linkTarget)
	}
}

func TestEngineRunExpandCopyPipeline(t *testing.T) {
	tmpDir := t.TempDir()
	source := filepath.Join(tmpDir, "template.txt")
	target := filepath.Join(tmpDir, "output.txt")

	if err := os.WriteFile(source, []byte("Hello {{.Username}}!"), 0644); err != nil {
		t.Fatal(err)
	}

	reg := NewRegistry()
	reg.Register(&ExpandOp{})
	reg.Register(&CopyOp{})

	engine := New(reg, Options{
		Data: map[string]any{"Username": "david"},
	})
	graph := &Graph{
		Nodes: []*Node{
			{ID: ".greeting", Operations: []string{"expand", "copy"}, Source: source, Target: target},
		},
	}

	results, err := engine.Run(context.Background(), graph)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	if results[0].Status != StatusCompleted {
		t.Errorf("expected completed, got %s (error: %v)", results[0].Status, results[0].Error)
	}

	content, _ := os.ReadFile(target)
	if string(content) != "Hello david!" {
		t.Errorf("expected 'Hello david!', got %q", string(content))
	}

	if results[0].SourceChecksum == "" {
		t.Error("expected source checksum")
	}
	if results[0].TargetChecksum == "" {
		t.Error("expected target checksum")
	}
}

func TestEngineRunDecryptExpandCopyPipeline(t *testing.T) {
	tmpDir := t.TempDir()
	source := filepath.Join(tmpDir, "secret.txt.sops")
	target := filepath.Join(tmpDir, "secret.txt")

	if err := os.WriteFile(source, []byte("encrypted:token={{.Token}}"), 0644); err != nil {
		t.Fatal(err)
	}

	mockDecrypt := func(ciphertext []byte) ([]byte, error) {
		// Strip "encrypted:" prefix
		return []byte(strings.TrimPrefix(string(ciphertext), "encrypted:")), nil
	}

	reg := NewRegistry()
	reg.Register(&DecryptOp{})
	reg.Register(&ExpandOp{})
	reg.Register(&CopyOp{})

	engine := New(reg, Options{
		Data: map[string]any{
			"decryptor": mockDecrypt,
			"Token":     "abc123",
		},
	})
	graph := &Graph{
		Nodes: []*Node{
			{ID: ".secret", Operations: []string{"decrypt", "expand", "copy"}, Source: source, Target: target},
		},
	}

	results, err := engine.Run(context.Background(), graph)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	if results[0].Status != StatusCompleted {
		t.Fatalf("expected completed, got %s (error: %v)", results[0].Status, results[0].Error)
	}

	content, _ := os.ReadFile(target)
	if string(content) != "token=abc123" {
		t.Errorf("expected 'token=abc123', got %q", string(content))
	}
}

func TestEngineRunMultipleNodes(t *testing.T) {
	tmpDir := t.TempDir()

	source1 := filepath.Join(tmpDir, "src1.txt")
	target1 := filepath.Join(tmpDir, "tgt1.txt")
	source2 := filepath.Join(tmpDir, "src2.txt")
	target2 := filepath.Join(tmpDir, "sub", "tgt2.txt")

	if err := os.WriteFile(source1, []byte("file1"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source2, []byte("file2"), 0644); err != nil {
		t.Fatal(err)
	}

	reg := NewRegistry()
	reg.Register(&LinkOp{})

	engine := New(reg, Options{})
	graph := &Graph{
		Nodes: []*Node{
			{ID: "tgt1.txt", Operations: []string{"link"}, Source: source1, Target: target1},
			{ID: "sub/tgt2.txt", Operations: []string{"link"}, Source: source2, Target: target2},
		},
	}

	results, err := engine.Run(context.Background(), graph)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, r := range results {
		if r.Status != StatusCompleted {
			t.Errorf("node %s: expected completed, got %s", r.NodeID, r.Status)
		}
	}
}

func TestEngineRunUnknownOperation(t *testing.T) {
	reg := NewRegistry()
	engine := New(reg, Options{})
	graph := &Graph{
		Nodes: []*Node{
			{ID: "test", Operations: []string{"unknown_op"}},
		},
	}

	results, _ := engine.Run(context.Background(), graph)
	if results[0].Status != StatusFailed {
		t.Errorf("expected failed, got %s", results[0].Status)
	}
}

func TestEngineTopologicalSort(t *testing.T) {
	tmpDir := t.TempDir()

	// Create sources
	srcA := filepath.Join(tmpDir, "a.txt")
	srcB := filepath.Join(tmpDir, "b.txt")
	srcC := filepath.Join(tmpDir, "c.txt")
	if err := os.WriteFile(srcA, []byte("a"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(srcB, []byte("b"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(srcC, []byte("c"), 0644); err != nil {
		t.Fatal(err)
	}

	reg := NewRegistry()
	reg.Register(&LinkOp{})

	engine := New(reg, Options{})

	// B depends on A, C depends on B
	graph := &Graph{
		Nodes: []*Node{
			{ID: "c", Operations: []string{"link"}, Source: srcC, Target: filepath.Join(tmpDir, "out_c")},
			{ID: "a", Operations: []string{"link"}, Source: srcA, Target: filepath.Join(tmpDir, "out_a")},
			{ID: "b", Operations: []string{"link"}, Source: srcB, Target: filepath.Join(tmpDir, "out_b")},
		},
		Edges: []Edge{
			{From: "a", To: "b", Relation: "orders"},
			{From: "b", To: "c", Relation: "orders"},
		},
	}

	results, err := engine.Run(context.Background(), graph)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	// Verify execution order: a before b before c
	if results[0].NodeID != "a" {
		t.Errorf("expected first node 'a', got %q", results[0].NodeID)
	}
	if results[1].NodeID != "b" {
		t.Errorf("expected second node 'b', got %q", results[1].NodeID)
	}
	if results[2].NodeID != "c" {
		t.Errorf("expected third node 'c', got %q", results[2].NodeID)
	}
}

func TestEngineDryRun(t *testing.T) {
	tmpDir := t.TempDir()
	source := filepath.Join(tmpDir, "source.txt")
	target := filepath.Join(tmpDir, "target.txt")
	if err := os.WriteFile(source, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	reg := NewRegistry()
	reg.Register(&LinkOp{})

	engine := New(reg, Options{DryRun: true})
	graph := &Graph{
		Nodes: []*Node{
			{ID: ".bashrc", Operations: []string{"link"}, Source: source, Target: target},
		},
	}

	results, err := engine.Run(context.Background(), graph)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	if results[0].Status != StatusCompleted {
		t.Errorf("expected completed, got %s", results[0].Status)
	}

	// Target should NOT exist in dry-run mode
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Error("expected target to not exist in dry-run mode")
	}
}

func TestPreflightNoConflict(t *testing.T) {
	tmpDir := t.TempDir()
	source := filepath.Join(tmpDir, "source.txt")
	target := filepath.Join(tmpDir, "target.txt") // Doesn't exist
	if err := os.WriteFile(source, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	reg := NewRegistry()
	engine := New(reg, Options{})
	graph := &Graph{
		Nodes: []*Node{
			{ID: "test", Operations: []string{"link"}, Source: source, Target: target},
		},
	}

	result := engine.Preflight(graph)
	if result.HasConflicts() {
		t.Error("expected no conflicts")
	}
	if len(result.Ready) != 1 {
		t.Errorf("expected 1 ready node, got %d", len(result.Ready))
	}
}

func TestPreflightConflictRegularFile(t *testing.T) {
	tmpDir := t.TempDir()
	source := filepath.Join(tmpDir, "source.txt")
	target := filepath.Join(tmpDir, "target.txt")
	if err := os.WriteFile(source, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("existing"), 0644); err != nil { // Conflict!
		t.Fatal(err)
	}

	reg := NewRegistry()
	engine := New(reg, Options{})
	graph := &Graph{
		Nodes: []*Node{
			{ID: "test", Operations: []string{"link"}, Source: source, Target: target},
		},
	}

	result := engine.Preflight(graph)
	if !result.HasConflicts() {
		t.Error("expected conflict")
	}
	if result.Conflicts[0].Type != ConflictRegularFile {
		t.Errorf("expected regular file conflict, got %d", result.Conflicts[0].Type)
	}
}

func TestPreflightAlreadyDeployed(t *testing.T) {
	tmpDir := t.TempDir()
	source := filepath.Join(tmpDir, "source.txt")
	target := filepath.Join(tmpDir, "target.txt")
	if err := os.WriteFile(source, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(source, target); err != nil { // Already correct
		t.Fatal(err)
	}

	reg := NewRegistry()
	engine := New(reg, Options{})
	graph := &Graph{
		Nodes: []*Node{
			{ID: "test", Operations: []string{"link"}, Source: source, Target: target},
		},
	}

	result := engine.Preflight(graph)
	if result.HasConflicts() {
		t.Error("expected no conflicts for already-deployed symlink")
	}
	if len(result.AlreadyDone) != 1 {
		t.Errorf("expected 1 already-done, got %d", len(result.AlreadyDone))
	}
}

func TestPreflightDelegateSkipped(t *testing.T) {
	reg := NewRegistry()
	engine := New(reg, Options{})
	graph := &Graph{
		Nodes: []*Node{
			{ID: "manifest", Operations: []string{"delegate"}, DelegateTo: "lore"},
		},
	}

	result := engine.Preflight(graph)
	if result.HasConflicts() {
		t.Error("expected no conflicts for delegate node")
	}
	if len(result.Ready) != 1 {
		t.Errorf("expected 1 ready node, got %d", len(result.Ready))
	}
}

func TestFileOpsCount(t *testing.T) {
	ops := FileOps()
	if len(ops) != 8 {
		t.Errorf("expected 8 file ops, got %d", len(ops))
	}

	names := make(map[string]bool)
	for _, op := range ops {
		names[op.Name()] = true
	}

	expected := []string{"link", "copy", "expand", "decrypt", "delegate", "backup", "unlink", "remove"}
	for _, name := range expected {
		if !names[name] {
			t.Errorf("expected operation %q in FileOps()", name)
		}
	}
}

func TestBackupOperation(t *testing.T) {
	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "file.txt")
	if err := os.WriteFile(target, []byte("original"), 0644); err != nil {
		t.Fatal(err)
	}

	op := &BackupOp{}
	ctx := &Context{Context: context.Background(), Data: map[string]any{}}
	node := &Node{ID: "test", Target: target}
	state := &PipelineState{Metadata: make(map[string]string)}

	if err := op.Execute(ctx, node, state); err != nil {
		t.Fatalf("backup: %v", err)
	}

	// Original should be gone
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Error("expected original file to be moved")
	}

	// Backup path should be recorded in metadata
	backupPath := state.Metadata["backup_path"]
	if backupPath == "" {
		t.Fatal("expected backup_path in metadata")
	}

	// Backup should exist with original content
	content, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if string(content) != "original" {
		t.Errorf("expected 'original', got %q", string(content))
	}
}

func TestCopyOperationWithMode(t *testing.T) {
	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "script.sh")

	op := &CopyOp{}
	ctx := &Context{Context: context.Background()}
	node := &Node{ID: "test", Target: target, Mode: 0755}
	state := &PipelineState{
		Content:  []byte("#!/bin/sh\necho hello"),
		Metadata: make(map[string]string),
	}

	if err := op.Execute(ctx, node, state); err != nil {
		t.Fatalf("copy with mode: %v", err)
	}

	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0755 {
		t.Errorf("expected mode 0755, got %04o", info.Mode().Perm())
	}
}

func TestStatusString(t *testing.T) {
	tests := []struct {
		status Status
		want   string
	}{
		{StatusPending, "pending"},
		{StatusRunning, "running"},
		{StatusCompleted, "completed"},
		{StatusFailed, "failed"},
		{StatusSkipped, "skipped"},
	}

	for _, tt := range tests {
		if got := tt.status.String(); got != tt.want {
			t.Errorf("Status(%d).String() = %q, want %q", tt.status, got, tt.want)
		}
	}
}
