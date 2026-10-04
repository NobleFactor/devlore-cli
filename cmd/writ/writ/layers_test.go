// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package writ

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/cmd/internal/devlore"
)

// emptyLayerDirectories points XDG_DATA_HOME at a temporary root and leaves an empty directory where each layer
// goes, as `self install` did before #1030.
func emptyLayerDirectories(t *testing.T) {

	t.Helper()

	t.Setenv("XDG_DATA_HOME", t.TempDir())
	for _, layer := range LayerOrder {
		if err := os.MkdirAll(filepath.Join(devlore.WritLayersDir(), layer), 0o750); err != nil {
			t.Fatal(err)
		}
	}
}

// --- getConfiguredRepo ---

// TestGetConfiguredRepo_EmptyDirectoryIsNoLayer is #1030: deploy finds no layer in an empty directory.
func TestGetConfiguredRepo_EmptyDirectoryIsNoLayer(t *testing.T) {

	emptyLayerDirectories(t)

	for _, layer := range LayerOrder {
		if got := getConfiguredRepo(layer); got != "" {
			t.Errorf("getConfiguredRepo(%q) = %q over an empty directory; want none", layer, got)
		}
	}
}

// TestGetConfiguredRepo_WorkingTreeIsALayer pins the other side: a link to a git working tree is the layer.
func TestGetConfiguredRepo_WorkingTreeIsALayer(t *testing.T) {

	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if err := os.MkdirAll(devlore.WritLayersDir(), 0o750); err != nil {
		t.Fatal(err)
	}
	repo := workingTree(t)
	if err := os.Symlink(repo, filepath.Join(devlore.WritLayersDir(), "team")); err != nil {
		t.Fatal(err)
	}

	resolved, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatal(err)
	}
	if got := getConfiguredRepo("team"); got != resolved {
		t.Fatalf("getConfiguredRepo(team) = %q; want %q", got, resolved)
	}
}

// --- parseAdoptConfig ---

// TestParseAdoptConfig_UnregisteredLayerNamesRepoSet is #1030: adopt into a layer that isn't registered refuses, and
// names the command that registers it, not `self install`, which no longer makes layer directories.
func TestParseAdoptConfig_UnregisteredLayerNamesRepoSet(t *testing.T) {

	for name, setup := range map[string]func(*testing.T){
		"nothing there":      func(t *testing.T) { t.Setenv("XDG_DATA_HOME", t.TempDir()) },
		"an empty directory": emptyLayerDirectories,
	} {
		t.Run(name, func(t *testing.T) {

			setup(t)

			cmd := newAdoptCmd()
			if err := cmd.Flags().Set("project", "noblefactor"); err != nil {
				t.Fatal(err)
			}
			if err := cmd.Flags().Set("layer", "team"); err != nil {
				t.Fatal(err)
			}

			_, err := parseAdoptConfig(cmd, []string{filepath.Join(t.TempDir(), ".zshrc")})
			if err == nil {
				t.Fatal("adopt into an unregistered layer was accepted")
			}
			message := err.Error()
			if !strings.Contains(message, `layer "team" is not registered`) ||
				!strings.Contains(message, "writ repo set team") {
				t.Errorf("refusal = %q; want it to say team is not registered and name writ repo set team", message)
			}
			if strings.Contains(message, "self install") {
				t.Errorf("refusal = %q; it names self install, which makes no layer", message)
			}
		})
	}
}
