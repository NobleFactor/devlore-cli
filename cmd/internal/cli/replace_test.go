// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
)

// heldRunningEnv, set to 1 in a test binary's environment, turns [TestInstallBinary_HeldRunning] into a program
// that runs until its standard input closes; heldRunningReady is the line it prints once it is running.
const (
	heldRunningEnv   = "DEVLORE_TEST_HELD_RUNNING"
	heldRunningReady = "held running"
)

// openPrefix opens a root at `prefix` with its `bin` directory created, and closes it when the test ends.
//
// Parameters:
//   - `t`: the test harness.
//   - `prefix`: an existing directory to open.
//
// Returns:
//   - `fsroot.Dir`: the open root.
func openPrefix(t *testing.T, prefix string) fsroot.Dir {

	t.Helper()

	if err := os.MkdirAll(filepath.Join(prefix, "bin"), 0o750); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	prefixRoot, err := fsroot.OpenExisting(prefix)
	if err != nil {
		t.Fatalf("OpenExisting: %v", err)
	}
	t.Cleanup(func() { _ = prefixRoot.Close() })

	return prefixRoot
}

// writeTestFile writes `content` to `path`, failing the test when it cannot.
//
// Parameters:
//   - `t`: the test harness.
//   - `path`: the file to write; its directory must exist.
//   - `content`: what to write.
func writeTestFile(t *testing.T, path, content string) {

	t.Helper()

	if err := os.WriteFile(path, []byte(content), 0o750); err != nil {
		t.Fatalf("WriteFile %s: %v", path, err)
	}
}

// statTestFile returns the file info at `path`, with the identity of the file there now, failing the test when there
// is none.
//
// On Windows, [os.Stat] reads a file's identity lazily, by path, the first time [os.SameFile] asks for it, so info
// taken before a replacement and compared after it would carry the replacement's identity. Asking once here pins
// it to the file the path names now.
//
// Parameters:
//   - `t`: the test harness.
//   - `path`: the file to stat.
//
// Returns:
//   - `os.FileInfo`: the file's info, which [os.SameFile] compares by identity.
func statTestFile(t *testing.T, path string) os.FileInfo {

	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat %s: %v", path, err)
	}

	if !os.SameFile(info, info) {
		t.Fatalf("Stat %s: the file's identity cannot be read", path)
	}

	return info
}

// startHeldRunning copies this test binary to `path` and runs it from there until the test ends.
//
// A copy of a real executable, started from the place an install is about to write, is what holds a binary
// running: Linux refuses to open it for writing, and Windows to replace it, only while it is mapped as a running
// image. This returns once the copy says it is running, so the refusal holds from the moment it returns.
//
// Parameters:
//   - `t`: the test harness.
//   - `path`: where to place the copy and run it from; its directory must exist.
func startHeldRunning(t *testing.T, path string) {

	t.Helper()

	self, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}

	image, err := os.ReadFile(self)
	if err != nil {
		t.Fatalf("reading this test binary: %v", err)
	}
	if err := os.WriteFile(path, image, 0o750); err != nil {
		t.Fatalf("copying this test binary to %s: %v", path, err)
	}

	// The test's context is canceled before its cleanups run, which kills the process; the cleanup waits for it,
	// so the temporary directory is removed only after the image is released.
	cmd := exec.CommandContext(t.Context(), path, "-test.run=^TestInstallBinary_HeldRunning$")
	cmd.Env = append(os.Environ(), heldRunningEnv+"=1")

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("StdinPipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("StdoutPipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("starting %s: %v", path, err)
	}

	t.Cleanup(func() {
		_ = stdin.Close()
		_ = cmd.Wait()
	})

	ready, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil || ready != heldRunningReady+"\n" {
		t.Fatalf("the copy at %s did not say it was running: %q, %v", path, ready, err)
	}
}

