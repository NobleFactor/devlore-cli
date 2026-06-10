// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package devloretest

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/NobleFactor/devlore-cli/pkg/application"
	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/starlarkbridge"
	"github.com/NobleFactor/devlore-cli/pkg/platform"
)

// Expectation represents a single test assertion queued during script execution.
type Expectation struct {
	Kind    string         // "file_exists", "no_file", "unit_count", "error", "equal", "variable", "variable_namespace"
	Path    string         // for file expectations
	Content string         // optional expected content
	Count   int            // for unit_count
	Pattern string         // for error expectations
	Got     starlark.Value // for equal expectations
	Want    starlark.Value // for equal expectations

	// Fields for "variable" and "variable_namespace" expectations.
	VarName       string          // the parameter name to look up in the resolved variable map
	VarValue      *starlark.Value // when non-nil, assert variables[VarName].Value equals the wrapped value
	VarSource     *string         // when non-nil, assert variables[VarName].Source.String() equals *VarSource
	VarSourceKind *string         // when non-nil, assert variables[VarName].Source.Kind.String() equals *VarSourceKind
}

// Failure records a failed expectation.
type Failure struct {
	Expectation string `json:"expectation"`
	Message     string `json:"message"`
}

// TestContext is the `t` namespace injected into Starlark test scripts.
//
// Provides a temp directory and queues expectations that are checked after graph execution completes.
// File checks are scoped through op.Root when available.
type TestContext struct {
	tmpDir       string
	root         op.Root
	writer       io.Writer // graph-output channel: t.run writes each execution result here
	expectations []Expectation
	sources      *BindingSources        // shared pointer to the Runner's BindingSources; mutated by t.set_* builtins
	variables    map[string]op.Variable // populated by SetResolvedVariables after the resolver runs; consumed in Check
	// envSet records keys set via t.set_env; the runner reads this to drive os.Unsetenv on teardown.
	envSet map[string]string
}

// EnvSet returns the env vars set via t.set_env during script execution.
//
// The runner reads this map at teardown to issue os.Unsetenv for each key — keeps process-env mutations
// from leaking between tests.
//
// Returns:
//   - map[string]string: the set env vars; never nil, possibly empty.
func (tc *TestContext) EnvSet() map[string]string {

	if tc.envSet == nil {
		tc.envSet = make(map[string]string)
	}
	return tc.envSet
}

// NewTestContext creates a TestContext rooted at `tmpDir`.
//
// When `root` is non-nil, file checks (checkFileExists, checkNoFile) are scoped through op.Root.
//
// Parameters:
//   - `tmpDir`: the temp directory the test owns; used as the root for t.tmp paths.
//   - `root`: optional op.Root that scopes file-check I/O; nil falls back to plain os calls.
//   - `sources`: shared pointer to the Runner's BindingSources; t.set_* builtins write through this pointer
//     so the Runner sees what the .star configured.
//
// Returns:
//   - *TestContext: the constructed context.
func NewTestContext(tmpDir string, root op.Root, sources *BindingSources) *TestContext {
	return &TestContext{tmpDir: tmpDir, root: root, sources: sources}
}

// SetResolvedVariables records the resolver's variable map so variable expectations can be checked.
//
// Parameters:
//   - `v`: the resolved variable map from [op.VariableResolver.Variables].
func (tc *TestContext) SetResolvedVariables(v map[string]op.Variable) {
	tc.variables = v
}

// --- Published methods ---

