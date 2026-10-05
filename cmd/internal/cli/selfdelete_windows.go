// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

//go:build windows

package cli

import (
	"context"
	"fmt"
	"os/exec"
	"syscall"

	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
)

// Windows process-creation flags (golang.org/x/sys/windows mirrors these; named here to keep this file's imports to
// the standard library and fsroot).
const (
	createNoWindow  = 0x08000000 // CREATE_NO_WINDOW: the child gets no console window.
	detachedProcess = 0x00000008 // DETACHED_PROCESS: the child has no console and outlives this process.

	// deleteRetries bounds the wait: ping -n 2 pauses about a second between the loop's tries, so this is roughly a
	// two-minute ceiling on a process that never exits. A normal shutdown releases the image in moments.
	deleteRetries = 120
)

// deferRunningImageDeletion arranges for the running image to be deleted once this process exits, and reports whether
// it took ownership of that deletion.
//
// Windows refuses to delete a mapped image, so the running .exe cannot delete itself in place. The deletion is handed
// to cmd.exe -- always present, copied nowhere, left nowhere. Its loop retries the delete until this process releases
// the image and the delete succeeds, then stops; a cap bounds a process that never exits. ping, not timeout, is the
// pause: timeout fails without a console, which is exactly this detached case.
//
// It returns true when cmd.exe was started, so the caller leaves the running image out of its own in-place removal.
// It returns false only when cmd.exe could not be started; the caller then tries the delete in place, which fails and
// is reported and kept recorded, and a later run -- no longer the running image -- removes the file.
//
// Parameters:
//   - `prefixRoot`: the installation prefix the entry is relative to.
//   - `running`: the running image's recorded entry.
//
// Returns:
//   - `bool`: whether cmd.exe took ownership of the deletion.
func deferRunningImageDeletion(prefixRoot fsroot.Dir, running manifestEntry) bool {

	// The path is quoted here, with literal quote characters, rather than with %q in the format: %q applies Go's
	// escaping, which would double the backslashes in a Windows path.
	quoted := `"` + prefixRoot.NewPath(running.Path).Abs() + `"`

	// The outer quote pair wraps the whole command; cmd /c strips it (the command holds special characters, so the
	// inner quotes around the path are preserved). %%i is a literal %i on the command line -- doubling is fmt's, not
	// cmd's.
	commandLine := fmt.Sprintf(
		`cmd.exe /c "for /l %%i in (1,1,%d) do (del /f /q %s 2>nul & if not exist %s exit & ping -n 2 127.0.0.1 >nul)"`,
		deleteRetries, quoted, quoted,
	)

	// context.Background(): the child is detached and must outlive this process, so its lifetime is not tied to a
	// context that this process's exit would cancel.
	command := exec.CommandContext(context.Background(), "cmd.exe") //nolint:gosec // G204: cmd.exe is the system shell; the command line is built here, not from input
	command.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow | detachedProcess,
		CmdLine:       commandLine,
	}

	if err := command.Start(); err != nil {
		Warn("Could not schedule the running program's removal: %v", err)
		return false
	}

	// The child is detached and outlives this process; its handles are released, not waited on.
	_ = command.Process.Release() //nolint:errcheck // nothing to do if the handle cannot be released

	return true
}
