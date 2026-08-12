// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

//go:build unix

package file

import (
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
)

// --- Chown ---

// TestWriteText_AppliesChownWhenSpecified verifies the chown parameter drives applyChown through to os.Chown.
//
// The subject — uid/gid ownership read through syscall.Stat_t — exists only on Unix, so the build constraint scopes
// the test rather than skipping it; Windows compilation of the package's tests failed on Stat_t before this file
// existed (#373 phase 1b). Uses the current uid:gid as the target spec — the only spec that doesn't require
// CAP_CHOWN — so the test runs without privilege.
func TestWriteText_AppliesChownWhenSpecified(t *testing.T) {

	tmp := t.TempDir()
	path := filepath.Join(tmp, "owned.txt")

	p := testProvider(t, tmp)

	spec := strconv.Itoa(os.Getuid()) + ":" + strconv.Itoa(os.Getgid())
	_, _, err := p.WriteText(testActivation(t, p.RuntimeEnvironment()), path, "owned content", 0o644, spec)
	if err != nil {
		t.Fatalf("WriteText with chown=%q: %v", spec, err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	stat := info.Sys().(*syscall.Stat_t)
	if int(stat.Uid) != os.Getuid() {
		t.Errorf("uid = %d, want %d", stat.Uid, os.Getuid())
	}
	if int(stat.Gid) != os.Getgid() {
		t.Errorf("gid = %d, want %d", stat.Gid, os.Getgid())
	}
}
