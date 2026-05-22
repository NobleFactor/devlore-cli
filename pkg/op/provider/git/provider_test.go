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

// testActivation returns an [op.ActivationRecord] that satisfies the strict producer contract: non-nil with a
// non-empty SiteID derived from the test name. Test calls to producer constructors (NewResource for production,
// or producer methods like Clone) pass this in lieu of the real per-dispatch activation that the framework
// would build.
func testActivation(t *testing.T) *op.ActivationRecord {
	t.Helper()
	return op.NewActivationRecord(nil, nil, &op.RuntimeEnvironment{Root: op.NewRootReaderWriter("/")})
}

// newTestProvider returns a Provider whose RuntimeEnvironment has Root anchored at "/" and whose cloneFn hook
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
		ProviderBase: op.NewProviderBase(&op.RuntimeEnvironment{Root: op.NewRootReaderWriter("/")}),
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

	result, state, err := p.Clone(testActivation(t), repo, dir, false, "", 0, "", false, false, "", false, false, nil)
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
		t.Fatalf("state = nil, want a *Receipt")
	}
	stateResource, ok := state.Resource().(*Resource)
	if !ok {
		t.Fatalf("state.Resource() = %T, want *Resource", state.Resource())
	}
	if stateResource.SourcePath.Abs() != dir {
		t.Errorf("state resource path = %q, want %q", stateResource.SourcePath.Abs(), dir)
	}
}

func TestClone_HookPropagatesError(t *testing.T) {

	hookErr := errors.New("clone failed")
	p := newTestProvider(t, func(_ []string) error {
		return hookErr
	})

	result, state, err := p.Clone(
		testActivation(t),
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
		testActivation(t),
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
		testActivation(t),
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

// --- m.5 producer-stamp contract ---

// TestProducerStamp_Clone verifies the m.5(iii) contract: a forward producer-method call results in a catalog entry
// whose producerID matches the dispatch's activation SiteID. Clone is git's sole true producer (Checkout and Pull
// mutate in place without changing the URI). Under non-graph dispatch (this test fixture) the
// Resource carries an empty producer stamp.
func TestProducerStamp_Clone(t *testing.T) {

	p := newTestProvider(t, func(_ []string) error { return nil })

	activation := op.NewActivationRecord(nil, nil, &op.RuntimeEnvironment{
		Root:    op.NewRootReaderWriter("/"),
		Catalog: op.NewResourceCatalog(),
	})

	const dir = "/tmp/clone-dest"
	result, _, err := p.Clone(
		activation,
		"https://example.com/repo.git", dir,
		false, "", 0, "", false, false, "", false, false, nil,
	)
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}

	if got := result.ProducerID(); got != "" {
		t.Errorf("producerID = %q, want empty (nil Unit)", got)
	}
}

// --- CompensateClone ---

func TestCompensateClone(t *testing.T) {

	tmp := t.TempDir()
	dir := tmp + "/to-remove"
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	ctx := &op.RuntimeEnvironment{Root: op.NewRootReaderWriter("/")}
	r, err := DiscoverResource(op.NewActivationRecord(nil, nil, ctx), dir)
	if err != nil {
		t.Fatalf("DiscoverResource(%q): %v", dir, err)
	}

	p := &Provider{ProviderBase: op.NewProviderBase(ctx)}
	if err := p.CompensateClone(NewReceipt(r)); err != nil {
		t.Fatalf("CompensateClone: %v", err)
	}

	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("directory %q still exists after compensation", dir)
	}
}

func TestCompensateClone_NoResource(t *testing.T) {

	p := &Provider{ProviderBase: op.NewProviderBase(&op.RuntimeEnvironment{})}
	if err := p.CompensateClone(nil); err != nil {
		t.Fatalf("CompensateClone(nil) = %v, want nil", err)
	}
}
