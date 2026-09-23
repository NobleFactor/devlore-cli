// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package extension

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/NobleFactor/devlore-cli/cmd/star/config"
)

// TestDefaultSearchPaths_PrefersTheDevlorePaths pins the probe order star's scopes resolve in.
//
// star is one of devlore's products, so its data lives under devlore/ like writ's (#918). The paths without it are
// what releases before that carried; each is probed after its replacement, so a machine holding both prefers the new
// one, and nothing breaks before writ redeploys or star is reinstalled. They go in #920.
func TestDefaultSearchPaths_PrefersTheDevlorePaths(t *testing.T) {

	root := t.TempDir()
	config.SetGitWorkspaceRoot(root)
	t.Cleanup(config.ResetGitWorkspaceRoot)

	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))

	paths := defaultSearchPaths()

	expected := []string{
		filepath.Join(root, "star", "extensions"),
		filepath.Join(root, "data", "devlore", "star", "extensions"),
		filepath.Join(root, "data", "star", "extensions"),
		"/usr/local/share/devlore/star/extensions",
		"/usr/local/share/star/extensions",
	}

	if !slices.Equal(paths, expected) {
		t.Errorf("search paths:\n  got:  %v\n  want: %v", paths, expected)
	}
}

// TestDefaultSearchPaths_OutsideARepositoryHasNoProjectScope pins that project scope is a checkout's, and appears
// only when there is one: the root is found by walking up for .git, not read from the environment.
func TestDefaultSearchPaths_OutsideARepositoryHasNoProjectScope(t *testing.T) {

	config.SetGitWorkspaceRoot("")
	t.Cleanup(config.ResetGitWorkspaceRoot)

	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "data"))

	paths := defaultSearchPaths()

	if len(paths) != 4 {
		t.Errorf("outside a repository the probes are the two user and two system paths; got %v", paths)
	}

	for _, path := range paths {
		if filepath.Base(path) != "extensions" {
			t.Errorf("a probe that is not an extensions directory: %s", path)
		}
	}
}