// manifestRecords reports whether the `selftest` manifest in `prefix` records `relative`, failing the test when
// there is no manifest to read.
//
// Parameters:
//   - `t`: the test harness.
//   - `prefix`: the installation prefix.
//   - `relative`: the path to look for, relative to `prefix`.
//
// Returns:
//   - `bool`: true when the manifest has an entry for `relative`.
func manifestRecords(t *testing.T, prefix, relative string) bool {

	t.Helper()

	m, err := readManifest(prefix, "selftest")
	if err != nil {
		t.Fatalf("readManifest: %v", err)
	}

	return slices.ContainsFunc(m.Files, func(entry manifestEntry) bool {
		return filepath.Clean(entry.Path) == relative
	})
}

// refuseRemoval places a file at `path` that the filesystem refuses to remove until the test ends.
//
// On Windows the refusal is the one a set-aside binary meets there: a running image, a copy of this test binary
// started from `path`. Elsewhere a running file can be removed, so the refusal is its directory's, made read-only
// for the test and writable again when it ends, so the temporary directory can be removed. Root removes a file
// from a read-only directory, so there the test is skipped.
//
// Parameters:
//   - `t`: the test harness.
//   - `path`: the file to place; its directory must exist.
func refuseRemoval(t *testing.T, path string) {

	t.Helper()

	if runtime.GOOS == "windows" {
		startHeldRunning(t, path)
		return
	}

	if os.Geteuid() == 0 {
		t.Skip("root removes a file from a read-only directory")
	}

	writeTestFile(t, path, "a binary nothing can remove")

	dir := filepath.Dir(path)
	if err := os.Chmod(dir, 0o550); err != nil {
		t.Fatalf("Chmod %s: %v", dir, err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o750) })
}

// --- installBinary ---

// TestInstallBinary_HeldRunning is the program [startHeldRunning] runs, not a test of its own.
//
// It returns at once unless [heldRunningEnv] is set. When it is set, it says it is running, then reads its standard
// input to the end: the test that started it holds the other end, so it runs until that test closes it, kills it,
// or exits.
func TestInstallBinary_HeldRunning(t *testing.T) {

	if os.Getenv(heldRunningEnv) != "1" {
		return
	}

	fmt.Println(heldRunningReady)
	_, _ = io.Copy(io.Discard, os.Stdin)
}

// TestInstallBinary_ReplacesARunningBinary is Requirement 9 of #947.
//
// Every build published before it failed here: `copyFile`'s truncating create is refused with ETXTBSY on Linux
// while the target runs, and Windows refuses to rewrite, remove or replace a running image. The replacement is a
// rename, so the running process keeps the file it started from and the name moves to a new one; on Unix that is
// a new inode at the same name, and on Windows the running image is renamed aside first.
func TestInstallBinary_ReplacesARunningBinary(t *testing.T) {

	prefix := t.TempDir()
	prefixRoot := openPrefix(t, prefix)

	target := filepath.Join(prefix, "bin", executableName("selftest"))
	startHeldRunning(t, target)
	running := statTestFile(t, target)

	source := filepath.Join(t.TempDir(), executableName("selftest"))
	writeTestFile(t, source, "the new build")

	installed, err := installBinary(prefixRoot, source, "selftest")
	if err != nil {
		t.Fatalf("installBinary over a running binary: %v", err)
	}
	if installed != target {
		t.Errorf("installBinary = %q, want %q", installed, target)
	}

	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(content) != "the new build" {
		t.Errorf("the target holds %d bytes that are not the new build", len(content))
	}

	if os.SameFile(running, statTestFile(t, target)) {
		t.Error("the target is still the running file; it was rewritten in place, not replaced")
	}

	if _, err := os.Stat(target + ".new"); !os.IsNotExist(err) {
		t.Errorf("the staged copy survived the replacement (stat error = %v)", err)
	}

	if runtime.GOOS == "windows" {
		aside := statTestFile(t, filepath.Join(prefix, "bin", setAsideName("selftest")))
		if !os.SameFile(running, aside) {
			t.Error("the set-aside binary is not the one that was running")
		}
	}
}

