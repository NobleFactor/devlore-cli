// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package devloretest

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// Expectation represents a single test assertion queued during script execution.
type Expectation struct {
	Kind    string         // "file_exists", "no_file", "node_count", "error", "equal"
	Path    string         // for file expectations
	Content string         // optional expected content
	Count   int            // for node_count
	Pattern string         // for error expectations
	Got     starlark.Value // for equal expectations
	Want    starlark.Value // for equal expectations
}

// Failure records a failed expectation.
type Failure struct {
	Expectation string `json:"expectation"`
	Message     string `json:"message"`
}

// TestContext is the `t` namespace injected into Starlark test scripts. It provides a temp directory and queues
// expectations that are checked after graph execution completes. File checks are scoped through op.Root when available.
type TestContext struct {
	tmpDir       string
	root         op.Root
	expectations []Expectation
}

// NewTestContext creates a TestContext rooted at the given temp directory. When root is non-nil, file checks
// (checkFileExists, checkNoFile) are scoped through op.Root.
func NewTestContext(tmpDir string, root op.Root) *TestContext {
	return &TestContext{tmpDir: tmpDir, root: root}
}

// --- Published methods ---

// Check evaluates all queued expectations against the executed graph and filesystem.
// Returns failures for any expectations that did not hold.
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

		case "node_count":
			if len(graph.Nodes) != exp.Count {
				failures = append(failures, Failure{
					Expectation: fmt.Sprintf("node_count(%d)", exp.Count),
					Message:     fmt.Sprintf("got %d nodes", len(graph.Nodes)),
				})
			}

		case "error":
			f := tc.checkError(exp, execErr)
			if f != nil {
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
func (tc *TestContext) Expectations() []Expectation {
	return tc.expectations
}

// StarlarkValue returns the `t` namespace as a Starlark struct.
func (tc *TestContext) StarlarkValue() starlark.Value {
	return starlarkstruct.FromStringDict(starlark.String("t"), starlark.StringDict{
		"tmp":               starlark.NewBuiltin("t.tmp", tc.starTmp),
		"mkdir":             starlark.NewBuiltin("t.mkdir", tc.starMkdir),
		"write":             starlark.NewBuiltin("t.write", tc.starWrite),
		"expect_file":       starlark.NewBuiltin("t.expect_file", tc.starExpectFile),
		"expect_no_file":    starlark.NewBuiltin("t.expect_no_file", tc.starExpectNoFile),
		"expect_node_count": starlark.NewBuiltin("t.expect_node_count", tc.starExpectNodeCount),
		"expect_error":      starlark.NewBuiltin("t.expect_error", tc.starExpectError),
		"expect_equal":      starlark.NewBuiltin("t.expect_equal", tc.starExpectEqual),
	})
}

// TmpDir returns the temp directory path.
func (tc *TestContext) TmpDir() string {
	return tc.tmpDir
}

// --- Internal methods ---

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
//   - abs: Absolute path to the file
//
// Returns:
//   - []byte: file contents
//   - error: any read error
func (tc *TestContext) readFile(abs string) ([]byte, error) {

	if tc.root != nil {
		return tc.root.ReadFile(tc.root.NewPath(abs))
	}

	return os.ReadFile(abs)
}

// starExpectEqual implements t.expect_equal(got, want).
func (tc *TestContext) starExpectEqual(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
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
func (tc *TestContext) starExpectError(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var pattern string
	if err := starlark.UnpackPositionalArgs("t.expect_error", args, kwargs, 1, &pattern); err != nil {
		return nil, err
	}

	// Validate the pattern compiles
	if _, err := regexp.Compile(pattern); err != nil {
		return nil, fmt.Errorf("t.expect_error: invalid regex: %v", err)
	}

	tc.expectations = append(tc.expectations, Expectation{
		Kind:    "error",
		Pattern: pattern,
	})
	return starlark.None, nil
}

// starExpectFile implements t.expect_file(path, content=None).
func (tc *TestContext) starExpectFile(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
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
func (tc *TestContext) starExpectNoFile(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
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

// starExpectNodeCount implements t.expect_node_count(n).
func (tc *TestContext) starExpectNodeCount(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var count int
	if err := starlark.UnpackPositionalArgs("t.expect_node_count", args, kwargs, 1, &count); err != nil {
		return nil, err
	}

	tc.expectations = append(tc.expectations, Expectation{
		Kind:  "node_count",
		Count: count,
	})
	return starlark.None, nil
}

// starMkdir implements t.mkdir(path) — creates a directory and parents for test setup.
func (tc *TestContext) starMkdir(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var path string
	if err := starlark.UnpackPositionalArgs("t.mkdir", args, kwargs, 1, &path); err != nil {
		return nil, err
	}

	if err := os.MkdirAll(path, 0o755); err != nil {
		return nil, fmt.Errorf("t.mkdir: %w", err)
	}

	return starlark.None, nil
}

// starWrite implements t.write(path, content) — writes a file for test setup.
func (tc *TestContext) starWrite(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var path, content string
	if err := starlark.UnpackPositionalArgs("t.write", args, kwargs, 2, &path, &content); err != nil {
		return nil, err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("t.write: %w", err)
	}

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return nil, fmt.Errorf("t.write: %w", err)
	}

	return starlark.None, nil
}

// starTmp implements t.tmp(relative) -> absolute path under temp dir.
func (tc *TestContext) starTmp(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
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
//   - abs: Absolute path to stat
//
// Returns:
//   - os.FileInfo: file metadata
//   - error: any stat error
func (tc *TestContext) stat(abs string) (os.FileInfo, error) {

	if tc.root != nil {
		return tc.root.Stat(tc.root.NewPath(abs))
	}

	return os.Stat(abs)
}
