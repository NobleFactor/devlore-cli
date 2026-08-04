// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

//go:build unix

package file

import (
	"os"
	"syscall"
)

// statIdentity returns the inode and device numbers that identify `info`'s file on its filesystem.
//
// Parameters:
//   - `info`: the stat (or lstat) result to read.
//
// Returns:
//   - `uint64`: the inode number.
//   - `uint64`: the device number.
func statIdentity(info os.FileInfo) (inode, device uint64) {

	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0
	}

	// Dev is int32 on Darwin and uint64 on Linux: the conversion is required on one platform and
	// flagged unnecessary on the other.
	//nolint:unconvert,gosec // Platform-varying Dev type; G115: overflow is not a practical concern.
	return stat.Ino, uint64(stat.Dev)
}