// TestInstallBinary_OverwritesAStaleStagedCopy is D5 of #947's plan.
//
// The staged copy has a fixed name so that a run that fails between writing it and renaming it leaves one file
// behind, not one per attempt, and the next run simply writes over it.
func TestInstallBinary_OverwritesAStaleStagedCopy(t *testing.T) {

	prefix := t.TempDir()
	prefixRoot := openPrefix(t, prefix)

	target := filepath.Join(prefix, "bin", executableName("selftest"))
	writeTestFile(t, target+".new", "left behind by a failed run, and longer than the new build")

	source := filepath.Join(t.TempDir(), executableName("selftest"))
	writeTestFile(t, source, "the new build")

	if _, err := installBinary(prefixRoot, source, "selftest"); err != nil {
		t.Fatalf("installBinary: %v", err)
	}

	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(content) != "the new build" {
		t.Errorf("target = %q, want the new build", content)
	}

	if _, err := os.Stat(target + ".new"); !os.IsNotExist(err) {
		t.Errorf("the stale staged copy survived the install (stat error = %v)", err)
	}
}

// TestInstallBinary_LeavesItselfInPlaceThroughALinkedPrefix is the early return, judged by identity.
//
// An installed copy installing itself has nothing to write. The check compared paths, so a prefix reached through
// a symbolic link named the same file by another path and defeated it: the install then copied the file onto
// itself, which truncates it, or, while it ran, failed with `text file busy`.
func TestInstallBinary_LeavesItselfInPlaceThroughALinkedPrefix(t *testing.T) {

	actual := filepath.Join(t.TempDir(), "actual")
	if err := os.MkdirAll(filepath.Join(actual, "bin"), 0o750); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	linked := filepath.Join(t.TempDir(), "linked")
	if err := os.Symlink(actual, linked); err != nil {
		t.Skipf("this platform will not create the link: %v", err)
	}

	installed := filepath.Join(actual, "bin", executableName("selftest"))
	writeTestFile(t, installed, "the installed build")
	before := statTestFile(t, installed)

	prefixRoot := openPrefix(t, linked)

	got, err := installBinary(prefixRoot, installed, "selftest")
	if err != nil {
		t.Fatalf("installBinary from the installed copy: %v", err)
	}
	if want := filepath.Join(linked, "bin", executableName("selftest")); got != want {
		t.Errorf("installBinary = %q, want %q", got, want)
	}

	content, err := os.ReadFile(installed)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(content) != "the installed build" {
		t.Errorf("the installed copy now holds %q; it was copied onto itself", content)
	}

	if !os.SameFile(before, statTestFile(t, installed)) {
		t.Error("the installed copy was replaced; it is already in place, so nothing should have been written")
	}
}

// --- retireSetAside ---

// TestRetireSetAside_RemovesOneNothingRuns is how a set-aside binary leaves: the install after it removes it.
//
// The name is passed, not derived, so this runs on every platform: only Windows sets a binary aside.
func TestRetireSetAside_RemovesOneNothingRuns(t *testing.T) {

	prefix := t.TempDir()
	prefixRoot := openPrefix(t, prefix)
	aside := filepath.Join(prefix, "bin", "selftest.exe.old")
	writeTestFile(t, aside, "the build set aside last time")

	retireSetAside(prefixRoot, "selftest.exe.old")

	if _, err := os.Stat(aside); !os.IsNotExist(err) {
		t.Errorf("the set-aside binary survived the install after it (stat error = %v)", err)
	}
}

// TestRetireSetAside_KeepsOneItCannotRemove leaves a set-aside binary that still runs, for the install to record.
//
// On Windows the image of the `self upgrade` that placed a program runs from the set-aside name while that
// program's own `self install` runs, and nothing can remove it. It stays on disk, so it stays in the record, where
// a later install removes it and `self uninstall` reaches it.
func TestRetireSetAside_KeepsOneItCannotRemove(t *testing.T) {

	prefix := t.TempDir()
	prefixRoot := openPrefix(t, prefix)
	aside := filepath.Join("bin", "selftest.exe.old")
	refuseRemoval(t, filepath.Join(prefix, aside))

	retireSetAside(prefixRoot, "selftest.exe.old")

	if _, err := os.Stat(filepath.Join(prefix, aside)); err != nil {
		t.Fatalf("the set-aside binary is gone, though nothing could remove it (stat error = %v)", err)
	}
	if got, want := setAsideToRecord(prefixRoot, "selftest.exe.old"), []string{aside}; !slices.Equal(got, want) {
		t.Errorf("setAsideToRecord = %v, want %v", got, want)
	}
}

