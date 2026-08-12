// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package writ

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// workingTree creates a temporary directory that passes the add-time git-working-tree validation.
func workingTree(t *testing.T) string {

	t.Helper()

	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o750); err != nil {
		t.Fatal(err)
	}
	return root
}

// sourceRepository creates a real single-commit git repository for offline file:// clone tests.
func sourceRepository(t *testing.T) string {

	t.Helper()

	root := t.TempDir()
	for _, args := range [][]string{
		{"init", "--quiet", "--initial-branch=main"},
		{"-c", "user.name=t", "-c", "user.email=t@invalid", "commit", "--quiet", "--allow-empty", "-m", "seed"},
	} {
		command := exec.CommandContext(context.Background(), "git", append([]string{"-C", root}, args...)...)
		command.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_SYSTEM="+os.DevNull)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	return root
}

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
	repo := workingTree(t)

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
	repo := workingTree(t)

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
	repo := workingTree(t)

	if _, err := runRepo(t, "add", "sideways", repo); err == nil || !strings.Contains(err.Error(), "unknown layer") {
		t.Fatalf("expected unknown-layer error, got %v", err)
	}
	if _, err := runRepo(t, "add", "personal", filepath.Join(repo, "absent")); err == nil {
		t.Fatal("expected missing-path error")
	}
	if _, err := runRepo(t, "add", "personal", t.TempDir()); err == nil || !strings.Contains(err.Error(), "not a git working tree") {
		t.Fatalf("expected working-tree validation error, got %v", err)
	}
	if _, err := runRepo(t, "add", "personal", repo, "elsewhere"); err == nil || !strings.Contains(err.Error(), "takes no destination") {
		t.Fatalf("expected no-destination error, got %v", err)
	}
	if _, err := runRepo(t, "add", "personal", "--branch", "main", repo); err == nil || !strings.Contains(err.Error(), "repository-url form") {
		t.Fatalf("expected branch-on-local error, got %v", err)
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
	repo := workingTree(t)

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

func TestIsRepositoryURL_Table(t *testing.T) {

	cases := []struct {
		location string
		want     bool
	}{
		{"https://github.com/me/x.git", true},
		{"ssh://git@host/x.git", true},
		{"file:///abs/src.git", true},
		{"git@github.com:me/x.git", true},
		{"host:path/to/repo", true},
		{"/abs/path", false},
		{"relative/path", false},
		{"./x:y", false},
		{`D:\a\b`, false},
		{"C:/x", false},
		{"~/Workspace/Personal", false},
	}
	for _, c := range cases {
		if got := isRepositoryURL(c.location); got != c.want {
			t.Errorf("isRepositoryURL(%q) = %v, want %v", c.location, got, c.want)
		}
	}
}

func TestRepo_Add_ClonesURL_DefaultHome(t *testing.T) {

	dataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	source := sourceRepository(t)

	out, err := runRepo(t, "add", "team", "file://"+source)
	if err != nil {
		t.Fatalf("add url: %v", err)
	}
	expected := filepath.Join(dataHome, "devlore", "writ", "repos", "team")
	if !strings.Contains(out, expected) {
		t.Fatalf("expected registration at the writ-owned home %s:\n%s", expected, out)
	}
	if _, err := os.Stat(filepath.Join(expected, ".git")); err != nil {
		t.Fatalf("clone missing at default home: %v", err)
	}
}

func TestRepo_Add_ClonesURL_PositionalDestination(t *testing.T) {

	t.Setenv("XDG_DATA_HOME", t.TempDir())
	source := sourceRepository(t)
	destination := filepath.Join(t.TempDir(), "Personal")

	if _, err := runRepo(t, "add", "personal", "file://"+source, destination); err != nil {
		t.Fatalf("add url with destination: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destination, ".git")); err != nil {
		t.Fatalf("clone missing at positional destination: %v", err)
	}

	if _, err := runRepo(t, "remove", "personal"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(destination); err != nil {
		t.Fatal("remove must not delete the cloned repository")
	}
}
