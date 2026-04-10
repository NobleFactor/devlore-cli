// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// testNode creates a node with the given action and source/target slots for testing.
func testNode(id string, action op.Action, source, target string) *op.Node {
	node := &op.Node{ID: id}
	node.SetAction(action)
	if source != "" {
		node.SetSlotImmediate("source", source)
	}
	if target != "" {
		node.SetSlotImmediate("target", target)
	}
	return node
}

// TestRetryPolicyComputeDelay tests backoff delay computation.
func TestRetryPolicyComputeDelay(t *testing.T) {
	t.Run("none backoff", func(t *testing.T) {
		policy := &op.RetryPolicy{
			MaxAttempts:  3,
			Backoff:      op.BackoffNone,
			InitialDelay: "100ms",
		}
		// All attempts get the same delay
		for i := 0; i < 3; i++ {
			d := policy.ComputeDelay(i)
			if d != 100*time.Millisecond {
				t.Errorf("attempt %d: expected 100ms, got %v", i, d)
			}
		}
	})

	t.Run("linear backoff", func(t *testing.T) {
		policy := &op.RetryPolicy{
			MaxAttempts:  3,
			Backoff:      op.BackoffLinear,
			InitialDelay: "100ms",
		}
		expected := []time.Duration{100 * time.Millisecond, 200 * time.Millisecond, 300 * time.Millisecond}
		for i, want := range expected {
			d := policy.ComputeDelay(i)
			if d != want {
				t.Errorf("attempt %d: expected %v, got %v", i, want, d)
			}
		}
	})

	t.Run("exponential backoff", func(t *testing.T) {
		policy := &op.RetryPolicy{
			MaxAttempts:  3,
			Backoff:      op.BackoffExponential,
			InitialDelay: "100ms",
		}
		expected := []time.Duration{100 * time.Millisecond, 200 * time.Millisecond, 400 * time.Millisecond}
		for i, want := range expected {
			d := policy.ComputeDelay(i)
			if d != want {
				t.Errorf("attempt %d: expected %v, got %v", i, want, d)
			}
		}
	})

	t.Run("max delay cap", func(t *testing.T) {
		policy := &op.RetryPolicy{
			MaxAttempts:  5,
			Backoff:      op.BackoffExponential,
			InitialDelay: "100ms",
			MaxDelay:     "300ms",
		}
		d := policy.ComputeDelay(3) // Would be 800ms without cap
		if d != 300*time.Millisecond {
			t.Errorf("expected 300ms cap, got %v", d)
		}
	})

	t.Run("empty initial delay", func(t *testing.T) {
		policy := &op.RetryPolicy{Backoff: op.BackoffLinear}
		if d := policy.ComputeDelay(0); d != 0 {
			t.Errorf("expected 0 for empty initial delay, got %v", d)
		}
	})
}

// TestRetryPolicyParseDuration tests duration string parsing.
func TestRetryPolicyParseDuration(t *testing.T) {
	t.Run("valid initial delay", func(t *testing.T) {
		p := &op.RetryPolicy{InitialDelay: "5s"}
		if d := p.ParseInitialDelay(); d != 5*time.Second {
			t.Errorf("expected 5s, got %v", d)
		}
	})

	t.Run("valid max delay", func(t *testing.T) {
		p := &op.RetryPolicy{MaxDelay: "1m30s"}
		if d := p.ParseMaxDelay(); d != 90*time.Second {
			t.Errorf("expected 1m30s, got %v", d)
		}
	})

	t.Run("empty string returns 0", func(t *testing.T) {
		p := &op.RetryPolicy{}
		if d := p.ParseInitialDelay(); d != 0 {
			t.Errorf("expected 0, got %v", d)
		}
	})

	t.Run("invalid string returns 0", func(t *testing.T) {
		p := &op.RetryPolicy{InitialDelay: "not-a-duration"}
		if d := p.ParseInitialDelay(); d != 0 {
			t.Errorf("expected 0 for invalid string, got %v", d)
		}
	})
}

