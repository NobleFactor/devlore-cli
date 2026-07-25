// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package sops

import (
	"os"
	"path/filepath"
	"testing"
)

// writeLocateFixture creates an empty config file at path, making parent directories as needed.
func writeLocateFixture(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("creation_rules: []\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestLocate_NearestInTree(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // isolate: no XDG fallback present
	deep := filepath.Join(root, "a", "b")
	writeLocateFixture(t, filepath.Join(deep, ".sops.yaml"))

	got := locate(root, deep, ".sops.yaml", "devlore/sops.yaml")
	assertChain(t, got, []string{filepath.Join(deep, ".sops.yaml")})
}

func TestLocate_AncestorWins_DeepestFirst(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	writeLocateFixture(t, filepath.Join(root, ".sops.yaml"))
	writeLocateFixture(t, filepath.Join(root, "a", ".sops.yaml"))
	start := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(start, 0o755); err != nil {
		t.Fatal(err)
	}

	got := locate(root, start, ".sops.yaml", "devlore/sops.yaml")
	assertChain(t, got, []string{
		filepath.Join(root, "a", ".sops.yaml"), // deepest first
		filepath.Join(root, ".sops.yaml"),
	})
}

func TestLocate_BoundedByRoot(t *testing.T) {
	parent := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	writeLocateFixture(t, filepath.Join(parent, ".sops.yaml")) // above root — must NOT be collected
	root := filepath.Join(parent, "root")
	start := filepath.Join(root, "x")
	if err := os.MkdirAll(start, 0o755); err != nil {
		t.Fatal(err)
	}

	assertChain(t, locate(root, start, ".sops.yaml", "devlore/sops.yaml"), nil)
}

func TestLocate_XDGFallback(t *testing.T) {
	root := t.TempDir()
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	writeLocateFixture(t, filepath.Join(xdg, "devlore", "sops.yaml"))

	got := locate(root, root, ".sops.yaml", "devlore/sops.yaml") // no in-tree config
	assertChain(t, got, []string{filepath.Join(xdg, "devlore", "sops.yaml")})
}

func TestLocate_InTreeThenFallback(t *testing.T) {
	root := t.TempDir()
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	writeLocateFixture(t, filepath.Join(root, ".sops.yaml"))
	writeLocateFixture(t, filepath.Join(xdg, "devlore", "sops.yaml"))

	got := locate(root, root, ".sops.yaml", "devlore/sops.yaml")
	assertChain(t, got, []string{
		filepath.Join(root, ".sops.yaml"),          // in-tree first
		filepath.Join(xdg, "devlore", "sops.yaml"), // fallback last
	})
}

func TestLocate_None(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	assertChain(t, locate(root, root, ".sops.yaml", "devlore/sops.yaml"), nil)
}

func TestLocate_StartDirOutsideRoot_OnlyFallback(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir() // not under root
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	writeLocateFixture(t, filepath.Join(outside, ".sops.yaml")) // must NOT be collected
	writeLocateFixture(t, filepath.Join(xdg, "devlore", "sops.yaml"))

	got := locate(root, outside, ".sops.yaml", "devlore/sops.yaml")
	assertChain(t, got, []string{filepath.Join(xdg, "devlore", "sops.yaml")})
}

func assertChain(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("chain length = %d %v, want %d %v", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("chain[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
