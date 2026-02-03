// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package execution

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runGraph is a test helper that converts a Graph to Executable slice and calls RunNodes.
func runGraph(ctx context.Context, e *GraphExecutor, g *Graph) ([]*Result, error) {
	executables := make([]Executable, len(g.Nodes))
	for i, n := range g.Nodes {
		executables[i] = n
	}
	return e.RunNodes(ctx, executables, g.Edges)
}

// testNode creates a node with source and path slots for testing.
func testNode(id string, ops []string, source, path string) *Node {
	node := &Node{ID: id, Operations: ops}
	if source != "" {
		node.SetSlotImmediate("source", source)
	}
	if path != "" {
		node.SetSlotImmediate("path", path)
	}
	return node
}

func TestRegistryRegisterAndGet(t *testing.T) {
	reg := NewOperationRegistry()
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
	reg := NewOperationRegistry()
	for _, op := range FileOps() {
		reg.Register(op)
	}

	names := reg.Names()
	if len(names) != 10 {
		t.Errorf("expected 10 operations, got %d", len(names))
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
	node := &Node{ID: "test"}
	node.SetSlotImmediate("source", source)
	node.SetSlotImmediate("path", target)

	if err := op.Execute(ctx, node); err != nil {
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
	node := &Node{ID: "test"}
	node.SetSlotImmediate("source", source)
	node.SetSlotImmediate("path", target)

	if err := op.Execute(ctx, node); err != nil {
		t.Fatalf("idempotent link: %v", err)
	}
}

func TestCopyOperation(t *testing.T) {
	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "output.txt")

	op := &CopyOp{}
	ctx := &Context{Context: context.Background()}
	node := &Node{ID: "test"}
	node.SetSlotImmediate("path", target)
	inputContent := []byte("file content")

	checksum, err := op.Write(ctx, node, inputContent)
	if err != nil {
		t.Fatalf("copy: %v", err)
	}

	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(content) != "file content" {
		t.Errorf("expected 'file content', got %q", string(content))
	}

	if checksum == "" {
		t.Error("expected target checksum to be set")
	}
	if !strings.HasPrefix(checksum, "sha256:") {
		t.Errorf("expected sha256: prefix, got %q", checksum)
	}
}

func TestCopyOperationCreatesParentDirs(t *testing.T) {
	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "deep", "nested", "output.txt")

	op := &CopyOp{}
	ctx := &Context{Context: context.Background()}
	node := &Node{ID: "test"}
	node.SetSlotImmediate("path", target)
	inputContent := []byte("nested content")

	if _, err := op.Write(ctx, node, inputContent); err != nil {
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

func TestRenderOperation(t *testing.T) {
	op := &RenderOp{}
	ctx := &Context{
		Context: context.Background(),
		Data:    map[string]any{"Username": "testuser", "Shell": "/bin/zsh"},
	}
	node := &Node{ID: ".bashrc", Project: "all"}
	node.SetSlotImmediate("source", "/environment/all/.bashrc")
	inputContent := []byte("# Shell: {{.Shell}}\n# User: {{.Username}}\n# Project: {{.Project}}")

	result, err := op.Transform(ctx, node, inputContent)
	if err != nil {
		t.Fatalf("expand: %v", err)
	}

	expected := "# Shell: /bin/zsh\n# User: testuser\n# Project: all"
	if string(result) != expected {
		t.Errorf("expected %q, got %q", expected, string(result))
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
	inputContent := []byte("encrypted-data")

	result, err := op.Transform(ctx, node, inputContent)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}

	if string(result) != "decrypted:encrypted-data" {
		t.Errorf("unexpected content: %q", string(result))
	}
}

func TestDecryptOperationNoDecryptor(t *testing.T) {
	op := &DecryptOp{}
	ctx := &Context{Context: context.Background(), Data: map[string]any{}}
	node := &Node{ID: "secret.txt"}
	inputContent := []byte("data")

	if _, err := op.Transform(ctx, node, inputContent); err == nil {
		t.Error("expected error when no decryptor configured")
	}
}

// NOTE: TestDelegateOperation removed - there is no delegation between tools.
// writ and lore share the same execution engine. When writ encounters a
// packages-manifest.yaml, the Package Graph Builder adds package nodes to
// the execution graph (NOT YET IMPLEMENTED).

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
	node := &Node{ID: "test"}
	node.SetSlotImmediate("path", target)

	if err := op.Execute(ctx, node); err != nil {
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
	node := &Node{ID: "test"}
	node.SetSlotImmediate("path", target)

	if err := op.Execute(ctx, node); err != nil {
		t.Fatalf("remove: %v", err)
	}

	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Error("expected file to be removed")
	}
}

func TestWriteOperation(t *testing.T) {
	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "output.txt")
	content := "hello world"

	op := &WriteOp{}
	ctx := &Context{Context: context.Background()}
	node := &Node{ID: "test"}
	node.SetSlotImmediate("path", target)
	node.SetSlotImmediate("content", content)

	if err := op.Execute(ctx, node); err != nil {
		t.Fatalf("write: %v", err)
	}

	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	if string(data) != content {
		t.Errorf("expected %q, got %q", content, string(data))
	}
}

