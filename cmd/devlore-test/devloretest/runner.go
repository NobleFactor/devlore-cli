// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package devloretest

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/NobleFactor/devlore-cli/pkg/op/starlarkbridge"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"github.com/NobleFactor/devlore-cli/pkg/application"
	"github.com/NobleFactor/devlore-cli/pkg/iox"
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// BindingSources captures the variable-resolver source maps the runner threads into the resolver at
// graph-execution time. The runner and the [TestContext] share a pointer to the same instance so .star-side
// setters (t.set_overrides, t.set_flags, t.set_env_prefix, t.set_config) and Go-side runner options write to
// the same place. EnvPrefix is treated as a program-name override for the resolver's env-prefix derivation;
// when non-empty it is passed in lieu of the spec's ProgramName so tests can simulate writ-style or
// lore-style env lookups under the devlore-test harness.
type BindingSources struct {
	Overrides map[string]any
	Flags     map[string]any
	EnvPrefix string
	Config    map[string]any
}

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
//
// Returns:
//   - Option: a runner option that sets dry-run mode.
func WithDryRun() Option {
	return func(r *Runner) { r.dryRun = true }
}

// WithTrace enables Starlark step-by-step trace logging.
//
// Returns:
//   - Option: a runner option that enables tracing.
func WithTrace() Option {
	return func(r *Runner) { r.trace = true }
}

// WithProvider restricts execution to a specific provider.
//
// Parameters:
//   - name: the provider name to restrict to.
//
// Returns:
//   - Option: a runner option that sets the provider filter.
func WithProvider(name string) Option {
	return func(r *Runner) { r.provider = name }
}

// WithWriter sets the output writer for executor messages.
//
// Parameters:
//   - w: the output writer.
//
// Returns:
//   - Option: a runner option that sets the writer.
func WithWriter(w io.Writer) Option {
	return func(r *Runner) { r.writer = w }
}

// WithReceivers sets the receiver factories to expose as Starlark globals.
//
// Parameters:
//   - receivers: the receiver factories to include.
//
// Returns:
//   - Option: a runner option that sets the receiver list.
func WithReceivers(receivers ...op.ReceiverType) Option {
	return func(r *Runner) {
		r.receivers = append(r.receivers, receivers...)
	}
}

// WithGraphBuilder enables the plan.* graph namespace.
//
// Returns:
//   - Option: a runner option that enables the graph builder.
func WithGraphBuilder() Option {
	return func(r *Runner) { r.withGraphBuilder = true }
}

// Runner orchestrates a single test script execution.
type Runner struct {
	script           string
	dryRun           bool
	trace            bool
	provider         string
	writer           io.Writer
	receivers        []op.ReceiverType
	withGraphBuilder bool
	graph            *op.Graph
	sources          *BindingSources
}

// WithOverrides supplies an explicit-runtime-force ([op.VariableSourceKindOverride]) map of variable values
// for the variable resolver to consume at execute time.
//
// Parameters:
//   - `m`: parameter-name keyed map of override values.
//
// Returns:
//   - Option: a runner option that records the override map.
func WithOverrides(m map[string]any) Option {
	return func(r *Runner) { r.sources.Overrides = m }
}

// WithFlags supplies a command-line-argument ([op.VariableSourceKindFlag]) map of variable values for the
// variable resolver to consume at execute time.
//
// Parameters:
//   - `m`: parameter-name keyed map of flag-derived values.
//
// Returns:
//   - Option: a runner option that records the flag map.
func WithFlags(m map[string]any) Option {
	return func(r *Runner) { r.sources.Flags = m }
}

// WithEnvPrefix overrides the program-name string the resolver uses to derive its env-var prefix. The
// resolver always derives `strings.ToUpper(programName) + "_"` as its env prefix; supplying a different
// program name here lets a test simulate writ-style or lore-style env lookups while running under the
// devlore-test harness. When unset, the resolver derives the prefix from the spec's ProgramName.
//
// Parameters:
//   - `programPrefix`: the program-name override (e.g., "writ" for `WRIT_*` env lookups).
//
// Returns:
//   - Option: a runner option that records the env prefix.
func WithEnvPrefix(programPrefix string) Option {
	return func(r *Runner) { r.sources.EnvPrefix = programPrefix }
}

