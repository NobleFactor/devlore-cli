// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package execution_test

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/NobleFactor/devlore-cli/internal/execution"
	"github.com/NobleFactor/devlore-cli/internal/execution/flow"
	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/file"
	filegen "github.com/NobleFactor/devlore-cli/pkg/op/provider/file/gen"
)

// --- Test helpers ---

// fileAction returns a reflected action from the file provider registry.
func fileAction(t *testing.T, p *file.Provider, name string) op.Action {
	t.Helper()
	reg := op.NewActionRegistry()
	op.RegisterReflectedActions(reg, "file", p, filegen.Params)
	a, ok := reg.Get(name)
	if !ok {
		t.Fatalf("action %q not registered", name)
	}
	return a
}

// failAction always returns error from Do. Action-only (no Undo).
type failAction struct{}

func (a *failAction) Name() string { return "test.fail" }
func (a *failAction) Do(_ *op.Context, _ map[string]any) (result op.Result, undo op.UndoState, err error) {
	return nil, nil, fmt.Errorf("deliberate failure")
}

// trackAction records Undo calls for ordering verification.
type trackAction struct {
	label string
	mu    *sync.Mutex
	log   *[]string
}

func (a *trackAction) Name() string { return "test.track." + a.label }
func (a *trackAction) Do(_ *op.Context, _ map[string]any) (result op.Result, undo op.UndoState, err error) {
	return nil, a.label, nil
}
func (a *trackAction) Undo(_ *op.Context, state op.UndoState) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	*a.log = append(*a.log, state.(string))
	return nil
}

// noopAction returns nil UndoState. Action-only (no compensation required).
type noopAction struct{}

func (a *noopAction) Name() string { return "test.noop" }
func (a *noopAction) Do(_ *op.Context, _ map[string]any) (result op.Result, undo op.UndoState, err error) {
	return nil, nil, nil
}

// conditionalFailAction fails when the "path" slot matches failPath.
type conditionalFailAction struct {
	failPath string
}

func (a *conditionalFailAction) Name() string { return "test.conditional_fail" }
func (a *conditionalFailAction) Do(_ *op.Context, slots map[string]any) (result op.Result, undo op.UndoState, err error) {
	path, _ := slots["path"].(string)
	if path == a.failPath {
		return nil, nil, fmt.Errorf("deliberate failure on %s", path)
	}
	return nil, nil, nil
}

// phasedGraph builds a single-phase graph with linear edges between nodes.
func phasedGraph(nodes []*op.Node) *op.Graph {
	ids := make([]string, len(nodes))
	var edges []op.Edge
	for i, n := range nodes {
		ids[i] = n.ID
		if i > 0 {
			edges = append(edges, op.Edge{From: nodes[i-1].ID, To: n.ID})
		}
	}
	return &op.Graph{
		State: op.StatePending,
		Nodes: nodes,
		Edges: edges,
		Phases: []*op.Phase{{
			ID:      "phase.test",
			Name:    "test",
			Status:  op.PhasePending,
			NodeIDs: ids,
		}},
	}
}

// --- Tests ---

// TestCompensationFileActions verifies that completed file actions (write,
// link) are fully compensated when a subsequent action fails.
func TestCompensationFileActions(t *testing.T) {
	tmpDir := t.TempDir()
	fp := &file.Provider{}

	writePath := filepath.Join(tmpDir, "write.txt")
	linkSource := filepath.Join(tmpDir, "source.txt")
	linkPath := filepath.Join(tmpDir, "link.txt")

	if err := os.WriteFile(linkSource, []byte("source content"), 0o644); err != nil {
		t.Fatal(err)
	}

	writeNode := &op.Node{ID: "write", Action: fileAction(t, fp, "file.write_text")}
	writeNode.SetSlotImmediate("content", "hello")
	writeNode.SetSlotImmediate("destination", writePath)
	writeNode.SetSlotImmediate("mode", os.FileMode(0o644))

	linkNode := &op.Node{ID: "link", Action: fileAction(t, fp, "file.link")}
	linkNode.SetSlotImmediate("source", linkSource)
	linkNode.SetSlotImmediate("path", linkPath)

	failNode := &op.Node{ID: "fail", Action: &failAction{}}

	g := phasedGraph([]*op.Node{writeNode, linkNode, failNode})
	executor := execution.NewGraphExecutor(execution.ExecutorOptions{Writer: io.Discard})

	err := executor.Run(context.Background(), g)
	if err == nil {
		t.Fatal("expected error from deliberate failure")
	}

	for _, p := range []string{writePath, linkPath} {
		if _, statErr := os.Stat(p); statErr == nil {
			t.Errorf("%s should not exist after compensation", filepath.Base(p))
		}
	}

	if _, err := os.Stat(linkSource); err != nil {
		t.Error("link source should still exist")
	}
}

