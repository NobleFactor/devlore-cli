// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

//go:build windows

package signing

import (
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"

	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
)

// The proof for the defect that opened issue #405: a generated private key written at 0600 into a 0700
// directory, both silently unenforced on Windows because os.Chmod there toggles only the read-only attribute.
//
// Asserting the mode bits would prove nothing — Mode().Perm() reports 0666 on Windows however private the
// object actually is (ruling 5) — so these tests read the access-control list back off the key and inspect
// it. A subtly wrong DACL fails OPEN: the key stays readable while every "did the call return nil" assertion
// passes.
//
// The two well-known trustees ruling 4 keeps alongside the owner buy real safety and cost nothing:
// restricting SYSTEM and Administrators is theater, since an administrator takes ownership at will.
const (
	systemAlias = "SY"
	systemSID   = "S-1-5-18"
	adminsAlias = "BA"
	adminsSID   = "S-1-5-32-544"
)

// TestGenerateLocalKey_PrivateKeyIsProtectedOnWindows is the assertion this campaign exists to make.
func TestGenerateLocalKey_PrivateKeyIsProtectedOnWindows(t *testing.T) {

	configRoot := fsroot.OpenWritableUnconfined(t.TempDir())
	t.Cleanup(func() { _ = configRoot.Close() })

	if _, err := localSigner(configRoot); err != nil {
		t.Fatalf("localSigner: %v", err)
	}

	keyPath := configRoot.NewPath(filepath.Join("signing", "ed25519"))

	control, sddl := readSecurity(t, keyPath.Abs())

	// Inheritance broken is the load-bearing part. Without SE_DACL_PROTECTED the config directory's
	// inherited ACEs survive on the key and it is private in name only.
	if control&windows.SE_DACL_PROTECTED == 0 {
		t.Errorf("private key's DACL is not protected; inherited ACEs survive: %s", sddl)
	}

	if !hasTrustee(sddl, systemAlias, systemSID) {
		t.Errorf("private key's DACL is missing SYSTEM: %s", sddl)
	}
	if !hasTrustee(sddl, adminsAlias, adminsSID) {
		t.Errorf("private key's DACL is missing Administrators: %s", sddl)
	}

	// Exactly three trustees. A fourth allow ACE would leave the DACL "protected" while granting someone
	// else read access to a signing key, so the count is as load-bearing as the flag.
	if got := strings.Count(sddl, "(A;"); got != 3 {
		t.Errorf("private key's DACL has %d allow ACEs, want 3 (owner, SYSTEM, Administrators): %s", got, sddl)
	}
}

// TestGenerateLocalKey_SigningDirectoryIsProtectedOnWindows covers the 0700 directory holding the key: a
// protected key inside a world-traversable directory is a weaker guarantee than it looks.
func TestGenerateLocalKey_SigningDirectoryIsProtectedOnWindows(t *testing.T) {

	configRoot := fsroot.OpenWritableUnconfined(t.TempDir())
	t.Cleanup(func() { _ = configRoot.Close() })

	if _, err := localSigner(configRoot); err != nil {
		t.Fatalf("localSigner: %v", err)
	}

	control, sddl := readSecurity(t, configRoot.NewPath("signing").Abs())

	if control&windows.SE_DACL_PROTECTED == 0 {
		t.Errorf("signing directory's DACL is not protected: %s", sddl)
	}
}

// TestGenerateLocalKey_PublicHalfIsNotProtected pins the deliberate asymmetry. The .pub half is public trust
// material at 0644, which grants `other`, so ruling 4's boundary leaves it inheriting its parent's DACL. A
// future reader who "fixes" this to match the private half would be locking up a file whose whole purpose is
// to be copied into someone else's allowed_signers.
func TestGenerateLocalKey_PublicHalfIsNotProtected(t *testing.T) {

	configRoot := fsroot.OpenWritableUnconfined(t.TempDir())
	t.Cleanup(func() { _ = configRoot.Close() })

	if _, err := localSigner(configRoot); err != nil {
		t.Fatalf("localSigner: %v", err)
	}

	control, sddl := readSecurity(t, configRoot.NewPath(filepath.Join("signing", "ed25519.pub")).Abs())

	if control&windows.SE_DACL_PROTECTED != 0 {
		t.Errorf("public key's DACL is protected; the .pub half is meant to be readable: %s", sddl)
	}
}

// region HELPER FUNCTIONS

// hasTrustee reports whether an SDDL string grants a trustee, in either the alias or numeric rendering.
//
// SDDL renders well-known accounts as two-letter aliases rather than SIDs, and which form appears varies by
// host — matching only the numeric form is what made phase 2's first CI run fail against a correct DACL. The
// match is anchored on the ACE's trustee field so an alias cannot match inside an unrelated SID.
func hasTrustee(sddl, alias, sid string) bool {

	return strings.Contains(sddl, ";;;"+alias+")") || strings.Contains(sddl, ";;;"+sid+")")
}

// readSecurity reads an object's DACL back off the filesystem.
//
// Parameters:
//   - `t`: the test harness.
//   - `path`: the absolute path of the object to inspect.
//
// Returns:
//   - `windows.SECURITY_DESCRIPTOR_CONTROL`: the descriptor's control flags, carrying SE_DACL_PROTECTED.
//   - `string`: the descriptor's SDDL form, printed on failure so the actual ACL is visible.
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
