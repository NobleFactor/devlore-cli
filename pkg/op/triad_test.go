// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// --- Root → Path → RecoverySite triad ---
//
// Tests exercising the cooperative triad: Root produces Path, Path flows to RecoverySite, RecoverySite
// delegates I/O to Root. Each suite runs against all three Root implementations to verify mode-agnostic
// behavior.

// triadEnv holds a fully wired triad for testing.
type triadEnv struct {
	Root op.Root
	Site *op.RecoverySite
	Dir  string // underlying directory
}

func newTriad(t *testing.T, root op.Root, dir string) triadEnv {
	t.Helper()
	ctx := &op.ExecutionContext{Root: root}
	site := op.NewRecoverySite(ctx)
	return triadEnv{Root: root, Site: site, Dir: dir}
}

func newTriadRW(t *testing.T) triadEnv {
	t.Helper()
	dir := t.TempDir()
	return newTriad(t, op.NewRootReaderWriter(dir), dir)
}

func newTriadConfined(t *testing.T) triadEnv {
	t.Helper()
	dir := t.TempDir()
	root, err := op.NewConfinedRoot(dir)
	if err != nil {
		t.Fatalf("NewConfinedRoot: %v", err)
	}
	t.Cleanup(func() { _ = root.Close() })
	return newTriad(t, root, dir)
}

// --- Path factory ---

func TestTriad_RootProducesPath(t *testing.T) {

	for _, tc := range []struct {
		name     string
		newTriad func(t *testing.T) triadEnv
	}{
		{"RootReaderWriter", newTriadRW},
		{"confinedRoot", newTriadConfined},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := tc.newTriad(t)

			p := env.Root.NewPath("sub/file.txt")
			if p.Rel() != "sub/file.txt" {
				t.Errorf("Rel() = %q, want %q", p.Rel(), "sub/file.txt")
			}
			wantAbs := filepath.Join(env.Dir, "sub/file.txt")
			if p.Abs() != wantAbs {
				t.Errorf("Abs() = %q, want %q", p.Abs(), wantAbs)
			}
			if p.Root() != env.Dir {
				t.Errorf("Root() = %q, want %q", p.Root(), env.Dir)
			}
		})
	}
}

func TestTriad_RootProducesPathFromAbsolute(t *testing.T) {

	for _, tc := range []struct {
		name     string
		newTriad func(t *testing.T) triadEnv
	}{
		{"RootReaderWriter", newTriadRW},
		{"confinedRoot", newTriadConfined},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := tc.newTriad(t)
			abs := filepath.Join(env.Dir, "deep/path.txt")

			p := env.Root.NewPath(abs)
			if p.Rel() != "deep/path.txt" {
				t.Errorf("Rel() = %q, want %q", p.Rel(), "deep/path.txt")
			}
			if p.Abs() != abs {
				t.Errorf("Abs() = %q, want %q", p.Abs(), abs)
			}
		})
	}
}

// --- ArchiveFile + RestoreFile round-trip ---

func TestTriad_ArchiveFileRestoreFile(t *testing.T) {

	for _, tc := range []struct {
		name     string
		newTriad func(t *testing.T) triadEnv
	}{
		{"RootReaderWriter", newTriadRW},
		{"confinedRoot", newTriadConfined},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := tc.newTriad(t)
			content := []byte("triad test content")

			// Create file via Root.
			abs := filepath.Join(env.Dir, "target.txt")
			if err := os.WriteFile(abs, content, 0o644); err != nil {
				t.Fatal(err)
			}

			// Archive via RecoverySite with Path from Root.
			p := env.Root.NewPath("target.txt")
			recoveryID, err := env.Site.ArchiveFile(p)
			if err != nil {
				t.Fatalf("ArchiveFile: %v", err)
			}

			// Original gone.
			if _, err := os.Lstat(abs); !os.IsNotExist(err) {
				t.Error("original still exists after archive")
			}

			// Recovery ID is opaque but under .devlore/recovery/.
			if !strings.HasPrefix(recoveryID, ".devlore/recovery/") {
				t.Errorf("recoveryID = %q, want .devlore/recovery/ prefix", recoveryID)
			}

			// Restore via RecoverySite.
			if err := env.Site.RestoreFile(p, recoveryID); err != nil {
				t.Fatalf("RestoreFile: %v", err)
			}

			// Original back with same content.
			got, err := os.ReadFile(abs)
			if err != nil {
				t.Fatalf("ReadFile: %v", err)
			}
			if string(got) != string(content) {
				t.Errorf("content = %q, want %q", got, content)
			}
		})
	}
}

