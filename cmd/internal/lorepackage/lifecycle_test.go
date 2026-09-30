// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package lorepackage

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/selector"
)

// ubuntu is an Ubuntu host, whose lineage is Debian, on arm64.
var ubuntu = selector.NewHostFromWords("Linux", "Ubuntu", []string{"Debian"}, "arm64")

// packageWith makes a package directory holding the named platform directories, each with a Deploy install script.
//
// Parameters:
//   - `t`: the test that owns the directory.
//   - `dirs`: the platform directories.
//
// Returns:
//   - `string`: the package's directory.
func packageWith(t *testing.T, dirs ...string) string {

	t.Helper()
	packageDir := t.TempDir()
	for _, dir := range dirs {
		scriptDir := filepath.Join(packageDir, dir, string(Deploy))
		if err := os.MkdirAll(scriptDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(scriptDir, "install.star"), []byte("def install():\n    pass\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return packageDir
}

// --- PlatformDirs ---

// TestPlatformDirs_FollowsTheHostsChain: lore's directories are selected like writ's, with Common first and no project
// (#944, Q24).
func TestPlatformDirs_FollowsTheHostsChain(t *testing.T) {

	packageDir := packageWith(t, "Ubuntu", "Darwin", "Common", "Debian.arm64", "Linux", "Fedora", "arm64", "Debian",
		"Unix")

	dirs, err := PlatformDirs(packageDir, ubuntu)
	if err != nil {
		t.Fatalf("PlatformDirs: %v", err)
	}
	want := []string{"Common", "arm64", "Unix", "Linux", "Debian", "Debian.arm64", "Ubuntu"}
	if !reflect.DeepEqual(dirs, want) {
		t.Errorf("PlatformDirs:\n got %v\nwant %v; Darwin and Fedora are other machines'", dirs, want)
	}
}

// TestPlatformDirs_RefusesALeftoverLinuxDebian: the retired two-word form has two OS words, and refuses the package.
func TestPlatformDirs_RefusesALeftoverLinuxDebian(t *testing.T) {

	packageDir := packageWith(t, "Common", "Linux.Debian", "Linux.Fedora")

	_, err := PlatformDirs(packageDir, ubuntu)
	var refusal *GrammarRefusal
	if !errors.As(err, &refusal) {
		t.Fatalf("PlatformDirs: %v, want a *GrammarRefusal", err)
	}
	if len(refusal.Errors) != 2 {
		t.Fatalf("refused %d names, want Linux.Debian and Linux.Fedora", len(refusal.Errors))
	}
	for _, e := range refusal.Errors {
		if e.Violation != selector.RepeatedPart {
			t.Errorf("%s: violation %v, want RepeatedPart (two OS words)", e.Name, e.Violation)
		}
	}
}

// --- DiscoverPhaseScripts ---

func TestDiscoverPhaseScripts_GeneralToSpecific(t *testing.T) {

	packageDir := packageWith(t, "Ubuntu", "Common", "Debian")
	lifecycle := &Lifecycle{Name: "probe"}

	scripts, err := lifecycle.DiscoverPhaseScripts(packageDir, ubuntu, Deploy, "install")
	if err != nil {
		t.Fatalf("DiscoverPhaseScripts: %v", err)
	}
	var dirs []string
	for _, script := range scripts {
		rel, _ := filepath.Rel(packageDir, script)
		dirs = append(dirs, filepath.Dir(filepath.Dir(rel)))
	}
	if want := []string{"Common", "Debian", "Ubuntu"}; !reflect.DeepEqual(dirs, want) {
		t.Errorf("scripts from %v, want %v", dirs, want)
	}
}
