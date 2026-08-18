// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package file

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- resolveOwnership ---

func TestResolveOwnership_Forms(t *testing.T) {

	cases := []struct {
		name    string
		user    string
		group   string
		wantUID int
		wantGID int
	}{
		{"numeric user only", "1000", "", 1000, -1},
		{"numeric user and group", "1000", "2000", 1000, 2000},
		{"numeric group only", "", "2000", -1, 2000},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotUID, gotGID, err := resolveOwnership(tc.user, tc.group)
			if err != nil {
				t.Fatalf("resolveOwnership(%q, %q): %v", tc.user, tc.group, err)
			}
			if gotUID != tc.wantUID {
				t.Errorf("uid: got %d, want %d", gotUID, tc.wantUID)
			}
			if gotGID != tc.wantGID {
				t.Errorf("gid: got %d, want %d", gotGID, tc.wantGID)
			}
		})
	}
}

func TestResolveOwnership_RejectsInvalid(t *testing.T) {

	cases := []struct {
		name  string
		user  string
		group string
		want  string
	}{
		{"both sides empty", "", "", "at least one of"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := resolveOwnership(tc.user, tc.group)
			if err == nil {
				t.Fatal("want error, got nil")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want containing %q", err.Error(), tc.want)
			}
		})
	}
}

// --- applyOwnership ---

func TestApplyOwnership_BothSidesEmptyIsNoOp(t *testing.T) {

	tmp := t.TempDir()
	target := filepath.Join(tmp, "test.txt")

	if err := os.WriteFile(target, []byte("test"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Both sides empty must short-circuit to nil error without invoking any syscall.
	if err := applyOwnership(target, "", ""); err != nil {
		t.Errorf("applyOwnership(empty, empty): %v", err)
	}
}

func TestApplyOwnership_RejectsUnresolvableUser(t *testing.T) {

	if err := applyOwnership("/tmp/anything", "no-such-user-exists-here", ""); err == nil {
		t.Error("unresolvable user: want error")
	}
}
