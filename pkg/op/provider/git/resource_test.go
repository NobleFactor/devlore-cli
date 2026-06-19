// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package git

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// --- Interface guards ---

func TestResource_ImplementsInterface(t *testing.T) {
	var _ op.Resource = (*Resource)(nil)
}

// --- Test helpers ---

// runGit runs git -C <dir> <args...> and fails the test on error.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()

	full := append([]string{"-C", dir}, args...)

	cmd := exec.Command("git", full...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@example.com",
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}

	return strings.TrimSpace(string(out))
}

// initRepo creates a fresh git repository at dir on a test branch.
//
// Using "test/k4" rather than "main" dodges any host-side hooks (project, user, or global) that block direct commits to
// protected branches.
func initRepo(t *testing.T, dir string) {
	t.Helper()
	runGit(t, dir, "init", "-b", "test/k4")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test")
	runGit(t, dir, "config", "commit.gpgsign", "false")
}

// commitFile writes name with content under dir, stages it, and commits.
func commitFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", name)
	runGit(t, dir, "commit", "-m", "add "+name)
}

// newRes constructs a *Resource for path against an unconfined runtime environment. Uses DiscoverResource
// because tests are not claiming production — the file/path being constructed pre-exists or is a fixture.
func newRes(t *testing.T, path string) *Resource {
	t.Helper()
	runtimeEnvironment := &op.RuntimeEnvironment{Root: fsroot.OpenWritableUnconfined("/")}
	r, err := DiscoverResource(runtimeEnvironment, path)
	if err != nil {
		t.Fatalf("DiscoverResource(%q): %v", path, err)
	}
	return r
}

// --- Addressing ---

func TestResource_Addressing_IsLocation(t *testing.T) {

	r := newRes(t, t.TempDir())

	if got := r.Addressing(); got != op.AddressingLocation {
		t.Errorf("Addressing() = %v, want AddressingLocation", got)
	}
}

// --- Digest ---

func TestResource_Digest_NotARepoErrors(t *testing.T) {

	tmp := t.TempDir()
	r := newRes(t, tmp)

	if _, err := r.Digest(); err == nil {
		t.Error("Digest on non-repo succeeded; want error")
	}
}

func TestResource_Digest_CleanRepo_IsSha256OfHead(t *testing.T) {

	tmp := t.TempDir()
	initRepo(t, tmp)
	commitFile(t, tmp, "README.md", "hello\n")

	headFull := runGit(t, tmp, "rev-parse", "HEAD")

	r := newRes(t, tmp)

	got, err := r.Digest()
	if err != nil {
		t.Fatalf("Digest: %v", err)
	}

	want := sha256.Sum256([]byte(headFull))
	expected := op.Digest{Algorithm: "sha256", Bytes: want[:]}

	if !got.Equal(expected) {
		t.Errorf("Digest = %s, want %s", got.String(), expected.String())
	}
}

func TestResource_Digest_StableAcrossCalls(t *testing.T) {

	tmp := t.TempDir()
	initRepo(t, tmp)
	commitFile(t, tmp, "README.md", "hello\n")

	r := newRes(t, tmp)

	first, err := r.Digest()
	if err != nil {
		t.Fatalf("Digest (first): %v", err)
	}

	second, err := r.Digest()
	if err != nil {
		t.Fatalf("Digest (second): %v", err)
	}

	if !first.Equal(second) {
		t.Errorf("two Digest calls disagree: %s vs %s", first.String(), second.String())
	}
}

func TestResource_Digest_ChangesAcrossCommits(t *testing.T) {

	tmp := t.TempDir()
	initRepo(t, tmp)
	commitFile(t, tmp, "a.txt", "first\n")

	r := newRes(t, tmp)

	first, err := r.Digest()
	if err != nil {
		t.Fatalf("Digest (first commit): %v", err)
	}

	commitFile(t, tmp, "b.txt", "second\n")

	second, err := r.Digest()
	if err != nil {
		t.Fatalf("Digest (second commit): %v", err)
	}

	if first.Equal(second) {
		t.Errorf("Digest did not change after a new commit: %s", first.String())
	}
}