// WithConfig supplies a configuration ([op.VariableSourceKindConfig]) map of variable values for the
// variable resolver to consume at execute time.
//
// Parameters:
//   - `m`: parameter-name keyed map of config values.
//
// Returns:
//   - Option: a runner option that records the config map.
func WithConfig(m map[string]any) Option {
	return func(r *Runner) { r.sources.Config = m }
}

// Sources returns the runner's [BindingSources] pointer. Used by [TestContext] to share the same source maps
// for inline .star-side setters (t.set_overrides etc.).
//
// Returns:
//   - *BindingSources: the shared sources pointer.
func (r *Runner) Sources() *BindingSources {
	return r.sources
}

// Graph returns the execution graph after Start completes. Returns nil before Start is called.
//
// Returns:
//   - *op.Graph: the execution graph, or nil if Start has not been called.
func (r *Runner) Graph() *op.Graph {
	return r.graph
}

// NewRunner creates a Runner for the given script path.
//
// Parameters:
//   - script: the path to the .star test script.
//   - opts: functional options to configure the runner.
//
// Returns:
//   - *Runner: the configured test runner.
func NewRunner(script string, opts ...Option) *Runner {
	r := &Runner{
		script:  script,
		writer:  io.Discard,
		sources: &BindingSources{},
	}
	for _, o := range opts {
		o(r)
	}
	return r
}

