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
	uigen "github.com/NobleFactor/devlore-cli/pkg/op/provider/ui/gen"

	// Ensure providers are registered via init().
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/starcode/gen"
)

// TestLoadIntegration executes a .star script that uses load("@devlore//starcode") and verifies that:
//  1. Pre-injected globals (ui) are available without load()
//  2. On-demand loading via @devlore// works
//  3. Loaded names are accessible in function scope (closure)
//  4. Providers not in With() are not in globals
//  5. Loader cache deduplicates factory calls
func TestLoadIntegration(t *testing.T) {

	rt := loreStar.NewRuntime(
		op.NewBindingConfig("test").
			WithReceivers(uigen.Receiver).
			WithWriter(&bytes.Buffer{}),
	)

	testdataDir, err := filepath.Abs("testdata")
	if err != nil {
		t.Fatalf("abs testdata: %v", err)
	}

	graph := &op.Graph{}
	reg := op.NewActionRegistry()
	ctx := op.Context{ContextBase: op.ContextBase{Root: op.NewRootReader(testdataDir)}}
	rt.RegisterActions(reg, ctx)
	globals := rt.BuildGlobals(graph, "test-project", reg)

	thread := &starlark.Thread{
		Name: "load-integration-test",
		Print: func(_ *starlark.Thread, msg string) {
			t.Logf("[star] %s", msg)
		},
	}
	rt.ConfigureThread(thread, graph, "test-project", reg)

	scriptPath := filepath.Join("testdata", "load_test.star")
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
	rt := loreStar.NewRuntime(
		op.NewBindingConfig("test").
			WithWriter(&bytes.Buffer{}),
	)

	graph := &op.Graph{}
	reg := op.NewActionRegistry()

	thread := &starlark.Thread{
		Name: "unknown-module-test",
		Print: func(_ *starlark.Thread, msg string) {
			t.Logf("[star] %s", msg)
		},
	}
	rt.ConfigureThread(thread, graph, "test-project", reg)

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
	rt := loreStar.NewRuntime(
		op.NewBindingConfig("test").
			WithWriter(&bytes.Buffer{}),
	)

	graph := &op.Graph{}
	reg := op.NewActionRegistry()

	thread := &starlark.Thread{
		Name: "bad-prefix-test",
		Print: func(_ *starlark.Thread, msg string) {
			t.Logf("[star] %s", msg)
		},
	}
	rt.ConfigureThread(thread, graph, "test-project", reg)

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
	if !b {
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
