// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package file

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/mem"
)

// makeCallableResource creates a compiled CallableResource from source.
func makeCallableResource(t *testing.T, source string) op.CallableResource {
	t.Helper()
	c := mem.NewCallable("file.Reducer", "test_reducer")
	c.SetSource([]byte(source))
	if err := c.Compile(); err != nil {
		t.Fatalf("Compile: %v", err)
	}
	return c
}

func TestWalkTreeAction_Integration(t *testing.T) {
	// Create a temp directory with a few files.
	dir := t.TempDir()
	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("content"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Create a callable that collects relative paths.
	callable := makeCallableResource(t, `def _callable(initial, resource, relative_path, stack):
    if initial == None:
        return [relative_path]
    return initial + [relative_path]
`)

	// Register reflected actions with fn in params.
	p := &Provider{}
	reg := op.NewActionRegistry()
	params := op.MethodParams{
		"WalkTree":           {"root", "fn", "honor_gitignore"},
		"CompensateWalkTree": nil, // needed to avoid orphan panic
	}
	op.RegisterReflectedActions(reg, "file", p, params)

	action, ok := reg.Get("file.walk_tree")
	if !ok {
		t.Fatal("file.walk_tree not registered")
	}

	// Verify it's compensable.
	ca, ok := action.(op.CompensableAction)
	if !ok {
		t.Fatal("file.walk_tree is not CompensableAction")
	}

	thread := &starlark.Thread{Name: "test"}
	ctx := &op.Context{
		Context: context.Background(),
		Thread:  thread,
		Writer:  &bytes.Buffer{},
	}

	result, complement, err := action.Do(ctx, map[string]any{
		"root":            dir,
		"fn":              callable,
		"honor_gitignore": false,
	})
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}

	// Verify collected paths.
	paths, ok := result.([]string)
	if !ok {
		t.Fatalf("result type = %T, want []string", result)
	}
	sort.Strings(paths)
	want := []string{"a.txt", "b.txt", "c.txt"}
	if len(paths) != len(want) {
		t.Fatalf("paths = %v, want %v", paths, want)
	}
	for i, got := range paths {
		if got != want[i] {
			t.Errorf("paths[%d] = %q, want %q", i, got, want[i])
		}
	}

	// Verify Undo(nil) is safe.
	if err := ca.Undo(ctx, nil); err != nil {
		t.Errorf("Undo(nil) = %v, want nil", err)
	}

	// Verify complement is a RecoveryStack (or nil for no-ops).
	if complement != nil {
		if _, ok := complement.(*op.RecoveryStack); !ok {
			t.Errorf("complement type = %T, want *op.RecoveryStack", complement)
		}
	}
}

func TestWalkTreeAction_DryRun(t *testing.T) {
	p := &Provider{}
	reg := op.NewActionRegistry()
	params := op.MethodParams{
		"WalkTree":           {"root", "fn", "honor_gitignore"},
		"CompensateWalkTree": nil,
	}
	op.RegisterReflectedActions(reg, "file", p, params)

	action, ok := reg.Get("file.walk_tree")
	if !ok {
		t.Fatal("file.walk_tree not registered")
	}

	callable := makeCallableResource(t, `def _callable(initial, resource, path, stack):
    return initial
`)

	var buf bytes.Buffer
	ctx := &op.Context{
		Context: context.Background(),
		Thread:  &starlark.Thread{Name: "test"},
		DryRun:  true,
		Writer:  &buf,
	}

	result, _, err := action.Do(ctx, map[string]any{
		"root":            "/tmp/test",
		"fn":              callable,
		"honor_gitignore": true,
	})
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if result != nil {
		t.Errorf("dry-run result = %v, want nil", result)
	}
	if buf.Len() == 0 {
		t.Error("dry-run produced no output")
	}
}
