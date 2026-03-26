// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package yaml_test

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op/bind"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	yamlprov "github.com/NobleFactor/devlore-cli/pkg/op/provider/yaml"
	yamlgen "github.com/NobleFactor/devlore-cli/pkg/op/provider/yaml/gen"
)

func TestMain(m *testing.M) {
	op.InitAll(op.NewActionRegistry(), op.Context{})
	os.Exit(m.Run())
}

func testCtx() op.Context {
	return op.Context{
		ContextBase: op.ContextBase{
			Context: context.Background(),
			Writer:  &bytes.Buffer{},
		},
	}
}

// region Starlark integration

func TestStarlark(t *testing.T) {
	ctx := testCtx()
	receiver := bind.WrapProviderInExecutingReceiver(yamlgen.Receiver, yamlprov.NewProvider(ctx))

	globals := starlark.StringDict{"yaml": receiver}

	thread := &starlark.Thread{
		Name:  "yaml-integration",
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
	assertIntEQ(t, result, "result_decode_count", 42)
	assertStringEQ(t, result, "result_roundtrip_key", "value")
}

// endregion

// region Action dispatch

func TestActions_Encode(t *testing.T) {
	ctx := testCtx()
	reg := op.NewActionRegistry()
	bind.RegisterActions(reg, yamlgen.Receiver, yamlgen.Params)

	a, ok := reg.Get("yaml.encode")
	if !ok {
		t.Fatal("action yaml.encode not registered")
	}

	result, _, err := a.Do(&ctx, map[string]any{"value": map[string]any{"key": "val"}})
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
	reg := op.NewActionRegistry()
	bind.RegisterActions(reg, yamlgen.Receiver, yamlgen.Params)

	a, ok := reg.Get("yaml.decode")
	if !ok {
		t.Fatal("action yaml.decode not registered")
	}

	result, _, err := a.Do(&ctx, map[string]any{"data": "color: green\n"})
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}

	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map[string]any", result)
	}
	if m["color"] != "green" {
		t.Errorf("color = %v, want green", m["color"])
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
