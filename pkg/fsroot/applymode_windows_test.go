// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

//go:build windows

package fsroot

import (
	"os"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

// The proof for issue #405. A DACL that is subtly wrong fails OPEN — the object stays accessible
// while every "did the call return nil" assertion passes — so these tests read the access-control
// list back off the object and inspect it. Asserting the absence of an error would prove nothing.
//
// The read-back uses the descriptor's SDDL form, which names the protection flag and every trustee
// in one auditable string, so a failure prints what the object actually carries.

// The two well-known trustees ruling 4 keeps alongside the owner: excluding them buys nothing (an
// administrator takes ownership at will) and breaks backup and anti-malware tooling.
//
// SDDL renders well-known accounts as two-letter aliases rather than numeric SIDs — SYSTEM prints
// as `SY`, Built-in Administrators as `BA` — so each trustee is matched in either rendering. The
// first CI run failed here on the numeric form alone while the DACL itself was already correct.
const (
	systemAlias   = "SY"
	systemSID     = "S-1-5-18"
	adminsAlias   = "BA"
	adminsSID     = "S-1-5-32-544"
	aceTrusteeEnd = ")"
)

func TestApplyMode_PrivateFileGetsProtectedDACL(t *testing.T) {

	root := testConfinedRoot(t)

	target := root.NewPath("secret.txt")
	if err := root.WriteFile(target, []byte("private"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	control, sddl := readSecurity(t, target.Abs())

	// The load-bearing assertion: inheritance must be broken. Without SE_DACL_PROTECTED the parent's
	// inherited ACEs survive and the file stays accessible no matter what was granted.
	if control&windows.SE_DACL_PROTECTED == 0 {
		t.Errorf("DACL is not protected; inherited ACEs survive and the file is not private: %s", sddl)
	}

	if !hasTrustee(sddl, systemAlias, systemSID) {
		t.Errorf("DACL is missing SYSTEM: %s", sddl)
	}
	if !hasTrustee(sddl, adminsAlias, adminsSID) {
		t.Errorf("DACL is missing Administrators: %s", sddl)
	}

	// Exactly three trustees — owner, SYSTEM, Administrators — and no fourth. A DACL carrying an
	// extra allow ACE would still be "protected" while granting someone else access, so the count
	// is as load-bearing as the protection flag.
	if got := strings.Count(sddl, "(A;"); got != 3 {
		t.Errorf("DACL has %d allow ACEs, want exactly 3 (owner, SYSTEM, Administrators): %s", got, sddl)
	}
}

func TestApplyMode_PrivateDirectoryGetsProtectedDACL(t *testing.T) {

	root := testConfinedRoot(t)

	target := root.NewPath("state")
	if err := root.Mkdir(target, 0o700); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	control, sddl := readSecurity(t, target.Abs())

	if control&windows.SE_DACL_PROTECTED == 0 {
		t.Errorf("directory DACL is not protected; inherited ACEs survive: %s", sddl)
	}
}

func TestApplyMode_ChmodRestrictsAnExistingFile(t *testing.T) {

	root := testConfinedRoot(t)

	target := root.NewPath("later.txt")
	if err := root.WriteFile(target, []byte("shared"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if control, sddl := readSecurity(t, target.Abs()); control&windows.SE_DACL_PROTECTED != 0 {
		t.Errorf("0o644 file has a protected DACL; a non-private mode must be left alone: %s", sddl)
	}

	if err := root.Chmod(target, 0o600); err != nil {
		t.Fatalf("Chmod: %v", err)
	}

	if control, sddl := readSecurity(t, target.Abs()); control&windows.SE_DACL_PROTECTED == 0 {
		t.Errorf("after Chmod(0o600) the DACL is still not protected: %s", sddl)
	}
}

func TestApplyMode_NonPrivateModeIsLeftAlone(t *testing.T) {

	root := testConfinedRoot(t)

	target := root.NewPath("shared.txt")
	if err := root.WriteFile(target, []byte("shared"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if control, sddl := readSecurity(t, target.Abs()); control&windows.SE_DACL_PROTECTED != 0 {
		t.Errorf("a 0o644 write produced a protected DACL; only private modes are enforced: %s", sddl)
	}
}

func TestApplyMode_ScratchRootRestrictsItsContents(t *testing.T) {

	scratch, err := OpenScratch("fsroot-acl-*")
	if err != nil {
		t.Fatalf("OpenScratch: %v", err)
	}
	t.Cleanup(func() { _ = scratch.Close() })

	// Scratch holds the most sensitive transient data — spooled archives, and decrypted plaintext if
	// it ever spools — so a private write there must be enforced exactly as in a target tree.
	target := scratch.NewPath("spool.bin")
	if err := scratch.WriteFile(target, []byte("transient"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if control, sddl := readSecurity(t, target.Abs()); control&windows.SE_DACL_PROTECTED == 0 {
		t.Errorf("scratch file DACL is not protected: %s", sddl)
	}
}

func TestApplyMode_OpenFileCreateIsEnforced(t *testing.T) {

	root := testConfinedRoot(t)

	target := root.NewPath("opened.txt")

	file, err := root.OpenFile(target, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if control, sddl := readSecurity(t, target.Abs()); control&windows.SE_DACL_PROTECTED == 0 {
		t.Errorf("OpenFile(O_CREATE, 0o600) did not enforce the mode: %s", sddl)
	}
}

// region HELPER FUNCTIONS

// hasTrustee reports whether the SDDL grants a trustee named by either its well-known alias or its
// numeric SID.
//
// The trustee is the final field of an ACE, so each form is matched with the `;;;` separator before
// it and the closing parenthesis after it — a bare substring search would match an alias inside an
// unrelated SID and report a grant that is not there.
//
// Parameters:
//   - `sddl`: the security descriptor's SDDL form.
//   - `alias`: the two-letter SDDL alias, e.g. "SY".
//   - `sid`: the numeric SID, e.g. "S-1-5-18".
//
// Returns:
//   - `bool`: true when an ACE names the trustee in either rendering.
func hasTrustee(sddl, alias, sid string) bool {

	return strings.Contains(sddl, ";;;"+alias+aceTrusteeEnd) ||
		strings.Contains(sddl, ";;;"+sid+aceTrusteeEnd)
}

// testConfinedRoot opens a confined root at a per-test temporary directory, closed at test end.
//
// Parameters:
//   - `t`: the test.
//
// Returns:
//   - `Root`: the confined root.
func testConfinedRoot(t *testing.T) Dir {

	t.Helper()

	root, err := OpenConfined(t.TempDir())
	if err != nil {
		t.Fatalf("OpenConfined: %v", err)
	}
	t.Cleanup(func() { _ = root.Close() })

	return root
}

// readSecurity returns the object's security-descriptor control bits and its SDDL form.
//
// Parameters:
//   - `t`: the test.
//   - `path`: the object to inspect.
//
// Returns:
//   - `control`: the control bits, carrying SE_DACL_PROTECTED.
//   - `sddl`: the SDDL form, printed on failure so the actual ACL is visible.
func readSecurity(
	t *testing.T, path string,
) (control windows.SECURITY_DESCRIPTOR_CONTROL, sddl string) {

	t.Helper()

	descriptor, err := windows.GetNamedSecurityInfo(
		path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatalf("GetNamedSecurityInfo(%s): %v", path, err)
	}

	control, _, err = descriptor.Control()
	if err != nil {
		t.Fatalf("Control(%s): %v", path, err)
	}

	return control, descriptor.String()
}

// endregion