// TestRetireSetAside_NothingSetAside covers the ordinary install: no set-aside binary, or no name for one.
//
// With no name, `bin/` itself is what the name would resolve to, and an empty `bin/` is removable: it must stay.
func TestRetireSetAside_NothingSetAside(t *testing.T) {

	prefix := t.TempDir()
	prefixRoot := openPrefix(t, prefix)

	retireSetAside(prefixRoot, "selftest.exe.old")
	retireSetAside(prefixRoot, "")

	if _, err := os.Stat(filepath.Join(prefix, "bin")); err != nil {
		t.Errorf("bin/ is gone after retiring nothing (stat error = %v)", err)
	}
}

// --- setAsideToRecord ---

// TestSetAsideToRecord_RecordsOneItFinds is the half of Requirement 9 that lets `self uninstall` reach it.
//
// The name is passed, not derived, so this runs on every platform: only Windows sets a binary aside.
func TestSetAsideToRecord_RecordsOneItFinds(t *testing.T) {

	prefix := t.TempDir()
	prefixRoot := openPrefix(t, prefix)
	writeTestFile(t, filepath.Join(prefix, "bin", "selftest.exe.old"), "the build that was running")

	got := setAsideToRecord(prefixRoot, "selftest.exe.old")

	if want := []string{filepath.Join("bin", "selftest.exe.old")}; !slices.Equal(got, want) {
		t.Errorf("setAsideToRecord = %v, want %v", got, want)
	}
}

// TestSetAsideToRecord_RecordsOneTheRecordAlreadyNames is the same build set aside twice.
//
// A set-aside binary on disk when the record is written is one this install made or one that still runs, and
// either way the tool's. Its content cannot tell it from the one the record names: the same build set aside
// twice, by a repeated install or by an upgrade retried from the installed copy, is byte for byte that file.
// Judged by content, it was left out of the record and handed to the retirement, which Windows refused while it
// ran, and it stayed on disk with nothing recording it.
func TestSetAsideToRecord_RecordsOneTheRecordAlreadyNames(t *testing.T) {

	prefix := t.TempDir()
	prefixRoot := openPrefix(t, prefix)
	aside := filepath.Join("bin", "selftest.exe.old")

	writeTestFile(t, filepath.Join(prefix, aside), "the build set aside, both times")
	if err := writeManifest(prefixRoot, "selftest", "1.0.0", []string{aside}); err != nil {
		t.Fatalf("writeManifest: %v", err)
	}

	got := setAsideToRecord(prefixRoot, "selftest.exe.old")

	if want := []string{aside}; !slices.Equal(got, want) {
		t.Errorf("setAsideToRecord = %v, want %v", got, want)
	}
}

// TestSetAsideToRecord_NothingSetAside covers the ordinary install: no set-aside binary, or no name for one.
func TestSetAsideToRecord_NothingSetAside(t *testing.T) {

	prefix := t.TempDir()
	prefixRoot := openPrefix(t, prefix)

	if got := setAsideToRecord(prefixRoot, "selftest.exe.old"); len(got) != 0 {
		t.Errorf("setAsideToRecord with no file = %v, want nothing", got)
	}
	if got := setAsideToRecord(prefixRoot, ""); len(got) != 0 {
		t.Errorf("setAsideToRecord with no name = %v, want nothing", got)
	}
}

// --- runSelfInstall ---

// TestRunSelfInstall_RecordsTheSetAsideBinary proves the recording is wired into the install, where it applies.
//
// A copy of this test binary runs from the prefix while the test binary installs itself there: Windows sets the
// running copy aside, and the install records it.
func TestRunSelfInstall_RecordsTheSetAsideBinary(t *testing.T) {

	if runtime.GOOS != "windows" {
		t.Skip("only Windows sets a running binary aside")
	}

	prefix := installSandbox(t)
	if err := os.MkdirAll(filepath.Join(prefix, "bin"), 0o750); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	target := filepath.Join(prefix, "bin", executableName("selftest"))
	startHeldRunning(t, target)
	running := statTestFile(t, target)

	installOnce(t, prefix)

	aside := filepath.Join("bin", setAsideName("selftest"))
	if !os.SameFile(running, statTestFile(t, filepath.Join(prefix, aside))) {
		t.Errorf("%s is not the binary that was running", aside)
	}
	if !manifestRecords(t, prefix, aside) {
		t.Errorf("the manifest does not record %s", aside)
	}
}

