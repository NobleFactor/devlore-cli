// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package migrate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	// Blank-import the op inventory so the file provider's gen package init() runs and registers its
	// ProviderReceiverType; buildRegistrationGraph looks it up via the receiver registry.
	_ "github.com/NobleFactor/devlore-cli/pkg/op/inventory"
)

func TestCommonAncestor(t *testing.T) {

	cases := []struct {
		a, b, want string
	}{
		{"/home/user/repo", "/home/user/.local/share/devlore/writ/layers/personal", "/home/user"},
		{"/opt/dotfiles", "/home/user/.local/share/devlore/writ/layers/personal", "/"},
		{"/home/user/a/b", "/home/user/a/b/c", "/home/user/a/b"},
		{"/same/path", "/same/path", "/same/path"},
	}

	for _, c := range cases {
		if got := commonAncestor(c.a, c.b); got != c.want {
			t.Errorf("commonAncestor(%q, %q) = %q, want %q", c.a, c.b, got, c.want)
		}
	}
}

func TestClearExistingLayer(t *testing.T) {

	t.Run("missing is a no-op", func(t *testing.T) {
		if err := clearExistingLayer(filepath.Join(t.TempDir(), "absent"), false); err != nil {
			t.Errorf("clearExistingLayer(absent) = %v, want nil", err)
		}
	})

	t.Run("symlink is removed", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(dir, "target")
		if err := os.MkdirAll(target, 0o755); err != nil {
			t.Fatal(err)
		}
		layer := filepath.Join(dir, "layer")
		if err := os.Symlink(target, layer); err != nil {
			t.Fatal(err)
		}
		if err := clearExistingLayer(layer, false); err != nil {
			t.Fatalf("clearExistingLayer(symlink) = %v", err)
		}
		if _, err := os.Lstat(layer); !os.IsNotExist(err) {
			t.Errorf("symlink survives; err = %v", err)
		}
	})

	t.Run("empty directory is removed", func(t *testing.T) {
		dir := t.TempDir()
		layer := filepath.Join(dir, "layer")
		if err := os.MkdirAll(layer, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := clearExistingLayer(layer, false); err != nil {
			t.Fatalf("clearExistingLayer(empty dir) = %v", err)
		}
		if _, err := os.Lstat(layer); !os.IsNotExist(err) {
			t.Errorf("empty dir survives; err = %v", err)
		}
	})

	t.Run("non-empty directory is refused", func(t *testing.T) {
		dir := t.TempDir()
		layer := filepath.Join(dir, "layer")
		if err := os.MkdirAll(layer, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(layer, "occupant.txt"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		err := clearExistingLayer(layer, false)
		if err == nil || !strings.Contains(err.Error(), "not empty") {
			t.Errorf("clearExistingLayer(non-empty) = %v, want the not-empty refusal", err)
		}
	})
}

// TestRegisterLayer_Link registers a repo in link mode: the layers parent is created and the layer directory becomes
// a symlink to the source; the source content is untouched.
func TestRegisterLayer_Link(t *testing.T) {

	root := t.TempDir()
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("HOME", root)

	sourceRoot := filepath.Join(root, "my-environment")
	if err := os.MkdirAll(filepath.Join(sourceRoot, "Home", "noblefactor"), 0o755); err != nil {
		t.Fatal(err)
	}
	seeded := filepath.Join(sourceRoot, "Home", "noblefactor", ".zshrc")
	if err := os.WriteFile(seeded, []byte("zsh"), 0o644); err != nil {
		t.Fatal(err)
	}

	layerDir := filepath.Join(root, "data", "devlore", "writ", "layers", "personal")

	if err := RegisterLayer(context.Background(), sourceRoot, layerDir, false, false); err != nil {
		t.Fatalf("RegisterLayer(link): %v", err)
	}

	info, err := os.Lstat(layerDir)
	if err != nil {
		t.Fatalf("lstat layer: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("layer %s is not a symlink", layerDir)
	}

	// The layer resolves into the source content.
	through := filepath.Join(layerDir, "Home", "noblefactor", ".zshrc")
	gotBytes, err := os.ReadFile(through)
	if err != nil {
		t.Fatalf("read through the layer symlink: %v", err)
	}
	if got := string(gotBytes); got != "zsh" {
		t.Errorf("content through layer = %q, want %q", got, "zsh")
	}

	// The source itself is untouched (link mode moves nothing).
	if _, err := os.Stat(seeded); err != nil {
		t.Errorf("source content disturbed: %v", err)
	}
}

// TestRegisterLayer_Move registers a repo in move mode: the content moves into the layer directory and the source
// location is gone.
func TestRegisterLayer_Move(t *testing.T) {

	root := t.TempDir()
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("HOME", root)

	sourceRoot := filepath.Join(root, "my-environment")
	if err := os.MkdirAll(filepath.Join(sourceRoot, "Home", "noblefactor"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceRoot, "Home", "noblefactor", ".zshrc"), []byte("zsh"), 0o644); err != nil {
		t.Fatal(err)
	}

	layerDir := filepath.Join(root, "data", "devlore", "writ", "layers", "personal")

	if err := RegisterLayer(context.Background(), sourceRoot, layerDir, true, false); err != nil {
		t.Fatalf("RegisterLayer(move): %v", err)
	}

	moved := filepath.Join(layerDir, "Home", "noblefactor", ".zshrc")
	gotBytes, err := os.ReadFile(moved)
	if err != nil {
		t.Fatalf("read moved content: %v", err)
	}
	if got := string(gotBytes); got != "zsh" {
		t.Errorf("moved content = %q, want %q", got, "zsh")
	}

	if _, err := os.Lstat(sourceRoot); !os.IsNotExist(err) {
		t.Errorf("source root survives after move; err = %v", err)
	}
}

// TestRegisterLayer_RefusesNonEmptyLayer pins the guard: an occupied layer directory refuses registration before
// any node dispatches.
func TestRegisterLayer_RefusesNonEmptyLayer(t *testing.T) {

	root := t.TempDir()
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("HOME", root)

	sourceRoot := filepath.Join(root, "my-environment")
	if err := os.MkdirAll(sourceRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	layerDir := filepath.Join(root, "data", "devlore", "writ", "layers", "personal")
	if err := os.MkdirAll(layerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(layerDir, "occupant.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := RegisterLayer(context.Background(), sourceRoot, layerDir, false, false)
	if err == nil || !strings.Contains(err.Error(), "not empty") {
		t.Errorf("RegisterLayer(occupied) = %v, want the not-empty refusal", err)
	}
}
