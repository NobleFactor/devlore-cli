// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

//go:build unix

package fsroot_test

import (
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
)

// --- CommonAncestor ---

// TestCommonAncestor_UnixPaths keeps the cases that pinned migrate's copy of this function, the layer registration
// graph's confinement root: Unix absolute paths, meaningless as drive-relative strings on Windows, so the build
// constraint scopes them rather than a skip.
//
// Parameters:
//   - `t`: the test harness.
func TestCommonAncestor_UnixPaths(t *testing.T) {

	for _, test := range []struct{ first, second, want string }{
		{"/home/user/repo", "/home/user/.local/share/devlore/writ/layers/personal", "/home/user"},
		{"/opt/dotfiles", "/home/user/.local/share/devlore/writ/layers/personal", "/"},
		{"/home/user/a/b", "/home/user/a/b/c", "/home/user/a/b"},
		{"/same/path", "/same/path", "/same/path"},
	} {
		ancestor, ok := fsroot.CommonAncestor(test.first, test.second)
		if !ok || ancestor != test.want {
			t.Errorf("CommonAncestor(%q, %q) = %q, %t; want %q", test.first, test.second, ancestor, ok, test.want)
		}
	}
}