// Check evaluates all queued expectations against the executed graph and filesystem.
//
// Returns failures for any expectations that did not hold.
//
// Parameters:
//   - `graph`: the executed graph (may be nil when no script assembly succeeded).
//   - `execErr`: the execution error (nil on success); used for `error`-kind expectations.
//
// Returns:
//   - []Failure: failures for expectations that did not hold; never nil (empty when all pass).
func (tc *TestContext) Check(graph *op.Graph, execErr error) []Failure {
	var failures []Failure

	for _, exp := range tc.expectations {
		switch exp.Kind {
		case "file_exists":
			f := tc.checkFileExists(exp)
			if f != nil {
				failures = append(failures, *f)
			}

		case "no_file":
			f := tc.checkNoFile(exp)
			if f != nil {
				failures = append(failures, *f)
			}

		case "unit_count":
			if graph == nil {
				// Immediate-mode scripts assemble no graph; a nil graph means zero planned units, so
				// expect_unit_count(0) is satisfied. A non-zero expectation against a nil graph is a
				// genuine miss — the script meant to plan units but never assembled a graph.
				if exp.Count != 0 {
					failures = append(failures, Failure{
						Expectation: fmt.Sprintf("unit_count(%d)", exp.Count),
						Message:     "no graph assembled (script did not assign `graph = plan.assemble([...])`)",
					})
				}
				continue
			}
			if got := graph.UnitCount(); got != exp.Count {
				failures = append(failures, Failure{
					Expectation: fmt.Sprintf("unit_count(%d)", exp.Count),
					Message:     fmt.Sprintf("got %d units", got),
				})
			}

		case "error":
			f := tc.checkError(exp, execErr)
			if f != nil {
				failures = append(failures, *f)
			}

		case "variable":
			if f := tc.checkVariable(exp); f != nil {
				failures = append(failures, *f)
			}

		case "variable_namespace":
			if f := tc.checkVariableNamespace(exp); f != nil {
				failures = append(failures, *f)
			}

		case "equal":
			eq, err := starlark.Equal(exp.Got, exp.Want)
			if err != nil {
				failures = append(failures, Failure{
					Expectation: fmt.Sprintf("equal(%s, %s)", exp.Got, exp.Want),
					Message:     fmt.Sprintf("comparison error: %v", err),
				})
			} else if !eq {
				failures = append(failures, Failure{
					Expectation: fmt.Sprintf("equal(%s, %s)", exp.Got, exp.Want),
					Message:     fmt.Sprintf("got %s, want %s", exp.Got, exp.Want),
				})
			}
		}
	}

	return failures
}

// Expectations returns the queued expectations.
//
// Returns:
//   - []Expectation: the queued expectation list; may be empty.
func (tc *TestContext) Expectations() []Expectation {
	return tc.expectations
}

// StarlarkValue returns the `t` namespace as a Starlark struct.
//
// Returns:
//   - starlark.Value: a starlark struct binding the t.* builtins.
func (tc *TestContext) StarlarkValue() starlark.Value {
	return starlarkstruct.FromStringDict(starlark.String("t"), starlark.StringDict{
		"tmp":                       starlark.NewBuiltin("t.tmp", tc.starTmp),
		"mkdir":                     starlark.NewBuiltin("t.mkdir", tc.starMkdir),
		"write":                     starlark.NewBuiltin("t.write", tc.starWrite),
		"expect_file":               starlark.NewBuiltin("t.expect_file", tc.starExpectFile),
		"expect_no_file":            starlark.NewBuiltin("t.expect_no_file", tc.starExpectNoFile),
		"expect_unit_count":         starlark.NewBuiltin("t.expect_unit_count", tc.starExpectUnitCount),
		"expect_error":              starlark.NewBuiltin("t.expect_error", tc.starExpectError),
		"expect_equal":              starlark.NewBuiltin("t.expect_equal", tc.starExpectEqual),
		"set_overrides":             starlark.NewBuiltin("t.set_overrides", tc.starSetOverrides),
		"set_flags":                 starlark.NewBuiltin("t.set_flags", tc.starSetFlags),
		"set_env_prefix":            starlark.NewBuiltin("t.set_env_prefix", tc.starSetEnvPrefix),
		"set_env":                   starlark.NewBuiltin("t.set_env", tc.starSetEnv),
		"set_config":                starlark.NewBuiltin("t.set_config", tc.starSetConfig),
		"run":                       starlark.NewBuiltin("t.run", tc.starRun),
		"expect_variable":           starlark.NewBuiltin("t.expect_variable", tc.starExpectVariable),
		"expect_variable_namespace": starlark.NewBuiltin("t.expect_variable_namespace", tc.starExpectVariableNamespace),
	})
}

// TmpDir returns the temp directory path.
//
// Returns:
//   - `string`: the temp directory the test owns.
func (tc *TestContext) TmpDir() string {
	return tc.tmpDir
}

// --- Internal methods ---

