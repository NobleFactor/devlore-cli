// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starcomplexity_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"reflect"

	"github.com/NobleFactor/devlore-cli/pkg/op/bind"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"github.com/NobleFactor/devlore-cli/cmd/star/provider/starcomplexity"
	_ "github.com/NobleFactor/devlore-cli/cmd/star/provider/starcomplexity/gen"
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

func sampleFiles(t *testing.T) []string {
	t.Helper()
	abs, err := filepath.Abs("testdata/sample.star")
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	return []string{abs}
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
	rt, ok := reg.TypeByReflection(reflect.TypeFor[starcomplexity.Provider]())
	if !ok {
		t.Fatal("starcomplexity provider type not registered")
	}
	return rt.(op.ProviderReceiverType)
}

func starlarkFileList(files []string) *starlark.List {
	elems := make([]starlark.Value, len(files))
	for i, f := range files {
		elems[i] = starlark.String(f)
	}
	return starlark.NewList(elems)
}

// region Starlark integration

func TestStarlark(t *testing.T) {
	ctx := testCtx()
	p := starcomplexity.NewProvider(ctx)
	receiver := bind.NewProvider(receiverType(t), p)

	globals := starlark.StringDict{
		"starcomplexity": receiver,
		"test_files":     starlarkFileList(sampleFiles(t)),
	}

	thread := &starlark.Thread{
		Name:  "starcomplexity-integration",
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
	assertIntGE(t, result, "result_file_count", 1)
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

func assertIntGE(t *testing.T, globals starlark.StringDict, key string, minimum int) {
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

// endregion