// --- ArchiveData + RestoreData round-trip ---

func TestTriad_ArchiveDataRestoreData(t *testing.T) {

	for _, tc := range []struct {
		name     string
		newTriad func(t *testing.T) triadEnv
	}{
		{"RootReaderWriter", newTriadRW},
		{"confinedRoot", newTriadConfined},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := tc.newTriad(t)
			original := []byte("memory data for triad")

			recoveryID, err := env.Site.ArchiveData(original)
			if err != nil {
				t.Fatalf("ArchiveData: %v", err)
			}

			if !strings.HasPrefix(recoveryID, ".devlore/recovery/") {
				t.Errorf("recoveryID = %q, want .devlore/recovery/ prefix", recoveryID)
			}

			got, err := env.Site.RestoreData(recoveryID)
			if err != nil {
				t.Fatalf("RestoreData: %v", err)
			}
			if string(got) != string(original) {
				t.Errorf("data = %q, want %q", got, original)
			}
		})
	}
}

// --- Nested path archival + parent recreation ---

func TestTriad_NestedPathRecreation(t *testing.T) {

	for _, tc := range []struct {
		name     string
		newTriad func(t *testing.T) triadEnv
	}{
		{"RootReaderWriter", newTriadRW},
		{"confinedRoot", newTriadConfined},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := tc.newTriad(t)

			nested := filepath.Join(env.Dir, "a", "b", "c")
			if err := os.MkdirAll(nested, 0o755); err != nil {
				t.Fatal(err)
			}
			absFile := filepath.Join(nested, "deep.txt")
			if err := os.WriteFile(absFile, []byte("deep"), 0o644); err != nil {
				t.Fatal(err)
			}

			p := env.Root.NewPath("a/b/c/deep.txt")
			recoveryID, err := env.Site.ArchiveFile(p)
			if err != nil {
				t.Fatal(err)
			}

			// Remove parent dirs to simulate pruneEmptyParents.
			if err := os.RemoveAll(filepath.Join(env.Dir, "a")); err != nil {
				t.Fatal(err)
			}

			// Restore recreates parents.
			if err := env.Site.RestoreFile(p, recoveryID); err != nil {
				t.Fatalf("RestoreFile: %v", err)
			}

			got, err := os.ReadFile(absFile)
			if err != nil {
				t.Fatalf("ReadFile: %v", err)
			}
			if string(got) != "deep" {
				t.Errorf("content = %q, want %q", got, "deep")
			}
		})
	}
}

// --- Root I/O through Path ---

func TestTriad_WriteReadThroughRoot(t *testing.T) {

	for _, tc := range []struct {
		name     string
		newTriad func(t *testing.T) triadEnv
	}{
		{"RootReaderWriter", newTriadRW},
		{"confinedRoot", newTriadConfined},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := tc.newTriad(t)

			p := env.Root.NewPath("written.txt")
			content := []byte("written via Root")

			if err := env.Root.WriteFile(p, content, 0o644); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}

			got, err := env.Root.ReadFile(p)
			if err != nil {
				t.Fatalf("ReadFile: %v", err)
			}
			if string(got) != string(content) {
				t.Errorf("content = %q, want %q", got, content)
			}

			info, err := env.Root.Stat(p)
			if err != nil {
				t.Fatalf("Stat: %v", err)
			}
			if info.Size() != int64(len(content)) {
				t.Errorf("size = %d, want %d", info.Size(), len(content))
			}
		})
	}
}

func TestTriad_MkdirAllThroughRoot(t *testing.T) {

	for _, tc := range []struct {
		name     string
		newTriad func(t *testing.T) triadEnv
	}{
		{"RootReaderWriter", newTriadRW},
		{"confinedRoot", newTriadConfined},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := tc.newTriad(t)

			dirPath := env.Root.NewPath("x/y/z")
			if err := env.Root.MkdirAll(dirPath, 0o755); err != nil {
				t.Fatalf("MkdirAll: %v", err)
			}

			info, err := env.Root.Stat(dirPath)
			if err != nil {
				t.Fatalf("Stat: %v", err)
			}
			if !info.IsDir() {
				t.Error("expected directory, got file")
			}
		})
	}
}

