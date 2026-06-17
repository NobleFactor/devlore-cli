// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package file

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// applyChown changes the owner and/or group of path according to the Dockerfile-style ownership string spec.
//
// An empty spec is a no-op — the function returns nil without invoking any system call, which is the contract that lets
// the four file-provider write methods always call applyChown unconditionally and rely on the empty-string
// short-circuit.
//
// Accepted spec shapes:
//   - ""             — no change (short-circuit; no syscall)
//   - "user"         — change owner only; group unchanged
//   - "user:group"   — change owner and group
//   - ":group"       — change group only; owner unchanged
//   - "uid"          — numeric form of "user"
//   - "uid:gid"      — numeric form of "user:group"
//   - ":gid"         — numeric form of ":group"
//
// User and group sides accept either a name (resolved via os/user) or a decimal integer (passed to os.Chown directly).
//
// Mixed forms are allowed: `"alice:1000"` resolves alice's uid and uses gid 1000.
//
// Parameters:
//   - path: the filesystem path to chown.
//   - spec: the Dockerfile-style ownership string.
//
// Returns:
//   - error: non-nil if spec is malformed, a name doesn't resolve, or os.Chown fails.
func applyChown(path string, spec string) error {

	if spec == "" {
		return nil
	}

	uid, gid, err := parseChown(spec)
	if err != nil {
		return fmt.Errorf("chown %q: %w", path, err)
	}

	if err := os.Chown(path, uid, gid); err != nil {
		return fmt.Errorf("chown %q: %w", path, err)
	}

	return nil
}

// checksumBytes computes the "sha256:<hex>" checksum string for `data`.
//
// Parameters:
//   - `data`: the bytes to hash.
//
// Returns:
//   - `string`: the checksum in "sha256:<hex>" form.
func checksumBytes(data []byte) string {
	h := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(h[:])
}

// checksumFile reads the file at `path` and returns its "sha256:<hex>" checksum.
//
// Parameters:
//   - `root`: the [fsroot.Root] used to read `path`.
//   - `path`: the path to hash.
//
// Returns:
//   - `string`: the checksum in "sha256:<hex>" form, or "" when the file cannot be read.
func checksumFile(root fsroot.Root, path string) string {

	data, err := root.ReadFile(root.NewPath(path))
	if err != nil {
		return ""
	}

	return checksumBytes(data)
}

// isDirNotEmpty reports whether `err` is the "directory not empty" (ENOTEMPTY) error.
//
// Parameters:
//   - `err`: the error to test.
//
// Returns:
//   - `bool`: true when `err` wraps [syscall.ENOTEMPTY].
func isDirNotEmpty(err error) bool {
	return errors.Is(err, syscall.ENOTEMPTY)
}

// matchDoubleStar reports whether `path` matches `pattern`, supporting `**` recursive wildcards.
//
// A pattern with no `**` is delegated to [filepath.Match] semantics; a single `**` is handled segment-by-segment; and
// multiple `**` fall back to matching the trailing component against the path's base name.
//
// Parameters:
//   - `pattern`: the glob pattern, which may contain `**`.
//   - `path`: the path to test.
//
// Returns:
//   - `bool`: true when `path` matches `pattern`.
func matchDoubleStar(pattern, path string) bool {

	parts := strings.Split(pattern, "**")
	if len(parts) == 1 {
		return pathMatch(pattern, path)
	}

	if len(parts) == 2 {
		return matchDoubleStarSingle(parts[0], parts[1], path)
	}

	tail := strings.TrimLeft(parts[len(parts)-1], string(filepath.Separator))
	return pathMatch(tail, filepath.Base(path))
}

// matchDoubleStarSingle reports whether `path` matches a single-`**` pattern split into `rawPrefix` and `rawSuffix`.
//
// The prefix must match the head of `path`; the suffix is then matched against every trailing sub-path so `**` spans
// zero or more intermediate segments.
//
// Parameters:
//   - `rawPrefix`: the pattern text before the `**`.
//   - `rawSuffix`: the pattern text after the `**`.
//   - `path`: the path to test.
//
// Returns:
//   - `bool`: true when `path` matches the prefix/suffix around `**`.
func matchDoubleStarSingle(rawPrefix, rawSuffix, path string) bool {

	prefix := strings.TrimRight(rawPrefix, string(filepath.Separator))
	suffix := strings.TrimLeft(rawSuffix, string(filepath.Separator))

	if prefix != "" {
		if !strings.HasPrefix(path, prefix+string(filepath.Separator)) && path != prefix {
			return false
		}
		path = strings.TrimPrefix(path, prefix+string(filepath.Separator))
	}

	segments := strings.Split(path, string(filepath.Separator))

	for i := range segments {
		tail := strings.Join(segments[i:], string(filepath.Separator))
		if pathMatch(suffix, tail) {
			return true
		}
	}

	return false
}

