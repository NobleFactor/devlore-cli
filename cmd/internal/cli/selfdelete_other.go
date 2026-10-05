// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

//go:build !windows

package cli

import "github.com/NobleFactor/devlore-cli/pkg/fsroot"

// deferRunningImageDeletion never defers on a non-Windows platform: unlinking a running binary succeeds here -- the
// directory entry goes at once, the inode lingers until the process exits -- so the running image is removed in place
// like any other recorded file. It always returns false, so the caller removes it inline (#1003).
//
// Parameters:
//   - `prefixRoot`: the installation prefix (unused here).
//   - `running`: the running image's recorded entry (unused here).
//
// Returns:
//   - `bool`: always false.
func deferRunningImageDeletion(_ fsroot.Dir, _ manifestEntry) bool {
	return false
}
