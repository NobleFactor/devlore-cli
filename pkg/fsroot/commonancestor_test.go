// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package fsroot_test

import (
	"path/filepath"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
)

// --- CommonAncestor ---

// TestCommonAncestor_TheDeepestDirectoryHoldingBoth proves the answer is the deepest directory over both paths: one
// path when it holds the other, their shared parent when they diverge, and the path itself when both are one.
//
// Parameters:
//   - `t`: the test harness.
func TestCommonAncestor_TheDeepestDirectoryHoldingBoth(t *testing.T) {

	root := t.TempDir()
	home := filepath.Join(root, "home", "you")
	layer := filepath.Join(home, "Workspace", "Personal")
	staging := filepath.Join(root, "srv", "staging")

	for _, test := range []struct{ first, second, want string }{
		{home, layer, home},
		{layer, home, home},
		{staging, layer, root},
		{staging, staging, staging},
	} {
		ancestor, ok := fsroot.CommonAncestor(test.first, test.second)
		if !ok || ancestor != test.want {
			t.Errorf("CommonAncestor(%s, %s) = %q, %t; want %q", test.first, test.second, ancestor, ok, test.want)
		}
	}
}

// TestCommonAncestor_AVolumesRootKeepsItsSeparator proves paths that share only their volume answer its root whole:
// `/` on Unix, `C:\` on Windows, never the drive-relative `C:`.
//
// Parameters:
//   - `t`: the test harness.
func TestCommonAncestor_AVolumesRootKeepsItsSeparator(t *testing.T) {

	volume := filepath.VolumeName(t.TempDir()) + string(filepath.Separator)
	first := filepath.Join(volume, "devlore-first", "scope")
	second := filepath.Join(volume, "devlore-second", "layer")

	ancestor, ok := fsroot.CommonAncestor(first, second)
	if !ok || ancestor != volume {
		t.Errorf("CommonAncestor(%s, %s) = %q, %t; want %q", first, second, ancestor, ok, volume)
	}
}
