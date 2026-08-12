// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

//go:build windows

package plan_test

import "os"

// testProcessUmask returns the process umask without changing it.
//
// Windows has no umask; production's processUmask reports zero there, and this mirrors it, so the
// deferred-default assertion (`0o666` through the mask) stays valid on every platform.
//
// Returns:
//   - `os.FileMode`: always zero.
func testProcessUmask() os.FileMode {

	return 0
}
