// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- installedPrefixOf ---

// TestInstalledPrefixOf_IsTheDirectoryAboveBin finds the prefix of a binary at `<prefix>/bin/<name>`.
func TestInstalledPrefixOf_IsTheDirectoryAboveBin(t *testing.T) {

	prefix := installedPrefix(t, nil, "writ")
	executable := filepath.Join(prefix, "bin", executableName("writ"))

	gotPrefix, gotBinary, err := installedPrefixOf(executable)
	if err != nil {
		t.Fatalf("installedPrefixOf: %v", err)
	}
	if gotPrefix != prefix || gotBinary != executable {
		t.Errorf("installedPrefixOf = %q, %q; want %q, %q", gotPrefix, gotBinary, prefix, executable)
	}
}

// TestInstalledPrefixOf_ResolvesLinks pins that a binary reached through a symbolic link is found where it is
// installed, not where the link is.
func TestInstalledPrefixOf_ResolvesLinks(t *testing.T) {

	prefix := installedPrefix(t, nil, "writ")
	installed := filepath.Join(prefix, "bin", executableName("writ"))

	link := filepath.Join(t.TempDir(), executableName("writ"))
	if err := os.Symlink(installed, link); err != nil {
		t.Skipf("this platform cannot make a symbolic link here: %v", err)
	}

	gotPrefix, gotBinary, err := installedPrefixOf(link)
	if err != nil {
		t.Fatalf("installedPrefixOf: %v", err)
	}
	if gotPrefix != prefix || gotBinary != installed {
		t.Errorf("installedPrefixOf = %q, %q; want %q, %q", gotPrefix, gotBinary, prefix, installed)
	}
}

// TestInstalledPrefixOf_RefusesABinaryOutsideBin pins that a binary not in a `bin/` directory has no prefix, which is
// a configuration error, naming the binary.
func TestInstalledPrefixOf_RefusesABinaryOutsideBin(t *testing.T) {

	elsewhere, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	binary := filepath.Join(elsewhere, executableName("writ"))
	writeTestFile(t, binary, "writ old")

	_, _, err = installedPrefixOf(binary)
	if ExitCode(err) != ExitConfig {
		t.Errorf("exit %d, want %d: %v", ExitCode(err), ExitConfig, err)
	}
	if err == nil || !strings.Contains(err.Error(), binary) {
		t.Errorf("error = %v; want it to name %s", err, binary)
	}
}

// TestInstalledPrefixOf_ReportsABinaryThatIsNotThere pins that a binary whose links cannot be resolved is reported,
// naming it.
func TestInstalledPrefixOf_ReportsABinaryThatIsNotThere(t *testing.T) {

	missing := filepath.Join(t.TempDir(), "bin", executableName("writ"))

	_, _, err := installedPrefixOf(missing)
	if err == nil || !strings.Contains(err.Error(), missing) {
		t.Errorf("installedPrefixOf = %v; want an error naming %s", err, missing)
	}
}
