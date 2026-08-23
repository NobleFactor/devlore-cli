// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package file

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// Kind-honest Exists (phase 4 PR 3/#611, ruled 2026-08-22): each taxonomy kind's Exists is lstat plus a
// kind test, so a claim activates only when the disk holds the CLAIMED kind — "claims are true when
// made". The sharp rows: a symlink to a regular file is NOT a regular (the door-one fix), and a
// directory is NOT a regular. The candidates are unlinked (no interning), so mismatched kinds probe the
// same paths without cross-kind claim conflicts.
func TestExists_IsKindHonest(t *testing.T) {

	dir := t.TempDir()
	environment := testEnvironment(t, dir)

	if err := os.WriteFile(filepath.Join(dir, "regular.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "a-directory"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "regular.txt"), filepath.Join(dir, "the-link")); err != nil {
		t.Fatal(err)
	}

	regularAt := func(path string) *Regular {
		base, err := buildCandidateAs(environment, path, reflect.TypeFor[*Regular]())
		if err != nil {
			t.Fatalf("candidate %s: %v", path, err)
		}
		return &Regular{entry: *base}
	}
	directoryAt := func(path string) *Directory {
		base, err := buildCandidateAs(environment, path, reflect.TypeFor[*Directory]())
		if err != nil {
			t.Fatalf("candidate %s: %v", path, err)
		}
		return &Directory{entry: *base}
	}
	linkAt := func(path string) *SymbolicLink {
		base, err := buildCandidateAs(environment, path, reflect.TypeFor[*SymbolicLink]())
		if err != nil {
			t.Fatalf("candidate %s: %v", path, err)
		}
		return &SymbolicLink{entry: *base}
	}

	cases := []struct {
		name   string
		exists bool
		probe  op.Resource
	}{
		{"a Regular claim over a regular file", true, regularAt("regular.txt")},
		{"a Regular claim over a directory", false, regularAt("a-directory")},
		{"a Regular claim over a symlink to a regular file", false, regularAt("the-link")},
		{"a Regular claim over nothing", false, regularAt("absent.txt")},
		{"a Directory claim over a directory", true, directoryAt("a-directory")},
		{"a Directory claim over a regular file", false, directoryAt("regular.txt")},
		{"a SymbolicLink claim over a link", true, linkAt("the-link")},
		{"a SymbolicLink claim over a regular file", false, linkAt("regular.txt")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.probe.Exists(); got != tc.exists {
				t.Errorf("Exists() = %t, want %t", got, tc.exists)
			}
		})
	}
}
