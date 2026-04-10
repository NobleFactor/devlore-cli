// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

		"github.com/NobleFactor/devlore-cli/pkg/op"

	_ "github.com/NobleFactor/devlore-cli/pkg/op/inventory"

	"gopkg.in/yaml.v3"
)

// buildTestGraph creates a simple graph programmatically for lifecycle tests.
func buildTestGraph() *op.Graph {

	nodeA := &op.Node{
		ID:       "a",
		Receiver: "file.link",
		Status:   op.StatusPending,
	}
	nodeA.SetSlotImmediate("source", "/src/a.txt")
	nodeA.SetSlotImmediate("path", "/dst/a.txt")

	nodeB := &op.Node{
		ID:       "b",
		Receiver: "file.copy",
		Status:   op.StatusPending,
	}
	nodeB.SetSlotImmediate("source", "/src/b.txt")
	nodeB.SetSlotImmediate("path", "/dst/b.txt")

	return &op.Graph{
		Version:   "1",
		Timestamp: time.Date(2026, 2, 16, 12, 0, 0, 0, time.UTC),
		State:     op.StatePending,
		Provenance: op.Provenance{
			SourceRoot: "/src",
			TargetRoot: "/dst",
		},
		Children: []op.SubgraphChild{
			{Node: nodeA},
			{Node: nodeB},
		},
		Edges: []op.Edge{{From: "a", To: "b"}},
	}
}

func TestGraphBuildFromGo(t *testing.T) {
	g := buildTestGraph()

	nodes := g.Nodes()
	if len(nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(nodes))
	}
	if len(g.Edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(g.Edges))
	}
	if nodes[0].Receiver != "file.link" {
		t.Errorf("expected node a receiver 'file.link', got %q", nodes[0].Receiver)
	}
	if nodes[1].Receiver != "file.copy" {
		t.Errorf("expected node b receiver 'file.copy', got %q", nodes[1].Receiver)
	}
	if g.Edges[0].From != "a" || g.Edges[0].To != "b" {
		t.Errorf("expected edge a→b, got %s→%s", g.Edges[0].From, g.Edges[0].To)
	}

	// Verify slot values.
	if src := nodes[0].SlotByName("source"); src != "/src/a.txt" {
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

	// Serialize.
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := g.Serialize(enc); err != nil {
		t.Fatalf("serialize: %v", err)
	}
	enc.Close()

	// Deserialize.
	var loaded op.Graph
	if err := yaml.Unmarshal(buf.Bytes(), &loaded); err != nil {
		t.Fatalf("deserialize YAML: %v", err)
	}

	nodes := loaded.Nodes()
	if len(nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(nodes))
	}
	if nodes[0].Receiver != "file.link" {
		t.Errorf("expected 'file.link', got %q", nodes[0].Receiver)
	}
	if nodes[1].Receiver != "file.copy" {
		t.Errorf("expected 'file.copy', got %q", nodes[1].Receiver)
	}

	// Verify edges preserved.
	if len(loaded.Edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(loaded.Edges))
	}
	if loaded.Edges[0].From != "a" || loaded.Edges[0].To != "b" {
		t.Errorf("expected edge a→b, got %s→%s", loaded.Edges[0].From, loaded.Edges[0].To)
	}
}

func TestGraphDeserializeJSON(t *testing.T) {
	g := buildTestGraph()

	// Serialize.
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	if err := g.Serialize(enc); err != nil {
		t.Fatalf("serialize: %v", err)
	}

	// Deserialize.
	var loaded op.Graph
	if err := json.Unmarshal(buf.Bytes(), &loaded); err != nil {
		t.Fatalf("deserialize JSON: %v", err)
	}

	nodes := loaded.Nodes()
	if len(nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(nodes))
	}
	if nodes[0].Receiver != "file.link" {
		t.Errorf("expected 'file.link', got %q", nodes[0].Receiver)
	}
	if nodes[1].Receiver != "file.copy" {
		t.Errorf("expected 'file.copy', got %q", nodes[1].Receiver)
	}
}

