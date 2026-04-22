// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package git

import (
	"errors"
	"os"
	"reflect"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// newTestProvider returns a Provider whose ExecutionContext has Root anchored at "/" and whose cloneFn hook
// is replaced with the supplied function. Tests use the hook to capture the argv that would have been passed
// to `git clone` without executing the real binary.
//
// Parameters:
//   - t:    the test harness.
//   - hook: the test-only replacement for doClone's exec path; nil means fall through to the real binary
//     (tests never do this).
//
// Returns:
//   - *Provider: the initialized provider bound to a root-anchored execution context.
func newTestProvider(t *testing.T, hook func(args []string) error) *Provider {
	t.Helper()
	return &Provider{
		ProviderBase: op.NewProviderBase(&op.ExecutionContext{Root: op.NewRootReaderWriter("/")}),
		cloneFn:      hook,
	}
}

// --- Clone ---

func TestClone_HookReceivesArgv(t *testing.T) {

	var gotArgs []string
	p := newTestProvider(t, func(args []string) error {
		gotArgs = args
		return nil
	})

	const repo = "https://example.com/repo.git"
	const dir = "/tmp/clone-dest"

	result, state, err := p.Clone(repo, dir, false, "", 0, "", false, false, "", false, false, nil)
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}

	want := []string{"clone", repo, dir}
	if !reflect.DeepEqual(gotArgs, want) {
		t.Errorf("cloneFn args =\n  got: %q\n want: %q", gotArgs, want)
	}
	if result.SourcePath.Abs() != dir {
		t.Errorf("result.SourcePath.Abs() = %q, want %q", result.SourcePath.Abs(), dir)
	}

	if state == nil {
		t.Fatalf("state = nil, want a *Resource")
	}
	if state.SourcePath.Abs() != dir {
		t.Errorf("state.SourcePath.Abs() = %q, want %q", state.SourcePath.Abs(), dir)
	}
}

func TestClone_HookPropagatesError(t *testing.T) {

	hookErr := errors.New("clone failed")
	p := newTestProvider(t, func(_ []string) error {
		return hookErr
	})

	result, state, err := p.Clone(
		"https://example.com/repo.git", "/tmp/dest",
		false, "", 0, "", false, false, "", false, false, nil,
	)
	if !errors.Is(err, hookErr) {
		t.Fatalf("Clone error = %v, want %v", err, hookErr)
	}
	if result != nil {
		t.Errorf("result = %v, want nil", result)
	}
	if state != nil {
		t.Errorf("state = %v, want nil", state)
	}
}

func TestClone_DirectoryDerivedFromRepository(t *testing.T) {

	var gotArgs []string
	p := newTestProvider(t, func(args []string) error {
		gotArgs = args
		return nil
	})

	result, _, err := p.Clone(
		"https://example.com/org/repo.git", "",
		false, "", 0, "", false, false, "", false, false, nil,
	)
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}

	// guessDirName → "repo"; under Root="/", SourcePath.Abs() resolves to "/repo".
	if len(gotArgs) != 3 {
		t.Fatalf("args = %q, want 3 entries", gotArgs)
	}
	if gotArgs[len(gotArgs)-1] != "/repo" {
		t.Errorf("directory arg = %q, want %q", gotArgs[len(gotArgs)-1], "/repo")
	}
	if result.SourcePath.Abs() != "/repo" {
		t.Errorf("result.SourcePath.Abs() = %q, want %q", result.SourcePath.Abs(), "/repo")
	}
}

func TestClone_OptionsReachHook(t *testing.T) {

	var gotArgs []string
	p := newTestProvider(t, func(args []string) error {
		gotArgs = args
		return nil
	})

	const repo = "https://example.com/repo.git"
	const dir = "/tmp/shallow"

	_, _, err := p.Clone(
		repo, dir,
		false,  // bare
		"main", // branch
		1,      // depth
		"",     // filter
		false,  // noCheckout
		true,   // noTags
		"",     // origin
		false,  // recurseSubmodules
		true,   // singleBranch
		map[string]any{"template": "/etc/gt"},
	)
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}

	want := []string{
		"clone",
		"--branch", "main",
		"--depth", "1",
		"--no-tags",
		"--single-branch",
		"--template=/etc/gt",
		repo, dir,
	}
	if !reflect.DeepEqual(gotArgs, want) {
		t.Errorf("cloneFn args =\n  got: %q\n want: %q", gotArgs, want)
	}
}

// --- CompensateClone ---

func TestCompensateClone(t *testing.T) {

	tmp := t.TempDir()
	dir := tmp + "/to-remove"
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	ctx := &op.ExecutionContext{Root: op.NewRootReaderWriter("/")}
	r, err := NewResource(ctx, dir)
	if err != nil {
		t.Fatalf("NewResource(%q): %v", dir, err)
	}

	p := &Provider{ProviderBase: op.NewProviderBase(ctx)}
	if err := p.CompensateClone(r); err != nil {
		t.Fatalf("CompensateClone: %v", err)
	}

	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("directory %q still exists after compensation", dir)
	}
}

func TestCompensateClone_NoResource(t *testing.T) {

	p := &Provider{ProviderBase: op.NewProviderBase(&op.ExecutionContext{})}
	if err := p.CompensateClone(nil); err != nil {
		t.Fatalf("CompensateClone(nil) = %v, want nil", err)
	}
}
