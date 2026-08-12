// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

//go:build windows

package op

import "os"

// processUmask returns the process umask without changing it.
//
// Windows has no umask, so the mask is always zero and `{{ umask base }}` resolves to the base mode
// unmasked.
//
// Returns:
//   - `os.FileMode`: zero, always.
func processUmask() os.FileMode {
	return 0
}
