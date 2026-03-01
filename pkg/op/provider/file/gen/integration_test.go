// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package file_test

import (
	"os"
	"path/filepath"
	"testing"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	provider "github.com/NobleFactor/devlore-cli/pkg/op/provider/file"
	filegen "github.com/NobleFactor/devlore-cli/pkg/op/provider/file/gen"
)

// TestImmediateBindings executes a .star script that exercises every immediate binding
// exposed by the file provider's generated receiver.
//
// The script sets result_* globals which this test inspects.
func TestImmediateBindings(t *testing.T) {
	t.Skip("https://github.com/NobleFactor/devlore-cli/issues/170")
	tmp := t.TempDir()

	// Create a fixture file for read/exists/is_file tests.
	fixture := filepath.Join(tmp, "fixture.txt")
	if err := os.WriteFile(fixture, []byte("fixture content"), 0o644); err != nil {
		t.Fatalf("creating fixture: %v", err)
	}

	receiver := filegen.NewFileReceiver(&provider.Provider{Root: tmp})

	globals := starlark.StringDict{
		"file":    receiver,
		"tmp_dir": starlark.String(tmp),
		"fixture": starlark.String(fixture),
		"sep":     starlark.String(string(filepath.Separator)),
	}

	thread := &starlark.Thread{
		Name: "immediate-integration-test",
		Print: func(_ *starlark.Thread, msg string) {
			t.Logf("[star] %s", msg)
		},
	}

	scriptPath := "testdata/immediate_test.star"
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

	// Pure functions
	assertBool(t, result, "result_join")
	assertBool(t, result, "result_join_single")
	assertBool(t, result, "result_name")
	assertBool(t, result, "result_parent")
	assertBool(t, result, "result_exists_file")
	assertBool(t, result, "result_exists_missing")
	assertBool(t, result, "result_is_dir_true")
	assertBool(t, result, "result_is_dir_false")
	assertBool(t, result, "result_is_file_true")
	assertBool(t, result, "result_is_file_false")
	assertBool(t, result, "result_mkdir")
	assertBool(t, result, "result_mkdir_nested")
	assertIntGE(t, result, "result_glob_count", 1)
	assertBool(t, result, "result_read_has_path")

	// Compensable actions
	assertBool(t, result, "result_write_text")
	assertBool(t, result, "result_write_bytes")
	assertBool(t, result, "result_write_text_nested")
	assertBool(t, result, "result_link")
	assertBool(t, result, "result_move_dest_exists")
	assertBool(t, result, "result_move_src_gone")
	assertBool(t, result, "result_backup_created")
	assertBool(t, result, "result_backup_src_gone")
	assertBool(t, result, "result_copy")

	// walk_tree
	assertBool(t, result, "result_walk_simple")
	assertIntGE(t, result, "result_walk_file_count", 1)
	assertBool(t, result, "result_walk_has_dirs")
	assertBool(t, result, "result_walk_has_files")

	// Completion sentinel
	assertBool(t, result, "result_done")
}

// TestPlannedBindings executes a .star script that exercises every planned binding
// exposed by the file provider's generated receiver.
//
// Planned bindings do NOT execute file operations — they build an execution graph.
// This test verifies that nodes, slots, and edges are created correctly.
func TestPlannedBindings(t *testing.T) {
	t.Skip("https://github.com/NobleFactor/devlore-cli/issues/171")
	graph := &op.Graph{}
	reg := op.NewActionRegistry()
	op.RegisterReflectedActions(reg, "file", &provider.Provider{}, filegen.Params)

	plan := filegen.NewFilePlan(graph, "test-project", reg)

	globals := starlark.StringDict{
		"file": plan,
	}

	thread := &starlark.Thread{
		Name: "planned-integration-test",
		Print: func(_ *starlark.Thread, msg string) {
			t.Logf("[star] %s", msg)
		},
	}

	scriptPath := "testdata/planned_test.star"
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

	// All bindings return Output type
	assertBool(t, result, "result_write_text_type")
	assertBool(t, result, "result_write_bytes_type")
	assertBool(t, result, "result_link_type")
	assertBool(t, result, "result_move_type")
	assertBool(t, result, "result_backup_type")
	assertBool(t, result, "result_remove_type")
	assertBool(t, result, "result_remove_all_type")
	assertBool(t, result, "result_unlink_type")
	assertBool(t, result, "result_mkdir_type")
	assertBool(t, result, "result_glob_type")
	assertBool(t, result, "result_read_type")
	assertBool(t, result, "result_copy_type")
	assertBool(t, result, "result_exists_type")
	assertBool(t, result, "result_is_dir_type")
	assertBool(t, result, "result_is_file_type")
	assertBool(t, result, "result_name_type")
	assertBool(t, result, "result_parent_type")

	// Promise chaining
	assertBool(t, result, "result_chain_done")
	assertBool(t, result, "result_chain_move_done")

	// Output attributes
	assertBool(t, result, "result_output_has_node_id")

	// Completion sentinel
	assertBool(t, result, "result_done")

	// ── Graph structure verification ──────────────────────────────────────────

	// 17 standalone calls + 4 chained calls = 21 nodes total
	if len(graph.Nodes) != 21 {
		t.Errorf("graph.Nodes = %d, want 21", len(graph.Nodes))
	}

	// Verify action names are present for all expected actions.
	actionCounts := make(map[string]int)
	for _, n := range graph.Nodes {
		actionCounts[n.ActionName()]++
	}

	expectedActions := map[string]int{
		"file.write_text":  3, // 1 standalone + 2 chain sources
		"file.write_bytes": 1,
		"file.copy":        1,
		"file.exists":      1,
		"file.is_dir":      1,
		"file.is_file":     1,
		"file.link":        1,
		"file.move":        2, // 1 standalone + 1 chained
		"file.backup":      2, // 1 standalone + 1 chained
		"file.name":        1,
		"file.parent":      1,
		"file.remove":      1,
		"file.remove_all":  1,
		"file.unlink":      1,
		"file.mkdir":       1,
		"file.glob":        1,
		"file.read":        1,
	}

	for action, want := range expectedActions {
		if got := actionCounts[action]; got != want {
			t.Errorf("action %q: count = %d, want %d", action, got, want)
		}
	}

	// All nodes belong to the test project.
	for _, n := range graph.Nodes {
		if n.Project != "test-project" {
			t.Errorf("node %q: project = %q, want %q", n.ID, n.Project, "test-project")
		}
	}

	// Promise chains create edges.
	// 2 chains: write_text→backup, write_text→move = 2 edges
	if len(graph.Edges) != 2 {
		t.Errorf("graph.Edges = %d, want 2", len(graph.Edges))
	}

	// Verify slot values on the first write_text node.
	firstNode := graph.Nodes[0]
	if firstNode.ActionName() != "file.write_text" {
		t.Errorf("first node action = %q, want %q", firstNode.ActionName(), "file.write_text")
	}
	slots := firstNode.ResolvedSlots(nil)
	if slots["destination"] != "/tmp/planned.txt" {
		t.Errorf("write_text destination slot = %v, want %q", slots["destination"], "/tmp/planned.txt")
	}
	if slots["content"] != "hello planned" {
		t.Errorf("write_text content slot = %v, want %q", slots["content"], "hello planned")
	}
	if slots["mode"] != 0o644 {
		t.Errorf("write_text mode slot = %v, want %d", slots["mode"], 0o644)
	}
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
