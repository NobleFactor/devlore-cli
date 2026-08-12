// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

//go:build unix

package plan_test

import (
	"os"
	"syscall"
)

// testProcessUmask returns the process umask without changing it.
//
// POSIX offers no read-only umask query, so the mask is read via a [syscall.Umask] round-trip.
// This mirrors the seam production reads through; the test package cannot reach op's unexported
// processUmask.
//
// Returns:
//   - `os.FileMode`: the process umask.
func testProcessUmask() os.FileMode {

	mask := syscall.Umask(0)
	syscall.Umask(mask)

	return os.FileMode(mask) //nolint:gosec // umask values are small
}