// TestCompensationOrdering verifies that compensation runs in LIFO order
// (last completed action is compensated first).
func TestCompensationOrdering(t *testing.T) {
	var mu sync.Mutex
	var log []string

	nodeA := &op.Node{ID: "a", Action: &trackAction{label: "A", mu: &mu, log: &log}}
	nodeB := &op.Node{ID: "b", Action: &trackAction{label: "B", mu: &mu, log: &log}}
	nodeC := &op.Node{ID: "c", Action: &trackAction{label: "C", mu: &mu, log: &log}}
	nodeFail := &op.Node{ID: "fail", Action: &failAction{}}

	g := phasedGraph([]*op.Node{nodeA, nodeB, nodeC, nodeFail})
	executor := execution.NewGraphExecutor(execution.ExecutorOptions{Writer: io.Discard})

	if err := executor.Run(context.Background(), g); err == nil {
		t.Fatal("expected error")
	}

	if len(log) != 3 {
		t.Fatalf("expected 3 undo calls, got %d: %v", len(log), log)
	}
	if log[0] != "C" || log[1] != "B" || log[2] != "A" {
		t.Errorf("expected LIFO order [C B A], got %v", log)
	}
}

// TestCompensationDryRun verifies that dry-run produces nil UndoState and
// that unwinding nil states causes no errors or filesystem changes.
func TestCompensationDryRun(t *testing.T) {
	tmpDir := t.TempDir()
	fp := &file.Provider{}

	writePath := filepath.Join(tmpDir, "write.txt")
	linkSource := filepath.Join(tmpDir, "source.txt")
	linkPath := filepath.Join(tmpDir, "link.txt")

	if err := os.WriteFile(linkSource, []byte("source"), 0o644); err != nil {
		t.Fatal(err)
	}

	writeNode := &op.Node{ID: "write", Action: fileAction(t, fp, "file.write_text")}
	writeNode.SetSlotImmediate("content", "hello")
	writeNode.SetSlotImmediate("destination", writePath)
	writeNode.SetSlotImmediate("mode", os.FileMode(0o644))

	linkNode := &op.Node{ID: "link", Action: fileAction(t, fp, "file.link")}
	linkNode.SetSlotImmediate("source", linkSource)
	linkNode.SetSlotImmediate("path", linkPath)

	failNode := &op.Node{ID: "fail", Action: &failAction{}}

	g := phasedGraph([]*op.Node{writeNode, linkNode, failNode})
	executor := execution.NewGraphExecutor(execution.ExecutorOptions{
		DryRun: true,
		Writer: io.Discard,
	})

	err := executor.Run(context.Background(), g)
	if err == nil {
		t.Fatal("expected error")
	}

	for _, p := range []string{writePath, linkPath} {
		if _, statErr := os.Stat(p); statErr == nil {
			t.Errorf("%s should not exist in dry-run mode", filepath.Base(p))
		}
	}
}

// TestCompensationNilState verifies that a non-compensable action (nil UndoState)
// mixed with compensable actions unwinds cleanly.
func TestCompensationNilState(t *testing.T) {
	tmpDir := t.TempDir()
	fp := &file.Provider{}
	writePath := filepath.Join(tmpDir, "write.txt")

	noopNode := &op.Node{ID: "noop", Action: &noopAction{}}

	writeNode := &op.Node{ID: "write", Action: fileAction(t, fp, "file.write_text")}
	writeNode.SetSlotImmediate("content", "hello")
	writeNode.SetSlotImmediate("destination", writePath)
	writeNode.SetSlotImmediate("mode", os.FileMode(0o644))

	failNode := &op.Node{ID: "fail", Action: &failAction{}}

	g := phasedGraph([]*op.Node{noopNode, writeNode, failNode})
	executor := execution.NewGraphExecutor(execution.ExecutorOptions{Writer: io.Discard})

	err := executor.Run(context.Background(), g)
	if err == nil {
		t.Fatal("expected error")
	}

	if _, statErr := os.Stat(writePath); statErr == nil {
		t.Error("write.txt should not exist after compensation")
	}
}

