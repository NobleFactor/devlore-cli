// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package fsroot

import (
	"os"
	"testing"
)

// TestIsPrivateMode pins the one distinction Windows can honor faithfully (issue #405).
//
// The predicate decides whether a write gets a protected DACL, so a wrong answer here fails open on
// Windows for every call site at once. It runs on every platform because the rule is arithmetic,
// not a syscall.
//
// The boundary is **other**, not **group and other**: we can express "other is excluded" but have
// no group principal to grant to, so a group grant collapses into the owner rather than falling
// back to an inherited — potentially world-readable — DACL.
func TestIsPrivateMode(t *testing.T) {

	for _, tc := range []struct {
		perm os.FileMode
		want bool
		why  string
	}{
		{0o600, true, "owner read/write only"},
		{0o400, true, "owner read only"},
		{0o700, true, "owner everything, typical of a state directory"},
		{0o000, true, "no access at all is still not shared"},
		{0o640, true, "group is inexpressible on Windows, so it collapses into the owner"},
		{0o660, true, "same: a group grant cannot be honored, and failing open is not an option"},
		{0o620, true, "group write is still not other access"},
		{0o644, false, "other can read"},
		{0o666, false, "the Create default — unrestricted by definition"},
		{0o755, false, "other can traverse"},
		{0o604, false, "other can read even with no group access"},
		{0o001, false, "other can execute"},
	} {
		if got := isPrivateMode(tc.perm); got != tc.want {
			t.Errorf("isPrivateMode(%#o) = %v, want %v (%s)", tc.perm, got, tc.want, tc.why)
		}
	}
}

// TestIsPrivateMode_IgnoresTypeAndSetuidBits verifies the predicate reads permission bits only.
//
// [os.FileMode] packs type and setuid/sticky bits above the permission bits; a directory mode or a
// setuid file must not change the private/not-private answer.
func TestIsPrivateMode_IgnoresTypeAndSetuidBits(t *testing.T) {

	if !isPrivateMode(os.ModeDir | 0o700) {
		t.Error("isPrivateMode(ModeDir|0o700) = false, want true (type bits are not permissions)")
	}
	if !isPrivateMode(os.ModeSetuid | 0o600) {
		t.Error("isPrivateMode(ModeSetuid|0o600) = false, want true")
	}
	if isPrivateMode(os.ModeDir | 0o755) {
		t.Error("isPrivateMode(ModeDir|0o755) = true, want false")
	}
}