func TestGraphRoundTripYAML(t *testing.T) {
	g := buildTestGraph()

	// Serialize.
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := g.Serialize(enc); err != nil {
		t.Fatalf("serialize: %v", err)
	}
	enc.Close()

	// Deserialize.
	var loaded op.Graph
	if err := yaml.Unmarshal(buf.Bytes(), &loaded); err != nil {
		t.Fatalf("deserialize: %v", err)
	}

	// Compare structure.
	origNodes := g.Nodes()
	loadedNodes := loaded.Nodes()
	if len(loadedNodes) != len(origNodes) {
		t.Fatalf("node count: expected %d, got %d", len(origNodes), len(loadedNodes))
	}
	for i, n := range loadedNodes {
		if n.Receiver != origNodes[i].Receiver {
			t.Errorf("node %d: expected receiver %q, got %q", i, origNodes[i].Receiver, n.Receiver)
		}
		if n.ID != origNodes[i].ID {
			t.Errorf("node %d: expected ID %q, got %q", i, origNodes[i].ID, n.ID)
		}
	}

	if len(loaded.Edges) != len(g.Edges) {
		t.Fatalf("edge count: expected %d, got %d", len(g.Edges), len(loaded.Edges))
	}
	if loaded.Version != g.Version {
		t.Errorf("version: expected %q, got %q", g.Version, loaded.Version)
	}
	if loaded.Version != g.Version {
		t.Errorf("version: expected %q, got %q", g.Version, loaded.Version)
	}
}

func TestGraphRoundTripJSON(t *testing.T) {
	g := buildTestGraph()

	// Serialize.
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	if err := g.Serialize(enc); err != nil {
		t.Fatalf("serialize: %v", err)
	}

	// Deserialize.
	var loaded op.Graph
	if err := json.Unmarshal(buf.Bytes(), &loaded); err != nil {
		t.Fatalf("deserialize: %v", err)
	}

	// Compare structure.
	origNodes := g.Nodes()
	loadedNodes := loaded.Nodes()
	if len(loadedNodes) != len(origNodes) {
		t.Fatalf("node count: expected %d, got %d", len(origNodes), len(loadedNodes))
	}
	for i, n := range loadedNodes {
		if n.Receiver != origNodes[i].Receiver {
			t.Errorf("node %d: expected receiver %q, got %q", i, origNodes[i].Receiver, n.Receiver)
		}
	}
	if len(loaded.Edges) != len(g.Edges) {
		t.Fatalf("edge count: expected %d, got %d", len(g.Edges), len(loaded.Edges))
	}
}