// TestPhasedExecutionSuccess tests a 4-subgraph pipeline that succeeds.
func TestPhasedExecutionSuccess(t *testing.T) {
	tmpDir := t.TempDir()

	// Create source files for link operations
	sources := make(map[string]string)
	for _, name := range []string{"probe.txt", "pkg.txt", "config.txt", "verify.txt"} {
		path := filepath.Join(tmpDir, "src-"+name)
		if err := os.WriteFile(path, []byte("content-"+name), 0o644); err != nil {
			t.Fatal(err)
		}
		sources[name] = path
	}

	executor := newExecutor(t)

	// TODO: restructure to use Children/Subgraph tree once executor supports it
	graph := &op.Graph{
		State: op.StatePending,
		Children: []op.SubgraphChild{
			{Subgraph: &op.Subgraph{
				ID: "phase.prepare", Name: "prepare", Status: op.SubgraphPending,
				Children: []op.SubgraphChild{
					{Node: testNode("probe", linkAction(), sources["probe.txt"], filepath.Join(tmpDir, "out-probe"))},
				},
			}},
			{Subgraph: &op.Subgraph{
				ID: "phase.install", Name: "install", Status: op.SubgraphPending,
				Compensate: "phase.install.compensate",
				Children: []op.SubgraphChild{
					{Node: testNode("pkg", linkAction(), sources["pkg.txt"], filepath.Join(tmpDir, "out-pkg"))},
				},
			}},
			{Subgraph: &op.Subgraph{
				ID: "phase.provision", Name: "provision", Status: op.SubgraphPending,
				Compensate: "phase.provision.compensate",
				Children: []op.SubgraphChild{
					{Node: testNode("config", linkAction(), sources["config.txt"], filepath.Join(tmpDir, "out-config"))},
				},
			}},
			{Subgraph: &op.Subgraph{
				ID: "phase.verify", Name: "verify", Status: op.SubgraphPending,
				Children: []op.SubgraphChild{
					{Node: testNode("check", linkAction(), sources["verify.txt"], filepath.Join(tmpDir, "out-verify"))},
				},
			}},
		},
	}

	_, err := executor.Run(graph)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}

	if graph.State != op.StateExecuted {
		t.Errorf("expected state executed, got %s", graph.State)
	}

	// All subgraphs should be completed
	for _, c := range graph.Children {
		if c.Subgraph != nil && c.Subgraph.Status != op.SubgraphCompleted {
			t.Errorf("subgraph %s: expected completed, got %s", c.Subgraph.Name, c.Subgraph.Status)
		}
	}

	// All nodes should be completed
	for _, n := range graph.Nodes() {
		if n.Status != op.StatusCompleted {
			t.Errorf("node %s: expected completed, got %s", n.ID, n.Status)
		}
	}
}

