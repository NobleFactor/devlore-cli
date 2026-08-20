// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

//go:build !windows

package document

import (
	"os"
	"path/filepath"
	"testing"
)

// TestWrite_WithPermOverridesPermission asserts WithPerm reaches the file, and is Unix-scoped deliberately.
//
// 0o644 grants **other**, so it is not a private mode: fsroot leaves such a file inheriting its parent's DACL
// rather than protecting it, which is the correct meaning of "not private". There is therefore no enforcement
// state to observe on Windows, and Mode().Perm() reports 0666 there regardless — the assertion has nothing
// true to say. This is the one case in the permission set where platform scoping is the right answer rather
// than an evasion; every other case moved to a portable fact instead (issue #435).
func TestWrite_WithPermOverridesPermission(t *testing.T) {

	path := filepath.Join(t.TempDir(), "perm.yaml")
	doc := testDoc{Name: "frank", Count: 0}

	if err := WriteFile(path, &doc, WithPerm(0o644)); err != nil {
		t.Fatalf("Write: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o644 {
		t.Errorf("permission = %o, want %o", perm, 0o644)
	}
}