func TestWriteCreatesParentDirs(t *testing.T) {
	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "a", "b", "c", "output.txt")
	content := "nested content"

	op := &WriteOp{}
	ctx := &Context{Context: context.Background()}
	node := &Node{ID: "test"}
	node.SetSlotImmediate("path", target)
	node.SetSlotImmediate("content", content)

	if err := op.Execute(ctx, node); err != nil {
		t.Fatalf("write: %v", err)
	}

	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	if string(data) != content {
		t.Errorf("expected %q, got %q", content, string(data))
	}
}

func TestWriteRequiresContent(t *testing.T) {
	op := &WriteOp{}
	ctx := &Context{Context: context.Background()}
	node := &Node{ID: "test"}
	node.SetSlotImmediate("path", "/tmp/test.txt")

	err := op.Execute(ctx, node)
	if err == nil {
		t.Fatal("expected error when content is missing")
	}
	if !strings.Contains(err.Error(), "no content") {
		t.Errorf("expected 'no content' error, got: %v", err)
	}
}

func TestWriteDryRun(t *testing.T) {
	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "should-not-exist.txt")

	op := &WriteOp{}
	ctx := &Context{Context: context.Background(), DryRun: true}
	node := &Node{ID: "test"}
	node.SetSlotImmediate("path", target)
	node.SetSlotImmediate("content", "test")

	if err := op.Execute(ctx, node); err != nil {
		t.Fatalf("write dry-run: %v", err)
	}

	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Error("expected file to not be created in dry-run mode")
	}
}

