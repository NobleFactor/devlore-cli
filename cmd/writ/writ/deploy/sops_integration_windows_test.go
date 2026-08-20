// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

//go:build windows

package deploy_test

import (
	"testing"

	"golang.org/x/sys/windows"
)

// assertPrivateFile asserts the file at `path` is private in this platform's own terms — here, a protected
// DACL.
//
// The portable-fact pattern (#433, #435): Mode().Perm() reports 0666 on Windows however private the file
// actually is, so the truth is the access-control list, read back in the shape phase 2's applymode tests
// established. Inheritance broken is the load-bearing part: without SE_DACL_PROTECTED the parent's inherited
// ACEs survive on the decrypted plaintext and it is private in name only.
func assertPrivateFile(t *testing.T, path string) {

	t.Helper()

	descriptor, err := windows.GetNamedSecurityInfo(
		path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatalf("GetNamedSecurityInfo(%s): %v", path, err)
	}

	control, _, err := descriptor.Control()
	if err != nil {
		t.Fatalf("Control(%s): %v", path, err)
	}

	if control&windows.SE_DACL_PROTECTED == 0 {
		t.Errorf("%s DACL is not protected; inherited ACEs survive on decrypted plaintext: %s",
			path, descriptor.String())
	}
}
