// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

//go:build unix

package migrate

import "testing"

// commonAncestor answers an OS-native path, because its result becomes the run's confinement Root.
//
// The cases below feed Unix absolute paths — "/home/user/..." — which on Windows are drive-relative
// strings with no meaning, so the constraint scopes this rather than skipping it. The Windows behavior
// that matters is covered by TestRegisterLayer_Link, which exercises the result as a real root.

func TestCommonAncestor(t *testing.T) {

	cases := []struct {
		a, b, want string
	}{
		{"/home/user/repo", "/home/user/.local/share/devlore/writ/layers/personal", "/home/user"},
		{"/opt/dotfiles", "/home/user/.local/share/devlore/writ/layers/personal", "/"},
		{"/home/user/a/b", "/home/user/a/b/c", "/home/user/a/b"},
		{"/same/path", "/same/path", "/same/path"},
	}

	for _, c := range cases {
		if got := commonAncestor(c.a, c.b); got != c.want {
			t.Errorf("commonAncestor(%q, %q) = %q, want %q", c.a, c.b, got, c.want)
		}
	}
}
