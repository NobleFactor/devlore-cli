// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package git

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/application"
	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// testEnvironment builds a session rooted at `dir` through the real constructor.
//
// Tests travel the same construction path production does: the session mints the root from the spec's anchor and
// wires the recovery site and resource catalog itself, so nothing here hand-assembles filesystem access.
//
// Parameters:
//   - `t`: the test harness.
//   - `dir`: the anchor the session's root is minted at.
//
// Returns:
//   - `*op.RuntimeEnvironment`: the constructed session, closed at test cleanup.
func testEnvironment(t *testing.T, dir string) *op.RuntimeEnvironment {

	t.Helper()

	runtimeEnvironment, err := op.NewRuntimeEnvironment(context.Background(),
		op.NewRuntimeEnvironmentSpec("test").
			WithRoot(dir, fsroot.ModeWritableUnconfined).
			WithApplication(&application.Application{Name: "test"}))
	if err != nil {
		t.Fatalf("op.NewRuntimeEnvironment: %v", err)
	}
	t.Cleanup(func() { _ = runtimeEnvironment.Close() })

	return runtimeEnvironment
}

// testActivation returns an [*op.ActivationRecord] for non-graph dispatch, rooted at `rootDir`.
//
// `Graph` and `Unit` are both nil — Resources produced through this activation carry an empty producer
// stamp. Test calls to producer constructors (NewResource for production, or producer methods like Clone)
// pass this in lieu of the real per-dispatch activation that the framework would build.
//
// The root is a caller-supplied directory (t.TempDir()), never the Unix literal "/": a
// whole-filesystem root cannot address absolute Windows paths (#392), and anchoring at the temp
// directory confines the fixture properly besides.
//
// Parameters:
//   - `t`: the test harness.
//   - `rootDir`: the directory the activation's Root is anchored at.
//
// Returns:
//   - `*op.ActivationRecord`: the non-graph activation.
func testActivation(t *testing.T, rootDir string) *op.ActivationRecord {
	t.Helper()
	return op.NewActivationRecord(nil, "", testEnvironment(t, rootDir))
}

// newTestProvider returns a Provider rooted at `rootDir` whose cloneFn hook is replaced with the supplied
// function. Tests use the hook to capture the argv that would have been passed to `git clone` without
// executing the real binary.
//
// Parameters:
//   - `t`: the test harness.
//   - `rootDir`: the directory the provider's Root is anchored at (see [testActivation] on why never "/").
//   - `hook`: the test-only replacement for doClone's exec path; nil means fall through to the real binary
//     (tests never do this).
//
// Returns:
//   - `*Provider`: the initialized provider bound to a fsroot-anchored execution context.
func newTestProvider(t *testing.T, rootDir string, hook func(args []string) error) *Provider {
	t.Helper()
	return &Provider{
		ProviderBase: op.NewProviderBase(testEnvironment(t, rootDir)),
		cloneFn:      hook,
	}
}

// --- Clone ---

func TestClone_HookReceivesArgv(t *testing.T) {

	root := t.TempDir()
	var gotArgs []string
	p := newTestProvider(t, root, func(args []string) error {
		gotArgs = args
		return nil
	})

	const repo = "https://example.com/repo.git"
	dir := filepath.Join(root, "clone-dest")

	result, state, err := p.Clone(testActivation(t, root), repo, dir, false, "", 0, "", false, false, "", false, false, nil)
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

	root := t.TempDir()
	hookErr := errors.New("clone failed")
	p := newTestProvider(t, root, func(_ []string) error {
		return hookErr
	})

	result, state, err := p.Clone(
		testActivation(t, root),
		"https://example.com/repo.git", filepath.Join(root, "dest"),
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

	root := t.TempDir()
	var gotArgs []string
	p := newTestProvider(t, root, func(args []string) error {
		gotArgs = args
		return nil
	})

	result, _, err := p.Clone(
		testActivation(t, root),
		"https://example.com/org/repo.git", "",
		false, "", 0, "", false, false, "", false, false, nil,
	)
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}

	// guessDirName → "repo"; SourcePath.Abs() resolves it under the fixture root.
	wantDir := filepath.Join(root, "repo")
	if len(gotArgs) != 3 {
		t.Fatalf("args = %q, want 3 entries", gotArgs)
	}
	if gotArgs[len(gotArgs)-1] != wantDir {
		t.Errorf("directory arg = %q, want %q", gotArgs[len(gotArgs)-1], wantDir)
	}
	if result.SourcePath.Abs() != wantDir {
		t.Errorf("result.SourcePath.Abs() = %q, want %q", result.SourcePath.Abs(), wantDir)
	}
}

func TestClone_OptionsReachHook(t *testing.T) {

	root := t.TempDir()
	var gotArgs []string
	p := newTestProvider(t, root, func(args []string) error {
		gotArgs = args
		return nil
	})

	const repo = "https://example.com/repo.git"
	dir := filepath.Join(root, "shallow")

	_, _, err := p.Clone(
		testActivation(t, root),
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

// TestClone_ProducerStamp verifies the m.5(iii) contract.
//
// A forward producer-method call flows through [op.ResourceCatalog.GetOrCreate], which stamps `Unit.ID()`
// as the catalog entry's producerID. Clone is git's sole true producer (Checkout and Pull mutate in place
// without changing the URI). Under non-graph dispatch (this test fixture) the Resource carries an empty
// producer stamp.
func TestClone_ProducerStamp(t *testing.T) {

	root := t.TempDir()
	p := newTestProvider(t, root, func(_ []string) error { return nil })

	activation := op.NewActivationRecord(nil, "", testEnvironment(t, root))

	result, _, err := p.Clone(
		activation,
		"https://example.com/repo.git", filepath.Join(root, "clone-dest"),
		false, "", 0, "", false, false, "", false, false, nil,
	)
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}

	if got := result.ProducerID(); got != "" {
		t.Errorf("producerID = %q, want empty (no caller id)", got)
	}
}

// --- CompensateClone ---

func TestCompensateClone_RemovesDirectory(t *testing.T) {

	tmp := t.TempDir()
	dir := filepath.Join(tmp, "to-remove")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	runtimeEnvironment := testEnvironment(t, tmp)
	r, err := DiscoverResource(runtimeEnvironment, dir)
	if err != nil {
		t.Fatalf("DiscoverResource(%q): %v", dir, err)
	}

	p := &Provider{ProviderBase: op.NewProviderBase(runtimeEnvironment)}
	if err := p.CompensateClone(testActivation(t, tmp), NewReceipt(r)); err != nil {
		t.Fatalf("CompensateClone: %v", err)
	}

	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("directory %q still exists after compensation", dir)
	}
}

func TestCompensateClone_NoResource(t *testing.T) {

	p := &Provider{ProviderBase: op.NewProviderBase(testEnvironment(t, t.TempDir()))}
	if err := p.CompensateClone(testActivation(t, t.TempDir()), nil); err != nil {
		t.Fatalf("CompensateClone(nil) = %v, want nil", err)
	}
}