func TestEngineRunLinkPipeline(t *testing.T) {
	tmpDir := t.TempDir()
	source := filepath.Join(tmpDir, "source.txt")
	target := filepath.Join(tmpDir, "target.txt")
	if err := os.WriteFile(source, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	reg := NewOperationRegistry()
	reg.Register(&LinkOp{})

	engine := NewGraphExecutor(reg, ExecutorOptions{})
	graph := &Graph{
		Nodes: []*Node{
			testNode(".bashrc", []string{"link"}, source, target),
		},
	}

	results, err := runGraph(context.Background(), engine, graph)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != ResultCompleted {
		t.Errorf("expected completed, got %s", results[0].Status)
	}

	linkTarget, _ := os.Readlink(target)
	if linkTarget != source {
		t.Errorf("expected symlink to %s, got %s", source, linkTarget)
	}
}

func TestEngineRunRenderCopyPipeline(t *testing.T) {
	tmpDir := t.TempDir()
	source := filepath.Join(tmpDir, "template.txt")
	target := filepath.Join(tmpDir, "output.txt")

	if err := os.WriteFile(source, []byte("Hello {{.Username}}!"), 0644); err != nil {
		t.Fatal(err)
	}

	reg := NewOperationRegistry()
	reg.Register(&RenderOp{})
	reg.Register(&CopyOp{})

	engine := NewGraphExecutor(reg, ExecutorOptions{
		Data: map[string]any{"Username": "david"},
	})
	graph := &Graph{
		Nodes: []*Node{
			testNode(".greeting", []string{"render", "copy"}, source, target),
		},
	}

	results, err := runGraph(context.Background(), engine, graph)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	if results[0].Status != ResultCompleted {
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

func TestEngineRunDecryptRenderCopyPipeline(t *testing.T) {
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

	reg := NewOperationRegistry()
	reg.Register(&DecryptOp{})
	reg.Register(&RenderOp{})
	reg.Register(&CopyOp{})

	engine := NewGraphExecutor(reg, ExecutorOptions{
		Data: map[string]any{
			"decryptor": mockDecrypt,
			"Token":     "abc123",
		},
	})
	graph := &Graph{
		Nodes: []*Node{
			testNode(".secret", []string{"decrypt", "render", "copy"}, source, target),
		},
	}

	results, err := runGraph(context.Background(), engine, graph)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	if results[0].Status != ResultCompleted {
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

	reg := NewOperationRegistry()
	reg.Register(&LinkOp{})

	engine := NewGraphExecutor(reg, ExecutorOptions{})
	graph := &Graph{
		Nodes: []*Node{
			testNode("tgt1.txt", []string{"link"}, source1, target1),
			testNode("sub/tgt2.txt", []string{"link"}, source2, target2),
		},
	}

	results, err := runGraph(context.Background(), engine, graph)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, r := range results {
		if r.Status != ResultCompleted {
			t.Errorf("node %s: expected completed, got %s", r.NodeID, r.Status)
		}
	}
}

func TestEngineRunUnknownOperation(t *testing.T) {
	reg := NewOperationRegistry()
	engine := NewGraphExecutor(reg, ExecutorOptions{})
	graph := &Graph{
		Nodes: []*Node{
			{ID: "test", Operations: []string{"unknown_op"}},
		},
	}

	results, _ := runGraph(context.Background(), engine, graph)
	if results[0].Status != ResultFailed {
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

	reg := NewOperationRegistry()
	reg.Register(&LinkOp{})

	engine := NewGraphExecutor(reg, ExecutorOptions{})

	// B depends on A, C depends on B
	graph := &Graph{
		Nodes: []*Node{
			testNode("c", []string{"link"}, srcC, filepath.Join(tmpDir, "out_c")),
			testNode("a", []string{"link"}, srcA, filepath.Join(tmpDir, "out_a")),
			testNode("b", []string{"link"}, srcB, filepath.Join(tmpDir, "out_b")),
		},
		Edges: []Edge{
			{From: "a", To: "b", Relation: "orders"},
			{From: "b", To: "c", Relation: "orders"},
		},
	}

	results, err := runGraph(context.Background(), engine, graph)
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

	reg := NewOperationRegistry()
	reg.Register(&LinkOp{})

	engine := NewGraphExecutor(reg, ExecutorOptions{DryRun: true})
	graph := &Graph{
		Nodes: []*Node{
			testNode(".bashrc", []string{"link"}, source, target),
		},
	}

	results, err := runGraph(context.Background(), engine, graph)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	if results[0].Status != ResultCompleted {
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

	graph := &Graph{
		Nodes: []*Node{
			testNode("test", []string{"link"}, source, target),
		},
	}

	result := Preflight(graph)
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

	graph := &Graph{
		Nodes: []*Node{
			testNode("test", []string{"link"}, source, target),
		},
	}

	result := Preflight(graph)
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

	graph := &Graph{
		Nodes: []*Node{
			testNode("test", []string{"link"}, source, target),
		},
	}

	result := Preflight(graph)
	if result.HasConflicts() {
		t.Error("expected no conflicts for already-deployed symlink")
	}
	if len(result.AlreadyDone) != 1 {
		t.Errorf("expected 1 already-done, got %d", len(result.AlreadyDone))
	}
}

func TestPreflightPackagesManifest(t *testing.T) {
	// packages-manifest nodes use the "packages" operation (NOT YET IMPLEMENTED)
	// The preflight should treat them as ready since there's no filesystem conflict
	graph := &Graph{
		Nodes: []*Node{
			{ID: "manifest", Operations: []string{"packages"}},
		},
	}

	result := Preflight(graph)
	if result.HasConflicts() {
		t.Error("expected no conflicts for packages-manifest node")
	}
	if len(result.Ready) != 1 {
		t.Errorf("expected 1 ready node, got %d", len(result.Ready))
	}
}

func TestFileOpsCount(t *testing.T) {
	ops := FileOps()
	if len(ops) != 10 {
		t.Errorf("expected 10 file ops, got %d", len(ops))
	}

	names := make(map[string]bool)
	for _, op := range ops {
		names[op.Name()] = true
	}

	// NOTE: No "delegate" operation - writ and lore share the same execution.
	// Package operations (install, configure, verify) are NOT YET IMPLEMENTED.
	// mkdir removed - all file operations implicitly create directories.
	expected := []string{"link", "copy", "render", "decrypt", "backup", "unlink", "remove", "write", "validate", "move"}
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
	node := &Node{ID: "test"}
	node.SetSlotImmediate("path", target)

	if err := op.Execute(ctx, node); err != nil {
		t.Fatalf("backup: %v", err)
	}

	// Original should be gone
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Error("expected original file to be moved")
	}

	// Backup path should be recorded in node annotations
	backupPath := node.Annotations["backup_path"]
	if backupPath == "" {
		t.Fatal("expected backup_path in node annotations")
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
	node := &Node{ID: "test", Mode: 0755}
	node.SetSlotImmediate("path", target)
	inputContent := []byte("#!/bin/sh\necho hello")

	if _, err := op.Write(ctx, node, inputContent); err != nil {
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

func TestResultStatusString(t *testing.T) {
	tests := []struct {
		status ResultStatus
		want   string
	}{
		{ResultPending, "pending"},
		{ResultRunning, "running"},
		{ResultCompleted, "completed"},
		{ResultFailed, "failed"},
		{ResultSkipped, "skipped"},
	}

	for _, tt := range tests {
		if got := tt.status.String(); got != tt.want {
			t.Errorf("ResultStatus(%d).String() = %q, want %q", tt.status, got, tt.want)
		}
	}
}

func TestRemoveOperationPrunesEmptyDirs(t *testing.T) {
	tmpDir := t.TempDir()
	// Create nested structure: tmpDir/a/b/c/file.txt
	nested := filepath.Join(tmpDir, "a", "b", "c")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(nested, "file.txt")
	if err := os.WriteFile(target, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	op := &RemoveOp{}
	ctx := &Context{
		Context: context.Background(),
		Data: map[string]any{
			"prune_empty_dirs": true,
			"prune_boundary":   tmpDir,
		},
	}
	node := &Node{ID: "test"}
	node.SetSlotImmediate("path", target)

	if err := op.Execute(ctx, node); err != nil {
		t.Fatalf("remove: %v", err)
	}

	// File should be gone
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Error("expected file to be removed")
	}

	// All empty parent dirs up to boundary should be gone
	if _, err := os.Stat(filepath.Join(tmpDir, "a")); !os.IsNotExist(err) {
		t.Error("expected a/ to be pruned")
	}

	// Boundary dir should still exist
	if _, err := os.Stat(tmpDir); err != nil {
		t.Errorf("boundary dir should still exist: %v", err)
	}
}

func TestRemoveOperationPruneStopsAtNonEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	// Create nested structure: tmpDir/a/b/file.txt and tmpDir/a/other.txt
	nested := filepath.Join(tmpDir, "a", "b")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(nested, "file.txt")
	other := filepath.Join(tmpDir, "a", "other.txt")
	if err := os.WriteFile(target, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(other, []byte("keep"), 0644); err != nil {
		t.Fatal(err)
	}

	op := &RemoveOp{}
	ctx := &Context{
		Context: context.Background(),
		Data: map[string]any{
			"prune_empty_dirs": true,
			"prune_boundary":   tmpDir,
		},
	}
	node := &Node{ID: "test"}
	node.SetSlotImmediate("path", target)

	if err := op.Execute(ctx, node); err != nil {
		t.Fatalf("remove: %v", err)
	}

	// File and b/ should be gone
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Error("expected file to be removed")
	}
	if _, err := os.Stat(nested); !os.IsNotExist(err) {
		t.Error("expected b/ to be pruned")
	}

	// a/ should still exist (has other.txt)
	if _, err := os.Stat(filepath.Join(tmpDir, "a")); err != nil {
		t.Error("expected a/ to remain (not empty)")
	}
	// other.txt should still exist
	if _, err := os.Stat(other); err != nil {
		t.Error("expected other.txt to remain")
	}
}

func TestUnlinkOperationPrunesEmptyDirs(t *testing.T) {
	tmpDir := t.TempDir()
	source := filepath.Join(tmpDir, "source.txt")
	nested := filepath.Join(tmpDir, "a", "b")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(nested, "link.txt")
	if err := os.Symlink(source, target); err != nil {
		t.Fatal(err)
	}

	op := &UnlinkOp{}
	ctx := &Context{
		Context: context.Background(),
		Data: map[string]any{
			"prune_empty_dirs": true,
			"prune_boundary":   tmpDir,
		},
	}
	node := &Node{ID: "test"}
	node.SetSlotImmediate("path", target)

	if err := op.Execute(ctx, node); err != nil {
		t.Fatalf("unlink: %v", err)
	}

	// Symlink and parent dirs should be gone
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Error("expected symlink to be removed")
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "a")); !os.IsNotExist(err) {
		t.Error("expected a/ to be pruned")
	}
}

func TestRemoveNoPruneWithoutFlag(t *testing.T) {
	tmpDir := t.TempDir()
	nested := filepath.Join(tmpDir, "a", "b")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(nested, "file.txt")
	if err := os.WriteFile(target, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	op := &RemoveOp{}
	// No prune flags set
	ctx := &Context{Context: context.Background()}
	node := &Node{ID: "test"}
	node.SetSlotImmediate("path", target)

	if err := op.Execute(ctx, node); err != nil {
		t.Fatalf("remove: %v", err)
	}

	// File should be gone
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Error("expected file to be removed")
	}

	// Parent dirs should remain (no pruning without flag)
	if _, err := os.Stat(nested); err != nil {
		t.Error("expected b/ to remain (no prune flag)")
	}
}
