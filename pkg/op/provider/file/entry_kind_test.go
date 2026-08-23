// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package file

import (
	"io/fs"
	"testing"
)

// The kind vocabulary round-trips its four canonical spellings, refuses everything else, and the zero
// value is the permissive `entry` default (phase 4 PR 3/#611, ruled 2026-08-22).

func TestEntryKind_RoundTripAndRefusal(t *testing.T) {

	for _, kind := range []EntryKind{EntryKindEntry, EntryKindRegular, EntryKindDirectory, EntryKindSymbolicLink} {
		var parsed EntryKind
		if err := parsed.UnmarshalText([]byte(kind.String())); err != nil {
			t.Fatalf("UnmarshalText(%q): %v", kind.String(), err)
		}
		if parsed != kind {
			t.Fatalf("round-trip of %q = %v, want %v", kind.String(), parsed, kind)
		}
	}

	var k EntryKind
	if err := k.UnmarshalText([]byte("symlink")); err == nil {
		t.Fatal("UnmarshalText(\"symlink\"): want a refusal — the canonical spelling is symbolic_link")
	}

	var zero EntryKind
	if zero != EntryKindEntry {
		t.Fatalf("the zero value = %v, want the permissive entry default", zero)
	}
}

// admits is lstat-strict: the symlink bit decides before the type bits, entry admits every taxonomy
// kind and only taxonomy kinds.
func TestEntryKind_AdmitsIsLstatStrict(t *testing.T) {

	regular := fs.FileMode(0o644)
	directory := fs.ModeDir | 0o755
	symbolicLink := fs.ModeSymlink | 0o777
	fifo := fs.ModeNamedPipe | 0o644

	cases := []struct {
		kind EntryKind
		mode fs.FileMode
		want bool
	}{
		{EntryKindEntry, regular, true},
		{EntryKindEntry, directory, true},
		{EntryKindEntry, symbolicLink, true},
		{EntryKindEntry, fifo, false},
		{EntryKindRegular, regular, true},
		{EntryKindRegular, directory, false},
		{EntryKindRegular, symbolicLink, false},
		{EntryKindDirectory, directory, true},
		{EntryKindDirectory, regular, false},
		{EntryKindSymbolicLink, symbolicLink, true},
		{EntryKindSymbolicLink, regular, false},
	}

	for _, tc := range cases {
		if got := tc.kind.admits(tc.mode); got != tc.want {
			t.Errorf("%s.admits(%v) = %t, want %t", tc.kind, tc.mode, got, tc.want)
		}
	}
}
