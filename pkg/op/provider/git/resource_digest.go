// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package git

import (
	"crypto/sha256"
	"fmt"
	"os/exec"
	"strings"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// region EXPORTED METHODS

// region Behaviors

// Addressing reports that git.Resource is location-keyed: identity is the local clone's filesystem location, and
// the bytes under that location (commit SHAs, working-tree contents) are mutable. The catalog uses
// [op.AddressingLocation] semantics — content drift triggers shadow chains, not new URIs.
func (r *Resource) Addressing() op.AddressingMode {
	return op.AddressingLocation
}

// Etag returns a cheap stat-derived change-detection token for the local clone:
//
//   - Bare repository: the 7-character HEAD short-id (e.g., "a1b2c3d").
//   - Working tree, clean: the 7-character HEAD short-id.
//   - Working tree, dirty: HEAD short-id + "-" + 7-character prefix of the tree SHA covering the current
//     index + working tree.
//
// The dirty fingerprint is derived from `git stash create` followed by `git rev-parse <stash>^{tree}`. The
// stash commit's own SHA cannot be used directly: commit objects include author/committer timestamps, so
// two calls on the same unchanged tree state would produce different commit SHAs (catalog would falsely
// detect drift on every Resolve). The tree SHA is content-addressed and timestamp-free — same tree state
// same SHA, different tree state different SHA. This lets the catalog detect drift within the dirty state
// without false-positive drift on identical state.
//
// Always fresh — re-reads HEAD and (when dirty) re-runs the stash-create + rev-parse pair at call time.
// Errors when the path is not a git repository or HEAD cannot be read.
func (r *Resource) Etag() (string, error) {

	abs := r.SourcePath.Abs()

	repo, bare := isGitRepo(abs)
	if !repo {
		return "", fmt.Errorf("git.Resource: etag: %s is not a git repository", abs)
	}

	head := readHEADSha(abs)
	if head == "" {
		return "", fmt.Errorf("git.Resource: etag: cannot read HEAD at %s", abs)
	}

	short := head
	if len(short) > 7 {
		short = short[:7]
	}

	if bare {
		return short, nil
	}

	stashID := readStashCreateID(abs)
	if stashID == "" {
		return short, nil
	}

	suffix := stashID
	if len(suffix) > 7 {
		suffix = suffix[:7]
	}

	return short + "-" + suffix, nil
}

// Digest returns the honest content hash for the local clone:
//
//   - Clean repository (bare or working-tree): sha256 of HEAD's hex string.
//   - Dirty working tree: sha256 of HEAD + "\n" + tree SHA over the index + working tree.
//
// The HEAD SHA-1 itself already content-addresses git's commit graph; wrapping it in a sha256 layer keeps the
// algorithm consistent with the rest of the system (the catalog stores `op.Digest` values uniformly and round-
// trips them through [op.ParseDigest], which only accepts the sha256 allowlist). For dirty working trees, the
// tree SHA (derived from stash-create followed by rev-parse to the tree, not the commit SHA which would carry
// timestamps) captures the index + working-tree state deterministically — same state same digest.
//
// Always fresh — recomputes at call time. Errors when the path is not a git repository or HEAD cannot be read.
func (r *Resource) Digest() (op.Digest, error) {

	abs := r.SourcePath.Abs()

	repo, bare := isGitRepo(abs)
	if !repo {
		return op.Digest{}, fmt.Errorf("git.Resource: digest: %s is not a git repository", abs)
	}

	head := readHEADSha(abs)
	if head == "" {
		return op.Digest{}, fmt.Errorf("git.Resource: digest: cannot read HEAD at %s", abs)
	}

	h := sha256.New()
	h.Write([]byte(head))

	if !bare {
		stashID := readStashCreateID(abs)
		if stashID != "" {
			h.Write([]byte("\n"))
			h.Write([]byte(stashID))
		}
	}

	return op.Digest{Algorithm: "sha256", Bytes: h.Sum(nil)}, nil
}

// endregion

// endregion

// region UNEXPORTED HELPERS

// readStashCreateID returns a deterministic tree SHA over the index + working-tree state at path, or "" when
// clean / not a working tree / the command fails.
//
// Two-step: `git stash create` constructs a stash commit object covering both the index and working tree
// without actually stashing; `git rev-parse <stash>^{tree}` then projects to the tree SHA. The intermediate
// stash commit's own SHA cannot be used directly — commit objects include author/committer timestamps, so
// two calls on the same unchanged tree state would produce different commit SHAs (catalog would falsely
// detect drift on every Resolve). Tree SHAs are content-addressed and timestamp-free: same tree state same
// SHA, different tree state different SHA, regardless of when the call runs.
//
// Untracked files are not included by stash-create's default scope; callers that need untracked-file
// fingerprinting must add it separately.
func readStashCreateID(path string) string {

	stash := runGitOutput(path, "stash", "create")
	if stash == "" {
		return ""
	}

	return runGitOutput(path, "rev-parse", stash+"^{tree}")
}

// runGitOutput runs `git -C path <args...>` and returns the trimmed stdout, or "" on any error.
func runGitOutput(path string, args ...string) string {

	cmd := exec.Command("git", append([]string{"-C", path}, args...)...)

	out, err := cmd.Output()
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(out))
}

// endregion