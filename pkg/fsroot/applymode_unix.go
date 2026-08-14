// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

//go:build !windows

package fsroot

import "os"

// applyMode is a no-op on Unix, where the creating syscall already honored the mode.
//
// The Windows build carries the real implementation: see applymode_windows.go and issue #405. This
// file exists so the call sites in root.go stay platform-free.
//
// Parameters:
//   - `path`: the absolute path just created or modified; unused here.
//   - `perm`: the permission bits the caller requested; unused here.
//   - `isDir`: whether `path` is a directory; unused here.
//
// Returns:
//   - `error`: always nil.
func applyMode(_ string, _ os.FileMode, _ bool) error { return nil }