func TestResource_Digest_DirtyDiffersFromClean(t *testing.T) {

	tmp := t.TempDir()
	initRepo(t, tmp)
	commitFile(t, tmp, "README.md", "hello\n")

	r := newRes(t, tmp)

	clean, err := r.Digest()
	if err != nil {
		t.Fatalf("Digest (clean): %v", err)
	}

	if err := os.WriteFile(filepath.Join(tmp, "README.md"), []byte("hello, world\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	dirty, err := r.Digest()
	if err != nil {
		t.Fatalf("Digest (dirty): %v", err)
	}

	if clean.Equal(dirty) {
		t.Errorf("Digest did not change when working tree became dirty: %s", clean.String())
	}
}

// TestResource_Digest_FormatIsCanonical confirms the returned digest round-trips through op.ParseDigest —
// catalog persistence depends on this.
func TestResource_Digest_FormatIsCanonical(t *testing.T) {

	tmp := t.TempDir()
	initRepo(t, tmp)
	commitFile(t, tmp, "README.md", "hello\n")

	r := newRes(t, tmp)

	got, err := r.Digest()
	if err != nil {
		t.Fatalf("Digest: %v", err)
	}

	roundTrip, err := op.ParseDigest(got.String())
	if err != nil {
		t.Fatalf("ParseDigest(%q): %v", got.String(), err)
	}

	if !roundTrip.Equal(got) {
		t.Errorf("ParseDigest round-trip changed value: got %s after parsing %s", roundTrip.String(), got.String())
	}
}

// TestResource_Digest_HexEncodedHeadIsConsistent confirms the canonical "sha256:<hex>" form encodes the bytes
// we computed (no double-encoding bugs).
func TestResource_Digest_HexEncodedHeadIsConsistent(t *testing.T) {

	tmp := t.TempDir()
	initRepo(t, tmp)
	commitFile(t, tmp, "README.md", "hello\n")

	headFull := runGit(t, tmp, "rev-parse", "HEAD")

	r := newRes(t, tmp)

	got, err := r.Digest()
	if err != nil {
		t.Fatalf("Digest: %v", err)
	}

	want := sha256.Sum256([]byte(headFull))
	wantString := "sha256:" + hex.EncodeToString(want[:])

	if got.String() != wantString {
		t.Errorf("Digest.String() = %q, want %q", got.String(), wantString)
	}
}

// --- Etag ---

func TestResource_Etag_NotARepoErrors(t *testing.T) {

	tmp := t.TempDir()
	r := newRes(t, tmp)

	if _, err := r.Etag(); err == nil {
		t.Error("Etag on non-repo succeeded; want error")
	}
}

func TestResource_Etag_CleanRepo_IsHeadShortID(t *testing.T) {

	tmp := t.TempDir()
	initRepo(t, tmp)
	commitFile(t, tmp, "README.md", "hello\n")

	headFull := runGit(t, tmp, "rev-parse", "HEAD")
	wantShort := headFull[:7]

	r := newRes(t, tmp)

	got, err := r.Etag()
	if err != nil {
		t.Fatalf("Etag: %v", err)
	}
	if got != wantShort {
		t.Errorf("Etag = %q, want %q", got, wantShort)
	}
}

func TestResource_Etag_DirtyRepo_HasStatusFingerprint(t *testing.T) {

	tmp := t.TempDir()
	initRepo(t, tmp)
	commitFile(t, tmp, "README.md", "hello\n")

	// Modify the working tree without committing — repo is now dirty.
	if err := os.WriteFile(filepath.Join(tmp, "README.md"), []byte("hello, world\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	headFull := runGit(t, tmp, "rev-parse", "HEAD")
	headShort := headFull[:7]

	r := newRes(t, tmp)

	got, err := r.Etag()
	if err != nil {
		t.Fatalf("Etag: %v", err)
	}
	if !strings.HasPrefix(got, headShort+"-") {
		t.Errorf("Etag = %q, want prefix %q with status fingerprint suffix", got, headShort+"-")
	}
	if len(got) != len(headShort)+1+7 {
		t.Errorf("Etag = %q (len %d), want %d-char form (%q + '-' + 7-char hash)", got, len(got), len(headShort)+8, headShort)
	}
}

// TestResource_Etag_DirtyRepo_StableAcrossCalls is the regression test for the stash-create-vs-tree-SHA
// decision: two calls on the same unchanged dirty state must produce the same Etag, even though the
// underlying `git stash create` commit object includes timestamps that drift between calls. The
// implementation projects to the tree SHA (timestamp-free) rather than the commit SHA.
func TestResource_Etag_DirtyRepo_StableAcrossCalls(t *testing.T) {

	tmp := t.TempDir()
	initRepo(t, tmp)
	commitFile(t, tmp, "a.txt", "first\n")

	if err := os.WriteFile(filepath.Join(tmp, "a.txt"), []byte("v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := newRes(t, tmp)

	first, err := r.Etag()
	if err != nil {
		t.Fatalf("Etag (first): %v", err)
	}

	// Sleep a beat so any timestamp-derived SHA would differ on the second call. Tree-SHA-derived
	// values are unaffected.
	time.Sleep(1100 * time.Millisecond)

	second, err := r.Etag()
	if err != nil {
		t.Fatalf("Etag (second): %v", err)
	}

	if first != second {
		t.Errorf("Etag drifted across calls on the same dirty state: %q vs %q", first, second)
	}
}

// TestResource_Etag_DirtyRepo_ChangesWithEdit confirms the catalog can detect within-dirty-state mutations.
//
// Editing a tracked file (without committing) should change the Etag, even though HEAD has not moved.
func TestResource_Etag_DirtyRepo_ChangesWithEdit(t *testing.T) {

	tmp := t.TempDir()
	initRepo(t, tmp)
	commitFile(t, tmp, "a.txt", "first\n")

	// First dirty state.
	if err := os.WriteFile(filepath.Join(tmp, "a.txt"), []byte("v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := newRes(t, tmp)

	first, err := r.Etag()
	if err != nil {
		t.Fatalf("Etag (first dirty): %v", err)
	}

	// Different dirty state — same HEAD, different working tree.
	if err := os.WriteFile(filepath.Join(tmp, "a.txt"), []byte("v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	second, err := r.Etag()
	if err != nil {
		t.Fatalf("Etag (second dirty): %v", err)
	}

	if first == second {
		t.Errorf("Etag did not change between two distinct dirty states: %q", first)
	}
}
