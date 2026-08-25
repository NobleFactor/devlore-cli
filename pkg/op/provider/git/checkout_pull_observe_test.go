// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package git

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/application"
	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/sink"
	"github.com/NobleFactor/devlore-cli/pkg/status"
)

// newNarratingProvider returns a Provider whose RuntimeEnvironment routes `git` invocations through the process
// runner with a capturing narrator installed. In dry-run the runner narrates `[dry-run] $ git ...` and returns
// without executing, so Checkout and Pull tests read the constructed argv from the returned buffer without
// launching git.
//
// Parameters:
//   - `t`: the test harness.
//   - `dryRun`: when true, the runner narrates and skips execution; when false, git runs for real.
//
// Returns:
//   - `*Provider`: the initialized provider bound to a runnable, fsroot-anchored execution context.
//   - `*bytes.Buffer`: the narrator's capture buffer; holds the narrated command lines after a call.
func newNarratingProvider(t *testing.T, dryRun bool) (*Provider, *bytes.Buffer) {
	t.Helper()
	captureSink, buf := sink.Capture()

	runtimeEnvironment, err := op.NewRuntimeEnvironment(context.Background(),
		op.NewRuntimeEnvironmentSpec("test").
			WithRoot(t.TempDir()).
			WithApplication(&application.Application{Name: "test", Flags: map[string]any{"dry_run": dryRun}}).
			WithStatus(status.NewNarrator("test", captureSink)))
	if err != nil {
		t.Fatalf("op.NewRuntimeEnvironment: %v", err)
	}
	t.Cleanup(func() { _ = runtimeEnvironment.Close() })

	return &Provider{ProviderBase: op.NewProviderBase(runtimeEnvironment)}, buf
}

// newObserveProvider returns a Provider for Observe tests, which read `.git` off disk via the resource's absolute
// path and never shell out through the runner seam, so a bare Root-anchored context suffices.
//
// Parameters:
//   - `t`: the test harness.
//
// Returns:
//   - `*Provider`: the initialized provider bound to an fsroot-anchored execution context.
func newObserveProvider(t *testing.T) *Provider {
	t.Helper()
	return &Provider{ProviderBase: op.NewProviderBase(testEnvironment(t, t.TempDir()))}
}

// --- Checkout ---

func TestCheckout_BuildsArgv(t *testing.T) {

	p, buf := newNarratingProvider(t, true)
	repo := newRes(t, filepath.Join(t.TempDir(), "checkout-repo"))

	got, err := p.Checkout(repo, "release-1.2")
	if err != nil {
		t.Fatalf("Checkout: %v", err)
	}
	if got != repo {
		t.Errorf("Checkout returned %p, want the same Resource %p (identity unchanged)", got, repo)
	}

	out := buf.String()
	// The argv, not the binary: exec.Cmd.String() renders the LookPath-RESOLVED executable, so the line reads
	// "/usr/bin/git -C ..." on Unix and "C:\...\git.exe -C ..." on Windows. Asserting a leading "git " only
	// ever worked because "/usr/bin/git" ends with "git"; ".exe" breaks the same substring. Assert the part
	// this test is actually about.
	want := "-C " + repo.SourcePath().Abs() + " checkout release-1.2"
	if !strings.Contains(out, want) {
		t.Errorf("narrated command =\n  %q\nwant it to contain\n  %q", out, want)
	}
	if !strings.Contains(out, "[dry-run] $ ") {
		t.Errorf("narration = %q, want the dry-run prefix", out)
	}
}

func TestCheckout_PropagatesError(t *testing.T) {

	p, _ := newNarratingProvider(t, false)

	// A path that does not exist: `git -C <missing> checkout` cannot chdir and exits non-zero.
	repo := newRes(t, filepath.Join(t.TempDir(), "not-a-repo"))

	got, err := p.Checkout(repo, "main")
	if err == nil {
		t.Fatal("Checkout against a missing directory = nil error, want failure")
	}
	if got != nil {
		t.Errorf("Checkout result = %v, want nil on error", got)
	}
}

func TestCheckout_MovesRef(t *testing.T) {

	p, _ := newNarratingProvider(t, false)

	dir := t.TempDir()
	initRepo(t, dir)
	commitFile(t, dir, "README.md", "hello\n")
	runGit(t, dir, "branch", "feature")

	repo := newRes(t, dir)

	if _, err := p.Checkout(repo, "feature"); err != nil {
		t.Fatalf("Checkout(feature): %v", err)
	}

	obs, err := p.Observe(repo)
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if obs.ObservedRef != "feature" {
		t.Errorf("ObservedRef after checkout = %q, want %q", obs.ObservedRef, "feature")
	}
}

// --- Pull ---

