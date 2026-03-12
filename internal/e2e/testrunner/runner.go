// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package testrunner

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"github.com/NobleFactor/devlore-cli/internal/execution"
	loreStar "github.com/NobleFactor/devlore-cli/internal/starlark"
	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/platform"

	// Blank imports register provider actions and callable extractor via init().
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/mem"
)

// Result is the structured output of a test run.
type Result struct {
	Passed           bool      `json:"passed"`
	NodeCount        int       `json:"node_count"`
	ExpectationCount int       `json:"expectation_count"`
	Failures         []Failure `json:"failures"`
	Trace            []string  `json:"trace,omitempty"`
}

// Option configures a Runner.
type Option func(*Runner)

// WithDryRun enables dry-run mode (plan only, no side effects).
func WithDryRun() Option {
	return func(r *Runner) { r.dryRun = true }
}

// WithTrace enables Starlark step-by-step trace logging.
func WithTrace() Option {
	return func(r *Runner) { r.trace = true }
}

// WithProvider restricts execution to a specific provider.
func WithProvider(name string) Option {
	return func(r *Runner) { r.provider = name }
}

// WithWriter sets the output writer for executor messages.
func WithWriter(w io.Writer) Option {
	return func(r *Runner) { r.writer = w }
}

// WithReceivers sets the Starlark namespaces to expose as globals.
func WithReceivers(names ...string) Option {
	return func(r *Runner) { r.receivers = names }
}

// Runner orchestrates a single test script execution.
type Runner struct {
	script    string
	dryRun    bool
	trace     bool
	provider  string
	writer    io.Writer
	receivers []string
	graph     *op.Graph
}

// Graph returns the execution graph after Start completes. Returns nil before Start is called.
func (r *Runner) Graph() *op.Graph {
	return r.graph
}

// New creates a Runner for the given script path.
func New(script string, opts ...Option) *Runner {
	r := &Runner{
		script: script,
		writer: io.Discard,
	}
	for _, o := range opts {
		o(r)
	}
	return r
}

// Start executes the test script and returns structured results.
func (r *Runner) Start(ctx context.Context) (*Result, error) {
	// 1. Create temp directory
	tmpDir, err := os.MkdirTemp("", "devlore-test-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// 2. Create BindingSet with requested receivers
	bs := loreStar.NewBindingSet(op.BindingConfig{
		Writer:      r.writer,
		ProgramName: "devlore-test",
		Receivers:   r.receivers,
	})

	// 3. Create ActionRegistry with all provider actions
	reg := op.NewActionRegistry()
	root := op.NewRootReaderWriter(tmpDir)
	defer root.Close()
	opCtx := op.Context{ContextBase: op.ContextBase{Root: root}}
	opCtx.RecoverySite = op.NewRecoverySite(opCtx)
	bs.RegisterActions(reg, opCtx)

	// 4. Create Platform
	plat := platform.New()

	// 5. Create Graph
	graph := op.NewGraph("devlore-test")
	r.graph = graph

	// 6. Build Starlark globals
	globals := bs.BuildGlobals(graph, "devlore-test", reg)

	// 7. Create TestContext rooted at .devlore/tmp/ under tmpDir
	testTmpDir := filepath.Join(tmpDir, ".devlore", "tmp")
	if err := os.MkdirAll(testTmpDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating test tmp dir: %w", err)
	}
	tc := NewTestContext(testTmpDir, root)
	globals["t"] = tc.StarlarkValue()

	// 8. Set up tracer
	tracer := NewTracer(r.trace)

	// 9. Configure thread
	thread := &starlark.Thread{
		Name:  "devlore-test",
		Print: tracer.PrintHandler(),
	}
	bs.ConfigureThread(thread, graph, "devlore-test", reg)

	// 10. Read and execute .star script
	scriptData, err := os.ReadFile(r.script)
	if err != nil {
		return nil, fmt.Errorf("reading script %s: %w", r.script, err)
	}

	opts := &syntax.FileOptions{
		Set:             true,
		GlobalReassign:  true,
		TopLevelControl: true,
	}

	if tracer.Enabled() {
		tracer.Record("script: %s", r.script)
		tracer.Record("tmpdir: %s", tmpDir)
	}

	_, err = starlark.ExecFileOptions(opts, thread, r.script, scriptData, globals)
	if err != nil {
		// Check if we expected an error
		if hasErrorExpectation(tc) {
			return r.buildResult(graph, tc, tracer, err), nil
		}
		return nil, fmt.Errorf("executing script: %w", err)
	}

	// 11. Hydrate graph (actions are already set by planned bindings, but
	//     this ensures any deserialized stubs are resolved)
	if err := op.HydrateGraph(graph, reg); err != nil {
		return nil, fmt.Errorf("hydrating graph: %w", err)
	}

	// 12. Wrap nodes in a single phase for saga-pattern compensation.
	//     Without phases, runFlat executes but does not unwind the recovery
	//     stack on failure. Wrapping gives us RunPhased semantics.
	if len(graph.Phases) == 0 && len(graph.Nodes) > 0 {
		phase := &op.Phase{
			ID:     "phase.test",
			Name:   "test",
			Status: op.PhasePending,
		}
		for _, n := range graph.Nodes {
			phase.NodeIDs = append(phase.NodeIDs, n.ID)
		}
		graph.Phases = []*op.Phase{phase}
	}

	// 13. Execute graph
	executor := execution.NewGraphExecutor(execution.ExecutorOptions{
		Root:     tmpDir,
		DryRun:   r.dryRun,
		Writer:   r.writer,
		Platform: plat,
	})

	if tracer.Enabled() {
		tracer.Record("executing graph: %d nodes", len(graph.Nodes))
	}

	execErr := executor.Run(ctx, graph)

	if tracer.Enabled() {
		if execErr != nil {
			tracer.Record("execution error: %v", execErr)
		} else {
			tracer.Record("execution completed: state=%s", graph.State)
		}
	}

	// 14. Check expectations
	return r.buildResult(graph, tc, tracer, execErr), nil
}

// buildResult evaluates expectations and constructs the Result.
func (r *Runner) buildResult(graph *op.Graph, tc *TestContext, tracer *Tracer, execErr error) *Result {
	failures := tc.Check(graph, execErr)
	if failures == nil {
		failures = []Failure{}
	}

	result := &Result{
		Passed:           len(failures) == 0,
		NodeCount:        len(graph.Nodes),
		ExpectationCount: len(tc.Expectations()),
		Failures:         failures,
	}

	// If there were no error expectations but execution failed, report it
	if execErr != nil && !hasErrorExpectation(tc) {
		result.Passed = false
		result.Failures = append(result.Failures, Failure{
			Expectation: "execution",
			Message:     execErr.Error(),
		})
	}

	if tracer.Enabled() {
		result.Trace = tracer.Entries()
	}

	return result
}

// hasErrorExpectation returns true if any expectation is of kind "error".
func hasErrorExpectation(tc *TestContext) bool {
	for _, exp := range tc.Expectations() {
		if exp.Kind == "error" {
			return true
		}
	}
	return false
}
