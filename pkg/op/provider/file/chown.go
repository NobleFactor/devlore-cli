// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package file

import (
	"fmt"
	"os"
	"os/user"
	"strconv"
	"strings"
)

// applyChown changes the owner and/or group of path according to the Dockerfile-style ownership
// string spec. An empty spec is a no-op — the function returns nil without invoking any system call,
// which is the contract that lets the four file-provider write methods always call applyChown
// unconditionally and rely on the empty-string short-circuit.
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
// User and group sides accept either a name (resolved via os/user) or a decimal integer (passed to
// os.Chown directly). Mixed forms are allowed: `"alice:1000"` resolves alice's uid and uses gid 1000.
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

// parseChown splits a Dockerfile-style ownership string into uid and gid integers suitable for
// os.Chown. Each side resolves either a name via os/user or a numeric form via strconv. Empty sides
// produce -1 — the os.Chown sentinel for "leave this side unchanged."
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

// resolveUser converts the user side of a chown spec into a uid. Numeric input passes through; a name
// is looked up via os/user.Lookup.
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

// resolveGroup converts the group side of a chown spec into a gid. Numeric input passes through; a
// name is looked up via os/user.LookupGroup.
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