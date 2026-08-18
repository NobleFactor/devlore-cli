// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

//go:build unix

package file

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// Ownership resolution is Unix-only, and the constraint scopes these rather than skipping them.
//
// os.Getuid and os.Getgid return **-1** on Windows, so a test that formats them builds the string "-1" —
// which resolveOwnership reads as a numeric id, then rejects because both sides land on the -1 "leave
// unchanged" sentinel. The failure is the platform having no such concept, not the code being wrong.

func TestResolveOwnership_LooksUpNamedUser(t *testing.T) {

	// Looking up the current user by name should always succeed and return the same uid as os.Getuid.
	currentUID := os.Getuid()
	currentName := strconv.Itoa(currentUID)

	gotUID, gotGID, err := resolveOwnership(currentName, "")
	if err != nil {
		t.Fatalf("resolveOwnership(%q, \"\"): %v", currentName, err)
	}
	if gotUID != currentUID {
		t.Errorf("uid: got %d, want %d", gotUID, currentUID)
	}
	if gotGID != -1 {
		t.Errorf("gid: got %d, want -1 (group side absent)", gotGID)
	}
}

func TestApplyOwnership_CurrentUserIsNoOp(t *testing.T) {

	// Setting ownership to the current uid and gid is a no-op-ish operation that needs no CAP_CHOWN.
	tmp := t.TempDir()
	target := filepath.Join(tmp, "test.txt")

	if err := os.WriteFile(target, []byte("test"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	user, group := strconv.Itoa(os.Getuid()), strconv.Itoa(os.Getgid())
	if err := applyOwnership(target, user, group); err != nil {
		t.Errorf("applyOwnership(%q, %q): %v", user, group, err)
	}
}
