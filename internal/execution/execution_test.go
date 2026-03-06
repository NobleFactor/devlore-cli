// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

//go:build ignore
// +build ignore

package execution_test

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/internal/execution"
	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/encryption"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/file"
	filegen "github.com/NobleFactor/devlore-cli/pkg/op/provider/file/gen"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/template"
)

// slotsFrom extracts immediate slot values from a node for direct action testing.
func slotsFrom(node *op.Node) map[string]any {
	slots := make(map[string]any)
	if node.Slots != nil {
		for k, sv := range node.Slots {
			if sv.IsImmediate() {
				slots[k] = sv.Immediate
			}
		}
	}
	return slots
}

// runGraph is a test helper that calls RunNodes with the graph's nodes and edges.
func runGraph(ctx context.Context, e *execution.GraphExecutor, g *op.Graph) ([]*execution.NodeResult, error) {
	return e.RunNodes(ctx, g.Nodes, g.Edges)
}

// testNode creates a node with the given action and source/path slots for testing.
func testNode(id string, action op.Action, source, path string) *op.Node {
	node := &op.Node{ID: id, Action: action}
	if source != "" {
		node.SetSlotImmediate("source", source)
	}
	if path != "" {
		node.SetSlotImmediate("path", path)
	}
	return node
}

func TestRegistryRegisterAndGet(t *testing.T) {
	reg := op.NewActionRegistry()
	filegen.Register(reg)

	act, ok := reg.Get("file.link")
	if !ok {
		t.Fatal("expected file.link action to be registered")
	}
	if act.Name() != "file.link" {
		t.Errorf("expected name 'file.link', got %q", act.Name())
	}

	_, ok = reg.Get("nonexistent")
	if ok {
		t.Error("expected nonexistent action to not be found")
	}
}

func TestRegistryNames(t *testing.T) {
	reg := op.NewActionRegistry()
	filegen.Register(reg)

	names := reg.Names()
	if len(names) != 12 {
		t.Errorf("expected 12 file actions, got %d", len(names))
	}
}

func TestAllProvidersCount(t *testing.T) {
	reg := op.NewActionRegistry()
	op.InitAll(reg, op.Context{})

	names := reg.Names()
	sort.Strings(names)
	if len(names) != 36 {
		t.Errorf("expected 36 total actions, got %d: %v", len(names), names)
	}

	expected := []string{
		"file.backup", "file.copy", "file.glob", "file.link", "file.mkdir", "file.move", "file.read", "file.remove", "file.remove_all", "file.unlink", "file.write_bytes", "file.write_text",
		"encryption.decrypt",
		"template.render",
		"pkg.install", "pkg.upgrade", "pkg.remove", "pkg.update", "pkg.installed", "pkg.not_installed", "pkg.version_gte",
		"shell.exec", "shell.power_shell",
		"service.start", "service.stop", "service.restart", "service.enable", "service.disable", "service.exists", "service.running", "service.enabled",
		"net.download",
		"archive.extract",
		"git.clone", "git.checkout", "git.pull",
	}
	nameSet := make(map[string]bool)
	for _, n := range names {
		nameSet[n] = true
	}
	for _, name := range expected {
		if !nameSet[name] {
			t.Errorf("expected action %q in registry", name)
		}
	}
}