// TestRunSelfInstall_KeepsARunningSetAsideRecorded is the same build set aside twice, while it still runs.
//
// A copy of this test binary runs from the set-aside name an earlier install recorded, and it is the build this
// install places: byte for byte the file the record names. Nothing can remove it while it runs, so the install
// keeps it in the record, for a later install to remove and for `self uninstall` to reach.
func TestRunSelfInstall_KeepsARunningSetAsideRecorded(t *testing.T) {

	if runtime.GOOS != "windows" {
		t.Skip("only Windows sets a running binary aside")
	}

	prefix := installSandbox(t)
	prefixRoot := openPrefix(t, prefix)
	aside := filepath.Join("bin", setAsideName("selftest"))

	startHeldRunning(t, filepath.Join(prefix, aside))
	if err := writeManifest(prefixRoot, "selftest", "1.0.0", []string{aside}); err != nil {
		t.Fatalf("writeManifest: %v", err)
	}

	installOnce(t, prefix)

	if _, err := os.Stat(filepath.Join(prefix, aside)); err != nil {
		t.Fatalf("the running set-aside binary is gone (stat error = %v)", err)
	}
	if !manifestRecords(t, prefix, aside) {
		t.Errorf("the manifest no longer records %s, which still runs", aside)
	}
}

// TestRunSelfInstall_RetiresAStaleSetAside is the install after a set-aside removing it once nothing runs it.
//
// It goes whether or not a record names it: the name is the installer's own, and the binary there was replaced.
func TestRunSelfInstall_RetiresAStaleSetAside(t *testing.T) {

	if runtime.GOOS != "windows" {
		t.Skip("only Windows sets a running binary aside")
	}

	for _, recorded := range []bool{true, false} {
		t.Run(fmt.Sprintf("recorded=%t", recorded), func(t *testing.T) {

			prefix := installSandbox(t)
			prefixRoot := openPrefix(t, prefix)
			aside := filepath.Join("bin", setAsideName("selftest"))

			writeTestFile(t, filepath.Join(prefix, aside), "the build set aside last time")
			if recorded {
				if err := writeManifest(prefixRoot, "selftest", "1.0.0", []string{aside}); err != nil {
					t.Fatalf("writeManifest: %v", err)
				}
			}

			installOnce(t, prefix)

			if _, err := os.Stat(filepath.Join(prefix, aside)); !os.IsNotExist(err) {
				t.Errorf("the stale set-aside binary survived the install (stat error = %v)", err)
			}
			if manifestRecords(t, prefix, aside) {
				t.Errorf("the manifest records %s, which the install should have removed", aside)
			}
		})
	}
}

// TestRunSelfInstall_FailsWhenTheManifestCannotBeWritten is the last bullet of Requirement 9.
//
// The manifest is the record of what the tool owns. An install that places files and cannot record them has
// stranded them, and `self upgrade` judges each program by its child's exit status, so the failure is the
// install's, not a warning beside a success.
func TestRunSelfInstall_FailsWhenTheManifestCannotBeWritten(t *testing.T) {

	prefix := installSandbox(t)

	// A directory where the manifest goes: nothing can write the file over it, on any platform.
	if err := os.MkdirAll(filepath.Join(prefix, relativeManifestPath("selftest")), 0o750); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	info := SelfInstallInfo{Name: "selftest", Version: "1.0.0"}
	err := runSelfInstall(&cobra.Command{Use: "selftest"}, prefix, info, installFlags{Shells: []string{"bash"}})

	if err == nil {
		t.Fatal("runSelfInstall succeeded without writing its manifest")
	}
	if want := "failed to write manifest " + manifestPath(prefix, "selftest"); !strings.Contains(err.Error(), want) {
		t.Fatalf("runSelfInstall failed before it reached its manifest: %v", err)
	}
}