// parseChown splits a Dockerfile-style ownership string into uid and gid integers suitable for os.Chown.
//
// Each side resolves either a name via os/user or a numeric form via strconv. Empty sides produce -1 — the os.Chown
// sentinel for "leave this side unchanged."
//
// Parameters:
//   - spec: the ownership string; must be non-empty (callers short-circuit on empty before calling).
//
// Returns:
//   - int:   resolved uid, or -1 if the user side is empty.
//   - int:   resolved gid, or -1 if the group side is empty.
//   - error: non-nil if either side fails to resolve.
func parseChown(spec string) (int, int, error) {

	userSide, groupSide, hasColon := strings.Cut(spec, ":")

	uid := -1
	if userSide != "" {
		resolved, err := resolveUser(userSide)
		if err != nil {
			return 0, 0, err
		}
		uid = resolved
	}

	gid := -1
	if hasColon && groupSide != "" {
		resolved, err := resolveGroup(groupSide)
		if err != nil {
			return 0, 0, err
		}
		gid = resolved
	}

	if uid == -1 && gid == -1 {
		return 0, 0, fmt.Errorf("invalid ownership %q: at least one of user or group must be present", spec)
	}

	return uid, gid, nil
}

// pathMatch wraps [filepath.Match], treating a malformed-pattern error as no match.
//
// Parameters:
//   - `pattern`: the [filepath.Match] pattern.
//   - `name`: the name to test.
//
// Returns:
//   - `bool`: true when `name` matches `pattern` and the pattern is well-formed.
func pathMatch(pattern, name string) bool {
	ok, err := filepath.Match(pattern, name)
	return err == nil && ok
}

// preArchiveDigest computes the digest of the bytes at `path` before archival.
//
// Returns the zero [op.Digest] (not an error) when the file cannot be hashed — symlinks, unreadable files, etc.
// Callers can record the digest when available without blocking the archive when not.
//
// Parameters:
//   - `root`: the [fsroot.Root] used to read `path`.
//   - `path`: the absolute path whose bytes are hashed.
//
// Returns:
//   - `op.Digest`: the parsed digest, or the zero value when the bytes cannot be hashed or parsed.
func preArchiveDigest(root fsroot.Root, path string) op.Digest {

	checksum := checksumFile(root, path)
	if checksum == "" {
		return op.Digest{}
	}

	digest, err := op.ParseDigest(checksum)
	if err != nil {
		return op.Digest{}
	}

	return digest
}

// resolveGroup converts the group side of a chown spec into a gid. Numeric input passes through.
//
// A name is looked up via os/user.LookupGroup.
//
// Parameters:
//   - s: the group side; non-empty.
//
// Returns:
//   - int:   the resolved gid.
//   - error: non-nil if the name doesn't resolve or the numeric form is out of range.
func resolveGroup(s string) (int, error) {

	if gid, err := strconv.Atoi(s); err == nil {
		return gid, nil
	}

	g, err := user.LookupGroup(s)
	if err != nil {
		return 0, fmt.Errorf("lookup group %q: %w", s, err)
	}

	gid, err := strconv.Atoi(g.Gid)
	if err != nil {
		return 0, fmt.Errorf("group %q has non-numeric gid %q: %w", s, g.Gid, err)
	}

	return gid, nil
}

// resolveUser converts the user side of a chown spec into a uid. Numeric input passes through.
//
// A name is looked up via os/user.Lookup.
//
// Parameters:
//   - s: the user side; non-empty.
//
// Returns:
//   - int:   the resolved uid.
//   - error: non-nil if the name doesn't resolve or the numeric form is out of range.
func resolveUser(s string) (int, error) {

	if uid, err := strconv.Atoi(s); err == nil {
		return uid, nil
	}

	u, err := user.Lookup(s)
	if err != nil {
		return 0, fmt.Errorf("lookup user %q: %w", s, err)
	}

	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		return 0, fmt.Errorf("user %q has non-numeric uid %q: %w", s, u.Uid, err)
	}

	return uid, nil
}

// splitFindPattern splits `pattern` into a base directory and the match expression beneath it.
//
// When the pattern contains `**`, the base is everything before it; otherwise the base is the pattern's directory and
// the match is its base name.
//
// Parameters:
//   - `pattern`: the find pattern to split.
//
// Returns:
//   - `string`: the base directory portion.
//   - `string`: the match expression portion.
func splitFindPattern(pattern string) (root, match string) {

	idx := strings.Index(pattern, "**")
	if idx < 0 {
		return filepath.Dir(pattern), filepath.Base(pattern)
	}

	root = strings.TrimRight(pattern[:idx], string(filepath.Separator))
	match = pattern[idx:]

	return root, match
}