// Start executes the test script and returns structured results.
//
// Parameters:
//   - ctx: the execution context (used for cancellation).
//
// Returns:
//   - *Result: the test outcome with pass/fail status and failures.
//   - error: non-nil if script loading or graph execution fails unexpectedly.
func (r *Runner) Start(ctx context.Context) (_ *Result, err error) {

	// 1. Create temp directory

	tmpDir, err := os.MkdirTemp("", "devlore-test-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }() //nolint:errcheck // best-effort cleanup

	// 2. Create ReceiverRegistry and Spec

	receiverRegistry := op.NewReceiverRegistry()
	root := op.NewRootReaderWriter(tmpDir)
	defer iox.Close(&err, root)

	spec := op.NewRuntimeEnvironmentSpec("devlore-test", receiverRegistry).
		WithModules(receiverRegistry.Modules()...).
		WithRoot(root).
		WithApplication(&application.Application{
			Name:  "devlore-test",
			Flags: map[string]any{"dry-run": r.dryRun},
		})

	runtimeEnvironment := op.NewRuntimeEnvironment(ctx, spec)
	graph := op.NewGraph()
	graph.Rebind(runtimeEnvironment)
	r.graph = graph

	// 3. Create Runtime. The graph reference no longer flows through the env data bag (killed in 13.0(n)
	//    Phase 1); flow.Provider reads it from ActivationRecord.Graph stamped by the executor.

	bs := starlarkbridge.NewRuntime(spec)

	// 4. Build Starlark globals
	globals := bs.Predeclared()

	// 5. Create TestContext rooted at .devlore/tmp/ under tmpDir

	testTmpDir := filepath.Join(tmpDir, ".devlore", "tmp")

	if err := os.MkdirAll(testTmpDir, 0o750); err != nil {
		return nil, fmt.Errorf("creating test tmp dir: %w", err)
	}

	tc := NewTestContext(testTmpDir, root, r.sources)
	globals["t"] = tc.StarlarkValue()

	// 6. Set up tracer

	tracer := NewTracer(r.trace)

	// 7. Configure thread

	thread := &starlark.Thread{
		Name:  "devlore-test",
		Print: tracer.PrintHandler(),
	}

	// 8. Read and execute .star script

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

	// 9. Wrap nodes in a main phase for saga-pattern compensation.

	wrapUngroupedNodes(graph)

	// 10. Build variable resolver from accumulated sources and resolve. Phase 1 has no graph.Parameters()
	// surface yet (Phase 3 adds it), so we pass nil parameters and the resolver produces an empty map.
	// The resolver still records sources from runner options and t.set_* calls so future phases can build
	// on the same wiring without harness changes.

	resolverProgramName := r.sources.EnvPrefix
	if resolverProgramName == "" {
		resolverProgramName = spec.ProgramName
	}

	resolver := op.NewVariableResolver(&application.Application{
		Name:      resolverProgramName,
		Flags:     r.sources.Flags,
		Config:    r.sources.Config,
		Overrides: r.sources.Overrides,
	})
	resolveErrs := resolver.Resolve(nil)
	if len(resolveErrs) > 0 {
		return nil, fmt.Errorf("variable resolution: %v", resolveErrs)
	}
	tc.SetResolvedVariables(resolver.Variables())

	// 11. Execute graph

	executor := op.NewGraphExecutor(ctx, spec)
	defer func() { _ = executor.Close() }()

	if tracer.Enabled() {
		tracer.Record("executing graph: %d nodes", len(graph.Nodes()))
	}

	_, execErr := executor.Run(graph, resolver.Variables())

	if tracer.Enabled() {
		if execErr != nil {
			tracer.Record("execution error: %v", execErr)
		} else {
			tracer.Record("execution completed: state=%s", graph.State)
		}
	}

	// 11. Check expectations

	return r.buildResult(graph, tc, tracer, execErr), nil
}

// buildResult evaluates expectations and constructs the Result.
//
// Parameters:
//   - graph: the executed graph.
//   - tc: the test context with expectations.
//   - tracer: the trace collector.
//   - execErr: the execution error (nil on success).
//
// Returns:
//   - *Result: the structured test result.
func (r *Runner) buildResult(graph *op.Graph, tc *TestContext, tracer *Tracer, execErr error) *Result {
	failures := tc.Check(graph, execErr)
	if failures == nil {
		failures = []Failure{}
	}

	result := &Result{
		Passed:           len(failures) == 0,
		NodeCount:        len(graph.Nodes()),
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

// wrapUngroupedNodes wraps root-level node children into a "test" main subgraph.
// Nodes created by choose-branch lambdas already belong to branch subgraphs; the remaining
// root-level nodes (predicates, choose, top-level actions) must be wrapped so the executor
// runs them and choose can dispatch branch subgraphs internally.
func wrapUngroupedNodes(graph *op.Graph) {

	var nodeChildren []op.SubgraphChild
	var sgChildren []op.SubgraphChild

	for _, c := range graph.Root.Children {
		if c.Node != nil {
			nodeChildren = append(nodeChildren, c)
		} else {
			sgChildren = append(sgChildren, c)
		}
	}

	if len(nodeChildren) == 0 {
		return
	}

	// Collect IDs of nodes being moved into the main subgraph.
	nodeIDs := make(map[string]bool, len(nodeChildren))
	for _, c := range nodeChildren {
		nodeIDs[c.ChildID()] = true
	}

	// Move edges whose endpoints are both in the main subgraph.
	var mainEdges, keptEdges []op.Edge
	for _, e := range graph.Root.Edges {
		if nodeIDs[e.From] && nodeIDs[e.To] {
			mainEdges = append(mainEdges, e)
		} else {
			keptEdges = append(keptEdges, e)
		}
	}
	graph.Root.Edges = keptEdges

	mainSG := op.NewSubgraph("subgraph.test")
	mainSG.Name = "test"
	mainSG.Children = nodeChildren
	mainSG.Edges = mainEdges
	mainSG.Status = op.SubgraphPending

	// Prepend main subgraph so it runs before branch subgraphs.
	graph.Root.Children = append([]op.SubgraphChild{{Subgraph: mainSG}}, sgChildren...)
}

// hasErrorExpectation returns true if any expectation is of kind "error".
//
// Parameters:
//   - tc: the test context to check.
//
// Returns:
//   - bool: true if at least one expectation has kind "error".
func hasErrorExpectation(tc *TestContext) bool {
	for _, exp := range tc.Expectations() {
		if exp.Kind == "error" {
			return true
		}
	}
	return false
}
