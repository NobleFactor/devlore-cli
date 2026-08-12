// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

//go:build windows

package file

import "os"

// statIdentity returns the inode and device numbers that identify `info`'s file on its filesystem.
//
// Windows file metadata carries neither number, so both are zero: [statTupleEtag] falls back to size
// and mtime alone, and [Provider.Observe] reports no inode or device.
//
// Parameters:
//   - `info`: unused.
//
// Returns:
//   - `inode`: zero, always.
//   - `device`: zero, always.
func statIdentity(_ os.FileInfo) (inode, device uint64) {

	return 0, 0
}