// checkError evaluates an `error`-kind expectation against the script's execution error.
//
// Parameters:
//   - `exp`: the expectation carrying the regex pattern to match against the execution error.
//   - `execErr`: the execution error (nil means execution succeeded).
//
// Returns:
//   - *Failure: nil when the expectation holds; otherwise a Failure describing the mismatch.
func (tc *TestContext) checkError(exp Expectation, execErr error) *Failure {
	if execErr == nil {
		return &Failure{
			Expectation: fmt.Sprintf("error(%q)", exp.Pattern),
			Message:     "execution succeeded, expected error",
		}
	}
	matched, err := regexp.MatchString(exp.Pattern, execErr.Error())
	if err != nil {
		return &Failure{
			Expectation: fmt.Sprintf("error(%q)", exp.Pattern),
			Message:     fmt.Sprintf("invalid pattern: %v", err),
		}
	}
	if !matched {
		return &Failure{
			Expectation: fmt.Sprintf("error(%q)", exp.Pattern),
			Message:     fmt.Sprintf("error %q did not match pattern", execErr.Error()),
		}
	}
	return nil
}

// checkFileExists evaluates a `file_exists`-kind expectation against the filesystem.
//
// Parameters:
//   - `exp`: the expectation carrying the target path and optional content expectation.
//
// Returns:
//   - *Failure: nil when the file exists (and content matches, when supplied); otherwise a Failure
//     describing the mismatch.
func (tc *TestContext) checkFileExists(exp Expectation) *Failure {

	info, err := tc.stat(exp.Path)
	if err != nil {
		return &Failure{
			Expectation: fmt.Sprintf("file_exists(%s)", exp.Path),
			Message:     "file not found",
		}
	}
	if info.IsDir() {
		return &Failure{
			Expectation: fmt.Sprintf("file_exists(%s)", exp.Path),
			Message:     "path is a directory, not a file",
		}
	}

	if exp.Content != "" {
		data, err := tc.readFile(exp.Path)
		if err != nil {
			return &Failure{
				Expectation: fmt.Sprintf("file_exists(%s, content=...)", exp.Path),
				Message:     fmt.Sprintf("cannot read file: %v", err),
			}
		}
		if string(data) != exp.Content {
			return &Failure{
				Expectation: fmt.Sprintf("file_exists(%s, content=%q)", exp.Path, exp.Content),
				Message:     fmt.Sprintf("content mismatch: got %q", string(data)),
			}
		}
	}

	return nil
}

// checkNoFile evaluates a `no_file`-kind expectation against the filesystem.
//
// Parameters:
//   - `exp`: the expectation carrying the path that must NOT exist.
//
// Returns:
//   - *Failure: nil when the file does not exist; otherwise a Failure describing the unexpected
//     presence or stat error.
func (tc *TestContext) checkNoFile(exp Expectation) *Failure {

	_, err := tc.stat(exp.Path)
	if err == nil {
		return &Failure{
			Expectation: fmt.Sprintf("no_file(%s)", exp.Path),
			Message:     "file exists but should not",
		}
	}
	if !os.IsNotExist(err) {
		return &Failure{
			Expectation: fmt.Sprintf("no_file(%s)", exp.Path),
			Message:     fmt.Sprintf("unexpected error: %v", err),
		}
	}
	return nil
}

// readFile reads a file, using root-scoped I/O when root is available. Falls back to os.ReadFile otherwise.
//
// Parameters:
//   - `abs`: absolute path to the file.
//
// Returns:
//   - []byte: file contents.
//   - `error`: any read error.
func (tc *TestContext) readFile(abs string) ([]byte, error) {

	if tc.root != nil {
		return tc.root.ReadFile(tc.root.NewPath(abs))
	}

	return os.ReadFile(abs)
}

// starExpectEqual implements t.expect_equal(got, want).
func (tc *TestContext) starExpectEqual(
	_ *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var got, want starlark.Value
	if err := starlark.UnpackPositionalArgs("t.expect_equal", args, kwargs, 2, &got, &want); err != nil {
		return nil, err
	}

	tc.expectations = append(tc.expectations, Expectation{
		Kind: "equal",
		Got:  got,
		Want: want,
	})
	return starlark.None, nil
}