// TestCompensationPartialFailure verifies that only completed actions are
// compensated — the failing action and actions after it are not compensated.
func TestCompensationPartialFailure(t *testing.T) {
	tmpDir := t.TempDir()
	fp := &file.Provider{}

	firstPath := filepath.Join(tmpDir, "first.txt")
	thirdPath := filepath.Join(tmpDir, "third.txt")

	firstNode := &op.Node{ID: "first", Action: fileAction(t, fp, "file.write_text")}
	firstNode.SetSlotImmediate("content", "first")
	firstNode.SetSlotImmediate("destination", firstPath)
	firstNode.SetSlotImmediate("mode", os.FileMode(0o644))

	failNode := &op.Node{ID: "fail", Action: &failAction{}}

	thirdNode := &op.Node{ID: "third", Action: fileAction(t, fp, "file.write_text")}
	thirdNode.SetSlotImmediate("content", "third")
	thirdNode.SetSlotImmediate("destination", thirdPath)
	thirdNode.SetSlotImmediate("mode", os.FileMode(0o644))

	g := phasedGraph([]*op.Node{firstNode, failNode, thirdNode})
	executor := execution.NewGraphExecutor(execution.ExecutorOptions{Writer: io.Discard})

	err := executor.Run(context.Background(), g)
	if err == nil {
		t.Fatal("expected error")
	}

	// First write should be compensated (file removed).
	if _, statErr := os.Stat(firstPath); statErr == nil {
		t.Error("first.txt should not exist after compensation")
	}

	// Third write never executed (fail stopped the phase).
	if _, statErr := os.Stat(thirdPath); statErr == nil {
		t.Error("third.txt should not exist (never executed)")
	}
}

// TestCompensationGather verifies that gather compensates completed iterations
// when a later iteration fails. Uses Gather.Do directly.
func TestCompensationGather(t *testing.T) {
	tmpDir := t.TempDir()
	fp := &file.Provider{}

	paths := []string{
		filepath.Join(tmpDir, "a.txt"),
		filepath.Join(tmpDir, "b.txt"),
		filepath.Join(tmpDir, "c.txt"),
	}

	writeNode := &op.Node{ID: "write", Action: fileAction(t, fp, "file.write_text")}
	writeNode.SetSlotImmediate("content", "gather test")
	writeNode.SetSlotImmediate("mode", os.FileMode(0o644))
	writeNode.SetSlotProxy("destination", "gather", "")

	cfail := &conditionalFailAction{failPath: paths[2]}
	cfailNode := &op.Node{ID: "cfail", Action: cfail}
	cfailNode.SetSlotProxy("path", "gather", "")

	g := &op.Graph{
		State: op.StatePending,
		Nodes: []*op.Node{writeNode, cfailNode},
		Edges: []op.Edge{{From: "write", To: "cfail"}},
		Phases: []*op.Phase{{
			ID:      "phase.body",
			Name:    "body",
			Status:  op.PhasePending,
			NodeIDs: []string{"write", "cfail"},
		}},
	}

	ctx := &op.Context{
		Context: context.Background(),
		Writer:  io.Discard,
		Graph:   g,
		NodeID:  "gather",
	}

	gather := &flow.Gather{}
	slots := map[string]any{
		"items": []any{paths[0], paths[1], paths[2]},
		"do":    "phase.body",
		"limit": 1,
	}

	_, _, err := gather.Do(ctx, slots)
	if err == nil {
		t.Fatal("expected error from conditional failure on c.txt")
	}

	// a.txt and b.txt: compensated by gather's undoCompleted.
	// c.txt: written then undone by executeIteration's internal unwind.
	for _, p := range paths {
		if _, statErr := os.Stat(p); statErr == nil {
			t.Errorf("%s should not exist after gather compensation", filepath.Base(p))
		}
	}
}
