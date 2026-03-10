// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

//go:build darwin

package snapshot

import (
	"io/fs"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// lockWorktree sets the user immutable flag (UF_IMMUTABLE / uchg) on all files
// and directories in the worktree. This prevents any modification without
// clearing the flag first. No root required.
//
// Parameters:
//   - worktreePath: root of the worktree to lock
//
// Returns:
//   - error: filesystem walk or chflags failure
func lockWorktree(worktreePath string) error {
	return filepath.WalkDir(worktreePath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Skip .git directory — git needs it writable for worktree bookkeeping
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		return unix.Chflags(path, unix.UF_IMMUTABLE)
	})
}

// unlockWorktree clears the user immutable flag on all files and directories.
// Must be called before Close() so git can remove the worktree.
//
// Parameters:
//   - worktreePath: root of the worktree to unlock
//
// Returns:
//   - error: filesystem walk or chflags failure
func unlockWorktree(worktreePath string) error {
	return filepath.WalkDir(worktreePath, func(path string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		return unix.Chflags(path, 0)
	})
}

// verifyWorktree on Darwin is a no-op — the immutable flag prevents tampering.
func verifyWorktree(_, _ string) error {
	return nil
}
