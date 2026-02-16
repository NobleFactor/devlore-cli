// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package execution_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/NobleFactor/devlore-cli/internal/execution"
	"github.com/NobleFactor/devlore-cli/internal/execution/provider"
	"github.com/NobleFactor/devlore-cli/internal/execution/provider/file"
	"github.com/NobleFactor/devlore-cli/internal/execution/provider/template"

	"gopkg.in/yaml.v3"
)

// buildTestGraph creates a simple graph programmatically for lifecycle tests.
func buildTestGraph() *execution.Graph {
	fp := &file.Provider{}

	nodeA := &execution.Node{
		ID:     "a",
		Action: &file.Link{Impl: fp},
		Status: execution.StatusPending,
	}
	nodeA.SetSlotImmediate("source", "/src/a.txt")
	nodeA.SetSlotImmediate("path", "/dst/a.txt")

	nodeB := &execution.Node{
		ID:     "b",
		Action: &file.Copy{Impl: fp},
		Status: execution.StatusPending,
	}
	nodeB.SetSlotImmediate("source", "/src/b.txt")
	nodeB.SetSlotImmediate("path", "/dst/b.txt")

	return &execution.Graph{
		Version:   "1",
		Tool:      "writ",
		Timestamp: time.Date(2026, 2, 16, 12, 0, 0, 0, time.UTC),
		State:     execution.StatePending,
		Platform:  execution.Platform{OS: "darwin", Arch: "arm64"},
		Context: execution.GraphContext{
			SourceRoot: "/src",
			TargetRoot: "/dst",
		},
		Nodes: []*execution.Node{nodeA, nodeB},
		Edges: []execution.Edge{{From: "a", To: "b"}},
	}
}

func TestGraphBuildFromGo(t *testing.T) {
	g := buildTestGraph()

	if len(g.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(g.Nodes))
	}
	if len(g.Edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(g.Edges))
	}
	if g.Nodes[0].ActionName() != "file.link" {
		t.Errorf("expected node a action 'file.link', got %q", g.Nodes[0].ActionName())
	}
	if g.Nodes[1].ActionName() != "file.copy" {
		t.Errorf("expected node b action 'file.copy', got %q", g.Nodes[1].ActionName())
	}
	if g.Edges[0].From != "a" || g.Edges[0].To != "b" {
		t.Errorf("expected edge a→b, got %s→%s", g.Edges[0].From, g.Edges[0].To)
	}

	// Verify slot values
	if src := g.Nodes[0].GetSlot("source"); src != "/src/a.txt" {
		t.Errorf("expected source '/src/a.txt', got %v", src)
	}
}

func TestGraphSerializeYAML(t *testing.T) {
	g := buildTestGraph()

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := g.Serialize(enc); err != nil {
		t.Fatalf("serialize YAML: %v", err)
	}
	enc.Close()

	data := buf.String()

	// Verify key fields appear in YAML output
	if !bytes.Contains(buf.Bytes(), []byte("file.link")) {
		t.Error("expected 'file.link' in YAML output")
	}
	if !bytes.Contains(buf.Bytes(), []byte("file.copy")) {
		t.Error("expected 'file.copy' in YAML output")
	}
	if !bytes.Contains(buf.Bytes(), []byte("/src/a.txt")) {
		t.Errorf("expected source path in YAML output, got:\n%s", data)
	}
}

func TestGraphSerializeJSON(t *testing.T) {
	g := buildTestGraph()

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := g.Serialize(enc); err != nil {
		t.Fatalf("serialize JSON: %v", err)
	}

	data := buf.String()

	if !bytes.Contains(buf.Bytes(), []byte("file.link")) {
		t.Error("expected 'file.link' in JSON output")
	}
	if !bytes.Contains(buf.Bytes(), []byte("file.copy")) {
		t.Error("expected 'file.copy' in JSON output")
	}
	if !bytes.Contains(buf.Bytes(), []byte("/src/a.txt")) {
		t.Errorf("expected source path in JSON output, got:\n%s", data)
	}
}