func TestTriad_RenameThroughRoot(t *testing.T) {

	for _, tc := range []struct {
		name     string
		newTriad func(t *testing.T) triadEnv
	}{
		{"RootReaderWriter", newTriadRW},
		{"confinedRoot", newTriadConfined},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := tc.newTriad(t)

			old := env.Root.NewPath("old.txt")
			if err := env.Root.WriteFile(old, []byte("data"), 0o644); err != nil {
				t.Fatal(err)
			}

			dst := env.Root.NewPath("new.txt")
			if err := env.Root.Rename(old, dst); err != nil {
				t.Fatalf("Rename: %v", err)
			}

			if _, err := env.Root.Lstat(old); !os.IsNotExist(err) {
				t.Error("old file still exists after rename")
			}

			got, err := env.Root.ReadFile(dst)
			if err != nil {
				t.Fatalf("ReadFile: %v", err)
			}
			if string(got) != "data" {
				t.Errorf("content = %q, want %q", got, "data")
			}
		})
	}
}

// --- RootReader rejects writes ---

func TestTriad_RootReaderRejectsWrites(t *testing.T) {

	dir := t.TempDir()
	root := op.NewRootReader(dir)

	p := root.NewPath("file.txt")

	if err := root.WriteFile(p, []byte("data"), 0o644); !errors.Is(err, op.ErrReadOnly) {
		t.Errorf("WriteFile err = %v, want ErrReadOnly", err)
	}
	if err := root.MkdirAll(p, 0o755); !errors.Is(err, op.ErrReadOnly) {
		t.Errorf("MkdirAll err = %v, want ErrReadOnly", err)
	}
	if err := root.Remove(p); !errors.Is(err, op.ErrReadOnly) {
		t.Errorf("Remove err = %v, want ErrReadOnly", err)
	}
	if err := root.Rename(p, p); !errors.Is(err, op.ErrReadOnly) {
		t.Errorf("Rename err = %v, want ErrReadOnly", err)
	}
}

func TestTriad_RootReaderAllowsReads(t *testing.T) {

	dir := t.TempDir()
	abs := filepath.Join(dir, "readable.txt")
	if err := os.WriteFile(abs, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	root := op.NewRootReader(dir)
	p := root.NewPath("readable.txt")

	data, err := root.ReadFile(p)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("content = %q, want %q", data, "hello")
	}

	info, err := root.Stat(p)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Size() != 5 {
		t.Errorf("size = %d, want 5", info.Size())
	}
}

// --- Multiple archives share recovery directory ---

func TestTriad_MultipleArchivesCoexist(t *testing.T) {

	env := newTriadRW(t)

	for i, name := range []string{"a.txt", "b.txt", "c.txt"} {
		abs := filepath.Join(env.Dir, name)
		if err := os.WriteFile(abs, []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}

		id, err := env.Site.ArchiveFile(env.Root.NewPath(name))
		if err != nil {
			t.Fatalf("ArchiveFile(%s): %v", name, err)
		}
		if !strings.HasPrefix(id, ".devlore/recovery/") {
			t.Errorf("[%d] recoveryID = %q, want .devlore/recovery/ prefix", i, id)
		}
	}

	// All three recovery entries should exist.
	entries, err := os.ReadDir(filepath.Join(env.Dir, ".devlore", "recovery"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Errorf("recovery entries = %d, want 3", len(entries))
	}
}

// --- Path JSON serialization with Root ---

func TestTriad_PathJSONFromRoot(t *testing.T) {

	dir := t.TempDir()
	root := op.NewRootReaderWriter(dir)

	p := root.NewPath("sub/file.txt")
	data, err := p.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}

	var p2 op.Path
	if err := p2.UnmarshalJSON(data); err != nil {
		t.Fatal(err)
	}

	if p2.Root() != p.Root() {
		t.Errorf("Root() = %q, want %q", p2.Root(), p.Root())
	}
	if p2.Rel() != p.Rel() {
		t.Errorf("Rel() = %q, want %q", p2.Rel(), p.Rel())
	}
	if p2.Abs() != p.Abs() {
		t.Errorf("Abs() = %q, want %q", p2.Abs(), p.Abs())
	}
}

// --- Confined root blocks traversal ---

func TestTriad_ConfinedRootBlocksTraversal(t *testing.T) {

	env := newTriadConfined(t)

	// Create a path that tries to escape via ..
	p := env.Root.NewPath("../escape.txt")

	// Confined root should reject this.
	_, err := env.Root.Stat(p)
	if err == nil {
		t.Error("Stat(../escape.txt) should fail in confined mode")
	}
}
