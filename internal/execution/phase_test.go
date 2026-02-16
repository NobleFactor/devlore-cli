// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package execution_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/NobleFactor/devlore-cli/internal/execution"
	"github.com/NobleFactor/devlore-cli/internal/execution/provider/file"
)

// TestRetryPolicyComputeDelay tests backoff delay computation.
func TestRetryPolicyComputeDelay(t *testing.T) {
	t.Run("none backoff", func(t *testing.T) {
		policy := &execution.RetryPolicy{
			MaxAttempts:  3,
			Backoff:      execution.BackoffNone,
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
		policy := &execution.RetryPolicy{
			MaxAttempts:  3,
			Backoff:      execution.BackoffLinear,
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
		policy := &execution.RetryPolicy{
			MaxAttempts:  3,
			Backoff:      execution.BackoffExponential,
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
		policy := &execution.RetryPolicy{
			MaxAttempts:  5,
			Backoff:      execution.BackoffExponential,
			InitialDelay: "100ms",
			MaxDelay:     "300ms",
		}
		d := policy.ComputeDelay(3) // Would be 800ms without cap
		if d != 300*time.Millisecond {
			t.Errorf("expected 300ms cap, got %v", d)
		}
	})

	t.Run("empty initial delay", func(t *testing.T) {
		policy := &execution.RetryPolicy{Backoff: execution.BackoffLinear}
		if d := policy.ComputeDelay(0); d != 0 {
			t.Errorf("expected 0 for empty initial delay, got %v", d)
		}
	})
}

// TestRetryPolicyParseDuration tests duration string parsing.
func TestRetryPolicyParseDuration(t *testing.T) {
	t.Run("valid initial delay", func(t *testing.T) {
		p := &execution.RetryPolicy{InitialDelay: "5s"}
		if d := p.ParseInitialDelay(); d != 5*time.Second {
			t.Errorf("expected 5s, got %v", d)
		}
	})

	t.Run("valid max delay", func(t *testing.T) {
		p := &execution.RetryPolicy{MaxDelay: "1m30s"}
		if d := p.ParseMaxDelay(); d != 90*time.Second {
			t.Errorf("expected 1m30s, got %v", d)
		}
	})

	t.Run("empty string returns 0", func(t *testing.T) {
		p := &execution.RetryPolicy{}
		if d := p.ParseInitialDelay(); d != 0 {
			t.Errorf("expected 0, got %v", d)
		}
	})

	t.Run("invalid string returns 0", func(t *testing.T) {
		p := &execution.RetryPolicy{InitialDelay: "not-a-duration"}
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

	fp := &file.Provider{}
	executor := execution.NewGraphExecutor(execution.ExecutorOptions{})

	graph := &execution.Graph{
		State: execution.StatePending,
		Phases: []*execution.Phase{
			{
				ID:      "phase.prepare",
				Name:    "prepare",
				Status:  execution.PhasePending,
				NodeIDs: []string{"probe"},
			},
			{
				ID:         "phase.install",
				Name:       "install",
				Status:     execution.PhasePending,
				NodeIDs:    []string{"pkg"},
				Compensate: "phase.install.compensate",
			},
			{
				ID:         "phase.provision",
				Name:       "provision",
				Status:     execution.PhasePending,
				NodeIDs:    []string{"config"},
				Compensate: "phase.provision.compensate",
			},
			{
				ID:      "phase.verify",
				Name:    "verify",
				Status:  execution.PhasePending,
				NodeIDs: []string{"check"},
			},
		},
		Nodes: []*execution.Node{
			testNode("probe", &file.Link{Impl: fp}, sources["probe.txt"], filepath.Join(tmpDir, "out-probe")),
			testNode("pkg", &file.Link{Impl: fp}, sources["pkg.txt"], filepath.Join(tmpDir, "out-pkg")),
			testNode("config", &file.Link{Impl: fp}, sources["config.txt"], filepath.Join(tmpDir, "out-config")),
			testNode("check", &file.Link{Impl: fp}, sources["verify.txt"], filepath.Join(tmpDir, "out-verify")),
		},
	}

	err := executor.Run(context.Background(), graph)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}

	if graph.State != execution.StateExecuted {
		t.Errorf("expected state executed, got %s", graph.State)
	}

	// All phases should be completed
	for _, p := range graph.Phases {
		if p.Status != execution.PhaseCompleted {
			t.Errorf("phase %s: expected completed, got %s", p.Name, p.Status)
		}
	}

	// All nodes should be completed
	for _, n := range graph.Nodes {
		if n.Status != execution.StatusCompleted {
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

	fp := &file.Provider{}
	failOp := &testRetryOp{
		name: "fail-provision",
		fn: func(ctx *execution.Context, slots map[string]any) error {
			return fmt.Errorf("permission denied")
		},
	}

	executor := execution.NewGraphExecutor(execution.ExecutorOptions{})

	graph := &execution.Graph{
		State: execution.StatePending,
		Phases: []*execution.Phase{
			{
				ID:         "phase.prepare",
				Name:       "prepare",
				Status:     execution.PhasePending,
				NodeIDs:    []string{"node-prepare"},
				Compensate: "phase.prepare.compensate",
			},
			{
				ID:         "phase.install",
				Name:       "install",
				Status:     execution.PhasePending,
				NodeIDs:    []string{"node-install"},
				Compensate: "phase.install.compensate",
			},
			{
				ID:         "phase.provision",
				Name:       "provision",
				Status:     execution.PhasePending,
				NodeIDs:    []string{"node-provision"},
				Compensate: "phase.provision.compensate",
			},
			{
				ID:      "phase.verify",
				Name:    "verify",
				Status:  execution.PhasePending,
				NodeIDs: []string{"node-verify"},
			},
			// Compensating phases
			{
				ID:      "phase.prepare.compensate",
				Name:    "prepare.compensate",
				Status:  execution.PhasePending,
				NodeIDs: []string{"comp-prepare"},
			},
			{
				ID:      "phase.install.compensate",
				Name:    "install.compensate",
				Status:  execution.PhasePending,
				NodeIDs: []string{"comp-install"},
			},
			{
				ID:      "phase.provision.compensate",
				Name:    "provision.compensate",
				Status:  execution.PhasePending,
				NodeIDs: []string{"comp-provision"},
			},
		},
		Nodes: []*execution.Node{
			testNode("node-prepare", &file.Link{Impl: fp}, src1, filepath.Join(tmpDir, "out1")),
			testNode("node-install", &file.Link{Impl: fp}, src2, filepath.Join(tmpDir, "out2")),
			{ID: "node-provision", Action: failOp},
			testNode("node-verify", &file.Link{Impl: fp}, src1, filepath.Join(tmpDir, "out4")),
			testNode("comp-prepare", &file.Link{Impl: fp}, compensateSrc, filepath.Join(tmpDir, "comp-out1")),
			testNode("comp-install", &file.Link{Impl: fp}, compensateSrc, filepath.Join(tmpDir, "comp-out2")),
			testNode("comp-provision", &file.Link{Impl: fp}, compensateSrc, filepath.Join(tmpDir, "comp-out3")),
		},
	}

	err := executor.Run(context.Background(), graph)
	if err == nil {
		t.Fatal("expected failure")
	}
	if !strings.Contains(err.Error(), "phase provision failed") {
		t.Errorf("expected provision failure, got: %v", err)
	}

	if graph.State != execution.StateFailed {
		t.Errorf("expected state failed, got %s", graph.State)
	}

	// Phases 1 and 2 should be rolled_back (they completed then were compensated)
	prepare := graph.PhaseByID("phase.prepare")
	install := graph.PhaseByID("phase.install")
	provision := graph.PhaseByID("phase.provision")
	verify := graph.PhaseByID("phase.verify")

	if prepare.Status != execution.PhaseRolledBack {
		t.Errorf("prepare: expected rolled_back, got %s", prepare.Status)
	}
	if install.Status != execution.PhaseRolledBack {
		t.Errorf("install: expected rolled_back, got %s", install.Status)
	}
	if provision.Status != execution.PhaseFailed {
		t.Errorf("provision: expected failed, got %s", provision.Status)
	}
	if verify.Status != execution.PhaseSkipped {
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

	retryOp := &testRetryOp{
		name: "retry-test",
		fn: func(ctx *execution.Context, slots map[string]any) error {
			attemptCount++
			if attemptCount == 1 {
				return fmt.Errorf("transient failure")
			}
			// Create the file on retry
			return os.WriteFile(delayedSrc, []byte("ok"), 0644)
		},
	}

	executor := execution.NewGraphExecutor(execution.ExecutorOptions{})

	graph := &execution.Graph{
		State: execution.StatePending,
		Phases: []*execution.Phase{
			{
				ID:      "phase.install",
				Name:    "install",
				Status:  execution.PhasePending,
				NodeIDs: []string{"retry-node"},
				Retry: &execution.RetryPolicy{
					MaxAttempts:  2,
					Backoff:      execution.BackoffNone,
					InitialDelay: "1ms", // Minimal delay for tests
				},
			},
		},
		Nodes: []*execution.Node{
			{ID: "retry-node", Action: retryOp},
		},
	}

	err := executor.Run(context.Background(), graph)
	if err != nil {
		t.Fatalf("expected success after retry, got: %v", err)
	}

	if graph.State != execution.StateExecuted {
		t.Errorf("expected executed, got %s", graph.State)
	}

	phase := graph.Phases[0]
	if phase.Status != execution.PhaseCompleted {
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

	fp := &file.Provider{}
	failOp := &testRetryOp{
		name: "always-fail",
		fn: func(ctx *execution.Context, slots map[string]any) error {
			return fmt.Errorf("permanent failure")
		},
	}

	executor := execution.NewGraphExecutor(execution.ExecutorOptions{})

	graph := &execution.Graph{
		State: execution.StatePending,
		Phases: []*execution.Phase{
			{
				ID:      "phase.prepare",
				Name:    "prepare",
				Status:  execution.PhasePending,
				NodeIDs: []string{"prepare-node"},
			},
			{
				ID:      "phase.install",
				Name:    "install",
				Status:  execution.PhasePending,
				NodeIDs: []string{"fail-node"},
				Retry: &execution.RetryPolicy{
					MaxAttempts:  2,
					Backoff:      execution.BackoffNone,
					InitialDelay: "1ms",
				},
			},
		},
		Nodes: []*execution.Node{
			testNode("prepare-node", &file.Link{Impl: fp}, src, filepath.Join(tmpDir, "out1")),
			{ID: "fail-node", Action: failOp},
		},
	}

	err := executor.Run(context.Background(), graph)
	if err == nil {
		t.Fatal("expected failure")
	}

	phase := graph.PhaseByID("phase.install")
	if phase.Status != execution.PhaseFailed {
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

	fp := &file.Provider{}
	executor := execution.NewGraphExecutor(execution.ExecutorOptions{})
	graph := &execution.Graph{
		State: execution.StatePending,
		Nodes: []*execution.Node{
			testNode("test", &file.Link{Impl: fp}, source, target),
		},
	}

	err := executor.Run(context.Background(), graph)
	if err != nil {
		t.Fatalf("non-phased run: %v", err)
	}

	if graph.State != execution.StateExecuted {
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
	g := &execution.Graph{
		Version: "1",
		Tool:    "lore",
		State:   execution.StatePending,
		Phases: []*execution.Phase{
			{
				ID:     "phase.install",
				Name:   "install",
				Status: execution.PhasePending,
				Retry: &execution.RetryPolicy{
					MaxAttempts:  3,
					Backoff:      execution.BackoffExponential,
					InitialDelay: "1s",
					MaxDelay:     "30s",
				},
				NodeIDs:    []string{"pkg-ripgrep"},
				Compensate: "phase.install.compensate",
			},
			{
				ID:     "phase.verify",
				Name:   "verify",
				Status: execution.PhasePending,
			},
		},
		Nodes: []*execution.Node{
			{ID: "pkg-ripgrep", Action: execution.StubAction("package-install")},
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
	g := &execution.Graph{
		Phases: []*execution.Phase{
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

// testRetryOp is a test-only action that executes a function.
type testRetryOp struct {
	name string
	fn   func(ctx *execution.Context, slots map[string]any) error
}

func (o *testRetryOp) Name() string { return o.name }
func (o *testRetryOp) Do(ctx *execution.Context, slots map[string]any) (execution.Result, execution.UndoState, error) {
	return nil, nil, o.fn(ctx, slots)
}
func (o *testRetryOp) Undo(_ *execution.Context, _ map[string]any, _ execution.UndoState) error {
	return nil
}
