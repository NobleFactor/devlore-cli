// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package git_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	appnetprov "github.com/NobleFactor/devlore-cli/pkg/op/provider/appnet"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/file"
	gitprov "github.com/NobleFactor/devlore-cli/pkg/op/provider/git"
	gitgen "github.com/NobleFactor/devlore-cli/pkg/op/provider/git/gen"

	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/appnet/gen" // register appnet.Resource constructor
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/file/gen"   // register file.Resource constructor
)

func TestMain(m *testing.M) {
	op.InitAll(op.NewActionRegistry(), op.Context{})
	os.Exit(m.Run())
}

// createBareRepo creates a bare git repo with one commit on "main".
func createBareRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	work := filepath.Join(dir, "work")
	bare := filepath.Join(dir, "bare.git")

	run := func(args ...string) {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = work
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}

	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}

	run("git", "init", "-b", "main")
	run("git", "config", "user.email", "test@test.com")
	run("git", "config", "user.name", "Test")

	if err := os.WriteFile(filepath.Join(work, "README.md"), []byte("# test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("git", "add", "README.md")
	run("git", "commit", "--no-verify", "-m", "initial")

	// Clone to bare.
	cmd := exec.Command("git", "clone", "--bare", work, bare)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("git clone --bare: %v", err)
	}

	return bare
}

func testCtx(t *testing.T) op.Context {
	t.Helper()
	return op.Context{
		ContextBase: op.ContextBase{
			Context: context.Background(),
			Writer:  &bytes.Buffer{},
		},
	}
}

// region Starlark integration

func TestStarlark(t *testing.T) {
	bareRepo := createBareRepo(t)
	cloneDest := filepath.Join(t.TempDir(), "cloned")

	ctx := testCtx(t)
	p := gitprov.NewProvider(ctx)
	receiver := op.WrapProviderInExecutingReceiver(gitgen.Receiver, p)

	globals := starlark.StringDict{
		"git_prov":        receiver,
		"test_repo_url":   starlark.String(bareRepo),
		"test_clone_dest": starlark.String(cloneDest),
	}

	thread := &starlark.Thread{
		Name:  "git-integration",
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

	// Verify clone actually happened.
	if _, err := os.Stat(filepath.Join(cloneDest, "README.md")); err != nil {
		t.Errorf("README.md not found in clone: %v", err)
	}
}

// endregion

// region Action dispatch

func TestActions_Clone(t *testing.T) {
	bareRepo := createBareRepo(t)
	cloneDest := filepath.Join(t.TempDir(), "action_cloned")

	ctx := testCtx(t)
	reg := op.NewActionRegistry()
	op.RegisterActions(reg, gitgen.Receiver, gitgen.Params)

	a, ok := reg.Get("git.clone")
	if !ok {
		t.Fatal("action git.clone not registered")
	}

	urlRes, err := appnetprov.ResourceFromValue(bareRepo)
	if err != nil {
		t.Fatalf("ResourceFromValue: %v", err)
	}

	result, complement, doErr := a.Do(&ctx, map[string]any{
		"url":         urlRes,
		"destination": file.NewResource(cloneDest),
	})
	if doErr != nil {
		t.Fatalf("Do() error = %v", doErr)
	}

	res, ok := result.(gitprov.Resource)
	if !ok {
		t.Fatalf("result type = %T, want git.Resource", result)
	}
	if res.ClonePath != cloneDest {
		t.Errorf("ClonePath = %q, want %q", res.ClonePath, cloneDest)
	}
	if complement == nil {
		t.Error("complement = nil, want non-nil tombstone")
	}

	// Verify clone.
	if _, err := os.Stat(filepath.Join(cloneDest, "README.md")); err != nil {
		t.Errorf("README.md not found: %v", err)
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

// endregion