func TestGraphDeserializeYAML(t *testing.T) {
	g := buildTestGraph()

	// Serialize
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := g.Serialize(enc); err != nil {
		t.Fatalf("serialize: %v", err)
	}
	enc.Close()

	// Deserialize
	var loaded execution.Graph
	if err := yaml.Unmarshal(buf.Bytes(), &loaded); err != nil {
		t.Fatalf("deserialize YAML: %v", err)
	}

	if len(loaded.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(loaded.Nodes))
	}

	// Deserialized nodes should have stubActions
	for _, n := range loaded.Nodes {
		if n.ActionName() == "" {
			t.Errorf("node %s: expected non-empty action name after deserialization", n.ID)
		}
	}
	if loaded.Nodes[0].ActionName() != "file.link" {
		t.Errorf("expected 'file.link', got %q", loaded.Nodes[0].ActionName())
	}
	if loaded.Nodes[1].ActionName() != "file.copy" {
		t.Errorf("expected 'file.copy', got %q", loaded.Nodes[1].ActionName())
	}

	// Verify edges preserved
	if len(loaded.Edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(loaded.Edges))
	}
	if loaded.Edges[0].From != "a" || loaded.Edges[0].To != "b" {
		t.Errorf("expected edge a→b, got %s→%s", loaded.Edges[0].From, loaded.Edges[0].To)
	}

	// Verify stub actions are not executable
	ctx := &execution.Context{Context: context.Background()}
	_, _, err := loaded.Nodes[0].Action.Do(ctx, nil)
	if err == nil {
		t.Error("expected stub action to return error from Do()")
	}
}

func TestGraphDeserializeJSON(t *testing.T) {
	g := buildTestGraph()

	// Serialize
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	if err := g.Serialize(enc); err != nil {
		t.Fatalf("serialize: %v", err)
	}

	// Deserialize
	var loaded execution.Graph
	if err := json.Unmarshal(buf.Bytes(), &loaded); err != nil {
		t.Fatalf("deserialize JSON: %v", err)
	}

	if len(loaded.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(loaded.Nodes))
	}
	if loaded.Nodes[0].ActionName() != "file.link" {
		t.Errorf("expected 'file.link', got %q", loaded.Nodes[0].ActionName())
	}
	if loaded.Nodes[1].ActionName() != "file.copy" {
		t.Errorf("expected 'file.copy', got %q", loaded.Nodes[1].ActionName())
	}
}

func TestGraphRoundTripYAML(t *testing.T) {
	g := buildTestGraph()

	// Serialize
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := g.Serialize(enc); err != nil {
		t.Fatalf("serialize: %v", err)
	}
	enc.Close()

	// Deserialize
	var loaded execution.Graph
	if err := yaml.Unmarshal(buf.Bytes(), &loaded); err != nil {
		t.Fatalf("deserialize: %v", err)
	}

	// Compare structure
	if len(loaded.Nodes) != len(g.Nodes) {
		t.Fatalf("node count: expected %d, got %d", len(g.Nodes), len(loaded.Nodes))
	}
	for i, n := range loaded.Nodes {
		if n.ActionName() != g.Nodes[i].ActionName() {
			t.Errorf("node %d: expected action %q, got %q", i, g.Nodes[i].ActionName(), n.ActionName())
		}
		if n.ID != g.Nodes[i].ID {
			t.Errorf("node %d: expected ID %q, got %q", i, g.Nodes[i].ID, n.ID)
		}
	}

	if len(loaded.Edges) != len(g.Edges) {
		t.Fatalf("edge count: expected %d, got %d", len(g.Edges), len(loaded.Edges))
	}
	if loaded.Version != g.Version {
		t.Errorf("version: expected %q, got %q", g.Version, loaded.Version)
	}
	if loaded.Tool != g.Tool {
		t.Errorf("tool: expected %q, got %q", g.Tool, loaded.Tool)
	}
}

func TestGraphRoundTripJSON(t *testing.T) {
	g := buildTestGraph()

	// Serialize
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	if err := g.Serialize(enc); err != nil {
		t.Fatalf("serialize: %v", err)
	}

	// Deserialize
	var loaded execution.Graph
	if err := json.Unmarshal(buf.Bytes(), &loaded); err != nil {
		t.Fatalf("deserialize: %v", err)
	}

	// Compare structure
	if len(loaded.Nodes) != len(g.Nodes) {
		t.Fatalf("node count: expected %d, got %d", len(g.Nodes), len(loaded.Nodes))
	}
	for i, n := range loaded.Nodes {
		if n.ActionName() != g.Nodes[i].ActionName() {
			t.Errorf("node %d: expected action %q, got %q", i, g.Nodes[i].ActionName(), n.ActionName())
		}
	}
	if len(loaded.Edges) != len(g.Edges) {
		t.Fatalf("edge count: expected %d, got %d", len(g.Edges), len(loaded.Edges))
	}
}