func TestLinkAction(t *testing.T) {
	tmpDir := t.TempDir()
	source := filepath.Join(tmpDir, "source.txt")
	target := filepath.Join(tmpDir, "target.txt")

	if err := os.WriteFile(source, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := &file.Provider{}
	action := &filegen.Link{Impl: p}
	ctx := &op.Context{Context: context.Background()}
	node := &op.Node{ID: "test"}
	node.SetSlotImmediate("source", source)
	node.SetSlotImmediate("path", target)

	if _, _, err := action.Do(ctx, slotsFrom(node)); err != nil {
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

func TestLinkActionIdempotent(t *testing.T) {
	tmpDir := t.TempDir()
	source := filepath.Join(tmpDir, "source.txt")
	target := filepath.Join(tmpDir, "target.txt")

	if err := os.WriteFile(source, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(source, target); err != nil {
		t.Fatal(err)
	}

	p := &file.Provider{}
	action := &filegen.Link{Impl: p}
	ctx := &op.Context{Context: context.Background()}
	node := &op.Node{ID: "test"}
	node.SetSlotImmediate("source", source)
	node.SetSlotImmediate("path", target)

	if _, _, err := action.Do(ctx, slotsFrom(node)); err != nil {
		t.Fatalf("idempotent link: %v", err)
	}
}

func TestRenderAction(t *testing.T) {
	tmpDir := t.TempDir()
	source := filepath.Join(tmpDir, "template.txt")
	target := filepath.Join(tmpDir, "output.txt")
	templateContent := "# Shell: {{.Shell}}\n# User: {{.Username}}\n# Project: {{.Project}}"
	if err := os.WriteFile(source, []byte(templateContent), 0o644); err != nil {
		t.Fatal(err)
	}

	p := &template.Provider{}
	action := &template.Render{Impl: p}
	ctx := &op.Context{Context: context.Background()}
	node := &op.Node{ID: ".bashrc", Project: "all"}
	node.SetSlotImmediate("template_data", map[string]any{"Username": "testuser", "Shell": "/bin/zsh"})
	node.SetSlotImmediate("source", source)
	node.SetSlotImmediate("path", target)
	node.SetSlotImmediate("content", []byte(templateContent))
	node.SetSlotImmediate("project", "all")

	result, _, err := action.Do(ctx, slotsFrom(node))
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	rendered, ok := result.([]byte)
	if !ok {
		t.Fatalf("expected []byte result, got %T", result)
	}
	expected := "# Shell: /bin/zsh\n# User: testuser\n# Project: all"
	if string(rendered) != expected {
		t.Errorf("expected %q, got %q", expected, string(rendered))
	}
}

func TestDecryptAction(t *testing.T) {
	tmpDir := t.TempDir()
	source := filepath.Join(tmpDir, "secret.txt")
	if err := os.WriteFile(source, []byte("encrypted-data"), 0o644); err != nil {
		t.Fatal(err)
	}

	ep := &encryption.Provider{}
	action := &encryption.Decrypt{Impl: ep}

	// Provide a mock decryptor
	mockDecrypt := func(source string, ciphertext []byte) ([]byte, error) {
		return []byte("decrypted:" + string(ciphertext)), nil
	}

	ctx := &op.Context{Context: context.Background()}
	node := &op.Node{ID: "secret.txt"}
	node.SetSlotImmediate("source", source)
	node.SetSlotImmediate("decryptor", mockDecrypt)
	node.SetSlotImmediate("content", []byte("encrypted-data"))

	result, _, err := action.Do(ctx, slotsFrom(node))
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}

	decrypted, ok := result.([]byte)
	if !ok {
		t.Fatalf("expected []byte result, got %T", result)
	}
	if string(decrypted) != "decrypted:encrypted-data" {
		t.Errorf("unexpected content: %q", string(decrypted))
	}
}

// TestDecryptActionNilDecryptor verifies that a nil decryptor slot causes
// the provider to return an error (decryptor is required).
func TestDecryptActionNilDecryptor(t *testing.T) {
	ep := &encryption.Provider{}
	_, err := ep.Decrypt(nil, "secret.txt", []byte("data"))
	if err == nil {
		t.Error("expected error when decryptor is nil")
	}
}

func TestUnlinkAction(t *testing.T) {
	tmpDir := t.TempDir()
	source := filepath.Join(tmpDir, "source.txt")
	target := filepath.Join(tmpDir, "link.txt")

	if err := os.WriteFile(source, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(source, target); err != nil {
		t.Fatal(err)
	}

	p := &file.Provider{}
	action := &filegen.Unlink{Impl: p}
	ctx := &op.Context{Context: context.Background()}
	node := &op.Node{ID: "test"}
	node.SetSlotImmediate("path", target)
	node.SetSlotImmediate("prune", false)
	node.SetSlotImmediate("prune_boundary", "")

	if _, _, err := action.Do(ctx, slotsFrom(node)); err != nil {
		t.Fatalf("unlink: %v", err)
	}

	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Error("expected symlink to be removed")
	}
}

func TestRemoveAction(t *testing.T) {
	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "file.txt")
	if err := os.WriteFile(target, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := &file.Provider{}
	action := &filegen.Remove{Impl: p}
	ctx := &op.Context{Context: context.Background()}
	node := &op.Node{ID: "test"}
	node.SetSlotImmediate("path", target)
	node.SetSlotImmediate("prune", false)
	node.SetSlotImmediate("prune_boundary", "")

	if _, _, err := action.Do(ctx, slotsFrom(node)); err != nil {
		t.Fatalf("remove: %v", err)
	}

	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Error("expected file to be removed")
	}
}

func TestWriteAction(t *testing.T) {
	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "output.txt")
	content := "hello world"

	p := &file.Provider{}
	action := &filegen.WriteText{Impl: p}
	ctx := &op.Context{Context: context.Background()}
	node := &op.Node{ID: "test"}
	node.SetSlotImmediate("destination", target)
	node.SetSlotImmediate("content", content)
	node.SetSlotImmediate("mode", os.FileMode(0o644))

	if _, _, err := action.Do(ctx, slotsFrom(node)); err != nil {
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

	p := &file.Provider{}
	action := &filegen.WriteText{Impl: p}
	ctx := &op.Context{Context: context.Background()}
	node := &op.Node{ID: "test"}
	node.SetSlotImmediate("destination", target)
	node.SetSlotImmediate("content", content)
	node.SetSlotImmediate("mode", os.FileMode(0o644))

	if _, _, err := action.Do(ctx, slotsFrom(node)); err != nil {
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
	p := &file.Provider{}
	_, _, err := p.WriteText("", "/tmp/test.txt", os.FileMode(0o644))
	if err == nil {
		t.Fatal("expected error when content is empty")
	}
	if !strings.Contains(err.Error(), "no content") {
		t.Errorf("expected 'no content' error, got: %v", err)
	}
}

func TestWriteDryRun(t *testing.T) {
	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "should-not-exist.txt")

	p := &file.Provider{}
	action := &filegen.WriteText{Impl: p}
	ctx := &op.Context{Context: context.Background(), DryRun: true, Writer: io.Discard}
	node := &op.Node{ID: "test"}
	node.SetSlotImmediate("destination", target)
	node.SetSlotImmediate("content", "test")
	node.SetSlotImmediate("mode", os.FileMode(0o644))

	if _, _, err := action.Do(ctx, slotsFrom(node)); err != nil {
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
	if err := os.WriteFile(source, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	fp := &file.Provider{}
	engine := execution.NewGraphExecutor(execution.ExecutorOptions{})
	graph := &op.Graph{
		Nodes: []*op.Node{
			testNode(".bashrc", &filegen.Link{Impl: fp}, source, target),
		},
	}

	results, err := runGraph(context.Background(), engine, graph)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != execution.ResultCompleted {
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

	templateContent := []byte("Hello {{.Username}}!")
	if err := os.WriteFile(source, templateContent, 0o644); err != nil {
		t.Fatal(err)
	}

	fp := &file.Provider{}
	tp := &template.Provider{}

	engine := execution.NewGraphExecutor(execution.ExecutorOptions{
		Data: map[string]any{"Username": "david"},
	})

	renderNode := testNode(".greeting:render", &template.Render{Impl: tp}, source, target)
	renderNode.SetSlotImmediate("content", templateContent)
	renderNode.SetSlotImmediate("template_data", map[string]any{"Username": "david"})
	renderNode.SetSlotImmediate("project", "")
	copyNode := testNode(".greeting", &filegen.Copy{Impl: fp}, "", target)
	copyNode.SetSlotImmediate("mode", os.FileMode(0o644))
	// Content flows from render to copy via promise slot
	copyNode.SetSlotPromise("content", ".greeting:render", "")
	graph := &op.Graph{
		Nodes: []*op.Node{renderNode, copyNode},
		Edges: []op.Edge{{From: ".greeting:render", To: ".greeting"}},
	}

	results, err := runGraph(context.Background(), engine, graph)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	// Both nodes should complete
	for _, r := range results {
		if r.Status != execution.ResultCompleted {
			t.Fatalf("node %s: expected completed, got %s (error: %v)", r.NodeID, r.Status, r.Error)
		}
	}

	content, _ := os.ReadFile(target)
	if string(content) != "Hello david!" {
		t.Errorf("expected 'Hello david!', got %q", string(content))
	}
}

func TestEngineRunDecryptRenderCopyPipeline(t *testing.T) {
	tmpDir := t.TempDir()
	source := filepath.Join(tmpDir, "secret.txt.sops")
	target := filepath.Join(tmpDir, "secret.txt")

	encryptedContent := []byte("encrypted:token={{.Token}}")
	if err := os.WriteFile(source, encryptedContent, 0o644); err != nil {
		t.Fatal(err)
	}

	mockDecrypt := func(source string, ciphertext []byte) ([]byte, error) {
		// Strip "encrypted:" prefix
		return []byte(strings.TrimPrefix(string(ciphertext), "encrypted:")), nil
	}

	fp := &file.Provider{}
	ep := &encryption.Provider{}
	tp := &template.Provider{}

	engine := execution.NewGraphExecutor(execution.ExecutorOptions{
		Data: map[string]any{
			"decryptor": mockDecrypt,
			"Token":     "abc123",
		},
	})
	// Chain: decrypt → render → copy
	decryptNode := testNode(".secret:decrypt", &encryption.Decrypt{Impl: ep}, source, "")
	decryptNode.SetSlotImmediate("content", encryptedContent)
	renderNode := testNode(".secret:render", &template.Render{Impl: tp}, source, target)
	renderNode.SetSlotImmediate("template_data", map[string]any{"Token": "abc123"})
	renderNode.SetSlotImmediate("project", "")
	// Content flows from decrypt to render via promise slot
	renderNode.SetSlotPromise("content", ".secret:decrypt", "")
	copyNode := testNode(".secret", &filegen.Copy{Impl: fp}, "", target)
	copyNode.SetSlotImmediate("mode", os.FileMode(0o644))
	// Content flows from render to copy via promise slot
	copyNode.SetSlotPromise("content", ".secret:render", "")

	graph := &op.Graph{
		Nodes: []*op.Node{decryptNode, renderNode, copyNode},
		Edges: []op.Edge{
			{From: ".secret:decrypt", To: ".secret:render"},
			{From: ".secret:render", To: ".secret"},
		},
	}

	results, err := runGraph(context.Background(), engine, graph)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	// All three nodes should complete
	for _, r := range results {
		if r.Status != execution.ResultCompleted {
			t.Fatalf("node %s: expected completed, got %s (error: %v)", r.NodeID, r.Status, r.Error)
		}
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

	if err := os.WriteFile(source1, []byte("file1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source2, []byte("file2"), 0o644); err != nil {
		t.Fatal(err)
	}

	fp := &file.Provider{}
	engine := execution.NewGraphExecutor(execution.ExecutorOptions{})
	graph := &op.Graph{
		Nodes: []*op.Node{
			testNode("tgt1.txt", &filegen.Link{Impl: fp}, source1, target1),
			testNode("sub/tgt2.txt", &filegen.Link{Impl: fp}, source2, target2),
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
		if r.Status != execution.ResultCompleted {
			t.Errorf("node %s: expected completed, got %s", r.NodeID, r.Status)
		}
	}
}

func TestEngineRunUnknownAction(t *testing.T) {
	engine := execution.NewGraphExecutor(execution.ExecutorOptions{})
	graph := &op.Graph{
		Nodes: []*op.Node{
			{ID: "test", Action: op.StubAction("unknown_op")},
		},
	}

	results, _ := runGraph(context.Background(), engine, graph)
	if results[0].Status != execution.ResultFailed {
		t.Errorf("expected failed, got %s", results[0].Status)
	}
}

func TestEngineTopologicalSort(t *testing.T) {
	tmpDir := t.TempDir()

	// Create sources
	srcA := filepath.Join(tmpDir, "a.txt")
	srcB := filepath.Join(tmpDir, "b.txt")
	srcC := filepath.Join(tmpDir, "c.txt")
	if err := os.WriteFile(srcA, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(srcB, []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(srcC, []byte("c"), 0o644); err != nil {
		t.Fatal(err)
	}

	fp := &file.Provider{}
	engine := execution.NewGraphExecutor(execution.ExecutorOptions{})

	// B depends on A, C depends on B
	graph := &op.Graph{
		Nodes: []*op.Node{
			testNode("c", &filegen.Link{Impl: fp}, srcC, filepath.Join(tmpDir, "out_c")),
			testNode("a", &filegen.Link{Impl: fp}, srcA, filepath.Join(tmpDir, "out_a")),
			testNode("b", &filegen.Link{Impl: fp}, srcB, filepath.Join(tmpDir, "out_b")),
		},
		Edges: []op.Edge{
			{From: "a", To: "b"},
			{From: "b", To: "c"},
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
	if err := os.WriteFile(source, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	fp := &file.Provider{}
	engine := execution.NewGraphExecutor(execution.ExecutorOptions{DryRun: true})
	graph := &op.Graph{
		Nodes: []*op.Node{
			testNode(".bashrc", &filegen.Link{Impl: fp}, source, target),
		},
	}

	results, err := runGraph(context.Background(), engine, graph)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	if results[0].Status != execution.ResultCompleted {
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
	if err := os.WriteFile(source, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	graph := &op.Graph{
		Nodes: []*op.Node{
			testNode("test", op.StubAction("file.link"), source, target),
		},
	}

	result := execution.Preflight(graph)
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
	if err := os.WriteFile(source, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("existing"), 0o644); err != nil { // Conflict!
		t.Fatal(err)
	}

	graph := &op.Graph{
		Nodes: []*op.Node{
			testNode("test", op.StubAction("file.link"), source, target),
		},
	}

	result := execution.Preflight(graph)
	if !result.HasConflicts() {
		t.Error("expected conflict")
	}
	if result.Conflicts[0].Type != execution.ConflictRegularFile {
		t.Errorf("expected regular file conflict, got %d", result.Conflicts[0].Type)
	}
}

func TestPreflightAlreadyDeployed(t *testing.T) {
	tmpDir := t.TempDir()
	source := filepath.Join(tmpDir, "source.txt")
	target := filepath.Join(tmpDir, "target.txt")
	if err := os.WriteFile(source, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(source, target); err != nil { // Already correct
		t.Fatal(err)
	}

	graph := &op.Graph{
		Nodes: []*op.Node{
			testNode("test", op.StubAction("file.link"), source, target),
		},
	}

	result := execution.Preflight(graph)
	if result.HasConflicts() {
		t.Error("expected no conflicts for already-deployed symlink")
	}
	if len(result.AlreadyDone) != 1 {
		t.Errorf("expected 1 already-done, got %d", len(result.AlreadyDone))
	}
}

func TestBackupAction(t *testing.T) {
	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "file.txt")
	if err := os.WriteFile(target, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := &file.Provider{}
	action := &filegen.Backup{Impl: p}
	ctx := &op.Context{Context: context.Background()}
	node := &op.Node{ID: "test"}
	node.SetSlotImmediate("path", target)
	node.SetSlotImmediate("backup_suffix", ".writ-backup")

	result, _, err := action.Do(ctx, slotsFrom(node))
	if err != nil {
		t.Fatalf("backup: %v", err)
	}

	// Original should be gone
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Error("expected original file to be moved")
	}

	// Backup path should be returned as Result
	backupPath, ok := result.(string)
	if !ok || backupPath == "" {
		t.Fatal("expected backup path as string result")
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

func TestCopyActionWithMode(t *testing.T) {
	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "script.sh")

	p := &file.Provider{}
	action := &filegen.Copy{Impl: p}
	ctx := &op.Context{Context: context.Background()}
	node := &op.Node{ID: "test"}
	node.SetSlotImmediate("content", []byte("#!/bin/sh\necho hello"))
	node.SetSlotImmediate("path", target)
	node.SetSlotImmediate("mode", os.FileMode(0o755))

	if _, _, err := action.Do(ctx, slotsFrom(node)); err != nil {
		t.Fatalf("copy with mode: %v", err)
	}

	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Errorf("expected mode 0o755, got %04o", info.Mode().Perm())
	}
}

func TestResultStatusString(t *testing.T) {
	tests := []struct {
		status execution.ResultStatus
		want   string
	}{
		{execution.ResultPending, "pending"},
		{execution.ResultRunning, "running"},
		{execution.ResultCompleted, "completed"},
		{execution.ResultFailed, "failed"},
		{execution.ResultSkipped, "skipped"},
	}

	for _, tt := range tests {
		if got := tt.status.String(); got != tt.want {
			t.Errorf("ResultStatus(%d).String() = %q, want %q", tt.status, got, tt.want)
		}
	}
}

func TestRemoveActionPrunesEmptyDirs(t *testing.T) {
	tmpDir := t.TempDir()
	// Create nested structure: tmpDir/a/b/c/file.txt
	nested := filepath.Join(tmpDir, "a", "b", "c")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(nested, "file.txt")
	if err := os.WriteFile(target, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := &file.Provider{}
	action := &filegen.Remove{Impl: p}
	ctx := &op.Context{Context: context.Background()}
	node := &op.Node{ID: "test"}
	node.SetSlotImmediate("path", target)
	node.SetSlotImmediate("prune", true)
	node.SetSlotImmediate("prune_boundary", tmpDir)

	if _, _, err := action.Do(ctx, slotsFrom(node)); err != nil {
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

func TestRemoveActionPruneStopsAtNonEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	// Create nested structure: tmpDir/a/b/file.txt and tmpDir/a/other.txt
	nested := filepath.Join(tmpDir, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(nested, "file.txt")
	other := filepath.Join(tmpDir, "a", "other.txt")
	if err := os.WriteFile(target, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(other, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := &file.Provider{}
	action := &filegen.Remove{Impl: p}
	ctx := &op.Context{Context: context.Background()}
	node := &op.Node{ID: "test"}
	node.SetSlotImmediate("path", target)
	node.SetSlotImmediate("prune", true)
	node.SetSlotImmediate("prune_boundary", tmpDir)

	if _, _, err := action.Do(ctx, slotsFrom(node)); err != nil {
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

func TestUnlinkActionPrunesEmptyDirs(t *testing.T) {
	tmpDir := t.TempDir()
	source := filepath.Join(tmpDir, "source.txt")
	nested := filepath.Join(tmpDir, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(nested, "link.txt")
	if err := os.Symlink(source, target); err != nil {
		t.Fatal(err)
	}

	p := &file.Provider{}
	action := &filegen.Unlink{Impl: p}
	ctx := &op.Context{Context: context.Background()}
	node := &op.Node{ID: "test"}
	node.SetSlotImmediate("path", target)
	node.SetSlotImmediate("prune", true)
	node.SetSlotImmediate("prune_boundary", tmpDir)

	if _, _, err := action.Do(ctx, slotsFrom(node)); err != nil {
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

func TestRequireStringSlot(t *testing.T) {
	t.Run("correct value", func(t *testing.T) {
		node := &op.Node{ID: "test"}
		node.SetSlotImmediate("path", "hello")
		val, err := node.RequireStringSlot("path")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if val != "hello" {
			t.Errorf("expected %q, got %q", "hello", val)
		}
	})

	t.Run("not set", func(t *testing.T) {
		node := &op.Node{ID: "test"}
		_, err := node.RequireStringSlot("path")
		if err == nil {
			t.Fatal("expected error for unset slot")
		}
		if !strings.Contains(err.Error(), "not set") {
			t.Errorf("expected 'not set' in error, got: %v", err)
		}
	})

	t.Run("wrong type", func(t *testing.T) {
		node := &op.Node{ID: "test"}
		node.SetSlotImmediate("count", 42)
		_, err := node.RequireStringSlot("count")
		if err == nil {
			t.Fatal("expected error for wrong type")
		}
		if !strings.Contains(err.Error(), "expected string, got int") {
			t.Errorf("expected 'expected string, got int' in error, got: %v", err)
		}
	})

	t.Run("empty string is valid", func(t *testing.T) {
		node := &op.Node{ID: "test"}
		node.SetSlotImmediate("path", "")
		val, err := node.RequireStringSlot("path")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if val != "" {
			t.Errorf("expected empty string, got %q", val)
		}
	})
}

func TestMoveAction(t *testing.T) {
	tmpDir := t.TempDir()
	source := filepath.Join(tmpDir, "original.txt")
	target := filepath.Join(tmpDir, "moved.txt")

	if err := os.WriteFile(source, []byte("move me"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := &file.Provider{}
	action := &filegen.Move{Impl: p}
	ctx := &op.Context{Context: context.Background()}
	node := &op.Node{ID: "test"}
	node.SetSlotImmediate("source", source)
	node.SetSlotImmediate("path", target)
	node.SetSlotImmediate("git_mv", func(string, string) error { return fmt.Errorf("no git") })

	if _, _, err := action.Do(ctx, slotsFrom(node)); err != nil {
		t.Fatalf("move: %v", err)
	}

	// Source should be gone
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Error("expected source to be gone after move")
	}

	// Target should exist with correct content
	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(content) != "move me" {
		t.Errorf("expected 'move me', got %q", string(content))
	}
}

func TestMoveActionCreatesParentDirs(t *testing.T) {
	tmpDir := t.TempDir()
	source := filepath.Join(tmpDir, "original.txt")
	target := filepath.Join(tmpDir, "deep", "nested", "moved.txt")

	if err := os.WriteFile(source, []byte("nested move"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := &file.Provider{}
	action := &filegen.Move{Impl: p}
	ctx := &op.Context{Context: context.Background()}
	node := &op.Node{ID: "test"}
	node.SetSlotImmediate("source", source)
	node.SetSlotImmediate("path", target)
	node.SetSlotImmediate("git_mv", func(string, string) error { return fmt.Errorf("no git") })

	if _, _, err := action.Do(ctx, slotsFrom(node)); err != nil {
		t.Fatalf("move: %v", err)
	}

	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(content) != "nested move" {
		t.Errorf("expected 'nested move', got %q", string(content))
	}
}

func TestMkdirAction(t *testing.T) {
	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "new", "dir")

	p := &file.Provider{}
	action := &filegen.Mkdir{Impl: p}
	ctx := &op.Context{Context: context.Background()}
	node := &op.Node{ID: "test"}
	node.SetSlotImmediate("path", target)
	node.SetSlotImmediate("mode", os.FileMode(0o750))

	if _, _, err := action.Do(ctx, slotsFrom(node)); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected directory")
	}
	if info.Mode().Perm() != 0o750 {
		t.Errorf("expected mode 0750, got %04o", info.Mode().Perm())
	}
}

func TestMkdirActionDefaultMode(t *testing.T) {
	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "default-mode-dir")

	p := &file.Provider{}
	action := &filegen.Mkdir{Impl: p}
	ctx := &op.Context{Context: context.Background()}
	node := &op.Node{ID: "test"}
	node.SetSlotImmediate("path", target)
	node.SetSlotImmediate("mode", os.FileMode(0o755))

	if _, _, err := action.Do(ctx, slotsFrom(node)); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected directory")
	}
	if info.Mode().Perm() != 0o755 {
		t.Errorf("expected default mode 0o755, got %04o", info.Mode().Perm())
	}
}

func TestSourceAction(t *testing.T) {
	tmpDir := t.TempDir()
	source := filepath.Join(tmpDir, "data.txt")
	content := "source file content"

	if err := os.WriteFile(source, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	p := &file.Provider{}
	action := &filegen.Read{Impl: p}
	ctx := &op.Context{Context: context.Background()}
	node := &op.Node{ID: "test"}
	node.SetSlotImmediate("path", source)

	result, _, err := action.Do(ctx, slotsFrom(node))
	if err != nil {
		t.Fatalf("source: %v", err)
	}

	data, ok := result.([]byte)
	if !ok {
		t.Fatalf("expected []byte result, got %T", result)
	}
	if string(data) != content {
		t.Errorf("expected %q, got %q", content, string(data))
	}
}

func TestRemoveNoPruneWithoutFlag(t *testing.T) {
	tmpDir := t.TempDir()
	nested := filepath.Join(tmpDir, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(nested, "file.txt")
	if err := os.WriteFile(target, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := &file.Provider{}
	action := &filegen.Remove{Impl: p}
	ctx := &op.Context{Context: context.Background()}
	node := &op.Node{ID: "test"}
	node.SetSlotImmediate("path", target)
	node.SetSlotImmediate("prune", false)
	node.SetSlotImmediate("prune_boundary", "")

	if _, _, err := action.Do(ctx, slotsFrom(node)); err != nil {
		t.Fatalf("remove: %v", err)
	}

	// File should be gone
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Error("expected file to be removed")
	}

	// Parent dirs should remain (prune=false)
	if _, err := os.Stat(nested); err != nil {
		t.Error("expected b/ to remain (prune=false)")
	}
}
