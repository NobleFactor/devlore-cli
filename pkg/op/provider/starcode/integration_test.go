// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starcode_test

import (
	"os"
	"path/filepath"
	"testing"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"github.com/NobleFactor/devlore-cli/pkg/op/provider/starcode"
	starcodegen "github.com/NobleFactor/devlore-cli/pkg/op/provider/starcode/gen"
)

func testdataDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs("testdata")
	if err != nil {
		t.Fatalf("abs testdata: %v", err)
	}
	return dir
}

// TestIntegrationEndToEnd executes a .star script that exercises the full starcode API surface: capture, paths,
// count, index, stats, analyze.
// The script sets result_* globals which this test inspects.
func TestIntegrationEndToEnd(t *testing.T) {
	root := testdataDir(t)

	// Create receiver exactly as init() would, but pointing at testdata
	receiver := starcodegen.NewStarcodeReceiver(&starcode.Provider{Root: root})

	globals := starlark.StringDict{
		"starcode": receiver,
	}

	thread := &starlark.Thread{
		Name: "integration-test",
		Print: func(_ *starlark.Thread, msg string) {
			t.Logf("[star] %s", msg)
		},
	}

	scriptPath := "testdata/integration.star"
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

	// Verify script completed
	assertBool(t, result, "result_done")

	// --- capture ---
	assertIntGE(t, result, "result_count", 1)

	pathsList, ok := result["result_paths"].(*starlark.List)
	if !ok {
		t.Fatal("result_paths is not a list")
	}
	if pathsList.Len() == 0 {
		t.Fatal("result_paths is empty")
	}

	// --- stats ---
	assertIntGE(t, result, "result_stats_file_count", 1)
	assertIntGE(t, result, "result_stats_total_bytes", 1)
	assertIntGE(t, result, "result_stats_total_loc", 1)
	assertIntGE(t, result, "result_stats_total_sloc", 1)
	assertStringNonEmpty(t, result, "result_stats_first_file_path")
	assertIntGE(t, result, "result_stats_first_file_loc", 0)

	// --- index ---
	assertIntGE(t, result, "result_index_file_count", 1)
	assertIntGE(t, result, "result_index_functions", 1)
	assertIntGE(t, result, "result_index_loads", 1)
	assertIntGE(t, result, "result_index_globals", 1)
	assertStringNonEmpty(t, result, "result_index_first_file_path")
	assertIntGE(t, result, "result_index_first_file_fn_count", 0)

	// --- analyze ---
	assertBool(t, result, "result_report_has_stats")
	assertBool(t, result, "result_report_has_complexity")
	assertBool(t, result, "result_report_has_index")
	assertIntGE(t, result, "result_hotspot_count", 1) // threshold=3 should catch several

	// --- analyze without index ---
	assertBool(t, result, "result_report_no_idx_index_is_none")

	// --- index without docstrings/globals ---
	assertIntEQ(t, result, "result_no_doc_globals_count", 0)

	// --- stats bytes-only ---
	assertIntEQ(t, result, "result_bytes_only_loc", 0)

	// --- hotspot fields ---
	assertBool(t, result, "result_hotspot_has_file")
	assertBool(t, result, "result_hotspot_has_name")
	assertBool(t, result, "result_hotspot_has_line")
	assertBool(t, result, "result_hotspot_has_cyclomatic")
}

func assertBool(t *testing.T, globals starlark.StringDict, key string) {
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

func assertStringNonEmpty(t *testing.T, globals starlark.StringDict, key string) {
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
	if string(s) == "" {
		t.Errorf("%s is empty", key)
	}
}