// starExpectError implements t.expect_error(pattern).
func (tc *TestContext) starExpectError(
	_ *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var pattern string
	if err := starlark.UnpackPositionalArgs("t.expect_error", args, kwargs, 1, &pattern); err != nil {
		return nil, err
	}

	// Validate the pattern compiles
	if _, err := regexp.Compile(pattern); err != nil {
		return nil, fmt.Errorf("t.expect_error: invalid regex: %w", err)
	}

	tc.expectations = append(tc.expectations, Expectation{
		Kind:    "error",
		Pattern: pattern,
	})
	return starlark.None, nil
}

// starExpectFile implements t.expect_file(path, content=None).
func (tc *TestContext) starExpectFile(
	_ *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var path string
	var content starlark.Value

	if err := starlark.UnpackArgs("t.expect_file", args, kwargs,
		"path", &path,
		"content?", &content,
	); err != nil {
		return nil, err
	}

	exp := Expectation{
		Kind: "file_exists",
		Path: path,
	}

	if content != nil && content != starlark.None {
		s, ok := content.(starlark.String)
		if !ok {
			return nil, fmt.Errorf("t.expect_file: content must be a string, got %s", content.Type())
		}
		exp.Content = string(s)
	}

	tc.expectations = append(tc.expectations, exp)
	return starlark.None, nil
}

// starExpectNoFile implements t.expect_no_file(path).
func (tc *TestContext) starExpectNoFile(
	_ *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var path string
	if err := starlark.UnpackPositionalArgs("t.expect_no_file", args, kwargs, 1, &path); err != nil {
		return nil, err
	}

	tc.expectations = append(tc.expectations, Expectation{
		Kind: "no_file",
		Path: path,
	})
	return starlark.None, nil
}

// starExpectUnitCount implements t.expect_unit_count(n).
//
// Asserts the total count of [ExecutableUnit] descendants of the assembled graph's Root — Nodes and
// Subgraphs both count.
func (tc *TestContext) starExpectUnitCount(
	_ *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var count int
	if err := starlark.UnpackPositionalArgs("t.expect_unit_count", args, kwargs, 1, &count); err != nil {
		return nil, err
	}

	tc.expectations = append(tc.expectations, Expectation{
		Kind:  "unit_count",
		Count: count,
	})
	return starlark.None, nil
}

// starMkdir implements t.mkdir(path) — creates a directory and parents for test setup.
func (tc *TestContext) starMkdir(
	_ *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var path string
	if err := starlark.UnpackPositionalArgs("t.mkdir", args, kwargs, 1, &path); err != nil {
		return nil, err
	}

	if err := os.MkdirAll(path, 0o750); err != nil {
		return nil, fmt.Errorf("t.mkdir: %w", err)
	}

	return starlark.None, nil
}

// starWrite implements t.write(path, content) — writes a file for test setup.
func (tc *TestContext) starWrite(
	_ *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var path, content string
	if err := starlark.UnpackPositionalArgs("t.write", args, kwargs, 2, &path, &content); err != nil {
		return nil, err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("t.write: %w", err)
	}

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return nil, fmt.Errorf("t.write: %w", err)
	}

	return starlark.None, nil
}

// starTmp implements t.tmp(relative) -> absolute path under temp dir.
func (tc *TestContext) starTmp(
	_ *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var relative string
	if err := starlark.UnpackPositionalArgs("t.tmp", args, kwargs, 1, &relative); err != nil {
		return nil, err
	}

	// Prevent path traversal
	if strings.Contains(relative, "..") {
		return nil, fmt.Errorf("t.tmp: path traversal not allowed: %s", relative)
	}

	return starlark.String(filepath.Join(tc.tmpDir, relative)), nil
}

// stat returns file info, using root-scoped I/O when root is available. Falls back to os.Stat otherwise.
//
// Parameters:
//   - `abs`: absolute path to stat.
//
// Returns:
//   - os.FileInfo: file metadata.
//   - `error`: any stat error.
func (tc *TestContext) stat(abs string) (os.FileInfo, error) {

	if tc.root != nil {
		return tc.root.Stat(tc.root.NewPath(abs))
	}

	return os.Stat(abs)
}

// --- Binding source setters ---

