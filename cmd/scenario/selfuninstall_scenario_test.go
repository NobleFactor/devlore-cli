// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package scenario

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestSelfUninstallFromInstalledCopyScenario drives `self uninstall` through the installed copy of the binary, so the
// running image is the file the uninstall must remove (#1003).
//
// TestSelfInstallScenario runs the uninstall through the built binary under build/, so the file it deletes is never
// the one running -- the case that fails on Windows, where a running image cannot be deleted. Here the installed copy
// at <prefix>/bin runs its own `self uninstall`, so on Windows it must schedule the image's deletion for after it
// exits. Nothing it installed may remain once it has.
func TestSelfUninstallFromInstalledCopyScenario(t *testing.T) {

	if os.Getenv("DEVLORE_SCENARIO_RUN") == "" {
		t.Skip("scenario runs under make test-scenario (DEVLORE_SCENARIO_RUN=1)")
	}

	for _, tool := range selfInstallTools {
		t.Run(tool, func(t *testing.T) {

			built := toolBinary(t, tool)
			sandbox := t.TempDir()
			prefix := filepath.Join(sandbox, "prefix")
			environment := sandboxEnvironment(sandbox)

			if out, err := run(t, built, environment, "self", "install", prefix, "--shell", "bash"); err != nil {
				t.Fatalf("%s self install failed: %v\n%s", tool, err, out)
			}

			recorded := manifestFiles(t, prefix, tool)
			if len(recorded) == 0 {
				t.Fatalf("%s manifest records no files", tool)
			}

			// The installed copy, not the built one: its own running image is <prefix>/bin/<tool>.
			installed := filepath.Join(prefix, "bin", tool+exeSuffix())
			if out, err := run(t, installed, environment, "self", "uninstall", prefix, "--force"); err != nil {
				t.Fatalf("%s self uninstall (from the installed copy) failed: %v\n%s", tool, err, out)
			}

			// On Windows the image is deleted after the uninstall process exits, by a cmd.exe that retries until the
			// image is released, up to its cap: the wait for the image is that cap (#1045). The poll keeps a fast
			// platform fast. Everything else is gone by then: removed during the uninstall, or, the manifest, in the
			// same cmd.exe command right after the image.
			if !gone(filepath.Join(prefix, "bin", tool+exeSuffix()), deferredDeletionWait) {
				t.Errorf("%s left its running image at %s after uninstall", tool, installed)
			}
			for _, relative := range recorded {
				if !gone(filepath.Join(prefix, relative), afterDeletionWait) {
					t.Errorf("%s left %s behind after uninstall", tool, relative)
				}
			}
			if !gone(filepath.Join(prefix, "share", tool, "manifest.json"), afterDeletionWait) {
				t.Errorf("%s left its manifest behind after uninstall", tool)
			}
		})
	}
}

// deferredDeletionWait is how long the scenario waits for the running image on Windows: the cmd.exe that deletes it
// retries about once a second, up to deleteRetries (120) times (cmd/internal/cli/selfdelete_windows.go), plus a
// margin. A cold image can be held well past ten seconds: on CI's windows-arm64 runner writ's was (#1045).
const deferredDeletionWait = 130 * time.Second

// afterDeletionWait is how long the scenario waits for anything else, all of it gone by the time the image is.
const afterDeletionWait = 10 * time.Second

// gone reports whether `path` does not exist, waiting up to `within` for a deletion another process carries out after
// this one observes it -- the Windows running-image case, where `cmd.exe` deletes the file once the uninstall has
// exited. It returns as soon as the path is gone.
//
// Parameters:
//   - `path`: the path that must not exist.
//   - `within`: the longest wait.
//
// Returns:
//   - `bool`: whether the path is gone.
func gone(path string, within time.Duration) bool {

	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}

	_, err := os.Stat(path)
	return os.IsNotExist(err)
}
