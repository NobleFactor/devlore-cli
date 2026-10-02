// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package adopt

import (
	"os"
	"path/filepath"
	"testing"
)

// scopesForTest returns the scopes the inference tests run against: System at the volume root that holds `root`, Home
// at `root`/home, then any more given, in that order.
//
// Parameters:
//   - `t`: the test harness.
//   - `root`: the directory the test works beneath.
//   - `more`: further scopes, after System and Home.
//
// Returns:
//   - `[]Scope`: the scopes, in scope order.
func scopesForTest(t *testing.T, root string, more ...Scope) []Scope {

	t.Helper()

	system := Scope{Name: "system", Directory: "System", Root: filepath.VolumeName(root) + string(filepath.Separator)}
	home := Scope{Name: "home", Directory: "Home", Root: filepath.Join(root, "home")}
	return append([]Scope{system, home}, more...)
}

// writeFileForTest writes a small file, creating its parent directories.
//
// Parameters:
//   - `t`: the test harness, failed when the file cannot be written.
//   - `path`: the file to write.
func writeFileForTest(t *testing.T, path string) {

	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("adopted"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// --- inferScope ---

// TestInferScope_TheDeepestRootWins proves an item belongs to the scope whose root is the deepest that holds it: Home
// over System beneath the home directory, a custom scope over System beneath its root, and System elsewhere.
//
// Parameters:
//   - `t`: the test harness.
func TestInferScope_TheDeepestRootWins(t *testing.T) {

	root := t.TempDir()
	staging := Scope{Name: "staging", Directory: "Staging", Root: filepath.Join(root, "srv", "staging")}
	scopes := scopesForTest(t, root, staging)

	for _, test := range []struct{ path, want string }{
		{filepath.Join(root, "home", ".zshrc"), "home"},
		{filepath.Join(root, "home"), "home"},
		{filepath.Join(root, "srv", "staging", "app.conf"), "staging"},
		{filepath.Join(root, "etc", "hosts"), "system"},
	} {
		scope, held := inferScope(test.path, scopes)
		if !held || scope.Name != test.want {
			t.Errorf("inferScope(%s) = %q, %v; want %q", test.path, scope.Name, held, test.want)
		}
	}
}

// TestInferScope_ARootsPrefixIsNotItsTree proves a root holds only what lies beneath it: `home` does not hold
// `homework`, which shares its name's prefix.
//
// Parameters:
//   - `t`: the test harness.
func TestInferScope_ARootsPrefixIsNotItsTree(t *testing.T) {

	root := t.TempDir()
	path := filepath.Join(root, "homework", "notes.txt")

	scope, held := inferScope(path, scopesForTest(t, root))
	if !held || scope.Name != "system" {
		t.Errorf("inferScope(%s) = %q, %v; want system", path, scope.Name, held)
	}
}

// TestInferScope_ACustomScopeInsideHome proves a custom scope rooted inside the home directory takes what lies
// beneath its root, since its root is the deeper.
//
// Parameters:
//   - `t`: the test harness.
func TestInferScope_ACustomScopeInsideHome(t *testing.T) {

	root := t.TempDir()
	sites := Scope{Name: "sites", Directory: "Sites", Root: filepath.Join(root, "home", "Sites")}
	scopes := scopesForTest(t, root, sites)

	for _, test := range []struct{ path, want string }{
		{filepath.Join(root, "home", "Sites", "index.html"), "sites"},
		{filepath.Join(root, "home", ".gitconfig"), "home"},
	} {
		scope, held := inferScope(test.path, scopes)
		if !held || scope.Name != test.want {
			t.Errorf("inferScope(%s) = %q, %v; want %q", test.path, scope.Name, held, test.want)
		}
	}
}

// TestInferScope_ARelocatedHome proves Home is wherever its root is: relocated to a sandbox, the sandbox's files are
// Home's and the user's own home directory falls to System.
//
// Parameters:
//   - `t`: the test harness.
func TestInferScope_ARelocatedHome(t *testing.T) {

	root := t.TempDir()
	scopes := scopesForTest(t, root)
	scopes[1].Root = filepath.Join(root, "sandbox")

	for _, test := range []struct{ path, want string }{
		{filepath.Join(root, "sandbox", ".zshrc"), "home"},
		{filepath.Join(root, "home", ".zshrc"), "system"},
	} {
		scope, held := inferScope(test.path, scopes)
		if !held || scope.Name != test.want {
			t.Errorf("inferScope(%s) = %q, %v; want %q", test.path, scope.Name, held, test.want)
		}
	}
}

// TestInferScope_ATieGoesToScopeOrder proves that of two scopes sharing the deepest root, the first in scope order
// wins.
//
// Parameters:
//   - `t`: the test harness.
func TestInferScope_ATieGoesToScopeOrder(t *testing.T) {

	shared := filepath.Join(t.TempDir(), "shared")
	scopes := []Scope{
		{Name: "first", Directory: "First", Root: shared},
		{Name: "second", Directory: "Second", Root: shared},
	}

	scope, held := inferScope(filepath.Join(shared, "settings.conf"), scopes)
	if !held || scope.Name != "first" {
		t.Errorf("inferScope = %q, %v; want first", scope.Name, held)
	}
}

// TestInferScope_NoRootHoldsThePath proves an item outside every scope's root belongs to no scope.
//
// Parameters:
//   - `t`: the test harness.
func TestInferScope_NoRootHoldsThePath(t *testing.T) {

	root := t.TempDir()
	scopes := []Scope{
		{Name: "home", Directory: "Home", Root: filepath.Join(root, "home")},
		{Name: "staging", Directory: "Staging", Root: filepath.Join(root, "staging")},
	}

	if scope, held := inferScope(filepath.Join(root, "elsewhere", "file"), scopes); held {
		t.Errorf("inferScope = %q, held; want no scope", scope.Name)
	}
}

// --- Collect ---

// TestCollect_EachItemLandsInItsScopesDirectory proves each item is batched under its scope's lower-case name, lands
// in that scope's directory in the layer, and keeps its path relative to that scope's root.
//
// Parameters:
//   - `t`: the test harness.
func TestCollect_EachItemLandsInItsScopesDirectory(t *testing.T) {

	root := t.TempDir()
	staging := Scope{Name: "staging", Directory: "Staging", Root: filepath.Join(root, "srv", "staging")}
	homeItem := filepath.Join(root, "home", ".adoptrc")
	stagingItem := filepath.Join(root, "srv", "staging", "app", "app.conf")
	writeFileForTest(t, homeItem)
	writeFileForTest(t, stagingItem)

	layer := filepath.Join(root, "layer")
	groups := Collect(&Config{
		Files:      []string{homeItem, stagingItem},
		TargetRoot: filepath.Join(root, "home"),
		Scopes:     scopesForTest(t, root, staging),
		LayerPath:  layer,
		Project:    "adopted",
	})

	for _, test := range []struct{ scope, source, destination string }{
		{"home", homeItem, filepath.Join(layer, "Home", "adopted", ".adoptrc")},
		{"staging", stagingItem, filepath.Join(layer, "Staging", "adopted", "app", "app.conf")},
	} {
		items := groups[test.scope]
		if len(items) != 1 {
			t.Fatalf("%s's batch holds %d items, want 1: %v", test.scope, len(items), groups)
		}
		if items[0].Source != test.source || items[0].DestPath != test.destination || items[0].Scope != test.scope {
			t.Errorf("%s's item = %+v, want %s adopted to %s in scope %s", test.scope, items[0], test.source,
				test.destination, test.scope)
		}
	}
}

// TestCollect_AnItemUnderNoRootIsNotCollected proves an item outside every scope's root is refused, as a missing item
// is, and the rest are still collected.
//
// Parameters:
//   - `t`: the test harness.
func TestCollect_AnItemUnderNoRootIsNotCollected(t *testing.T) {

	root := t.TempDir()
	homeItem := filepath.Join(root, "home", ".adoptrc")
	stray := filepath.Join(root, "elsewhere", "stray.conf")
	writeFileForTest(t, homeItem)
	writeFileForTest(t, stray)

	groups := Collect(&Config{
		Files:      []string{stray, homeItem},
		TargetRoot: filepath.Join(root, "home"),
		Scopes:     []Scope{{Name: "home", Directory: "Home", Root: filepath.Join(root, "home")}},
		LayerPath:  filepath.Join(root, "layer"),
		Project:    "adopted",
	})

	if len(groups) != 1 || len(groups["home"]) != 1 || groups["home"][0].Source != homeItem {
		t.Errorf("groups = %v, want Home's item alone", groups)
	}
}
