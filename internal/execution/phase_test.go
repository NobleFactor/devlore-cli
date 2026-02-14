// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package execution

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestRetryPolicyComputeDelay tests backoff delay computation.
func TestRetryPolicyComputeDelay(t *testing.T) {
	t.Run("none backoff", func(t *testing.T) {
		policy := &RetryPolicy{
			MaxAttempts:  3,
			Backoff:      BackoffNone,
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
		policy := &RetryPolicy{
			MaxAttempts:  3,
			Backoff:      BackoffLinear,
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
		policy := &RetryPolicy{
			MaxAttempts:  3,
			Backoff:      BackoffExponential,
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
		policy := &RetryPolicy{
			MaxAttempts:  5,
			Backoff:      BackoffExponential,
			InitialDelay: "100ms",
			MaxDelay:     "300ms",
		}
		d := policy.ComputeDelay(3) // Would be 800ms without cap
		if d != 300*time.Millisecond {
			t.Errorf("expected 300ms cap, got %v", d)
		}
	})

	t.Run("empty initial delay", func(t *testing.T) {
		policy := &RetryPolicy{Backoff: BackoffLinear}
		if d := policy.ComputeDelay(0); d != 0 {
			t.Errorf("expected 0 for empty initial delay, got %v", d)
		}
	})
}

// TestRetryPolicyParseDuration tests duration string parsing.
func TestRetryPolicyParseDuration(t *testing.T) {
	t.Run("valid initial delay", func(t *testing.T) {
		p := &RetryPolicy{InitialDelay: "5s"}
		if d := p.ParseInitialDelay(); d != 5*time.Second {
			t.Errorf("expected 5s, got %v", d)
		}
	})

	t.Run("valid max delay", func(t *testing.T) {
		p := &RetryPolicy{MaxDelay: "1m30s"}
		if d := p.ParseMaxDelay(); d != 90*time.Second {
			t.Errorf("expected 1m30s, got %v", d)
		}
	})

	t.Run("empty string returns 0", func(t *testing.T) {
		p := &RetryPolicy{}
		if d := p.ParseInitialDelay(); d != 0 {
			t.Errorf("expected 0, got %v", d)
		}
	})

	t.Run("invalid string returns 0", func(t *testing.T) {
		p := &RetryPolicy{InitialDelay: "not-a-duration"}
		if d := p.ParseInitialDelay(); d != 0 {
			t.Errorf("expected 0 for invalid string, got %v", d)
		}
	})
}

// TestPhasedExecutionSuccess tests a 4-phase pipeline that succeeds.
func TestPhasedExecutionSuccess(t *testing.T) {
	tmpDir := t.TempDir()

	// Create source files for link operations
	sources := make(map[string]string)
	for _, name := range []string{"probe.txt", "pkg.txt", "config.txt", "verify.txt"} {
		path := filepath.Join(tmpDir, "src-"+name)
		if err := os.WriteFile(path, []byte("content-"+name), 0644); err != nil {
			t.Fatal(err)
		}
		sources[name] = path
	}

	reg := NewOperationRegistry()
	reg.Register(&FileLinkOp{impl: &FileService{}})

	executor := NewGraphExecutor(reg, ExecutorOptions{})

	graph := &Graph{
		State: StatePending,
		Phases: []*Phase{
			{
				ID:      "phase.prepare",
				Name:    "prepare",
				Status:  PhasePending,
				NodeIDs: []string{"probe"},
			},
			{
				ID:         "phase.install",
				Name:       "install",
				Status:     PhasePending,
				NodeIDs:    []string{"pkg"},
				Compensate: "phase.install.compensate",
			},
			{
				ID:         "phase.provision",
				Name:       "provision",
				Status:     PhasePending,
				NodeIDs:    []string{"config"},
				Compensate: "phase.provision.compensate",
			},
			{
				ID:      "phase.verify",
				Name:    "verify",
				Status:  PhasePending,
				NodeIDs: []string{"check"},
			},
		},
		Nodes: []*Node{
			testNode("probe", "link", sources["probe.txt"], filepath.Join(tmpDir, "out-probe")),
			testNode("pkg", "link", sources["pkg.txt"], filepath.Join(tmpDir, "out-pkg")),
			testNode("config", "link", sources["config.txt"], filepath.Join(tmpDir, "out-config")),
			testNode("check", "link", sources["verify.txt"], filepath.Join(tmpDir, "out-verify")),
		},
	}

	err := executor.Run(context.Background(), graph)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}

	if graph.State != StateExecuted {
		t.Errorf("expected state executed, got %s", graph.State)
	}

	// All phases should be completed
	for _, p := range graph.Phases {
		if p.Status != PhaseCompleted {
			t.Errorf("phase %s: expected completed, got %s", p.Name, p.Status)
		}
	}

	// All nodes should be completed
	for _, n := range graph.Nodes {
		if n.Status != StatusCompleted {
			t.Errorf("node %s: expected completed, got %s", n.ID, n.Status)
		}
	}
}

// TestPhasedExecutionFailureWithRollback tests failure at phase 3 with LIFO rollback.
func TestPhasedExecutionFailureWithRollback(t *testing.T) {
	tmpDir := t.TempDir()

	// Create source files for phases 1 and 2 (these succeed)
	src1 := filepath.Join(tmpDir, "src1.txt")
	src2 := filepath.Join(tmpDir, "src2.txt")
	if err := os.WriteFile(src1, []byte("a"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src2, []byte("b"), 0644); err != nil {
		t.Fatal(err)
	}

	compensateSrc := filepath.Join(tmpDir, "compensate.txt")
	if err := os.WriteFile(compensateSrc, []byte("c"), 0644); err != nil {
		t.Fatal(err)
	}

	reg := NewOperationRegistry()
	reg.Register(&FileLinkOp{impl: &FileService{}})
	// Phase 3 uses an operation that always fails
	reg.Register(&testRetryOp{
		name: "fail-provision",
		fn: func(ctx *Context, node *Node) error {
			return fmt.Errorf("permission denied")
		},
	})

	executor := NewGraphExecutor(reg, ExecutorOptions{})

	graph := &Graph{
		State: StatePending,
		Phases: []*Phase{
			{
				ID:         "phase.prepare",
				Name:       "prepare",
				Status:     PhasePending,
				NodeIDs:    []string{"node-prepare"},
				Compensate: "phase.prepare.compensate",
			},
			{
				ID:         "phase.install",
				Name:       "install",
				Status:     PhasePending,
				NodeIDs:    []string{"node-install"},
				Compensate: "phase.install.compensate",
			},
			{
				ID:         "phase.provision",
				Name:       "provision",
				Status:     PhasePending,
				NodeIDs:    []string{"node-provision"},
				Compensate: "phase.provision.compensate",
			},
			{
				ID:      "phase.verify",
				Name:    "verify",
				Status:  PhasePending,
				NodeIDs: []string{"node-verify"},
			},
			// Compensating phases
			{
				ID:      "phase.prepare.compensate",
				Name:    "prepare.compensate",
				Status:  PhasePending,
				NodeIDs: []string{"comp-prepare"},
			},
			{
				ID:      "phase.install.compensate",
				Name:    "install.compensate",
				Status:  PhasePending,
				NodeIDs: []string{"comp-install"},
			},
			{
				ID:      "phase.provision.compensate",
				Name:    "provision.compensate",
				Status:  PhasePending,
				NodeIDs: []string{"comp-provision"},
			},
		},
		Nodes: []*Node{
			testNode("node-prepare", "link", src1, filepath.Join(tmpDir, "out1")),
			testNode("node-install", "link", src2, filepath.Join(tmpDir, "out2")),
			{ID: "node-provision", Operation: "fail-provision"},
			testNode("node-verify", "link", src1, filepath.Join(tmpDir, "out4")),
			testNode("comp-prepare", "link", compensateSrc, filepath.Join(tmpDir, "comp-out1")),
			testNode("comp-install", "link", compensateSrc, filepath.Join(tmpDir, "comp-out2")),
			testNode("comp-provision", "link", compensateSrc, filepath.Join(tmpDir, "comp-out3")),
		},
	}

	err := executor.Run(context.Background(), graph)
	if err == nil {
		t.Fatal("expected failure")
	}
	if !strings.Contains(err.Error(), "phase provision failed") {
		t.Errorf("expected provision failure, got: %v", err)
	}

	if graph.State != StateFailed {
		t.Errorf("expected state failed, got %s", graph.State)
	}

	// Phases 1 and 2 should be rolled_back (they completed then were compensated)
	prepare := graph.PhaseByID("phase.prepare")
	install := graph.PhaseByID("phase.install")
	provision := graph.PhaseByID("phase.provision")
	verify := graph.PhaseByID("phase.verify")

	if prepare.Status != PhaseRolledBack {
		t.Errorf("prepare: expected rolled_back, got %s", prepare.Status)
	}
	if install.Status != PhaseRolledBack {
		t.Errorf("install: expected rolled_back, got %s", install.Status)
	}
	if provision.Status != PhaseFailed {
		t.Errorf("provision: expected failed, got %s", provision.Status)
	}
	if verify.Status != PhaseSkipped {
		t.Errorf("verify: expected skipped, got %s", verify.Status)
	}

	// Rollback log should have 2 entries (install, prepare) in LIFO order
	if len(graph.Rollback) != 2 {
		t.Fatalf("expected 2 rollback entries, got %d", len(graph.Rollback))
	}
	if graph.Rollback[0].Phase != "install" {
		t.Errorf("first rollback: expected install, got %s", graph.Rollback[0].Phase)
	}
	if graph.Rollback[1].Phase != "prepare" {
		t.Errorf("second rollback: expected prepare, got %s", graph.Rollback[1].Phase)
	}
}

// TestPhasedExecutionRetryThenSucceed tests that a phase retries and succeeds.
func TestPhasedExecutionRetryThenSucceed(t *testing.T) {
	tmpDir := t.TempDir()

	src := filepath.Join(tmpDir, "src.txt")
	if err := os.WriteFile(src, []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Phase 1's source will be created after first attempt
	delayedSrc := filepath.Join(tmpDir, "delayed-src.txt")

	attemptCount := 0

	reg := NewOperationRegistry()
	reg.Register(&FileLinkOp{impl: &FileService{}})
	// Register a custom op that creates the file on second attempt
	reg.Register(&testRetryOp{
		name: "retry-test",
		fn: func(ctx *Context, node *Node) error {
			attemptCount++
			if attemptCount == 1 {
				return fmt.Errorf("transient failure")
			}
			// Create the file on retry
			return os.WriteFile(delayedSrc, []byte("ok"), 0644)
		},
	})

	executor := NewGraphExecutor(reg, ExecutorOptions{})

	graph := &Graph{
		State: StatePending,
		Phases: []*Phase{
			{
				ID:      "phase.install",
				Name:    "install",
				Status:  PhasePending,
				NodeIDs: []string{"retry-node"},
				Retry: &RetryPolicy{
					MaxAttempts:  2,
					Backoff:      BackoffNone,
					InitialDelay: "1ms", // Minimal delay for tests
				},
			},
		},
		Nodes: []*Node{
			{ID: "retry-node", Operation: "retry-test"},
		},
	}

	err := executor.Run(context.Background(), graph)
	if err != nil {
		t.Fatalf("expected success after retry, got: %v", err)
	}

	if graph.State != StateExecuted {
		t.Errorf("expected executed, got %s", graph.State)
	}

	phase := graph.Phases[0]
	if phase.Status != PhaseCompleted {
		t.Errorf("expected completed, got %s", phase.Status)
	}

	// Should have 2 attempts: 1 failed, 1 completed
	if len(phase.Attempts) != 2 {
		t.Fatalf("expected 2 attempts, got %d", len(phase.Attempts))
	}
	if phase.Attempts[0].Status != "failed" {
		t.Errorf("attempt 1: expected failed, got %s", phase.Attempts[0].Status)
	}
	if phase.Attempts[1].Status != "completed" {
		t.Errorf("attempt 2: expected completed, got %s", phase.Attempts[1].Status)
	}
}

// TestPhasedExecutionRetryExhausted tests that exhausted retries trigger rollback.
func TestPhasedExecutionRetryExhausted(t *testing.T) {
	tmpDir := t.TempDir()

	src := filepath.Join(tmpDir, "src.txt")
	if err := os.WriteFile(src, []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}

	reg := NewOperationRegistry()
	reg.Register(&FileLinkOp{impl: &FileService{}})
	reg.Register(&testRetryOp{
		name: "always-fail",
		fn: func(ctx *Context, node *Node) error {
			return fmt.Errorf("permanent failure")
		},
	})

	executor := NewGraphExecutor(reg, ExecutorOptions{})

	graph := &Graph{
		State: StatePending,
		Phases: []*Phase{
			{
				ID:      "phase.prepare",
				Name:    "prepare",
				Status:  PhasePending,
				NodeIDs: []string{"prepare-node"},
			},
			{
				ID:      "phase.install",
				Name:    "install",
				Status:  PhasePending,
				NodeIDs: []string{"fail-node"},
				Retry: &RetryPolicy{
					MaxAttempts:  2,
					Backoff:      BackoffNone,
					InitialDelay: "1ms",
				},
			},
		},
		Nodes: []*Node{
			testNode("prepare-node", "link", src, filepath.Join(tmpDir, "out1")),
			{ID: "fail-node", Operation: "always-fail"},
		},
	}

	err := executor.Run(context.Background(), graph)
	if err == nil {
		t.Fatal("expected failure")
	}

	phase := graph.PhaseByID("phase.install")
	if phase.Status != PhaseFailed {
		t.Errorf("expected failed, got %s", phase.Status)
	}

	// Should have 3 attempts (1 original + 2 retries)
	if len(phase.Attempts) != 3 {
		t.Fatalf("expected 3 attempts, got %d", len(phase.Attempts))
	}
	for _, a := range phase.Attempts {
		if a.Status != "failed" {
			t.Errorf("attempt %d: expected failed, got %s", a.Number, a.Status)
		}
	}
}

// TestNonPhasedGraphUnchanged verifies that graphs without phases still work.
func TestNonPhasedGraphUnchanged(t *testing.T) {
	tmpDir := t.TempDir()
	source := filepath.Join(tmpDir, "source.txt")
	target := filepath.Join(tmpDir, "target.txt")
	if err := os.WriteFile(source, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	reg := NewOperationRegistry()
	reg.Register(&FileLinkOp{impl: &FileService{}})

	executor := NewGraphExecutor(reg, ExecutorOptions{})
	graph := &Graph{
		State: StatePending,
		Nodes: []*Node{
			testNode("test", "link", source, target),
		},
	}

	err := executor.Run(context.Background(), graph)
	if err != nil {
		t.Fatalf("non-phased run: %v", err)
	}

	if graph.State != StateExecuted {
		t.Errorf("expected executed, got %s", graph.State)
	}

	linkTarget, err := os.Readlink(target)
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if linkTarget != source {
		t.Errorf("expected symlink to %s, got %s", source, linkTarget)
	}
}

// TestPhasedGraphSerialization tests that phased graphs round-trip through YAML.
func TestPhasedGraphSerialization(t *testing.T) {
	g := &Graph{
		Version: "1",
		Tool:    "lore",
		State:   StatePending,
		Phases: []*Phase{
			{
				ID:     "phase.install",
				Name:   "install",
				Status: PhasePending,
				Retry: &RetryPolicy{
					MaxAttempts:  3,
					Backoff:      BackoffExponential,
					InitialDelay: "1s",
					MaxDelay:     "30s",
				},
				NodeIDs:    []string{"pkg-ripgrep"},
				Compensate: "phase.install.compensate",
			},
			{
				ID:     "phase.verify",
				Name:   "verify",
				Status: PhasePending,
			},
		},
		Nodes: []*Node{
			{ID: "pkg-ripgrep", Operation: "package-install"},
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

// TestPhaseByID tests Graph.PhaseByID lookup.
func TestPhaseByID(t *testing.T) {
	g := &Graph{
		Phases: []*Phase{
			{ID: "phase.prepare", Name: "prepare"},
			{ID: "phase.install", Name: "install"},
		},
	}

	if p := g.PhaseByID("phase.install"); p == nil || p.Name != "install" {
		t.Error("expected to find phase.install")
	}
	if p := g.PhaseByID("nonexistent"); p != nil {
		t.Error("expected nil for nonexistent phase")
	}
}

// testRetryOp is a test-only operation that executes a function.
type testRetryOp struct {
	name string
	fn   func(ctx *Context, node *Node) error
}

func (o *testRetryOp) Name() string { return o.name }
func (o *testRetryOp) Execute(ctx *Context, node *Node) error {
	return o.fn(ctx, node)
}
