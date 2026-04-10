// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package template_test

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
	templateprov "github.com/NobleFactor/devlore-cli/pkg/op/provider/template"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/template/gen"
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
	rt, ok := reg.TypeByReflection(reflect.TypeFor[templateprov.Provider]())
	if !ok {
		t.Fatal("template provider type not registered")
	}
	return rt.(op.ProviderReceiverType)
}

// region Starlark integration

func TestStarlark(t *testing.T) {
	ctx := testCtx()
	receiver := bind.NewProvider(receiverType(t), templateprov.NewProvider(ctx))

	globals := starlark.StringDict{"template": receiver}

	thread := &starlark.Thread{
		Name:  "template-integration",
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
	assertStringContains(t, result, "result_text", "Alice")
	assertStringContains(t, result, "result_text", "project=test")
	assertStringEQ(t, result, "result_static", "static")
}

// endregion

// region Action dispatch

func TestActions_RenderText(t *testing.T) {
	ctx := testCtx()

	a, err := ctx.ActionByName("template.render_text")
	if err != nil {
		t.Fatalf("action template.render_text not registered: %v", err)
	}

	result, _, err := a.Do(ctx, map[string]any{
		"content": "Hello {{ .Name }}",
		"data":    map[string]any{"Name": "Bob"},
	})
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}

	s, ok := result.(string)
	if !ok {
		t.Fatalf("result type = %T, want string", result)
	}
	if !strings.Contains(s, "Bob") {
		t.Errorf("result = %q, want to contain 'Bob'", s)
	}
}

func TestActions_RenderBytes(t *testing.T) {
	ctx := testCtx()

	a, err := ctx.ActionByName("template.render_bytes")
	if err != nil {
		t.Fatalf("action template.render_bytes not registered: %v", err)
	}

	result, _, err := a.Do(ctx, map[string]any{
		"content": []byte("val={{ .X }}"),
		"data":    map[string]any{"X": "99"},
	})
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}

	b, ok := result.([]byte)
	if !ok {
		t.Fatalf("result type = %T, want []byte", result)
	}
	if !strings.Contains(string(b), "99") {
		t.Errorf("result = %q, want to contain '99'", string(b))
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

func assertStringContains(t *testing.T, globals starlark.StringDict, key, substr string) {
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
	if !strings.Contains(string(s), substr) {
		t.Errorf("%s = %q, want to contain %q", key, string(s), substr)
	}
}

// endregion
