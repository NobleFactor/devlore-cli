// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

//go:build !darwin

package snapshot

import "fmt"

// lockWorktree is a no-op on non-Darwin platforms.
// Immutable file flags require root on Linux and have no reliable equivalent on Windows.
func lockWorktree(_ string) error {
	return nil
}

// unlockWorktree is a no-op on non-Darwin platforms.
func unlockWorktree(_ string) error {
	return nil
}

// verifyWorktree checks that the worktree HEAD matches the expected commit hash.
// On non-Darwin platforms, filesystem-level immutability is not available without
// root, so we verify integrity on reuse instead.
//
// Parameters:
//   - worktreePath: path to the worktree to verify
//   - expectedHash: the commit hash the worktree should be pinned to
//
// Returns:
//   - error: if the worktree HEAD does not match expectedHash
func verifyWorktree(worktreePath, expectedHash string) error {
	actual, err := gitRevParseHEAD(worktreePath)
	if err != nil {
		return fmt.Errorf("verify worktree: %w", err)
	}
	if actual != expectedHash {
		return fmt.Errorf("worktree integrity: expected %s, got %s", expectedHash, actual)
	}
	return nil
}
