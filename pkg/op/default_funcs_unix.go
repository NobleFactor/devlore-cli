// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

//go:build unix

package op

import (
	"os"
	"syscall"
)

// processUmask returns the process umask without changing it.
//
// POSIX offers no read-only umask query, so the mask is read via a [syscall.Umask] round-trip: set
// zero, capture the previous value, restore it. The process umask is unchanged after the call.
//
// Returns:
//   - `os.FileMode`: the process umask.
func processUmask() os.FileMode {

	mask := syscall.Umask(0)
	syscall.Umask(mask)

	//nolint:gosec // G115: a umask fits in 12 permission bits; the conversion cannot overflow.
	return os.FileMode(mask)
}
