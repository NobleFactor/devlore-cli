// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package json_test

import (
	"bytes"
	"context"
	"os"
	"testing"

	"reflect"

	"github.com/NobleFactor/devlore-cli/pkg/op/bind"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	jsonprov "github.com/NobleFactor/devlore-cli/pkg/op/provider/json"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/json/gen"
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
	rt, ok := reg.TypeByReflection(reflect.TypeFor[jsonprov.Provider]())
	if !ok {
		t.Fatal("json provider type not registered")
	}
	return rt.(op.ProviderReceiverType)
}

// region Starlark integration

func TestStarlark(t *testing.T) {
	ctx := testCtx()
	receiver := bind.NewProvider(receiverType(t), jsonprov.NewProvider(ctx))

	globals := starlark.StringDict{"json": receiver}

	thread := &starlark.Thread{
		Name:  "json-integration",
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
	assertStringEQ(t, result, "result_encode_type", "string")
	assertBool(t, result, "result_encode_has_name")
	assertStringEQ(t, result, "result_decode_color", "blue")
	assertFloatEQ(t, result, "result_decode_count", 42)
	assertStringEQ(t, result, "result_indent_type", "string")
	assertBool(t, result, "result_indent_has_newline")
	assertStringEQ(t, result, "result_roundtrip_key", "value")
	assertIntEQ(t, result, "result_roundtrip_list_len", 3)
}

// endregion

// region Action dispatch

func TestActions_Encode(t *testing.T) {
	ctx := testCtx()

	a, err := ctx.ActionByName("json.encode")
	if err != nil {
		t.Fatalf("action json.encode not registered: %v", err)
	}

	result, _, err := a.Do(ctx, map[string]any{"value": map[string]any{"key": "val"}})
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}

	s, ok := result.(string)
	if !ok {
		t.Fatalf("result type = %T, want string", result)
	}
	if s == "" {
		t.Error("result is empty")
	}
}

func TestActions_Decode(t *testing.T) {
	ctx := testCtx()

	a, err := ctx.ActionByName("json.decode")
	if err != nil {
		t.Fatalf("action json.decode not registered: %v", err)
	}

	result, _, err := a.Do(ctx, map[string]any{"data": `{"color":"red"}`})
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}

	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map[string]any", result)
	}
	if m["color"] != "red" {
		t.Errorf("color = %v, want red", m["color"])
	}
}

func TestActions_EncodeIndent(t *testing.T) {
	ctx := testCtx()

	a, err := ctx.ActionByName("json.encode_indent")
	if err != nil {
		t.Fatalf("action json.encode_indent not registered: %v", err)
	}

	result, _, err := a.Do(ctx, map[string]any{"value": map[string]any{"a": 1}, "indent": "  "})
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}

	s, ok := result.(string)
	if !ok {
		t.Fatalf("result type = %T, want string", result)
	}
	if s == "" {
		t.Error("result is empty")
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

func assertFloatEQ(t *testing.T, globals starlark.StringDict, key string, want float64) {
	t.Helper()
	v, ok := globals[key]
	if !ok {
		t.Errorf("missing global %q", key)
		return
	}
	f, ok := v.(starlark.Float)
	if !ok {
		t.Errorf("%s: expected Float, got %s", key, v.Type())
		return
	}
	if float64(f) != want {
		t.Errorf("%s = %f, want %f", key, float64(f), want)
	}
}

func assertIntEQ(t *testing.T, globals starlark.StringDict, key string, want int) {
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
	if int(n) != want {
		t.Errorf("%s = %d, want %d", key, n, want)
	}
}

// endregion
