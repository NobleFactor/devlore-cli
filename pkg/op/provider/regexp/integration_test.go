// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package regexp_test

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op/bind"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	regexpprov "github.com/NobleFactor/devlore-cli/pkg/op/provider/regexp"
	regexpgen "github.com/NobleFactor/devlore-cli/pkg/op/provider/regexp/gen"
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
	receiver := bind.WrapProviderInExecutingReceiver(regexpgen.Receiver, regexpprov.NewProvider(ctx))

	globals := starlark.StringDict{"regexp": receiver}

	thread := &starlark.Thread{
		Name:  "regexp-integration",
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

	// match
	assertBool(t, result, "result_match_true")
	assertBoolFalse(t, result, "result_match_false")

	// find
	assertStringEQ(t, result, "result_find_first", "42")
	assertStringEQ(t, result, "result_find_none", "")

	// find_all
	assertIntEQ(t, result, "result_find_all_count", 2) // "42" and "3"

	// find_submatch
	assertStringEQ(t, result, "result_submatch_full", "42 lazy")
	assertStringEQ(t, result, "result_submatch_group1", "42")
	assertStringEQ(t, result, "result_submatch_group2", "lazy")

	// find_all_submatch
	assertIntEQ(t, result, "result_all_submatch_count", 2)

	// replace
	assertStringContains(t, result, "result_replace", "NUM")

	// replace_literal
	assertStringEQ(t, result, "result_replace_literal", "aXbXcX")

	// split
	assertIntEQ(t, result, "result_split_count", 3)
}

// endregion

// region Action dispatch

func TestActions_Match(t *testing.T) {
	ctx := testCtx()
	reg := op.NewActionRegistry()
	bind.RegisterActions(reg, regexpgen.Receiver, regexpgen.Params)

	a, ok := reg.Get("regexp.match")
	if !ok {
		t.Fatal("action regexp.match not registered")
	}

	result, _, err := a.Do(&ctx, map[string]any{"pattern": `\d+`, "text": "abc123"})
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if result != true {
		t.Errorf("result = %v, want true", result)
	}
}

func TestActions_Find(t *testing.T) {
	ctx := testCtx()
	reg := op.NewActionRegistry()
	bind.RegisterActions(reg, regexpgen.Receiver, regexpgen.Params)

	a, ok := reg.Get("regexp.find")
	if !ok {
		t.Fatal("action regexp.find not registered")
	}

	result, _, err := a.Do(&ctx, map[string]any{"pattern": `\d+`, "text": "abc123def"})
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if result != "123" {
		t.Errorf("result = %v, want 123", result)
	}
}

func TestActions_Replace(t *testing.T) {
	ctx := testCtx()
	reg := op.NewActionRegistry()
	bind.RegisterActions(reg, regexpgen.Receiver, regexpgen.Params)

	a, ok := reg.Get("regexp.replace")
	if !ok {
		t.Fatal("action regexp.replace not registered")
	}

	result, _, err := a.Do(&ctx, map[string]any{"pattern": `\d+`, "text": "a1b2", "replacement": "X"})
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if result != "aXbX" {
		t.Errorf("result = %v, want aXbX", result)
	}
}

func TestActions_Split(t *testing.T) {
	ctx := testCtx()
	reg := op.NewActionRegistry()
	bind.RegisterActions(reg, regexpgen.Receiver, regexpgen.Params)

	a, ok := reg.Get("regexp.split")
	if !ok {
		t.Fatal("action regexp.split not registered")
	}

	result, _, err := a.Do(&ctx, map[string]any{"pattern": `,`, "text": "a,b,c", "count": -1})
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	parts, ok := result.([]string)
	if !ok {
		t.Fatalf("result type = %T, want []string", result)
	}
	if len(parts) != 3 {
		t.Errorf("len(result) = %d, want 3", len(parts))
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

func assertBoolFalse(t *testing.T, globals starlark.StringDict, key string) {
	t.Helper()
	v, ok := globals[key]
	if !ok {
		t.Errorf("missing global %q", key)
		return
	}
	if v != starlark.False {
		t.Errorf("%s = %v, want false", key, v)
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
	if !bytes.Contains([]byte(string(s)), []byte(substr)) {
		t.Errorf("%s = %q, want to contain %q", key, string(s), substr)
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