// starSetOverrides implements t.set_overrides(dict).
func (tc *TestContext) starSetOverrides(
	_ *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var d *starlark.Dict
	if err := starlark.UnpackPositionalArgs("t.set_overrides", args, kwargs, 1, &d); err != nil {
		return nil, err
	}
	m, err := starlarkDictToGoMap(d)
	if err != nil {
		return nil, fmt.Errorf("t.set_overrides: %w", err)
	}
	tc.sources.Overrides = m
	return starlark.None, nil
}

// starSetFlags implements t.set_flags(dict).
func (tc *TestContext) starSetFlags(
	_ *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var d *starlark.Dict
	if err := starlark.UnpackPositionalArgs("t.set_flags", args, kwargs, 1, &d); err != nil {
		return nil, err
	}
	m, err := starlarkDictToGoMap(d)
	if err != nil {
		return nil, fmt.Errorf("t.set_flags: %w", err)
	}
	tc.sources.Flags = m
	return starlark.None, nil
}

// starSetEnvPrefix implements t.set_env_prefix(prefix).
func (tc *TestContext) starSetEnvPrefix(
	_ *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var prefix string
	if err := starlark.UnpackPositionalArgs("t.set_env_prefix", args, kwargs, 1, &prefix); err != nil {
		return nil, err
	}
	tc.sources.EnvPrefix = prefix
	return starlark.None, nil
}

