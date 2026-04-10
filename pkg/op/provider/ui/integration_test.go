// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package ui_test

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"reflect"

	"github.com/NobleFactor/devlore-cli/pkg/op/bind"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/ui"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/ui/gen"
)

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

func testCtx() (*op.ExecutionContext, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	ctx := &op.ExecutionContext{
		Context:     context.Background(),
		Writer:      buf,
		ProgramName: "test",
		Registry:    op.NewReceiverRegistry(),
	}
	return ctx, buf
}

func receiverType(t *testing.T) op.ProviderReceiverType {
	t.Helper()
	reg := op.NewReceiverRegistry()
	rt, ok := reg.TypeByReflection(reflect.TypeFor[ui.Provider]())
	if !ok {
		t.Fatal("ui provider type not registered")
	}
	return rt.(op.ProviderReceiverType)
}

// region Starlark integration

func TestStarlark(t *testing.T) {
	ctx, buf := testCtx()
	p := ui.NewProvider(ctx)
	p.Color = false // disable ANSI for easier assertion
	receiver := bind.NewProvider(receiverType(t), p)

	globals := starlark.StringDict{"ui": receiver}

	thread := &starlark.Thread{
		Name:  "ui-integration",
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
	assertBool(t, result, "result_note_is_none")
	assertBool(t, result, "result_success_is_none")
	assertBool(t, result, "result_warn_is_none")
	assertBool(t, result, "result_error_is_none")

	// Verify output was written to the buffer.
	output := buf.String()
	if !strings.Contains(output, "hello from note") {
		t.Errorf("output missing note message, got: %q", output)
	}
	if !strings.Contains(output, "operation completed") {
		t.Errorf("output missing success message, got: %q", output)
	}
	if !strings.Contains(output, "something looks off") {
		t.Errorf("output missing warn message, got: %q", output)
	}
	if !strings.Contains(output, "something went wrong") {
		t.Errorf("output missing error message, got: %q", output)
	}
}

// endregion

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

// endregion
