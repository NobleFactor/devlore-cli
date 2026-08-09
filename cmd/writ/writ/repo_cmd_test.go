// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package writ

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runRepo executes the repo command family against a sandboxed layers directory and returns its output.
func runRepo(t *testing.T, args ...string) (string, error) {

	t.Helper()

	cmd := newRepoCmd()
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func TestRepo_AddListRemove_RoundTrip(t *testing.T) {

	t.Setenv("XDG_DATA_HOME", t.TempDir())
	repo := t.TempDir()

	if _, err := runRepo(t, "add", "personal", repo); err != nil {
		t.Fatalf("add: %v", err)
	}

	listed, err := runRepo(t, "list")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(listed, "personal") || !strings.Contains(listed, repo) {
		t.Fatalf("list output missing the registration:\n%s", listed)
	}
	if !strings.Contains(listed, "base     (not registered)") {
		t.Fatalf("list output missing the unregistered marker:\n%s", listed)
	}

	if _, err := runRepo(t, "remove", "personal"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	listed, err = runRepo(t, "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(listed, "personal (not registered)") {
		t.Fatalf("expected personal unregistered after remove:\n%s", listed)
	}
}

func TestRepo_BareInvocation_Lists(t *testing.T) {

	t.Setenv("XDG_DATA_HOME", t.TempDir())

	listed, err := runRepo(t)
	if err != nil {
		t.Fatalf("bare repo: %v", err)
	}
	for _, layer := range LayerOrder {
		if !strings.Contains(listed, layer) {
			t.Fatalf("bare listing missing layer %s:\n%s", layer, listed)
		}
	}
}

func TestRepo_Aliases_RmAndLs(t *testing.T) {

	t.Setenv("XDG_DATA_HOME", t.TempDir())
	repo := t.TempDir()

	if _, err := runRepo(t, "add", "team", repo); err != nil {
		t.Fatal(err)
	}
	listed, err := runRepo(t, "ls")
	if err != nil {
		t.Fatalf("ls alias: %v", err)
	}
	if !strings.Contains(listed, "team") || !strings.Contains(listed, repo) {
		t.Fatalf("ls output missing the registration:\n%s", listed)
	}
	if _, err := runRepo(t, "rm", "team"); err != nil {
		t.Fatalf("rm alias: %v", err)
	}
}

func TestRepo_Add_Errors(t *testing.T) {

	t.Setenv("XDG_DATA_HOME", t.TempDir())
	repo := t.TempDir()

	if _, err := runRepo(t, "add", "sideways", repo); err == nil || !strings.Contains(err.Error(), "unknown layer") {
		t.Fatalf("expected unknown-layer error, got %v", err)
	}
	if _, err := runRepo(t, "add", "personal", filepath.Join(repo, "absent")); err == nil {
		t.Fatal("expected missing-path error")
	}
	if _, err := runRepo(t, "add", "personal", repo); err != nil {
		t.Fatal(err)
	}
	if _, err := runRepo(t, "add", "personal", repo); err == nil || !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("expected already-registered error, got %v", err)
	}
}

func TestRepo_Remove_Unregistered_Errors(t *testing.T) {

	t.Setenv("XDG_DATA_HOME", t.TempDir())

	if _, err := runRepo(t, "remove", "team"); err == nil || !strings.Contains(err.Error(), "not registered") {
		t.Fatalf("expected not-registered error, got %v", err)
	}
}

func TestRepo_List_MarksBrokenLink(t *testing.T) {

	dataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	repo := t.TempDir()

	if _, err := runRepo(t, "add", "personal", repo); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(repo); err != nil {
		t.Fatal(err)
	}

	listed, err := runRepo(t, "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(listed, "(broken)") {
		t.Fatalf("expected broken marker after target removal:\n%s", listed)
	}
}
