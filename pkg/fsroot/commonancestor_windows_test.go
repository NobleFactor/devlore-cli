// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

//go:build windows

package fsroot_test

import (
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
)

// --- CommonAncestor ---

// TestCommonAncestor_ADrivesRoot proves two paths that share only their drive answer the drive's root, `C:\`, which
// the segment-matching copies this function replaced answered as the drive-relative `C:`.
//
// Parameters:
//   - `t`: the test harness.
func TestCommonAncestor_ADrivesRoot(t *testing.T) {

	ancestor, ok := fsroot.CommonAncestor(`C:\ProgramData`, `C:\Users\you\Workspace\Personal`)
	if !ok || ancestor != `C:\` {
		t.Errorf(`CommonAncestor = %q, %t; want C:\`, ancestor, ok)
	}
}

// TestCommonAncestor_TwoVolumesShareNothing proves paths on two volumes, two drives or a drive and a share, have no
// common ancestor, where the copies answered `\`, the root of whatever drive the process stood on.
//
// Parameters:
//   - `t`: the test harness.
func TestCommonAncestor_TwoVolumesShareNothing(t *testing.T) {

	for _, test := range []struct{ first, second string }{
		{`C:\ProgramData`, `D:\repos\Personal`},
		{`C:\ProgramData`, `\\server\share\Personal`},
	} {
		if ancestor, ok := fsroot.CommonAncestor(test.first, test.second); ok {
			t.Errorf("CommonAncestor(%s, %s) = %q; want none", test.first, test.second, ancestor)
		}
	}
}

// TestCommonAncestor_CaseIsIgnored proves names compare without case, as Windows compares them.
//
// Parameters:
//   - `t`: the test harness.
func TestCommonAncestor_CaseIsIgnored(t *testing.T) {

	ancestor, ok := fsroot.CommonAncestor(`C:\Users\You`, `c:\users\you\Workspace`)
	if !ok || ancestor != `C:\Users\You` {
		t.Errorf(`CommonAncestor = %q, %t; want C:\Users\You`, ancestor, ok)
	}
}
