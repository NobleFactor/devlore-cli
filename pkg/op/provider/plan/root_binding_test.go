// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package plan_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/application"
	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/file"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/plan"

	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/file/gen"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/plan/gen"
)

// TestRun_BindsPendingResourcesToTheRunRoot pins the activation binding (4-resource-management.md §5.5,
// the run-from-elsewhere ruling, 2026-08-21): a graph planned under one root and executed under another
// verifies and observes its pending resources against the RUN's root — never the environment that
// constructed them.
//
// The sharp assertion is the trace: the ledger snapshot records the run's root as the binding, and the
// claimed rel is Active because it exists under the RUN root — the plan root never held the file at all.
func TestRun_BindsPendingResourcesToTheRunRoot(t *testing.T) {

	planRoot := t.TempDir() // the file NEVER exists here
	runRoot := t.TempDir()  // it exists here, under the claimed rel

	if err := os.WriteFile(filepath.Join(runRoot, "data.txt"), []byte("found under the run root"), 0o600); err != nil {
		t.Fatal(err)
	}

	graph := planReadText(t, planRoot)

	executor := op.NewGraphExecutor(graph, runSpec(runRoot))

	result, err := executor.Run(context.Background(), nil)
	if err != nil {
		t.Fatalf("Run under the run root: %v", err)
	}
	if got, ok := result.(string); !ok || got != "found under the run root" {
		t.Fatalf("result = %#v, want the run root's content", result)
	}

	assertTraceBinding(t, executor, runRoot, op.Active)
}

// TestRun_APendingRelAbsentUnderTheRunRootIsGone pins the inverse: the rel exists only under the PLAN
// root, so the run marks the entry Gone (pre-flight marks, phase 3 owns the failing consequence) and the
// dispatch fails to find it.
func TestRun_APendingRelAbsentUnderTheRunRootIsGone(t *testing.T) {

	planRoot := t.TempDir()
	runRoot := t.TempDir()

	if err := os.WriteFile(filepath.Join(planRoot, "data.txt"), []byte("only where it was planned"), 0o600); err != nil {
		t.Fatal(err)
	}

	graph := planReadText(t, planRoot)

	executor := op.NewGraphExecutor(graph, runSpec(runRoot))

	if _, err := executor.Run(context.Background(), nil); err == nil {
		t.Fatal("Run succeeded although the claimed rel does not exist under the run root")
	}

	assertTraceBinding(t, executor, runRoot, op.Gone)
}

// planReadText plans a one-node file.read_text graph claiming the rel "data.txt" under `planRoot`.
func planReadText(t *testing.T, planRoot string) *op.Graph {

	t.Helper()

	environment, err := op.NewRuntimeEnvironment(context.Background(), op.NewRuntimeEnvironmentSpec("test").
		WithApplication(&application.Application{Name: "test"}).
		WithRoot(planRoot))
	if err != nil {
		t.Fatalf("op.NewRuntimeEnvironment: %v", err)
	}
	t.Cleanup(func() { _ = environment.Close() })

	provider := plan.NewProvider(environment)

	invocation, err := provider.Plan(file.ReadText, nil, map[string]any{"resource": "data.txt"})
	if err != nil {
		t.Fatalf("provider.Plan: %v", err)
	}

	graph, err := provider.AssembleDefinition([]*op.Invocation{invocation}, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("provider.AssembleDefinition: %v", err)
	}

	return graph
}

// runSpec builds a run spec rooted at `runRoot`.
func runSpec(runRoot string) *op.RuntimeEnvironmentSpec {

	return op.NewRuntimeEnvironmentSpec("test").
		WithApplication(&application.Application{Name: "test"}).
		WithRoot(runRoot)
}

// assertTraceBinding asserts the trace's ledger snapshot records `runRoot` as the binding and holds the
// claimed rel in `state`.
func assertTraceBinding(t *testing.T, executor *op.GraphExecutor, runRoot string, state op.ResourceState) {

	t.Helper()

	trace := executor.Trace()
	if trace == nil || trace.Catalog == nil {
		t.Fatal("trace carries no ledger snapshot")
	}

	if trace.Catalog.Root != runRoot {
		t.Fatalf("trace binding root = %q, want the run root %q", trace.Catalog.Root, runRoot)
	}

	for _, entry := range trace.Catalog.Entries {
		if strings.Contains(entry.URI, "data.txt") {
			if entry.State != state {
				t.Fatalf("entry %s state = %v, want %v", entry.URI, entry.State, state)
			}
			return
		}
	}

	t.Fatalf("trace carries no entry for the claimed rel; entries: %+v", trace.Catalog.Entries)
}
