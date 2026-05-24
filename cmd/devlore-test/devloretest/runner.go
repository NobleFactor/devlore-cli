// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package devloretest

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"

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
	UnitCount        int       `json:"unit_count"`
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

	// 1. Create a temp directory

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

	// 3. Read the script.

	scriptData, err := os.ReadFile(r.script)
	if err != nil {
		return nil, fmt.Errorf("reading script %s: %w", r.script, err)
	}

	// 4. Create TestContext rooted at .devlore/tmp/ under tmpDir.

	testTmpDir := filepath.Join(tmpDir, ".devlore", "tmp")

	if err := os.MkdirAll(testTmpDir, 0o750); err != nil {
		return nil, fmt.Errorf("creating test tmp dir: %w", err)
	}

	tc := NewTestContext(testTmpDir, root, r.sources)

	// 5. Set up tracer.

	tracer := NewTracer(r.trace)

	if tracer.Enabled() {
		tracer.Record("script: %s", r.script)
		tracer.Record("tmpdir: %s", tmpDir)
	}

	// 6. Run the planning session via op.Plan.

	var scriptExecErr error
	var scriptGlobals starlark.StringDict

	graph, planErr := op.Plan(ctx, spec, func(env *op.RuntimeEnvironment) (*op.Graph, error) {

		bridge := starlarkbridge.NewRuntime(env)
		globals := bridge.Predeclared()
		globals["t"] = tc.StarlarkValue()

		thread := &starlark.Thread{
			Name:  "devlore-test",
			Print: tracer.PrintHandler(),
		}

		opts := &syntax.FileOptions{
			Set:             true,
			GlobalReassign:  true,
			TopLevelControl: true,
		}

		scriptGlobals, scriptExecErr = starlark.ExecFileOptions(opts, thread, r.script, scriptData, globals)
		if scriptExecErr != nil && !hasErrorExpectation(tc) {
			return nil, fmt.Errorf("executing script: %w", scriptExecErr)
		}

		return graphFromGlobals(scriptGlobals), nil
	})
	if planErr != nil {
		return nil, planErr
	}

	r.graph = graph

	// 7. Execute the graph post-Plan.

	if !r.dryRun && graph != nil && scriptExecErr == nil {
		executor := op.NewGraphExecutor(graph, spec)
		if _, runErr := executor.Run(ctx, nil); runErr != nil {
			scriptExecErr = runErr
		}
	}

	// 8. Build the Result.

	if tracer.Enabled() {
		if scriptExecErr != nil {
			tracer.Record("script error: %v", scriptExecErr)
		} else {
			tracer.Record("script completed")
		}
	}

	return r.buildResult(graph, tc, tracer, scriptExecErr), nil
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

	unitCount := 0
	if graph != nil {
		unitCount = graph.UnitCount()
	}

	result := &Result{
		Passed:           len(failures) == 0,
		UnitCount:        unitCount,
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

// graphFromGlobals unwraps the `graph` global from a script's post-execution starlark.StringDict
// and projects it back to a *op.Graph.
//
// Convention: .star test scripts that want the harness to assert against the assembled graph
// write `graph = plan.assemble([...])` at the top level. plan.assemble returns a *op.Graph that
// the bridge wraps as a starlarkbridge.Projector; this helper looks for that variable in `globals`
// and projects the wrapped value back to *op.Graph.
//
// Returns nil when:
//   - `globals` is nil (script didn't run).
//   - `globals["graph"]` doesn't exist (script didn't assign the conventional name).
//   - The value at that key isn't a Projector or doesn't project to *op.Graph.
//
// Scripts that don't assemble a graph (or use a different variable name) leave the runner with a
// nil graph, which surfaces as a clean "no graph assembled" failure in [TestContext.Check]
// rather than a panic.
//
// Parameters:
//   - `globals`: the starlark.StringDict returned by [starlark.ExecFileOptions].
//
// Returns:
//   - *op.Graph: the assembled graph, or nil if not present / not projectable.
func graphFromGlobals(globals starlark.StringDict) *op.Graph {

	if globals == nil {
		return nil
	}

	value, ok := globals["graph"]
	if !ok {
		return nil
	}

	projector, ok := value.(starlarkbridge.Projector)
	if !ok {
		return nil
	}

	projected, err := projector.Project(reflect.TypeFor[*op.Graph]())
	if err != nil {
		return nil
	}

	graph, ok := projected.(*op.Graph)
	if !ok {
		return nil
	}

	return graph
}
