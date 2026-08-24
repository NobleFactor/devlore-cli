// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package file

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// Step-52 test backfill: direct method units for the Provider surfaces that carried thin or no coverage —
// WalkTree / CompensateWalkTree (the fold + recovery-stack contract), the path-algebra actions Name / Parent /
// Root, and additional forward-path cases for Move and RemoveAll. Construction and assertion style mirror
// provider_test.go (testProvider, testActivation, writeTestFile).

// --- WalkTree ---

// TestWalkTree_FoldsEntriesInDepthFirstOrder verifies the fold contract: the accumulator threads through each
// invocation, so the final value is the fold over the whole tree in the walker's lexical, pre-order sequence
// (directories are folded before their contents).
func TestWalkTree_FoldsEntriesInDepthFirstOrder(t *testing.T) {
	tmp := t.TempDir()
	writeTestFile(t, tmp, "a.txt", "a")
	writeTestFile(t, tmp, "b.txt", "b")
	if err := os.MkdirAll(filepath.Join(tmp, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(tmp, "sub"), "c.txt", "c")

	p := testProvider(t, tmp)
	root, err := DiscoverDirectory(p.RuntimeEnvironment(), tmp)
	if err != nil {
		t.Fatalf("DiscoverDirectory: %v", err)
	}

	fold := func(initial any, entry Resource, relativePath string, stack *op.RecoveryStack) (any, error) {
		order, _ := initial.([]string)
		return append(order, relativePath), nil
	}

	result, _, err := p.WalkTree(testActivation(t, p.RuntimeEnvironment()), root, fold, true)
	if err != nil {
		t.Fatalf("WalkTree() error = %v", err)
	}

	got, ok := result.([]string)
	if !ok {
		t.Fatalf("WalkTree() result = %T, want []string", result)
	}

	want := []string{"a.txt", "b.txt", "sub", filepath.Join("sub", "c.txt")}
	if len(got) != len(want) {
		t.Fatalf("fold visited %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("fold order[%d] = %q, want %q (full: %v)", i, got[i], want[i], got)
		}
	}
}

// TestWalkTree_SkipsGitignoredEntries verifies the default filtering (includeGitignored=false): an entry matched by
// a root .gitignore rule is not folded, and the .git directory is always skipped regardless of the rules.
func TestWalkTree_SkipsGitignoredEntries(t *testing.T) {
	tmp := t.TempDir()
	writeTestFile(t, tmp, ".gitignore", "hidden-from-walk.txt\n")
	writeTestFile(t, tmp, "visible.txt", "keep me")
	writeTestFile(t, tmp, "hidden-from-walk.txt", "ignore me")
	if err := os.MkdirAll(filepath.Join(tmp, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(tmp, ".git"), "config", "[core]")

	p := testProvider(t, tmp)
	root, err := DiscoverDirectory(p.RuntimeEnvironment(), tmp)
	if err != nil {
		t.Fatalf("DiscoverDirectory: %v", err)
	}

	visited := collectWalk(t, p, root, false)

	if !visited["visible.txt"] {
		t.Error("visible.txt was not folded; it is not gitignored")
	}
	if visited["hidden-from-walk.txt"] {
		t.Error("hidden-from-walk.txt was folded; the .gitignore rule must exclude it")
	}
	assertGitDirSkipped(t, visited)
}

// TestWalkTree_IncludeGitignored_FoldsIgnoredEntries verifies that includeGitignored=true disables gitignore
// filtering so an otherwise-ignored entry is folded — while the .git directory stays unconditionally skipped.
func TestWalkTree_IncludeGitignored_FoldsIgnoredEntries(t *testing.T) {
	tmp := t.TempDir()
	writeTestFile(t, tmp, ".gitignore", "hidden-from-walk.txt\n")
	writeTestFile(t, tmp, "visible.txt", "keep me")
	writeTestFile(t, tmp, "hidden-from-walk.txt", "now visible")
	if err := os.MkdirAll(filepath.Join(tmp, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(tmp, ".git"), "config", "[core]")

	p := testProvider(t, tmp)
	root, err := DiscoverDirectory(p.RuntimeEnvironment(), tmp)
	if err != nil {
		t.Fatalf("DiscoverDirectory: %v", err)
	}

	visited := collectWalk(t, p, root, true)

	if !visited["visible.txt"] {
		t.Error("visible.txt was not folded")
	}
	if !visited["hidden-from-walk.txt"] {
		t.Error("hidden-from-walk.txt was not folded; includeGitignored must disable the .gitignore rule")
	}
	assertGitDirSkipped(t, visited)
}

// TestWalkTree_ReducerError_StackHoldsCompletedReceipts verifies the recovery-stack contract when the reducer fails
// mid-walk: WalkTree returns the reducer's error and the partially-accumulated stack, and the stack holds receipts
// only for the entries the reducer completed before the failure — never the failing entry or any entry past it.
func TestWalkTree_ReducerError_StackHoldsCompletedReceipts(t *testing.T) {
	tmp := t.TempDir()
	writeTestFile(t, tmp, "a.txt", "a")
	writeTestFile(t, tmp, "b.txt", "b")
	writeTestFile(t, tmp, "c.txt", "c")

	p := testProvider(t, tmp)
	root, err := DiscoverDirectory(p.RuntimeEnvironment(), tmp)
	if err != nil {
		t.Fatalf("DiscoverDirectory: %v", err)
	}

	// The reducer records a committed receipt for each entry it completes and fails on c.txt without recording one,
	// so the returned stack must hold receipts for a.txt and b.txt only.
	fold := func(initial any, entry Resource, relativePath string, stack *op.RecoveryStack) (any, error) {
		if relativePath == "c.txt" {
			return nil, errors.New("reducer boom on c.txt")
		}
		receipt := &op.ReceiptBase{}
		if commitErr := receipt.Commit(nil, entry, nil, nil); commitErr != nil {
			t.Fatalf("Commit: %v", commitErr)
		}
		stack.Push(receipt)
		return nil, nil
	}

	result, stack, err := p.WalkTree(testActivation(t, p.RuntimeEnvironment()), root, fold, true)
	if err == nil {
		t.Fatal("WalkTree() error = nil; want the reducer error")
	}
	if !strings.Contains(err.Error(), "reducer boom") {
		t.Errorf("WalkTree() error = %q, want it to mention \"reducer boom\"", err)
	}
	if result != nil {
		t.Errorf("WalkTree() result = %v, want nil on error", result)
	}
	if stack == nil {
		t.Fatal("WalkTree() stack = nil; want the partially-accumulated stack")
	}

	receipts := stack.Receipts()
	if len(receipts) != 2 {
		t.Fatalf("stack holds %d receipts, want 2 (the completed entries a.txt, b.txt)", len(receipts))
	}

	// filepath.Base, not p.Name: Abs() is OS-native, and p.Name speaks the slash-form language of the
	// projected Starlark surface. Feeding it a native path finds no separator on Windows and returns the
	// whole path — the mismatch #547 exists to remove, here in a test that only wants a file name.
	got := map[string]bool{}
	for _, receipt := range receipts {
		got[filepath.Base(receipt.Result().(Resource).Path().Abs())] = true
	}
	if !got["a.txt"] || !got["b.txt"] {
		t.Errorf("stack receipts name %v, want a.txt and b.txt", got)
	}
	if got["c.txt"] {
		t.Error("stack holds a receipt for c.txt; the failing entry must not be recorded")
	}
}

// --- CompensateWalkTree ---

// TestCompensateWalkTree_NilStack_IsNoOp verifies the nil-stack analog every flow-combinator compensator carries: a
// nil recovery stack unwinds to nothing.
func TestCompensateWalkTree_NilStack_IsNoOp(t *testing.T) {
	tmp := t.TempDir()
	p := testProvider(t, tmp)

	if err := p.CompensateWalkTree(testActivation(t, p.RuntimeEnvironment()), nil); err != nil {
		t.Errorf("CompensateWalkTree(nil) = %v, want nil", err)
	}
}

// TestCompensateWalkTree_EmptyStack_IsNoOp verifies that unwinding an empty (but non-nil) recovery stack — a walk
// that folded no entries — is a clean no-op.
func TestCompensateWalkTree_EmptyStack_IsNoOp(t *testing.T) {
	tmp := t.TempDir()
	p := testProvider(t, tmp)

	if err := p.CompensateWalkTree(testActivation(t, p.RuntimeEnvironment()), op.NewRecoveryStack()); err != nil {
		t.Errorf("CompensateWalkTree(empty) = %v, want nil", err)
	}
}

// --- Name (edge cases) ---

// TestName_EdgeCases exercises [Provider.Name] on the boundary paths its [filepath.Base] delegate must normalize:
// the empty path, the current directory, a dotfile, and a path that is only separators.
func TestName_EdgeCases(t *testing.T) {
	tmp := t.TempDir()
	p := testProvider(t, tmp)

	tests := []struct {
		path string
		want string
	}{
		{"", "."},
		{".", "."},
		{"/etc/.config", ".config"},
		{".hidden", ".hidden"},
		{"///", "/"},
	}
	for _, tt := range tests {
		if got := p.Name(tt.path); got != tt.want {
			t.Errorf("Name(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

// --- Parent (edge cases) ---

// TestParent_EdgeCases exercises [Provider.Parent] on the boundary paths its [filepath.Dir] delegate must normalize:
// the empty path, the current directory, a dotfile leaf, and a path that is only separators.
func TestParent_EdgeCases(t *testing.T) {
	tmp := t.TempDir()
	p := testProvider(t, tmp)

	tests := []struct {
		path string
		want string
	}{
		{"", "."},
		{".", "."},
		{"/a/b/.config", "/a/b"},
		{"///", "/"},
	}
	for _, tt := range tests {
		if got := p.Parent(tt.path); got != tt.want {
			t.Errorf("Parent(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

// --- Root ---

// TestRoot_ReturnsScopedRootPath verifies that Root returns the scoped root's name when a root is set.
func TestRoot_ReturnsScopedRootPath(t *testing.T) {
	tmp := t.TempDir()
	p := testProvider(t, tmp)

	if got := p.Root(); got != tmp {
		t.Errorf("Root() = %q, want %q", got, tmp)
	}
}

// TestRoot_NilRoot_ReturnsEmpty verifies that Root returns the empty string when the runtime environment has no root.
func TestRoot_NilRoot_ReturnsEmpty(t *testing.T) {
	p := Provider{ProviderBase: op.NewProviderBase(&op.RuntimeEnvironment{})}

	if got := p.Root(); got != "" {
		t.Errorf("Root() = %q, want empty string", got)
	}
}

// --- Move (forward path) ---

// TestMove_IntoSubdirectory verifies that Move creates the destination's missing parent chain, relocates the entry,
// and leaves no source behind.
func TestMove_IntoSubdirectory(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "source.txt")
	dst := filepath.Join(tmp, "nested", "deep", "dest.txt")
	if err := os.WriteFile(src, []byte("relocated"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := testProvider(t, tmp)
	result, _, err := p.Move(testActivation(t, p.RuntimeEnvironment()), mustDiscoverRegular(t, p, src), dst)
	if err != nil {
		t.Fatalf("Move() error = %v", err)
	}
	if result.Path().Abs() != dst {
		t.Errorf("result = %q, want %q", result.Path().Abs(), dst)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Error("source still exists after move")
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("ReadFile(dst) error = %v", err)
	}
	if string(got) != "relocated" {
		t.Errorf("dest content = %q, want %q", got, "relocated")
	}
}

// TestMove_OverwritesExistingDestination verifies the replace conflict policy (the default): an occupied destination
// is archived for compensation, then overwritten with the source's content.
func TestMove_OverwritesExistingDestination(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "source.txt")
	dst := filepath.Join(tmp, "dest.txt")
	if err := os.WriteFile(src, []byte("new content"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("old content"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := testProvider(t, tmp)
	_, receipt, err := p.Move(testActivation(t, p.RuntimeEnvironment()), mustDiscoverRegular(t, p, src), dst)
	if err != nil {
		t.Fatalf("Move() error = %v", err)
	}
	if receipt.RecoveryID() == "" {
		t.Error("receipt.RecoveryID() is empty; the displaced destination should have been archived")
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("ReadFile(dst) error = %v", err)
	}
	if string(got) != "new content" {
		t.Errorf("dest content = %q, want %q (the source content)", got, "new content")
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Error("source still exists after move")
	}
}

// TestMove_MissingSource_ReturnsError verifies that moving a non-existent source is an error wrapping
// [os.ErrNotExist].
func TestMove_MissingSource_ReturnsError(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "does-not-exist.txt")
	dst := filepath.Join(tmp, "dest.txt")

	p := testProvider(t, tmp)
	_, _, err := p.Move(testActivation(t, p.RuntimeEnvironment()), mustDiscoverRegular(t, p, src), dst)
	if err == nil {
		t.Fatal("Move() error = nil; want an error for a missing source")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Move() error = %v, want it to wrap os.ErrNotExist", err)
	}
}

// --- RemoveAll (subtree cases) ---

// TestRemoveAll_NestedTree_RoundTrip verifies that RemoveAll archives a multi-level subtree with sibling branches
// and that its compensation restores every file at every depth.
func TestRemoveAll_NestedTree_RoundTrip(t *testing.T) {
	tmp := t.TempDir()
	tree := filepath.Join(tmp, "tree")
	if err := os.MkdirAll(filepath.Join(tree, "a"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tree, "b", "c"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, tree, "top.txt", "top")
	writeTestFile(t, filepath.Join(tree, "a"), "a1.txt", "a-one")
	writeTestFile(t, filepath.Join(tree, "a"), "a2.txt", "a-two")
	writeTestFile(t, filepath.Join(tree, "b", "c"), "deep.txt", "deep-data")

	p := testProvider(t, tmp)
	_, receipt, err := p.RemoveAll(testActivation(t, p.RuntimeEnvironment()), mustDiscoverDirectory(t, p, tree), op.MissingResourcePolicyStop, false, "")
	if err != nil {
		t.Fatalf("RemoveAll() error = %v", err)
	}
	if _, err := os.Stat(tree); !os.IsNotExist(err) {
		t.Error("tree still exists after RemoveAll")
	}

	if err := p.CompensateFileMutation(testActivation(t, p.RuntimeEnvironment()), receipt); err != nil {
		t.Fatalf("CompensateFileMutation() error = %v", err)
	}

	restored := []struct {
		path string
		want string
	}{
		{filepath.Join(tree, "top.txt"), "top"},
		{filepath.Join(tree, "a", "a1.txt"), "a-one"},
		{filepath.Join(tree, "a", "a2.txt"), "a-two"},
		{filepath.Join(tree, "b", "c", "deep.txt"), "deep-data"},
	}
	for _, tc := range restored {
		got, err := os.ReadFile(tc.path)
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v — the subtree should be restored", tc.path, err)
		}
		if string(got) != tc.want {
			t.Errorf("restored %s = %q, want %q", tc.path, got, tc.want)
		}
	}
}

// TestRemoveAll_NonExistentPath_IsNoOp verifies that removing a non-existent target is a no-op: no product, no
// receipt, no error.
func TestRemoveAll_MissingTarget_FollowsThePolicy(t *testing.T) {

	// Mirrors the Remove and Unlink policy pins: Stop errors on a missing target; Ignore makes the call a
	// recorded no-op (§3, ruled 2026-08-22).
	tmp := t.TempDir()
	p := testProvider(t, tmp)

	if _, _, err := p.RemoveAll(testActivation(t, p.RuntimeEnvironment()),
		mustDiscoverDirectory(t, p, filepath.Join(tmp, "ghost")), op.MissingResourcePolicyStop, false, ""); err == nil {
		t.Fatal("RemoveAll(stop) on a missing target must error")
	}

	product, receipt, err := p.RemoveAll(testActivation(t, p.RuntimeEnvironment()),
		mustDiscoverDirectory(t, p, filepath.Join(tmp, "ghost")), op.MissingResourcePolicyIgnore, false, "")
	if err != nil {
		t.Fatalf("RemoveAll(ignore) error = %v", err)
	}
	if product != nil {
		t.Errorf("product = %v, want nil for the ignored no-op", product)
	}
	if receipt != nil {
		t.Errorf("receipt = %v, want nil (no mutation to compensate)", receipt)
	}
}

// --- test helpers ---

// collectWalk runs [Provider.WalkTree] with a set-collecting reducer and returns the relative paths it folded.
func collectWalk(t *testing.T, p Provider, root *Directory, includeGitignored bool) map[string]bool {
	t.Helper()

	visited := map[string]bool{}
	fold := func(initial any, entry Resource, relativePath string, stack *op.RecoveryStack) (any, error) {
		visited[relativePath] = true
		return nil, nil
	}

	if _, _, err := p.WalkTree(testActivation(t, p.RuntimeEnvironment()), root, fold, includeGitignored); err != nil {
		t.Fatalf("WalkTree: %v", err)
	}

	return visited
}

// assertGitDirSkipped fails the test when any folded relative path names the .git directory or an entry within it.
func assertGitDirSkipped(t *testing.T, visited map[string]bool) {
	t.Helper()

	for relativePath := range visited {
		if relativePath == ".git" || strings.HasPrefix(relativePath, ".git"+string(filepath.Separator)) {
			t.Errorf("walk entered .git (%q); the .git directory is always skipped", relativePath)
		}
	}
}