// starSetEnv implements t.set_env(dict).
//
// Sets each key=value pair in the process environment via os.Setenv and records the keys in tc.envSet
// so the runner can issue os.Unsetenv on teardown — keeps process-env mutations scoped to the test that
// authored them.
func (tc *TestContext) starSetEnv(
	_ *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {

	var d *starlark.Dict
	if err := starlark.UnpackPositionalArgs("t.set_env", args, kwargs, 1, &d); err != nil {
		return nil, err
	}

	if tc.envSet == nil {
		tc.envSet = make(map[string]string)
	}

	for _, k := range d.Keys() {
		keyStr, ok := starlark.AsString(k)
		if !ok {
			return nil, fmt.Errorf("t.set_env: key %v is not a string", k)
		}
		raw, _, err := d.Get(k)
		if err != nil {
			return nil, fmt.Errorf("t.set_env: get %q: %w", keyStr, err)
		}
		valStr, ok := starlark.AsString(raw)
		if !ok {
			return nil, fmt.Errorf("t.set_env: value for %q is %s, want string", keyStr, raw.Type())
		}
		if err := os.Setenv(keyStr, valStr); err != nil {
			return nil, fmt.Errorf("t.set_env: setenv %q: %w", keyStr, err)
		}
		tc.envSet[keyStr] = valStr
	}

	return starlark.None, nil
}

// starSetConfig implements t.set_config(dict).
func (tc *TestContext) starSetConfig(
	_ *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var d *starlark.Dict
	if err := starlark.UnpackPositionalArgs("t.set_config", args, kwargs, 1, &d); err != nil {
		return nil, err
	}
	m, err := starlarkDictToGoMap(d)
	if err != nil {
		return nil, fmt.Errorf("t.set_config: %w", err)
	}
	tc.sources.Config = m
	return starlark.None, nil
}

// starRun implements t.run(graph) — the test harness's default execute path.
//
// Replaces the pre-Step-16 runner-side auto-execute: scripts that build a graph and want it dispatched call
// `t.run(graph)` explicitly. The harness constructs a fresh [op.RuntimeEnvironmentSpec] from [TestContext.tmpDir]
// + [BindingSources] (the t.set_overrides / set_flags / set_config / set_env_prefix accumulated state) and runs
// the graph via [op.GraphExecutor.Run]. Variable bindings resolved during the executor's preflight pass land in
// [TestContext.variables] so subsequent `t.expect_variable` / `t.expect_variable_namespace` assertions see them.
//
// Scripts that want fine-grained spec control bypass this helper and call `plan.run(graph, plan.spec(...))`
// directly. `t.run` is the no-customization-needed sugar.
//
// Parameters:
//   - `args[0]`: the graph; must implement [starlarkbridge.Projector] (every `*op.Graph` value the bridge
//     returns from `plan.assemble` / `plan.load` satisfies this).
//
// Returns:
//   - starlark.Value: starlark.None on success.
//   - `error`: non-nil on argument-shape failure, projection failure, [op.NewConfinedRoot] failure,
//     [platform.Detect] failure, or any preflight / dispatch failure from [op.GraphExecutor.Run].
func (tc *TestContext) starRun(
	_ *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {

	if len(args) != 1 || len(kwargs) != 0 {
		return nil, fmt.Errorf(
			"t.run: expected exactly 1 positional argument (graph), got %d positional and %d keyword",
			len(args), len(kwargs),
		)
	}

	projector, ok := args[0].(starlarkbridge.Projector)
	if !ok {
		return nil, fmt.Errorf("t.run: graph argument of type %s is not a *op.Graph", args[0].Type())
	}

	projected, err := projector.Project(reflect.TypeFor[*op.Graph]())
	if err != nil {
		return nil, fmt.Errorf("t.run: project graph: %w", err)
	}

	graph, ok := projected.(*op.Graph)
	if !ok {
		return nil, fmt.Errorf("t.run: graph projection returned %T, want *op.Graph", projected)
	}

	spec, err := tc.buildSpec()
	if err != nil {
		return nil, err
	}

	executor := op.NewGraphExecutor(graph, spec)

	result, runErr := executor.Run(context.Background(), nil)
	tc.SetResolvedVariables(executor.LastVariables())

	if runErr != nil {
		return nil, runErr
	}

	// The graph-output channel carries the execution result — the return value of the graph's final unit,
	// distinct from result.Pipeline / status.Narrator output — so `--output graph=<dest>` receives it.
	if err := tc.emitResult(result); err != nil {
		return nil, err
	}

	return starlark.None, nil
}

// emitResult writes a t.run execution result to the graph-output channel as JSON.
//
// The result is the return value of the graph's final unit. A nil writer (tests that do not route the graph
// channel) or a nil result is a no-op.
//
// Parameters:
//   - `result`: the execution result to emit.
//
// Returns:
//   - `error`: non-nil if JSON marshaling or the write fails.
func (tc *TestContext) emitResult(result any) error {

	if tc.writer == nil || result == nil {
		return nil
	}

	data, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("t.run: marshal result: %w", err)
	}

	if _, err := fmt.Fprintln(tc.writer, string(data)); err != nil {
		return fmt.Errorf("t.run: write result: %w", err)
	}

	return nil
}

// buildSpec constructs a fresh [*op.RuntimeEnvironmentSpec] for [starRun] / [t.run].
//
// Each invocation mints a fresh [op.Root] anchored at [TestContext.tmpDir] (so successive `t.run` calls
// within one script don't share a closed Root); the [application.Application] carries the accumulated
// [BindingSources] state under program name "devlore-test" (or [BindingSources.EnvPrefix] when set).
//
// Returns:
//   - *op.RuntimeEnvironmentSpec: the constructed spec.
//   - `error`: non-nil when [op.NewConfinedRoot], [platform.Detect], or [platform.New] fails.
func (tc *TestContext) buildSpec() (*op.RuntimeEnvironmentSpec, error) {

	hostSpec, err := platform.Detect()
	if err != nil {
		return nil, fmt.Errorf("t.run: detect platform: %w", err)
	}

	hostPlatform, err := platform.New(hostSpec)
	if err != nil {
		return nil, fmt.Errorf("t.run: seal platform: %w", err)
	}

	root, err := op.NewConfinedRoot(tc.tmpDir)
	if err != nil {
		return nil, fmt.Errorf("t.run: open root %s: %w", tc.tmpDir, err)
	}

	programName := "devlore-test"
	if tc.sources.EnvPrefix != "" {
		programName = tc.sources.EnvPrefix
	}

	app := &application.Application{
		Name:      programName,
		Flags:     tc.sources.Flags,
		Overrides: tc.sources.Overrides,
		Config:    tc.sources.Config,
	}

	return op.NewRuntimeEnvironmentSpec(programName).
		WithRoot(root).
		WithPlatform(hostPlatform).
		WithApplication(app), nil
}

// --- Variable assertions ---

// starExpectVariable implements t.expect_variable(name, value=None, origin=None, origin_namespace=None).
// Each kwarg is independently optional; supplied kwargs are asserted, unsupplied kwargs are wildcarded.
func (tc *TestContext) starExpectVariable(
	_ *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var name string
	value := starlark.Value(starlark.None)
	origin := starlark.Value(starlark.None)
	originNs := starlark.Value(starlark.None)
	if err := starlark.UnpackArgs("t.expect_variable", args, kwargs,
		"name", &name,
		"value?", &value,
		"origin?", &origin,
		"origin_namespace?", &originNs,
	); err != nil {
		return nil, err
	}

	exp := Expectation{Kind: "variable", VarName: name}
	if value != starlark.None {
		v := value
		exp.VarValue = &v
	}
	if s, ok := origin.(starlark.String); ok {
		o := string(s)
		exp.VarSource = &o
	}
	if s, ok := originNs.(starlark.String); ok {
		ns := string(s)
		exp.VarSourceKind = &ns
	}

	tc.expectations = append(tc.expectations, exp)
	return starlark.None, nil
}

// starExpectVariableNamespace implements t.expect_variable_namespace(name, namespace).
func (tc *TestContext) starExpectVariableNamespace(
	_ *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var name, namespace string
	err := starlark.UnpackPositionalArgs("t.expect_variable_namespace", args, kwargs, 2, &name, &namespace)
	if err != nil {
		return nil, err
	}
	tc.expectations = append(tc.expectations, Expectation{
		Kind:          "variable_namespace",
		VarName:       name,
		VarSourceKind: &namespace,
	})
	return starlark.None, nil
}

// checkVariable evaluates an Expectation{Kind: "variable"} against the resolved variable map.
//
// Parameters:
//   - `exp`: the expectation carrying the variable name and optional value / source / kind assertions.
//
// Returns:
//   - *Failure: nil when the expectation holds; otherwise a Failure describing the mismatch.
func (tc *TestContext) checkVariable(exp Expectation) *Failure {

	v, ok := tc.variables[exp.VarName]
	if !ok {
		return &Failure{
			Expectation: fmt.Sprintf("variable(%s)", exp.VarName),
			Message:     "variable not resolved",
		}
	}

	if exp.VarValue != nil {
		goExpected := starlarkValueToGo(*exp.VarValue)
		if !equalValues(v.Value, goExpected) {
			return &Failure{
				Expectation: fmt.Sprintf("variable(%s, value=%v)", exp.VarName, goExpected),
				Message:     fmt.Sprintf("got %v (%T), want %v (%T)", v.Value, v.Value, goExpected, goExpected),
			}
		}
	}

	if exp.VarSource != nil {
		if got := v.Source.String(); got != *exp.VarSource {
			return &Failure{
				Expectation: fmt.Sprintf("variable(%s, source=%q)", exp.VarName, *exp.VarSource),
				Message:     fmt.Sprintf("got source %q", got),
			}
		}
	}

	if exp.VarSourceKind != nil {
		if got := v.Source.Kind.String(); got != *exp.VarSourceKind {
			return &Failure{
				Expectation: fmt.Sprintf("variable(%s, source_kind=%q)", exp.VarName, *exp.VarSourceKind),
				Message:     fmt.Sprintf("got source kind %q", got),
			}
		}
	}

	return nil
}

// checkVariableNamespace evaluates an Expectation{Kind: "variable_namespace"}.
//
// Parameters:
//   - `exp`: the expectation carrying the variable name and the required source kind.
//
// Returns:
//   - *Failure: nil when the resolved variable's source kind matches; otherwise a Failure
//     describing the mismatch.
func (tc *TestContext) checkVariableNamespace(exp Expectation) *Failure {

	v, ok := tc.variables[exp.VarName]
	if !ok {
		return &Failure{
			Expectation: fmt.Sprintf("variable_namespace(%s)", exp.VarName),
			Message:     "variable not resolved",
		}
	}

	if got := v.Source.Kind.String(); got != *exp.VarSourceKind {
		return &Failure{
			Expectation: fmt.Sprintf("variable_namespace(%s, %q)", exp.VarName, *exp.VarSourceKind),
			Message:     fmt.Sprintf("got source kind %q", got),
		}
	}

	return nil
}