// TestPhasedExecutionFailureWithRollback tests failure at subgraph 3 with LIFO rollback.
func TestPhasedExecutionFailureWithRollback(t *testing.T) {
	tmpDir := t.TempDir()

	// Create source files for subgraphs 1 and 2 (these succeed)
	src1 := filepath.Join(tmpDir, "src1.txt")
	src2 := filepath.Join(tmpDir, "src2.txt")
	if err := os.WriteFile(src1, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src2, []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}

	compensateSrc := filepath.Join(tmpDir, "compensate.txt")
	if err := os.WriteFile(compensateSrc, []byte("c"), 0o644); err != nil {
		t.Fatal(err)
	}

	failAction := &testRetryAction{
		name: "fail-provision",
		fn: func(ctx *op.ExecutionContext, slots map[string]any) error {
			return fmt.Errorf("permission denied")
		},
	}

	executor := newExecutor(t)

	// TODO: restructure to use Children/Subgraph tree once executor supports it
	graph := &op.Graph{
		State: op.StatePending,
		Children: []op.SubgraphChild{
			{Subgraph: &op.Subgraph{
				ID: "phase.prepare", Name: "prepare", Status: op.SubgraphPending,
				Compensate: "phase.prepare.compensate",
				Children: []op.SubgraphChild{
					{Node: testNode("node-prepare", linkAction(), src1, filepath.Join(tmpDir, "out1"))},
				},
			}},
			{Subgraph: &op.Subgraph{
				ID: "phase.install", Name: "install", Status: op.SubgraphPending,
				Compensate: "phase.install.compensate",
				Children: []op.SubgraphChild{
					{Node: testNode("node-install", linkAction(), src2, filepath.Join(tmpDir, "out2"))},
				},
			}},
			{Subgraph: &op.Subgraph{
				ID: "phase.provision", Name: "provision", Status: op.SubgraphPending,
				Compensate: "phase.provision.compensate",
				Children: []op.SubgraphChild{
					{Node: func() *op.Node { n := &op.Node{ID: "node-provision"}; n.SetAction(failAction); return n }()},
				},
			}},
			{Subgraph: &op.Subgraph{
				ID: "phase.verify", Name: "verify", Status: op.SubgraphPending,
				Children: []op.SubgraphChild{
					{Node: testNode("node-verify", linkAction(), src1, filepath.Join(tmpDir, "out4"))},
				},
			}},
			// Compensating subgraphs
			{Subgraph: &op.Subgraph{
				ID: "phase.prepare.compensate", Name: "prepare.compensate", Status: op.SubgraphPending,
				Children: []op.SubgraphChild{
					{Node: testNode("comp-prepare", linkAction(), compensateSrc, filepath.Join(tmpDir, "comp-out1"))},
				},
			}},
			{Subgraph: &op.Subgraph{
				ID: "phase.install.compensate", Name: "install.compensate", Status: op.SubgraphPending,
				Children: []op.SubgraphChild{
					{Node: testNode("comp-install", linkAction(), compensateSrc, filepath.Join(tmpDir, "comp-out2"))},
				},
			}},
			{Subgraph: &op.Subgraph{
				ID: "phase.provision.compensate", Name: "provision.compensate", Status: op.SubgraphPending,
				Children: []op.SubgraphChild{
					{Node: testNode("comp-provision", linkAction(), compensateSrc, filepath.Join(tmpDir, "comp-out3"))},
				},
			}},
		},
	}

	_, err := executor.Run(graph)
	if err == nil {
		t.Fatal("expected failure")
	}
	if !strings.Contains(err.Error(), "provision failed") {
		t.Errorf("expected provision failure, got: %v", err)
	}

	if graph.State != op.StateFailed {
		t.Errorf("expected state failed, got %s", graph.State)
	}

	// Subgraphs 1 and 2 should be rolled_back (they completed then were compensated)
	prepare := graph.SubgraphByID("phase.prepare")
	install := graph.SubgraphByID("phase.install")
	provision := graph.SubgraphByID("phase.provision")
	verify := graph.SubgraphByID("phase.verify")

	if prepare == nil || install == nil || provision == nil || verify == nil {
		t.Fatal("expected all subgraphs to be present in graph")
	}

	if prepare.Status != op.SubgraphRolledBack {
		t.Errorf("prepare: expected rolled_back, got %s", prepare.Status)
	}
	if install.Status != op.SubgraphRolledBack {
		t.Errorf("install: expected rolled_back, got %s", install.Status)
	}
	if provision.Status != op.SubgraphFailed {
		t.Errorf("provision: expected failed, got %s", provision.Status)
	}
	if verify.Status != op.SubgraphSkipped {
		t.Errorf("verify: expected skipped, got %s", verify.Status)
	}

	// Rollback log should have 2 entries (install, prepare) in LIFO order
	if len(graph.Rollback) != 2 {
		t.Fatalf("expected 2 rollback entries, got %d", len(graph.Rollback))
	}
	if graph.Rollback[0].Subgraph != "install" {
		t.Errorf("first rollback: expected install, got %s", graph.Rollback[0].Subgraph)
	}
	if graph.Rollback[1].Subgraph != "prepare" {
		t.Errorf("second rollback: expected prepare, got %s", graph.Rollback[1].Subgraph)
	}
}