func TestGraphHydrate(t *testing.T) {
	g := buildTestGraph()

	// Serialize to YAML
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := g.Serialize(enc); err != nil {
		t.Fatalf("serialize: %v", err)
	}
	enc.Close()

	// Deserialize
	var loaded execution.Graph
	if err := yaml.Unmarshal(buf.Bytes(), &loaded); err != nil {
		t.Fatalf("deserialize: %v", err)
	}

	// Verify stub before hydration
	ctx := &execution.Context{Context: context.Background()}
	_, _, err := loaded.Nodes[0].Action.Do(ctx, nil)
	if err == nil {
		t.Error("expected stub action to fail before hydration")
	}

	// Hydrate with real registry
	reg := execution.NewActionRegistry()
	provider.RegisterAll(reg)

	if err := loaded.Hydrate(reg); err != nil {
		t.Fatalf("hydrate: %v", err)
	}

	// After hydration, actions should be real
	if loaded.Nodes[0].ActionName() != "file.link" {
		t.Errorf("expected 'file.link' after hydration, got %q", loaded.Nodes[0].ActionName())
	}
	if loaded.Nodes[1].ActionName() != "file.copy" {
		t.Errorf("expected 'file.copy' after hydration, got %q", loaded.Nodes[1].ActionName())
	}

	// Actions should not be stubs — test by checking that they don't return the stub error
	// (We can't call Do without proper slots, but we can verify the action type changed)
	_, _, err = loaded.Nodes[0].Action.Do(&execution.Context{Context: context.Background(), DryRun: true, Logger: os.Stdout},
		map[string]any{"source": "/x", "path": "/y"})
	if err != nil {
		t.Errorf("expected hydrated action to succeed in dry-run, got: %v", err)
	}
}

func TestGraphHydrateUnknownAction(t *testing.T) {
	g := &execution.Graph{
		Nodes: []*execution.Node{
			{ID: "test", Action: execution.StubAction("nonexistent.action")},
		},
	}

	reg := execution.NewActionRegistry()
	provider.RegisterAll(reg)

	err := g.Hydrate(reg)
	if err == nil {
		t.Fatal("expected error for unknown action during hydration")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("unknown action")) {
		t.Errorf("expected 'unknown action' in error, got: %v", err)
	}
	if !bytes.Contains([]byte(err.Error()), []byte("nonexistent.action")) {
		t.Errorf("expected action name in error, got: %v", err)
	}
}

