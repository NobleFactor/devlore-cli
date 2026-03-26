// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package file_test

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op/bind"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/file"
	filegen "github.com/NobleFactor/devlore-cli/pkg/op/provider/file/gen"
)

func TestMain(m *testing.M) {
	op.InitAll(op.NewActionRegistry(), op.Context{})
	os.Exit(m.Run())
}

func testCtx(t *testing.T) (ctx op.Context, dir string) {
	t.Helper()
	dir = t.TempDir()
	root := op.NewRootReaderWriter(dir)
	ctx = op.Context{
		ContextBase: op.ContextBase{
			Context: context.Background(),
			Writer:  &bytes.Buffer{},
			Root:    root,
		},
	}
	ctx.RecoverySite = op.NewRecoverySite(ctx)
	return ctx, dir
}

// region Starlark integration

func TestStarlark(t *testing.T) {
	ctx, dir := testCtx(t)
	p := file.NewProvider(ctx)
	receiver := bind.WrapProviderInExecutingReceiver(filegen.Receiver, p)

	globals := starlark.StringDict{
		"file":     receiver,
		"test_dir": starlark.String(dir),
	}

	thread := &starlark.Thread{
		Name:  "file-integration",
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

	// path utilities
	assertStringContains(t, result, "result_join", "sub")
	assertStringEQ(t, result, "result_name", "c.txt")
	assertStringEQ(t, result, "result_parent", "/a/b")
	assertStringEQ(t, result, "result_root", dir)

	// mkdir + is_dir
	assertBool(t, result, "result_is_dir")

	// write_text + read_text + exists + is_file
	assertBool(t, result, "result_exists")
	assertBool(t, result, "result_is_file")
	assertStringEQ(t, result, "result_read", "hello world")

	// copy
	assertBool(t, result, "result_copy_exists")
	assertStringEQ(t, result, "result_copy_read", "hello world")

	// link
	assertBool(t, result, "result_link_exists")

	// remove
	assertBool(t, result, "result_removed")

	// defaults: write_text without mode
	assertStringEQ(t, result, "result_defaults_write", "default mode")

	// defaults: mkdir without mode
	assertBool(t, result, "result_defaults_mkdir")

	// defaults: glob without honor_gitignore
	assertListNotEmpty(t, result, "result_defaults_glob")

	// defaults: remove without prune/boundary
	assertBool(t, result, "result_defaults_remove")

	// find: recursive ** pattern
	assertListLen(t, result, "result_find_go", 2)  // top.go + deep/deep.go
	assertListLen(t, result, "result_find_md", 1)  // deep/notes.md
	assertListLen(t, result, "result_find_all", 3) // all three files
}

// endregion

// region Action dispatch

func TestActions_WriteText_ReadText(t *testing.T) {
	ctx, dir := testCtx(t)
	reg := op.NewActionRegistry()
	bind.RegisterActions(reg, filegen.Receiver, filegen.Params)

	// write_text
	writeAction, ok := reg.Get("file.write_text")
	if !ok {
		t.Fatal("action file.write_text not registered")
	}

	dest := file.NewResource(dir + "/action_test.txt")
	_, _, err := writeAction.Do(&ctx, map[string]any{
		"destination": dest,
		"content":     "action content",
		"mode":        os.FileMode(0o644),
	})
	if err != nil {
		t.Fatalf("write_text Do() error = %v", err)
	}

	// read_text
	readAction, ok := reg.Get("file.read_text")
	if !ok {
		t.Fatal("action file.read_text not registered")
	}

	result, _, err := readAction.Do(&ctx, map[string]any{
		"resource": dest,
	})
	if err != nil {
		t.Fatalf("read_text Do() error = %v", err)
	}

	s, ok := result.(string)
	if !ok {
		t.Fatalf("result type = %T, want string", result)
	}
	if s != "action content" {
		t.Errorf("result = %q, want 'action content'", s)
	}
}

func TestActions_Exists(t *testing.T) {
	ctx, dir := testCtx(t)
	reg := op.NewActionRegistry()
	bind.RegisterActions(reg, filegen.Receiver, filegen.Params)

	a, ok := reg.Get("file.exists")
	if !ok {
		t.Fatal("action file.exists not registered")
	}

	// Existing directory.
	result, _, err := a.Do(&ctx, map[string]any{"resource": file.NewResource(dir)})
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if result != true {
		t.Errorf("exists(%s) = %v, want true", dir, result)
	}

	// Non-existing path.
	result, _, err = a.Do(&ctx, map[string]any{"resource": file.NewResource(dir + "/nope")})
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if result != false {
		t.Errorf("exists(nope) = %v, want false", result)
	}
}

func TestActions_Join(t *testing.T) {
	ctx, _ := testCtx(t)
	reg := op.NewActionRegistry()
	bind.RegisterActions(reg, filegen.Receiver, filegen.Params)

	a, ok := reg.Get("file.join")
	if !ok {
		t.Fatal("action file.join not registered")
	}

	result, _, err := a.Do(&ctx, map[string]any{"parts": []string{"a", "b", "c.txt"}})
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}

	s, ok := result.(string)
	if !ok {
		t.Fatalf("result type = %T, want string", result)
	}
	if s != "a/b/c.txt" {
		t.Errorf("result = %q, want 'a/b/c.txt'", s)
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

func assertListLen(t *testing.T, globals starlark.StringDict, key string, want int) {
	t.Helper()
	v, ok := globals[key]
	if !ok {
		t.Errorf("missing global %q", key)
		return
	}
	list, ok := v.(*starlark.List)
	if !ok {
		t.Errorf("%s: expected List, got %s", key, v.Type())
		return
	}
	if list.Len() != want {
		t.Errorf("%s: len = %d, want %d", key, list.Len(), want)
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

func assertListNotEmpty(t *testing.T, globals starlark.StringDict, key string) {
	t.Helper()
	v, ok := globals[key]
	if !ok {
		t.Errorf("missing global %q", key)
		return
	}
	list, ok := v.(*starlark.List)
	if !ok {
		t.Errorf("%s: expected List, got %s", key, v.Type())
		return
	}
	if list.Len() == 0 {
		t.Errorf("%s: list is empty, want non-empty", key)
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

// endregion
