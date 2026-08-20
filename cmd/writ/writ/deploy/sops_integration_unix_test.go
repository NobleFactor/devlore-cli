// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

//go:build !windows

package deploy_test

import (
	"os"
	"testing"
)

// assertPrivateFile asserts the file at `path` is private in this platform's own terms — here, mode 0600.
//
// The portable-fact pattern (#433, #435): the unix half asserts the mode bits; the windows half reads the
// access-control list back, because Mode().Perm() reports 0666 there however private the file actually is.
func assertPrivateFile(t *testing.T, path string) {

	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(%s): %v", path, err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("%s mode = %v, want 0600", path, info.Mode().Perm())
	}
}
