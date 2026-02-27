// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlark_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	loreStar "github.com/NobleFactor/devlore-cli/internal/starlark"
	"github.com/NobleFactor/devlore-cli/pkg/op"

	// Ensure providers are registered via init().
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/starcode/gen"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/ui"
)

// TestLoadIntegration executes a .star script that uses load("@devlore//starcode") and verifies that:
//  1. Pre-injected globals (ui) are available without load()
//  2. On-demand loading via @devlore// works
//  3. Loaded names are accessible in function scope (closure)
//  4. Providers not in With() are not in globals
//  5. Loader cache deduplicates factory calls
func TestLoadIntegration(t *testing.T) {
	// Point WorkDir at the testdata directory so starcode.capture finds .star files.
	testdataDir, err := filepath.Abs("testdata")
	if err != nil {
		t.Fatalf("resolving testdata: %v", err)
	}

	bs := loreStar.NewBindingSet(op.BindingConfig{
		Writer:      &bytes.Buffer{},
		ProgramName: "test",
		Color:       false,
		WorkDir:     testdataDir,
	}).With("ui")

	graph := &op.Graph{}
	reg := op.NewActionRegistry()
	globals := bs.BuildGlobals(graph, "test-project", reg)

	thread := &starlark.Thread{
		Name: "load-integration-test",
		Print: func(_ *starlark.Thread, msg string) {
			t.Logf("[star] %s", msg)
		},
	}
	bs.ConfigureThread(thread, graph, "test-project", reg)

	scriptPath := filepath.Join(testdataDir, "load_test.star")
	data, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("reading script: %v", err)
	}

	opts := &syntax.FileOptions{
		Set:             true,
		GlobalReassign:  true,
		TopLevelControl: true,
	}

	result, err := starlark.ExecFileOptions(opts, thread, scriptPath, data, globals)
	if err != nil {
		t.Fatalf("exec script: %v", err)
	}

	assertStarlarkBool(t, result, "result_ui_available")
	assertStarlarkBool(t, result, "result_load_worked")
	assertStarlarkBool(t, result, "result_closure_works")
	assertStarlarkBool(t, result, "result_plan_not_injected")
	assertStarlarkIntGE(t, result, "result_file_count", 1)
}

// TestLoadIntegrationUnknownModule verifies that loading an unknown module produces a clear error.
func TestLoadIntegrationUnknownModule(t *testing.T) {
	bs := loreStar.NewBindingSet(op.BindingConfig{
		Writer:      &bytes.Buffer{},
		ProgramName: "test",
		Color:       false,
	})

	graph := &op.Graph{}
	reg := op.NewActionRegistry()

	thread := &starlark.Thread{
		Name: "unknown-module-test",
		Print: func(_ *starlark.Thread, msg string) {
			t.Logf("[star] %s", msg)
		},
	}
	bs.ConfigureThread(thread, graph, "test-project", reg)

	script := `load("@devlore//nonexistent_provider", "nonexistent_provider")`

	opts := &syntax.FileOptions{
		Set:            true,
		GlobalReassign: true,
	}

	_, err := starlark.ExecFileOptions(opts, thread, "test.star", []byte(script), starlark.StringDict{})
	if err == nil {
		t.Fatal("expected error for unknown provider, got nil")
	}
}

// TestLoadIntegrationBadPrefix verifies that a non-@devlore// load fails.
func TestLoadIntegrationBadPrefix(t *testing.T) {
	bs := loreStar.NewBindingSet(op.BindingConfig{
		Writer:      &bytes.Buffer{},
		ProgramName: "test",
		Color:       false,
	})

	graph := &op.Graph{}
	reg := op.NewActionRegistry()

	thread := &starlark.Thread{
		Name: "bad-prefix-test",
		Print: func(_ *starlark.Thread, msg string) {
			t.Logf("[star] %s", msg)
		},
	}
	bs.ConfigureThread(thread, graph, "test-project", reg)

	script := `load("@stdlib//json", "json")`

	opts := &syntax.FileOptions{
		Set:            true,
		GlobalReassign: true,
	}

	_, err := starlark.ExecFileOptions(opts, thread, "test.star", []byte(script), starlark.StringDict{})
	if err == nil {
		t.Fatal("expected error for non-@devlore// prefix, got nil")
	}
}

func assertStarlarkBool(t *testing.T, globals starlark.StringDict, key string) {
	t.Helper()
	v, ok := globals[key]
	if !ok {
		t.Errorf("missing global %q", key)
		return
	}
	b, ok := v.(starlark.Bool)
	if !ok {
		t.Errorf("%s: expected Bool, got %s", key, v.Type())
		return
	}
	if !bool(b) {
		t.Errorf("%s = false, want true", key)
	}
}

func assertStarlarkIntGE(t *testing.T, globals starlark.StringDict, key string, minimum int) {
	t.Helper()
	v, ok := globals[key]
	if !ok {
		t.Errorf("missing global %q", key)
		return
	}
	i, ok := v.(starlark.Int)
	if !ok {
		t.Errorf("%s: expected Int, got %s", key, v.Type())
		return
	}
	n, _ := i.Int64()
	if int(n) < minimum {
		t.Errorf("%s = %d, want >= %d", key, n, minimum)
	}
}
