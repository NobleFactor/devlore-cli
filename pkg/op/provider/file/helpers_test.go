// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package file

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// --- parseChown ---

func TestParseChown_Forms(t *testing.T) {

	cases := []struct {
		name    string
		spec    string
		wantUID int
		wantGID int
	}{
		{"numeric user only", "1000", 1000, -1},
		{"numeric user and group", "1000:2000", 1000, 2000},
		{"numeric group only", ":2000", -1, 2000},
		{"trailing colon, no group", "1000:", 1000, -1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotUID, gotGID, err := parseChown(tc.spec)
			if err != nil {
				t.Fatalf("parseChown(%q): %v", tc.spec, err)
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

func TestParseChown_RejectsInvalid(t *testing.T) {

	cases := []struct {
		name string
		spec string
		want string
	}{
		{"bare colon", ":", "at least one of"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := parseChown(tc.spec)
			if err == nil {
				t.Fatal("want error, got nil")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want containing %q", err.Error(), tc.want)
			}
		})
	}
}

func TestParseChown_LooksUpNamedUser(t *testing.T) {

	// Looking up the current user by name should always succeed and return the same uid as os.Getuid.
	currentUID := os.Getuid()
	currentName := strconv.Itoa(currentUID)

	gotUID, gotGID, err := parseChown(currentName)
	if err != nil {
		t.Fatalf("parseChown(%q): %v", currentName, err)
	}
	if gotUID != currentUID {
		t.Errorf("uid: got %d, want %d", gotUID, currentUID)
	}
	if gotGID != -1 {
		t.Errorf("gid: got %d, want -1 (group side absent)", gotGID)
	}
}

// --- applyChown ---

func TestApplyChown_EmptySpecIsNoOp(t *testing.T) {

	tmp := t.TempDir()
	target := filepath.Join(tmp, "test.txt")

	if err := os.WriteFile(target, []byte("test"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Empty spec must short-circuit to nil error without invoking any syscall.
	if err := applyChown(target, ""); err != nil {
		t.Errorf("applyChown(empty): %v", err)
	}
}

func TestApplyChown_CurrentUserIsNoOp(t *testing.T) {

	// Chown'ing to the current uid:gid is a no-op-ish operation that doesn't require CAP_CHOWN.
	tmp := t.TempDir()
	target := filepath.Join(tmp, "test.txt")

	if err := os.WriteFile(target, []byte("test"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	spec := strconv.Itoa(os.Getuid()) + ":" + strconv.Itoa(os.Getgid())
	if err := applyChown(target, spec); err != nil {
		t.Errorf("applyChown(%q): %v", spec, err)
	}
}

func TestApplyChown_RejectsMalformed(t *testing.T) {

	if err := applyChown("/tmp/anything", ":"); err == nil {
		t.Error("bare colon: want error")
	}
}
