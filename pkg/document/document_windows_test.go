// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

//go:build windows

package document

import (
	"testing"

	"golang.org/x/sys/windows"
)

// The Windows truth for a restrictive write is the access-control list, not the mode bits — Mode().Perm()
// reports 0666 here however private the file actually is (#405 ruling 5). These tests read the DACL back off
// the written document, in the shape phase 2's applymode tests established (#558). A subtly wrong DACL fails
// OPEN: the file stays readable while every "did the call return nil" assertion passes.

// TestWriteFile_DefaultPermissionIsEnforcedOnWindows proves the 0o600 default lands as a protected DACL — the
// assertion the unix mode-bit check cannot make here, and the one #558 exists for.
func TestWriteFile_DefaultPermissionIsEnforcedOnWindows(t *testing.T) {

	root := testRoot(t)
	p := root.NewPath("secret.yaml")

	if err := WriteFile(root, p, &testDoc{Name: "private", Count: 1}); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	control, sddl := readSecurity(t, p.Abs())

	// Inheritance broken is the load-bearing part. Without SE_DACL_PROTECTED the parent's inherited ACEs
	// survive on the document and it is private in name only.
	if control&windows.SE_DACL_PROTECTED == 0 {
		t.Errorf("document's DACL is not protected; inherited ACEs survive: %s", sddl)
	}
}

// TestWriteFile_NonPrivatePermissionIsLeftAlone pins the boundary: 0o644 grants other, so it is not a private
// mode, and fsroot leaves the file inheriting its parent's DACL — protection would be wrong, not extra.
func TestWriteFile_NonPrivatePermissionIsLeftAlone(t *testing.T) {

	root := testRoot(t)
	p := root.NewPath("shared.yaml")

	if err := WriteFile(root, p, &testDoc{Name: "shared", Count: 2}, WithPerm(0o644)); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if control, sddl := readSecurity(t, p.Abs()); control&windows.SE_DACL_PROTECTED != 0 {
		t.Errorf("a 0o644 document has a protected DACL; only private modes are enforced: %s", sddl)
	}
}

// region HELPER FUNCTIONS

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