func TestGraphLifecycle(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	dstDir := filepath.Join(tmpDir, "dst")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create source files
	srcLink := filepath.Join(srcDir, "config.txt")
	srcCopy := filepath.Join(srcDir, "data.txt")
	if err := os.WriteFile(srcLink, []byte("link content"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(srcCopy, []byte("copy content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Build graph
	fp := &file.Provider{}
	linkNode := &execution.Node{
		ID:     "config.txt",
		Action: &file.Link{Impl: fp},
		Status: execution.StatusPending,
	}
	linkNode.SetSlotImmediate("source", srcLink)
	linkNode.SetSlotImmediate("path", filepath.Join(dstDir, "config.txt"))

	copyNode := &execution.Node{
		ID:     "data.txt",
		Action: &file.Copy{Impl: fp},
		Status: execution.StatusPending,
	}
	copyNode.SetSlotImmediate("source", srcCopy)
	copyNode.SetSlotImmediate("path", filepath.Join(dstDir, "data.txt"))

	graph := &execution.Graph{
		Version:   "1",
		Tool:      "writ",
		Timestamp: time.Date(2026, 2, 16, 12, 0, 0, 0, time.UTC),
		State:     execution.StatePending,
		Platform:  execution.Platform{OS: "darwin", Arch: "arm64"},
		Context: execution.GraphContext{
			SourceRoot: srcDir,
			TargetRoot: dstDir,
		},
		Nodes: []*execution.Node{linkNode, copyNode},
	}

	// Step 1: Serialize to YAML
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := graph.Serialize(enc); err != nil {
		t.Fatalf("serialize: %v", err)
	}
	enc.Close()

	// Step 2: Deserialize from YAML
	var loaded execution.Graph
	if err := yaml.Unmarshal(buf.Bytes(), &loaded); err != nil {
		t.Fatalf("deserialize: %v", err)
	}

	// Step 3: Hydrate with real actions
	reg := execution.NewActionRegistry()
	provider.RegisterAll(reg)
	if err := loaded.Hydrate(reg); err != nil {
		t.Fatalf("hydrate: %v", err)
	}

	// Reset state to pending for execution
	loaded.State = execution.StatePending
	for _, n := range loaded.Nodes {
		n.Status = execution.StatusPending
	}

	// Step 4: Run the graph
	engine := execution.NewGraphExecutor(execution.ExecutorOptions{})
	if err := engine.Run(context.Background(), &loaded); err != nil {
		t.Fatalf("run: %v", err)
	}

	// Verify state updated
	if loaded.State != execution.StateExecuted {
		t.Errorf("expected state 'executed', got %q", loaded.State)
	}

	// Verify files created
	linkTarget, err := os.Readlink(filepath.Join(dstDir, "config.txt"))
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if linkTarget != srcLink {
		t.Errorf("expected symlink to %s, got %s", srcLink, linkTarget)
	}

	data, err := os.ReadFile(filepath.Join(dstDir, "data.txt"))
	if err != nil {
		t.Fatalf("read copied: %v", err)
	}
	if string(data) != "copy content" {
		t.Errorf("expected 'copy content', got %q", string(data))
	}
}

func TestGraphLifecycleWithPipeline(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	dstDir := filepath.Join(tmpDir, "dst")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create template source
	tmplContent := []byte("Hello {{.Username}}!")
	tmplPath := filepath.Join(srcDir, "greeting.tmpl")
	if err := os.WriteFile(tmplPath, tmplContent, 0644); err != nil {
		t.Fatal(err)
	}

	// Build graph: source → render → copy (pipeline with promise slots)
	fp := &file.Provider{}
	tp := &template.Provider{}

	sourceNode := &execution.Node{
		ID:     "greeting:source",
		Action: &file.Source{Impl: fp},
		Status: execution.StatusPending,
	}
	sourceNode.SetSlotImmediate("path", tmplPath)

	renderNode := &execution.Node{
		ID:     "greeting:render",
		Action: &template.Render{Impl: tp},
		Status: execution.StatusPending,
	}
	renderNode.SetSlotImmediate("source", tmplPath)
	// Content flows from source → render via promise slot
	renderNode.SetSlotPromise("content", "greeting:source", "")

	copyNode := &execution.Node{
		ID:     "greeting",
		Action: &file.Copy{Impl: fp},
		Status: execution.StatusPending,
	}
	copyNode.SetSlotImmediate("path", filepath.Join(dstDir, "greeting.txt"))
	// Content flows from render → copy via promise slot
	copyNode.SetSlotPromise("content", "greeting:render", "")

	graph := &execution.Graph{
		Version:   "1",
		Tool:      "writ",
		Timestamp: time.Date(2026, 2, 16, 12, 0, 0, 0, time.UTC),
		State:     execution.StatePending,
		Platform:  execution.Platform{OS: "darwin", Arch: "arm64"},
		Context: execution.GraphContext{
			SourceRoot: srcDir,
			TargetRoot: dstDir,
		},
		Nodes: []*execution.Node{sourceNode, renderNode, copyNode},
		Edges: []execution.Edge{
			{From: "greeting:source", To: "greeting:render"},
			{From: "greeting:render", To: "greeting"},
		},
	}

	// Serialize → Deserialize → Hydrate
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := graph.Serialize(enc); err != nil {
		t.Fatalf("serialize: %v", err)
	}
	enc.Close()

	var loaded execution.Graph
	if err := yaml.Unmarshal(buf.Bytes(), &loaded); err != nil {
		t.Fatalf("deserialize: %v", err)
	}

	reg := execution.NewActionRegistry()
	provider.RegisterAll(reg)
	if err := loaded.Hydrate(reg); err != nil {
		t.Fatalf("hydrate: %v", err)
	}

	// Reset state
	loaded.State = execution.StatePending
	for _, n := range loaded.Nodes {
		n.Status = execution.StatusPending
	}

	// Run with template data
	engine := execution.NewGraphExecutor(execution.ExecutorOptions{
		Data: map[string]any{"Username": "david"},
	})
	if err := engine.Run(context.Background(), &loaded); err != nil {
		t.Fatalf("run: %v", err)
	}

	// Verify rendered output
	result, err := os.ReadFile(filepath.Join(dstDir, "greeting.txt"))
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if string(result) != "Hello david!" {
		t.Errorf("expected 'Hello david!', got %q", string(result))
	}
}
