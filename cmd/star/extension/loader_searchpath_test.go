// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package extension

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/cmd/star/config"
)

// TestDefaultSearchPaths_PrefersTheDevlorePaths pins the probe order star's scopes resolve in.
//
// star is one of devlore's products, so its data lives under devlore/ like writ's (#918). The paths without
// it are what releases before that carried; each is probed after its replacement, so a machine holding both
// prefers the new one, and nothing breaks before writ redeploys or star is reinstalled. They go in #920.
//
// A path derived from the running binary is deliberately absent: it served no scope, and `self install`
// writes no extensions for it to find (#990).
//
// The exe-relative probe sits second, after the repository and ahead of every installed location: a star run
// from a checkout uses that checkout, and otherwise uses the tree it was installed into (#989).
func TestDefaultSearchPaths_PrefersTheDevlorePaths(t *testing.T) {

	root := t.TempDir()
	config.SetGitWorkspaceRoot(root)
	t.Cleanup(config.ResetGitWorkspaceRoot)

	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	t.Setenv("XDG_DATA_DIRS", filepath.Join(root, "sys"))

	paths := defaultSearchPaths()

	expected := []string{
		filepath.Join(root, "star", "extensions"),
		filepath.Join(root, "data", "devlore", "star", "extensions"),
		filepath.Join(root, "data", "star", "extensions"),
		filepath.Join(root, "sys", "devlore", "star", "extensions"),
		filepath.Join(root, "sys", "star", "extensions"),
	}

	if !slices.Equal(paths, expected) {
		t.Errorf("search paths:\n  got:  %v\n  want: %v", paths, expected)
	}
}

// TestDefaultSearchPaths_OutsideARepositoryHasNoProjectScope pins that project scope is a checkout's, and
// appears only when there is one: the root is found by walking up for .git, not read from the environment.
func TestDefaultSearchPaths_OutsideARepositoryHasNoProjectScope(t *testing.T) {

	config.SetGitWorkspaceRoot("")
	t.Cleanup(config.ResetGitWorkspaceRoot)

	root := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	t.Setenv("XDG_DATA_DIRS", filepath.Join(root, "sys"))

	paths := defaultSearchPaths()

	expected := []string{
		filepath.Join(root, "data", "devlore", "star", "extensions"),
		filepath.Join(root, "data", "star", "extensions"),
		filepath.Join(root, "sys", "devlore", "star", "extensions"),
		filepath.Join(root, "sys", "star", "extensions"),
	}

	if !slices.Equal(paths, expected) {
		t.Errorf("search paths:\n  got:  %v\n  want: %v", paths, expected)
	}

	for _, path := range paths {
		if filepath.Base(path) != "extensions" {
			t.Errorf("a probe that is not an extensions directory: %s", path)
		}
	}
}

// TestSourceOf classifies a search path by what it is rather than by where it sits in the list.
//
// The label was assigned by index against a three-entry array, so `${XDG_DATA_HOME}/star/extensions` -- a
// user path -- was reported as `system`, and adding probes would have shifted every later label (#989). The
// label is what someone reads when an extension resolved from somewhere they did not expect.
func TestSourceOf(t *testing.T) {

	root := t.TempDir()
	config.SetGitWorkspaceRoot(root)
	t.Cleanup(config.ResetGitWorkspaceRoot)

	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no home directory: %v", err)
	}

	for _, testCase := range []struct {
		name string
		dir  string
		want Source
	}{
		{
			name: "the repository's own",
			dir:  filepath.Join(root, "star", "extensions"),
			want: SourceProjectLocal,
		},
		{
			name: "under the home directory, whatever the spelling",
			dir:  filepath.Join(home, ".local", "share", "devlore", "star", "extensions"),
			want: SourceUser,
		},
		{
			name: "the pre-918 user path, which used to be called system",
			dir:  filepath.Join(home, ".local", "share", "star", "extensions"),
			want: SourceUser,
		},
		{
			name: "an install into a prefix under home is still the user's",
			dir:  filepath.Join(home, "opt", "devlore", "share", "devlore", "star", "extensions"),
			want: SourceUser,
		},
		{
			name: "outside home is system",
			dir:  filepath.Join(string(filepath.Separator), "usr", "local", "share", "devlore", "star", "extensions"),
			want: SourceSystem,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if got := sourceOf(testCase.dir); got != testCase.want {
				t.Errorf("sourceOf(%q) = %v, want %v", testCase.dir, got, testCase.want)
			}
		})
	}
}

// TestDefaultSearchPaths_HasNoDuplicates asserts the list does not repeat a directory.
//
// The sources overlap by construction: an exe-relative path for an install under `~/.local` names the same
// directory XDG_DATA_HOME does, and XDG_DATA_DIRS may repeat one of its own defaults. A repeat is not wrong
// in itself, but it makes one extension look like two.
func TestDefaultSearchPaths_HasNoDuplicates(t *testing.T) {

	config.SetGitWorkspaceRoot("")
	t.Cleanup(config.ResetGitWorkspaceRoot)

	root := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))

	// The same directory twice, and one of them with a redundant separator.
	t.Setenv("XDG_DATA_DIRS", strings.Join([]string{
		filepath.Join(root, "sys"),
		filepath.Join(root, "sys") + string(filepath.Separator),
	}, string(os.PathListSeparator)))

	paths := defaultSearchPaths()

	seen := map[string]bool{}
	for _, path := range paths {
		if seen[path] {
			t.Errorf("duplicate search path %q in %v", path, paths)
		}
		seen[path] = true
	}
}
