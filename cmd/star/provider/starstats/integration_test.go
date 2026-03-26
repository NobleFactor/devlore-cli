// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starstats_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op/bind"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"github.com/NobleFactor/devlore-cli/cmd/star/provider/starstats"
	starstatsgen "github.com/NobleFactor/devlore-cli/cmd/star/provider/starstats/gen"
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

func TestMain(m *testing.M) {
	op.InitAll(op.NewActionRegistry(), op.Context{})
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

func testCtx() op.Context {
	return op.Context{
		ContextBase: op.ContextBase{
			Context: context.Background(),
			Writer:  &bytes.Buffer{},
		},
	}
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
	p := starstats.NewProvider(ctx)
	receiver := bind.WrapProviderInExecutingReceiver(starstatsgen.Receiver, p)

	globals := starlark.StringDict{
		"starstats":  receiver,
		"test_files": starlarkFileList(sampleFiles(t)),
	}

	thread := &starlark.Thread{
		Name:  "starstats-integration",
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
	assertIntGE(t, result, "result_total_bytes", 1)
	assertIntGE(t, result, "result_total_loc", 1)
}

// endregion

// region Action dispatch

func TestActions_ComputeStats(t *testing.T) {
	ctx := testCtx()
	reg := op.NewActionRegistry()
	bind.RegisterActions(reg, starstatsgen.Receiver, starstatsgen.Params)

	a, ok := reg.Get("starstats.compute_stats")
	if !ok {
		t.Fatal("action starstats.compute_stats not registered")
	}

	result, _, err := a.Do(&ctx, map[string]any{
		"files":      sampleFiles(t),
		"with_bytes": true,
		"with_loc":   true,
	})
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}

	st, ok := result.(*starstats.Stats)
	if !ok {
		t.Fatalf("result type = %T, want *starstats.Stats", result)
	}
	if st.Totals.FileCount < 1 {
		t.Errorf("file_count = %d, want >= 1", st.Totals.FileCount)
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
