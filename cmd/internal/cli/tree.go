// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package cli

import (
	"fmt"
	"path/filepath"

	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
)

// OpenTree creates the directory at `dir` if it is absent, then opens a confined root at it.
//
// CLI-side trees are addressed before they exist — a first install creates its prefix, and the config, state
// and cache trees appear on first use — but [fsroot.OpenConfined] is a query: it opens what is there and errors
// when nothing is. This is the compound operation that reconciles the two, and it is deliberately named for the
// creation so that opening keeps meaning only opening.
//
// Creation runs through a root rather than [os.MkdirAll] because a mode is worth nothing on Windows without the
// access-control list that fsroot applies for it — a directory made 0o700 by os.MkdirAll is inherited-DACL and
// readable by every account with access to its parent. The chain is made through a root anchored at the volume,
// so every component is resolved by the kernel and no window opens between creating the path and opening it.
//
// The mode is 0o700 because the XDG Base Directory Specification says so: "If, when attempting to write a file,
// the destination directory is non-existent an attempt should be made to create it with permission 0700." That
// is owner-only, which is also what makes it enforceable on Windows.
//
// Parameters:
//   - `dir`: the absolute path of the tree to open, created when absent.
//
// Returns:
//   - `fsroot.Dir`: a confined root anchored at `dir`. The caller owns it and must Close it.
//   - `error`: when `dir` is not absolute on this platform, or the tree cannot be created or opened.
func OpenTree(dir string) (fsroot.Dir, error) {

	// A path that is not absolute on this platform cannot anchor a root. On Windows that includes a leading
	// separator with no volume, which is drive-relative and lands on whatever drive the process is standing on
	// — the defect #392 owns for the System target root, and not one to guess at here.
	if !filepath.IsAbs(dir) {
		return nil, fmt.Errorf("open tree %s: not an absolute path on this platform", dir)
	}

	volume, err := fsroot.OpenConfined(filepath.VolumeName(dir) + string(filepath.Separator))
	if err != nil {
		return nil, fmt.Errorf("open tree %s: anchor at volume: %w", dir, err)
	}

	//nolint:errcheck // diagnose-ignored-error: the volume anchor is released, not written; see docs/architecture/2.8-eventing-infrastructure.md
	defer volume.Close()

	if err := volume.MkdirAll(volume.NewPath(dir), 0o700); err != nil {
		return nil, fmt.Errorf("open tree %s: create: %w", dir, err)
	}

	root, err := fsroot.OpenConfined(dir)
	if err != nil {
		return nil, fmt.Errorf("open tree %s: %w", dir, err)
	}

	return root, nil
}