// TestPhasedExecutionRetryThenSucceed tests that a subgraph retries and succeeds.
func TestPhasedExecutionRetryThenSucceed(t *testing.T) {
	tmpDir := t.TempDir()

	src := filepath.Join(tmpDir, "src.txt")
	if err := os.WriteFile(src, []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Subgraph 1's source will be created after first attempt
	delayedSrc := filepath.Join(tmpDir, "delayed-src.txt")

	attemptCount := 0

	retryAction := &testRetryAction{
		name: "retry-test",
		fn: func(ctx *op.ExecutionContext, slots map[string]any) error {
			attemptCount++
			if attemptCount == 1 {
				return fmt.Errorf("transient failure")
			}
			// Create the file on retry
			return os.WriteFile(delayedSrc, []byte("ok"), 0o644)
		},
	}

	executor := newExecutor(t)

	installSubgraph := &op.Subgraph{
		ID: "phase.install", Name: "install", Status: op.SubgraphPending,
		Children: []op.SubgraphChild{
			{Node: func() *op.Node { n := &op.Node{ID: "retry-node"}; n.SetAction(retryAction); return n }()},
		},
		Retry: &op.RetryPolicy{
			MaxAttempts:  2,
			Backoff:      op.BackoffNone,
			InitialDelay: "1ms", // Minimal delay for tests
		},
	}

	graph := &op.Graph{
		State: op.StatePending,
		Children: []op.SubgraphChild{
			{Subgraph: installSubgraph},
		},
	}

	_, err := executor.Run(graph)
	if err != nil {
		t.Fatalf("expected success after retry, got: %v", err)
	}

	if graph.State != op.StateExecuted {
		t.Errorf("expected executed, got %s", graph.State)
	}

	if installSubgraph.Status != op.SubgraphCompleted {
		t.Errorf("expected completed, got %s", installSubgraph.Status)
	}

	// Should have 2 attempts: 1 failed, 1 completed
	if len(installSubgraph.Attempts) != 2 {
		t.Fatalf("expected 2 attempts, got %d", len(installSubgraph.Attempts))
	}
	if installSubgraph.Attempts[0].Status != "failed" {
		t.Errorf("attempt 1: expected failed, got %s", installSubgraph.Attempts[0].Status)
	}
	if installSubgraph.Attempts[1].Status != "completed" {
		t.Errorf("attempt 2: expected completed, got %s", installSubgraph.Attempts[1].Status)
	}
}

// TestPhasedExecutionRetryExhausted tests that exhausted retries trigger rollback.
func TestPhasedExecutionRetryExhausted(t *testing.T) {
	tmpDir := t.TempDir()

	src := filepath.Join(tmpDir, "src.txt")
	if err := os.WriteFile(src, []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}

	failAction := &testRetryAction{
		name: "always-fail",
		fn: func(ctx *op.ExecutionContext, slots map[string]any) error {
			return fmt.Errorf("permanent failure")
		},
	}

	executor := newExecutor(t)

	graph := &op.Graph{
		State: op.StatePending,
		Children: []op.SubgraphChild{
			{Subgraph: &op.Subgraph{
				ID: "phase.prepare", Name: "prepare", Status: op.SubgraphPending,
				Children: []op.SubgraphChild{
					{Node: testNode("prepare-node", linkAction(), src, filepath.Join(tmpDir, "out1"))},
				},
			}},
			{Subgraph: &op.Subgraph{
				ID: "phase.install", Name: "install", Status: op.SubgraphPending,
				Children: []op.SubgraphChild{
					{Node: func() *op.Node { n := &op.Node{ID: "fail-node"}; n.SetAction(failAction); return n }()},
				},
				Retry: &op.RetryPolicy{
					MaxAttempts:  2,
					Backoff:      op.BackoffNone,
					InitialDelay: "1ms",
				},
			}},
		},
	}

	_, err := executor.Run(graph)
	if err == nil {
		t.Fatal("expected failure")
	}

	sg := graph.SubgraphByID("phase.install")
	if sg == nil {
		t.Fatal("expected phase.install to be present in graph")
	}
	if sg.Status != op.SubgraphFailed {
		t.Errorf("expected failed, got %s", sg.Status)
	}

	// Should have 3 attempts (1 original + 2 retries)
	if len(sg.Attempts) != 3 {
		t.Fatalf("expected 3 attempts, got %d", len(sg.Attempts))
	}
	for _, a := range sg.Attempts {
		if a.Status != "failed" {
			t.Errorf("attempt %d: expected failed, got %s", a.Number, a.Status)
		}
	}
}

// TestNonPhasedGraphUnchanged verifies that graphs without subgraphs still work.
func TestNonPhasedGraphUnchanged(t *testing.T) {
	tmpDir := t.TempDir()
	source := filepath.Join(tmpDir, "source.txt")
	target := filepath.Join(tmpDir, "target.txt")
	if err := os.WriteFile(source, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	executor := newExecutor(t)
	graph := &op.Graph{
		State: op.StatePending,
		Children: []op.SubgraphChild{
			{Node: testNode("test", linkAction(), source, target)},
		},
	}

	_, err := executor.Run(graph)
	if err != nil {
		t.Fatalf("non-phased run: %v", err)
	}

	if graph.State != op.StateExecuted {
		t.Errorf("expected executed, got %s", graph.State)
	}

	linkTarget, err := os.Readlink(target)
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	// Under os.Root-scoped I/O the symlink target is the absolute path within
	// the filesystem, which readlink returns verbatim.
	if linkTarget != source && linkTarget != filepath.Base(source) {
		t.Errorf("expected symlink to %s (or %s), got %s", source, filepath.Base(source), linkTarget)
	}
}

// TestPhasedGraphSerialization tests that subgraph graphs round-trip through YAML.
func TestPhasedGraphSerialization(t *testing.T) {
	g := &op.Graph{
		Version: "1",
		State:   op.StatePending,
		Children: []op.SubgraphChild{
			{Subgraph: &op.Subgraph{
				ID: "phase.install", Name: "install", Status: op.SubgraphPending,
				Compensate: "phase.install.compensate",
				Retry: &op.RetryPolicy{
					MaxAttempts:  3,
					Backoff:      op.BackoffExponential,
					InitialDelay: "1s",
					MaxDelay:     "30s",
				},
				Children: []op.SubgraphChild{
					{Node: &op.Node{ID: "pkg-ripgrep", Receiver: "package-install"}},
				},
			}},
			{Subgraph: &op.Subgraph{
				ID: "phase.verify", Name: "verify", Status: op.SubgraphPending,
			}},
		},
	}

	content, err := g.CanonicalContent()
	if err != nil {
		t.Fatalf("CanonicalContent: %v", err)
	}

	yaml := string(content)
	if !strings.Contains(yaml, "phase.install") {
		t.Error("expected phase.install in canonical content")
	}
	if !strings.Contains(yaml, "max_attempts: 3") {
		t.Error("expected max_attempts in canonical content")
	}
	if !strings.Contains(yaml, "exponential") {
		t.Error("expected backoff strategy in canonical content")
	}
}

// TestSubgraphByID tests Graph.SubgraphByID lookup.
func TestSubgraphByID(t *testing.T) {
	g := &op.Graph{
		Children: []op.SubgraphChild{
			{Subgraph: &op.Subgraph{ID: "phase.prepare", Name: "prepare"}},
			{Subgraph: &op.Subgraph{ID: "phase.install", Name: "install"}},
		},
	}

	if p := g.SubgraphByID("phase.install"); p == nil || p.Name != "install" {
		t.Error("expected to find phase.install")
	}
	if p := g.SubgraphByID("nonexistent"); p != nil {
		t.Error("expected nil for nonexistent subgraph")
	}
}

// symlinkAction creates a symlink from source to target slot values.
type symlinkAction struct {
	name string
}

func (a *symlinkAction) Name() string           { return a.name }
func (a *symlinkAction) Params() []op.Parameter { return nil }
func (a *symlinkAction) Do(_ *op.ExecutionContext, slots map[string]any) (op.Result, op.Complement, error) {
	source, _ := slots["source"].(string)
	target, _ := slots["target"].(string)
	if source == "" || target == "" {
		return nil, nil, fmt.Errorf("symlink: source and target required")
	}
	if err := os.Symlink(source, target); err != nil {
		return nil, nil, err
	}
	return target, target, nil
}
func (a *symlinkAction) Undo(_ *op.ExecutionContext, state op.Complement) error {
	if path, ok := state.(string); ok && path != "" {
		os.Remove(path)
	}
	return nil
}

func newExecutor(t *testing.T) *op.GraphExecutor {
	t.Helper()
	e, err := op.NewGraphExecutor("test", op.Options{})
	if err != nil {
		t.Fatalf("NewGraphExecutor: %v", err)
	}
	return e
}

func linkAction() *symlinkAction {
	return &symlinkAction{name: "file.link"}
}

// testRetryAction is a test-only action that executes a function.
type testRetryAction struct {
	name string
	fn   func(ctx *op.ExecutionContext, slots map[string]any) error
}

func (o *testRetryAction) Name() string           { return o.name }
func (o *testRetryAction) Params() []op.Parameter { return nil }
func (o *testRetryAction) Do(ctx *op.ExecutionContext, slots map[string]any) (result op.Result, undo op.Complement, err error) {
	return nil, nil, o.fn(ctx, slots)
}
func (o *testRetryAction) Undo(_ *op.ExecutionContext, _ op.Complement) error {
	return nil
}
