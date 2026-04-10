// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package shell_test

import (
	"bytes"
	"context"
	"os"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op/bind"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	shellprov "github.com/NobleFactor/devlore-cli/pkg/op/provider/shell"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/shell/gen"
)

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

func testCtx() *op.ExecutionContext {
	return &op.ExecutionContext{
		Context:  context.Background(),
		Writer:   &bytes.Buffer{},
		Registry: op.NewReceiverRegistry(),
	}
}

func receiverType(t *testing.T) op.ProviderReceiverType {
	t.Helper()
	reg := op.NewReceiverRegistry()
	rt, ok := reg.TypeByReflection(reflect.TypeFor[shellprov.Provider]())
	if !ok {
		t.Fatal("shell provider type not registered")
	}
	return rt.(op.ProviderReceiverType)
}

// region Starlark integration

func TestStarlark(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell.exec uses sh -c, skipping on windows")
	}

	ctx := testCtx()
	receiver := bind.NewProvider(receiverType(t), shellprov.NewProvider(ctx))

	globals := starlark.StringDict{"shell": receiver}

	thread := &starlark.Thread{
		Name:  "shell-integration",
		Print: func(_ *starlark.Thread, msg string) { t.Logf("[star] %s", msg) },
	}

	data, err := os.ReadFile("testdata/integration.star")
	if err != nil {
		t.Fatalf("reading script: %v", err)
	}

	opts := &syntax.FileOptions{Set: true, GlobalReassign: true, TopLevelControl: true}
	result, err := starlark.ExecFileOptions(opts, thread, "testdata/integration.star", data, globals)
	if err != nil {
		t.Fatalf("exec script: %v", err)
	}

	assertBool(t, result, "result_done")
	assertStringEQ(t, result, "result_exec", "echo hello")
	assertStringEQ(t, result, "result_exec_type", "string")

	buf := ctx.Writer.(*bytes.Buffer)
	if !strings.Contains(buf.String(), "hello") {
		t.Errorf("output = %q, want to contain 'hello'", buf.String())
	}
}

// endregion

// region Action dispatch

func TestActions_Exec(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell.exec uses sh -c, skipping on windows")
	}

	ctx := testCtx()

	a, err := ctx.ActionByName("shell.exec")
	if err != nil {
		t.Fatalf("action shell.exec not registered: %v", err)
	}

	result, _, err := a.Do(ctx, map[string]any{"command": "echo action_test"})
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}

	s, ok := result.(string)
	if !ok {
		t.Fatalf("result type = %T, want string", result)
	}
	if s != "echo action_test" {
		t.Errorf("result = %q, want 'echo action_test'", s)
	}
	buf := ctx.Writer.(*bytes.Buffer)
	if !strings.Contains(buf.String(), "action_test") {
		t.Errorf("output = %q, want to contain 'action_test'", buf.String())
	}
}

func TestActions_Exec_EmptyCommand(t *testing.T) {
	ctx := testCtx()

	a, err := ctx.ActionByName("shell.exec")
	if err != nil {
		t.Fatalf("action shell.exec not registered: %v", err)
	}

	_, _, err = a.Do(ctx, map[string]any{"command": ""})
	if err == nil {
		t.Fatal("expected error for empty command, got nil")
	}
}

// endregion

// region Assertions

func assertBool(t *testing.T, globals starlark.StringDict, key string) {
	t.Helper()
	v, ok := globals[key]
	if !ok {
		t.Errorf("missing global %q", key)
		return
	}
	if v != starlark.True {
		t.Errorf("%s = %v, want true", key, v)
	}
}

func assertStringEQ(t *testing.T, globals starlark.StringDict, key, want string) {
	t.Helper()
	v, ok := globals[key]
	if !ok {
		t.Errorf("missing global %q", key)
		return
	}
	s, ok := v.(starlark.String)
	if !ok {
		t.Errorf("%s: expected String, got %s", key, v.Type())
		return
	}
	if string(s) != want {
		t.Errorf("%s = %q, want %q", key, string(s), want)
	}
}

// endregion