func TestGraphLifecycle(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	dstDir := filepath.Join(tmpDir, "dst")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create source files.
	srcLink := filepath.Join(srcDir, "config.txt")
	srcCopy := filepath.Join(srcDir, "data.txt")
	if err := os.WriteFile(srcLink, []byte("link content"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(srcCopy, []byte("copy content"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Build graph.
	linkNode := &op.Node{
		ID:       "config.txt",
		Receiver: "file.link",
		Status:   op.StatusPending,
	}
	linkNode.SetSlotImmediate("source", srcLink)
	linkNode.SetSlotImmediate("path", filepath.Join(dstDir, "config.txt"))

	copyContent, err := os.ReadFile(srcCopy)
	if err != nil {
		t.Fatal(err)
	}

	copyNode := &op.Node{
		ID:       "data.txt",
		Receiver: "file.copy",
		Status:   op.StatusPending,
	}
	copyNode.SetSlotImmediate("content", copyContent)
	copyNode.SetSlotImmediate("path", filepath.Join(dstDir, "data.txt"))
	copyNode.SetSlotImmediate("mode", os.FileMode(0o644))

	graph := &op.Graph{
		Version:   "1",
		Timestamp: time.Date(2026, 2, 16, 12, 0, 0, 0, time.UTC),
		State:     op.StatePending,
		Provenance: op.Provenance{
			SourceRoot: srcDir,
			TargetRoot: dstDir,
		},
		Children: []op.SubgraphChild{
			{Node: linkNode},
			{Node: copyNode},
		},
	}

	// Step 1: Serialize to YAML.
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := graph.Serialize(enc); err != nil {
		t.Fatalf("serialize: %v", err)
	}
	enc.Close()

	// Step 2: Deserialize from YAML.
	var loaded op.Graph
	if err := yaml.Unmarshal(buf.Bytes(), &loaded); err != nil {
		t.Fatalf("deserialize: %v", err)
	}

	// Step 3: Reset state and re-set typed slots that don't survive YAML round-trip.
	loaded.State = op.StatePending
	for _, n := range loaded.Nodes() {
		n.Status = op.StatusPending
		switch n.Receiver {
		case "file.copy":
			n.SetSlotImmediate("mode", os.FileMode(0o644))
			n.SetSlotImmediate("content", copyContent)
		}
	}

	// Step 4: Run the graph.
	engine, engErr := op.NewGraphExecutor("test", op.Options{Root: tmpDir})
	if engErr != nil {
		t.Fatalf("NewGraphExecutor: %v", engErr)
	}
	if _, err := engine.Run(&loaded); err != nil {
		t.Fatalf("run: %v", err)
	}

	// Verify state updated.
	if loaded.State != op.StateExecuted {
		t.Errorf("expected state 'executed', got %q", loaded.State)
	}

	// Verify files created.
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
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create template source.
	tmplContent := []byte("Hello {{.Username}}!")
	tmplPath := filepath.Join(srcDir, "greeting.tmpl")
	if err := os.WriteFile(tmplPath, tmplContent, 0o644); err != nil {
		t.Fatal(err)
	}

	// Build graph: source → render → copy (pipeline with promise slots).
	sourceNode := &op.Node{
		ID:       "greeting:source",
		Receiver: "file.read_text",
		Status:   op.StatusPending,
	}
	sourceNode.SetSlotImmediate("path", tmplPath)

	dstPath := filepath.Join(dstDir, "greeting.txt")

	renderNode := &op.Node{
		ID:       "greeting:render",
		Receiver: "template.render_text",
		Status:   op.StatusPending,
	}
	renderNode.SetSlotImmediate("source", tmplPath)
	renderNode.SetSlotImmediate("path", dstPath)
	renderNode.SetSlotImmediate("project", "")
	renderNode.SetSlotImmediate("template_data", map[string]any{"Username": "david"})
	renderNode.SetSlotPromise("content", "greeting:source", "")

	copyNode := &op.Node{
		ID:       "greeting",
		Receiver: "file.copy",
		Status:   op.StatusPending,
	}
	copyNode.SetSlotImmediate("path", dstPath)
	copyNode.SetSlotImmediate("mode", os.FileMode(0o644))
	copyNode.SetSlotPromise("content", "greeting:render", "")

	graph := &op.Graph{
		Version:   "1",
		Timestamp: time.Date(2026, 2, 16, 12, 0, 0, 0, time.UTC),
		State:     op.StatePending,
		Provenance: op.Provenance{
			SourceRoot: srcDir,
			TargetRoot: dstDir,
		},
		Children: []op.SubgraphChild{
			{Node: sourceNode},
			{Node: renderNode},
			{Node: copyNode},
		},
		Edges: []op.Edge{
			{From: "greeting:source", To: "greeting:render"},
			{From: "greeting:render", To: "greeting"},
		},
	}

	// Serialize → Deserialize.
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := graph.Serialize(enc); err != nil {
		t.Fatalf("serialize: %v", err)
	}
	enc.Close()

	var loaded op.Graph
	if err := yaml.Unmarshal(buf.Bytes(), &loaded); err != nil {
		t.Fatalf("deserialize: %v", err)
	}

	// Reset state and re-set typed slots that don't survive YAML round-trip.
	loaded.State = op.StatePending
	for _, n := range loaded.Nodes() {
		n.Status = op.StatusPending
		switch n.Receiver {
		case "template.render_text":
			n.SetSlotImmediate("template_data", map[string]any{"Username": "david"})
		case "file.copy":
			n.SetSlotImmediate("mode", os.FileMode(0o644))
		}
	}

	// Run.
	engine, engErr := op.NewGraphExecutor("test", op.Options{Root: tmpDir})
	if engErr != nil {
		t.Fatalf("NewGraphExecutor: %v", engErr)
	}
	if _, err := engine.Run(&loaded); err != nil {
		t.Fatalf("run: %v", err)
	}

	// Verify rendered output.
	result, err := os.ReadFile(filepath.Join(dstDir, "greeting.txt"))
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if string(result) != "Hello david!" {
		t.Errorf("expected 'Hello david!', got %q", string(result))
	}
}
