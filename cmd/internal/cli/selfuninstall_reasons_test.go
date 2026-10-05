// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/sink"
	"github.com/NobleFactor/devlore-cli/pkg/status"
)

// refuseOneRecordedFile makes one of the installed files unremovable by taking write permission from its parent
// directory, and returns that file's absolute path. It restores the mode on cleanup so t.TempDir can be removed.
//
// The parent-directory route, not a mode on the file itself: on Unix, removing a directory entry needs write on the
// directory, so this refuses the delete while leaving the file readable (its hash still computes, which is what
// separates "refused" from "unreadable").
func refuseOneRecordedFile(t *testing.T, prefix, tool string) string {

	t.Helper()

	victim := filepath.Join(prefix, "bin", executableName(tool))
	parent := filepath.Dir(victim)

	info, err := os.Stat(parent)
	if err != nil {
		t.Fatalf("stat %s: %v", parent, err)
	}
	if err := os.Chmod(parent, 0o500); err != nil {
		t.Fatalf("chmod %s: %v", parent, err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, info.Mode().Perm()) })

	return victim
}

// TestRunSelfUninstall_RefusedFileIsNotCalledModified is #1003: a file the filesystem refuses to delete is reported
// as refused, not "modified". Only a file whose content changed since it was recorded is modified.
func TestRunSelfUninstall_RefusedFileIsNotCalledModified(t *testing.T) {

	if runtime.GOOS == "windows" {
		t.Skip("the refused-delete setup uses Unix directory permissions; the running-image case is the Windows scenario")
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignores the directory permission that refuses the delete")
	}

	prefix, info := installIntoTempPrefix(t)
	victim := refuseOneRecordedFile(t, prefix, info.Name)

	captured, narration := sink.Capture()
	previous := UI()
	SetUI(status.NewNarrator(info.Name, captured))
	t.Cleanup(func() { SetUI(previous) })

	if err := runSelfUninstall(prefix, info); err != nil {
		t.Fatalf("runSelfUninstall: %v", err)
	}

	said := narration.String()
	if strings.Contains(said, "modified") {
		t.Errorf("a refused file was reported as modified:\n%s", said)
	}
	if _, err := os.Stat(victim); err != nil {
		t.Errorf("the refused file should have been left in place, but stat failed: %v", err)
	}
}

// TestRunSelfUninstall_KeepsManifestWhileAFileRemains is #1003: the manifest is the record of what the tool owns, so
// it is not deleted while a file it records is still on disk. Otherwise a file left behind can never be found again.
func TestRunSelfUninstall_KeepsManifestWhileAFileRemains(t *testing.T) {

	if runtime.GOOS == "windows" {
		t.Skip("the refused-delete setup uses Unix directory permissions; the running-image case is the Windows scenario")
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignores the directory permission that refuses the delete")
	}

	prefix, info := installIntoTempPrefix(t)
	refuseOneRecordedFile(t, prefix, info.Name)

	if err := runSelfUninstall(prefix, info); err != nil {
		t.Fatalf("runSelfUninstall: %v", err)
	}

	manifest := filepath.Join(prefix, "share", info.Name, "manifest.json")
	if _, err := os.Stat(manifest); err != nil {
		t.Errorf("the manifest was removed while a recorded file remained (stat: %v); a later run cannot finish", err)
	}
}