func TestPull_BuildsArgv(t *testing.T) {

	p, buf := newNarratingProvider(t, true)
	repo := newRes(t, filepath.Join(t.TempDir(), "pull-repo"))

	got, err := p.Pull(repo)
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if got != repo {
		t.Errorf("Pull returned %p, want the same Resource %p (identity unchanged)", got, repo)
	}

	out := buf.String()
	// The argv, not the binary: exec.Cmd.String() renders the LookPath-RESOLVED executable, so the line reads
	// "/usr/bin/git -C ..." on Unix and "C:\...\git.exe -C ..." on Windows. Asserting a leading "git " only
	// ever worked because "/usr/bin/git" ends with "git"; ".exe" breaks the same substring. Assert the part
	// this test is actually about.
	want := "-C " + repo.SourcePath().Abs() + " pull"
	if !strings.Contains(out, want) {
		t.Errorf("narrated command =\n  %q\nwant it to contain\n  %q", out, want)
	}
	if !strings.Contains(out, "[dry-run] $ ") {
		t.Errorf("narration = %q, want the dry-run prefix", out)
	}
}

func TestPull_PropagatesError(t *testing.T) {

	p, _ := newNarratingProvider(t, false)

	// A path that does not exist: `git -C <missing> pull` cannot chdir and exits non-zero.
	repo := newRes(t, filepath.Join(t.TempDir(), "not-a-repo"))

	got, err := p.Pull(repo)
	if err == nil {
		t.Fatal("Pull against a missing directory = nil error, want failure")
	}
	if got != nil {
		t.Errorf("Pull result = %v, want nil on error", got)
	}
}

// --- Observe ---

func TestObserve_NonRepoReportsAbsent(t *testing.T) {

	p := newObserveProvider(t)
	repo := newRes(t, t.TempDir()) // a plain directory, no .git

	obs, err := p.Observe(repo)
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if obs == nil {
		t.Fatal("Observe returned nil observation")
	}
	if obs.Exists {
		t.Errorf("Exists = true, want false for a non-git directory")
	}
	if obs.ObservedHEAD != "" || obs.ObservedRef != "" {
		t.Errorf("ObservedHEAD/Ref = %q/%q, want empty for a non-git directory", obs.ObservedHEAD, obs.ObservedRef)
	}
	if obs.Bare || obs.Dirty {
		t.Errorf("Bare/Dirty = %t/%t, want false/false for a non-git directory", obs.Bare, obs.Dirty)
	}
	if obs.Remotes != nil {
		t.Errorf("Remotes = %v, want nil for a non-git directory", obs.Remotes)
	}
}

func TestObserve_WorktreeReportsState(t *testing.T) {

	p := newObserveProvider(t)

	dir := t.TempDir()
	initRepo(t, dir) // inits on branch test/k4
	runGit(t, dir, "remote", "add", "origin", "https://example.com/org/repo.git")

	repo := newRes(t, dir)

	obs, err := p.Observe(repo)
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if !obs.Exists {
		t.Fatal("Exists = false, want true for an initialized repository")
	}
	if obs.Bare {
		t.Errorf("Bare = true, want false for a worktree repository")
	}
	if obs.ObservedRef != "test/k4" {
		t.Errorf("ObservedRef = %q, want %q", obs.ObservedRef, "test/k4")
	}
	remote, ok := obs.Remotes["origin"]
	if !ok {
		t.Fatalf("Remotes = %v, want an \"origin\" entry", obs.Remotes)
	}
	if remote.FetchURL != "https://example.com/org/repo.git" {
		t.Errorf("origin.FetchURL = %q, want %q", remote.FetchURL, "https://example.com/org/repo.git")
	}
}

func TestObserve_ReportsHEAD(t *testing.T) {

	p := newObserveProvider(t)

	dir := t.TempDir()
	initRepo(t, dir)
	commitFile(t, dir, "README.md", "hello\n")

	repo := newRes(t, dir)

	obs, err := p.Observe(repo)
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if obs.ObservedRef != "test/k4" {
		t.Errorf("ObservedRef = %q, want %q", obs.ObservedRef, "test/k4")
	}
	if obs.ObservedHEAD == "" {
		t.Fatal("ObservedHEAD = empty, want the committed SHA")
	}
	for _, r := range obs.ObservedHEAD {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			t.Errorf("ObservedHEAD = %q, want a lowercase hex SHA", obs.ObservedHEAD)
			break
		}
	}
}

func TestObserve_DirtyWorktree(t *testing.T) {

	p := newObserveProvider(t)

	dir := t.TempDir()
	initRepo(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("x"), 0o600); err != nil {
		t.Fatalf("seed untracked file: %v", err)
	}

	repo := newRes(t, dir)

	obs, err := p.Observe(repo)
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if !obs.Exists {
		t.Fatal("Exists = false, want true for an initialized repository")
	}
	if obs.Bare {
		t.Errorf("Bare = true, want false for a worktree repository")
	}
	if !obs.Dirty {
		t.Error("Dirty = false, want true for a worktree holding an untracked file")
	}
}

func TestObserve_BareRepo(t *testing.T) {

	p := newObserveProvider(t)

	dir := t.TempDir()
	runGit(t, dir, "init", "--bare")

	repo := newRes(t, dir)

	obs, err := p.Observe(repo)
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if !obs.Exists {
		t.Fatal("Exists = false, want true for a bare repository")
	}
	if !obs.Bare {
		t.Error("Bare = false, want true for a bare repository")
	}
	if obs.Dirty {
		t.Error("Dirty = true, want false (Observe skips the dirty probe for bare repositories)")
	}
}
