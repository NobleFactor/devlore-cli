// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package layers_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/NobleFactor/devlore-cli/cmd/internal/devlore"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/layers"
)

// registry points XDG_DATA_HOME at a temporary root, creates the layers directory, and returns it.
func registry(t *testing.T) string {

	t.Helper()

	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if err := os.MkdirAll(devlore.WritLayersDir(), 0o750); err != nil {
		t.Fatal(err)
	}
	return devlore.WritLayersDir()
}

// workingTree returns a temporary directory holding `.git`.
func workingTree(t *testing.T) string {

	t.Helper()

	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o750); err != nil {
		t.Fatal(err)
	}
	return root
}

// --- Read ---

func TestRead_NothingThereIsUnregistered(t *testing.T) {

	registry(t)

	if got := layers.Read("base"); got.State != layers.Unregistered || got.Target != "" || got.Root != "" {
		t.Fatalf("Read(base) = %+v with nothing there; want unregistered, no target, no root", got)
	}
}

func TestRead_EmptyDirectoryIsUnregistered(t *testing.T) {

	dir := filepath.Join(registry(t), "team")
	if err := os.Mkdir(dir, 0o750); err != nil {
		t.Fatal(err)
	}

	if got := layers.Read("team"); got.State != layers.Unregistered || got.Target != "" || got.Root != "" {
		t.Fatalf("Read(team) = %+v over an empty directory; want unregistered, no target, no root", got)
	}
}

func TestRead_WorkingTreeDirectoryIsRegistered(t *testing.T) {

	dir := filepath.Join(registry(t), "personal")
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o750); err != nil {
		t.Fatal(err)
	}

	got := layers.Read("personal")
	if got.State != layers.Registered || got.Link || got.Root != dir || got.Target != dir {
		t.Fatalf("Read(personal) = %+v over a working tree moved into place; want registered at %s", got, dir)
	}
}

func TestRead_LinkToWorkingTreeIsRegistered(t *testing.T) {

	path := filepath.Join(registry(t), "personal")
	tree := workingTree(t)
	if err := os.Symlink(tree, path); err != nil {
		t.Fatal(err)
	}
	resolved, err := filepath.EvalSymlinks(tree)
	if err != nil {
		t.Fatal(err)
	}

	got := layers.Read("personal")
	if got.State != layers.Registered || !got.Link || got.Target != tree || got.Root != resolved {
		t.Fatalf("Read(personal) = %+v; want registered, linked, target %s, root %s", got, tree, resolved)
	}
}

func TestRead_LinkToATreeThatIsNotARepositoryIsBroken(t *testing.T) {

	path := filepath.Join(registry(t), "base")
	tree := t.TempDir()
	if err := os.Symlink(tree, path); err != nil {
		t.Fatal(err)
	}

	if got := layers.Read("base"); got.State != layers.Broken || got.Target != tree || got.Root != "" {
		t.Fatalf("Read(base) = %+v, a link to a tree with no .git; want broken, target %s, no root", got, tree)
	}
}

func TestRead_DanglingLinkIsBroken(t *testing.T) {

	path := filepath.Join(registry(t), "base")
	gone := filepath.Join(t.TempDir(), "gone")
	if err := os.Symlink(gone, path); err != nil {
		t.Fatal(err)
	}

	if got := layers.Read("base"); got.State != layers.Broken || got.Target != gone {
		t.Fatalf("Read(base) = %+v, a link to nothing; want broken, target %s", got, gone)
	}
}

func TestRead_FileIsBroken(t *testing.T) {

	path := filepath.Join(registry(t), "team")
	if err := os.WriteFile(path, []byte("not a layer"), 0o600); err != nil {
		t.Fatal(err)
	}

	if got := layers.Read("team"); got.State != layers.Broken || got.Root != "" {
		t.Fatalf("Read(team) = %+v over a file; want broken, no root", got)
	}
}

// --- IsWorkingTree ---

func TestIsWorkingTree(t *testing.T) {

	worktree := t.TempDir()
	if err := os.WriteFile(filepath.Join(worktree, ".git"), []byte("gitdir: elsewhere\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	for name, test := range map[string]struct {
		root string
		want bool
	}{
		"a repository":      {workingTree(t), true},
		"a linked worktree": {worktree, true},
		"a plain directory": {t.TempDir(), false},
		"a file":            {file, false},
		"nothing":           {filepath.Join(t.TempDir(), "gone"), false},
	} {
		if got := layers.IsWorkingTree(test.root); got != test.want {
			t.Errorf("IsWorkingTree(%s) = %v; want %v", name, got, test.want)
		}
	}
}
