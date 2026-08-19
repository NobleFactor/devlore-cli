// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

//go:build !windows

package file

import (
	"fmt"
	"os"
)

// OwnerOnly reports whether the file at `path` grants nothing to group or other.
//
// Test-only, and deliberately so: no production path needs to ask this. It exists to verify that enforcement
// happened — that a write which requested owner-only permissions actually produced them — which is the one
// defect a seam assertion cannot catch, since the call can be correct while the effect is absent (#405 phase 2,
// where the unconfined root's WriteFile never applied the mode at all).
//
// It answers exclusion, never access. A caller asking whether it can open the file should open the file: the
// kernel evaluates the whole token, group memberships included, where reading permissions does not.
//
// Parameters:
//   - `path`: the absolute path to inspect.
//
// Returns:
//   - `bool`: true when neither group nor other is granted anything.
//   - `error`: non-nil when the file cannot be stat'd.
func OwnerOnly(path string) (bool, error) {

	info, err := os.Stat(path)
	if err != nil {
		return false, fmt.Errorf("owner-only check: stat %s: %w", path, err)
	}

	return info.Mode().Perm()&0o077 == 0, nil
}
