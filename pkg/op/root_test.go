// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op_test

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// ── Path ────────────────────────────────────────────────────────────────────────────────────────────

func TestNewPath(t *testing.T) {

	p := op.NewPath("/root", "rel/file.txt")
	if p.Rel() != "rel/file.txt" {
		t.Errorf("Rel() = %q, want %q", p.Rel(), "rel/file.txt")
	}
	if p.Abs() != "/root/rel/file.txt" {
		t.Errorf("Abs() = %q, want %q", p.Abs(), "/root/rel/file.txt")
	}
	if p.Root() != "/root" {
		t.Errorf("Root() = %q, want %q", p.Root(), "/root")
	}
	if p.String() != "/root/rel/file.txt" {
		t.Errorf("String() = %q, want %q", p.String(), "/root/rel/file.txt")
	}
}

func TestRoot_NewPath_Relative(t *testing.T) {

	dir := t.TempDir()
	roots := allRoots(t, dir)

	for _, tc := range roots {
		t.Run(tc.name, func(t *testing.T) {
			p := tc.root.NewPath("sub/file.txt")
			if p.Rel() != "sub/file.txt" {
				t.Errorf("Rel() = %q, want %q", p.Rel(), "sub/file.txt")
			}
			wantAbs := filepath.Join(dir, "sub/file.txt")
			if p.Abs() != wantAbs {
				t.Errorf("Abs() = %q, want %q", p.Abs(), wantAbs)
			}
			if p.Root() != dir {
				t.Errorf("Root() = %q, want %q", p.Root(), dir)
			}
		})
	}
}

func TestRoot_NewPath_Absolute(t *testing.T) {

	dir := t.TempDir()
	roots := allRoots(t, dir)

	absPath := filepath.Join(dir, "a", "b.txt")
	for _, tc := range roots {
		t.Run(tc.name, func(t *testing.T) {
			p := tc.root.NewPath(absPath)
			if p.Abs() != absPath {
				t.Errorf("Abs() = %q, want %q", p.Abs(), absPath)
			}
			wantRel := filepath.Join("a", "b.txt")
			if p.Rel() != wantRel {
				t.Errorf("Rel() = %q, want %q", p.Rel(), wantRel)
			}
		})
	}
}

func TestRoot_NewPath_CleansDotSegments(t *testing.T) {

	dir := t.TempDir()
	roots := allRoots(t, dir)

	for _, tc := range roots {
		t.Run(tc.name, func(t *testing.T) {
			p := tc.root.NewPath("a/../b/./c.txt")
			if p.Rel() != "b/c.txt" {
				t.Errorf("Rel() = %q, want %q", p.Rel(), "b/c.txt")
			}
		})
	}
}

// ── Path serialization ───────────────────────────────────────────────────────────────────────────────

func TestPath_MarshalJSON(t *testing.T) {

	p := op.NewPath("/project", "src/main.go")
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}

	want := `{"root":"/project","rel":"src/main.go"}`
	if string(data) != want {
		t.Errorf("JSON = %s, want %s", data, want)
	}
}

func TestPath_UnmarshalJSON(t *testing.T) {

	raw := `{"root":"/project","rel":"src/main.go"}`
	var p op.Path
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}

	if p.Root() != "/project" {
		t.Errorf("Root() = %q, want %q", p.Root(), "/project")
	}
	if p.Rel() != "src/main.go" {
		t.Errorf("Rel() = %q, want %q", p.Rel(), "src/main.go")
	}
	if p.Abs() != "/project/src/main.go" {
		t.Errorf("Abs() = %q, want %q", p.Abs(), "/project/src/main.go")
	}
}

func TestPath_JSONRoundTrip(t *testing.T) {

	original := op.NewPath("/data", "files/config.yaml")
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var restored op.Path
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if restored.Root() != original.Root() {
		t.Errorf("Root() = %q, want %q", restored.Root(), original.Root())
	}
	if restored.Rel() != original.Rel() {
		t.Errorf("Rel() = %q, want %q", restored.Rel(), original.Rel())
	}
	if restored.Abs() != original.Abs() {
		t.Errorf("Abs() = %q, want %q", restored.Abs(), original.Abs())
	}
}

// ── ReceiverName and Close ──────────────────────────────────────────────────────────────────────────────────

func TestRoot_Name(t *testing.T) {

	dir := t.TempDir()
	roots := allRoots(t, dir)

	for _, tc := range roots {
		t.Run(tc.name, func(t *testing.T) {
			if tc.root.Name() != dir {
				t.Errorf("ReceiverName() = %q, want %q", tc.root.Name(), dir)
			}
		})
	}
}

func TestRoot_Close(t *testing.T) {

	dir := t.TempDir()

	// RootReader — Close is a no-op.
	r := op.NewRootReader(dir)
	if err := r.Close(); err != nil {
		t.Errorf("RootReader.Close() = %v", err)
	}

	// RootReaderWriter — Close is a no-op.
	rw := op.NewRootReaderWriter(dir)
	if err := rw.Close(); err != nil {
		t.Errorf("RootReaderWriter.Close() = %v", err)
	}

	// confinedRoot — Close releases the file descriptor.
	cr, err := op.NewConfinedRoot(dir)
	if err != nil {
		t.Fatalf("NewConfinedRoot: %v", err)
	}
	if err := cr.Close(); err != nil {
		t.Errorf("confinedRoot.Close() = %v", err)
	}
}

// ── FS ──────────────────────────────────────────────────────────────────────────────────────────────

func TestRoot_FS(t *testing.T) {

	dir := t.TempDir()
	writeFixture(t, dir, "fstest.txt", "hello")
	roots := allRoots(t, dir)

	for _, tc := range roots {
		t.Run(tc.name, func(t *testing.T) {
			fsys := tc.root.FS()
			data, err := fs.ReadFile(fsys, "fstest.txt")
			if err != nil {
				t.Fatalf("fs.ReadFile: %v", err)
			}
			if string(data) != "hello" {
				t.Errorf("content = %q, want %q", data, "hello")
			}
		})
	}
}

// ── Read operations ─────────────────────────────────────────────────────────────────────────────────

func TestRoot_Stat(t *testing.T) {

	dir := t.TempDir()
	writeFixture(t, dir, "stat.txt", "data")
	roots := allRoots(t, dir)

	for _, tc := range roots {
		t.Run(tc.name, func(t *testing.T) {
			p := tc.root.NewPath("stat.txt")
			info, err := tc.root.Stat(p)
			if err != nil {
				t.Fatalf("Stat: %v", err)
			}
			if info.Name() != "stat.txt" {
				t.Errorf("ReceiverName = %q, want %q", info.Name(), "stat.txt")
			}
			if info.Size() != 4 {
				t.Errorf("Size = %d, want 4", info.Size())
			}
		})
	}
}

func TestRoot_Stat_NotExist(t *testing.T) {

	dir := t.TempDir()
	roots := allRoots(t, dir)

	for _, tc := range roots {
		t.Run(tc.name, func(t *testing.T) {
			p := tc.root.NewPath("no_such_file")
			_, err := tc.root.Stat(p)
			if err == nil {
				t.Fatal("expected error for missing file")
			}
			if !os.IsNotExist(err) {
				t.Errorf("expected not-exist error, got: %v", err)
			}
		})
	}
}

func TestRoot_Lstat(t *testing.T) {

	dir := t.TempDir()
	writeFixture(t, dir, "lstat.txt", "data")
	roots := allRoots(t, dir)

	for _, tc := range roots {
		t.Run(tc.name, func(t *testing.T) {
			p := tc.root.NewPath("lstat.txt")
			info, err := tc.root.Lstat(p)
			if err != nil {
				t.Fatalf("Lstat: %v", err)
			}
			if info.Name() != "lstat.txt" {
				t.Errorf("ReceiverName = %q, want %q", info.Name(), "lstat.txt")
			}
		})
	}
}

func TestRoot_Open(t *testing.T) {

	dir := t.TempDir()
	writeFixture(t, dir, "open.txt", "content")
	roots := allRoots(t, dir)

	for _, tc := range roots {
		t.Run(tc.name, func(t *testing.T) {
			p := tc.root.NewPath("open.txt")
			f, err := tc.root.Open(p)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			defer f.Close()

			buf := make([]byte, 7)
			n, err := f.Read(buf)
			if err != nil {
				t.Fatalf("Read: %v", err)
			}
			if string(buf[:n]) != "content" {
				t.Errorf("content = %q, want %q", buf[:n], "content")
			}
		})
	}
}

func TestRoot_ReadFile(t *testing.T) {

	dir := t.TempDir()
	writeFixture(t, dir, "readfile.txt", "bytes")
	roots := allRoots(t, dir)

	for _, tc := range roots {
		t.Run(tc.name, func(t *testing.T) {
			p := tc.root.NewPath("readfile.txt")
			data, err := tc.root.ReadFile(p)
			if err != nil {
				t.Fatalf("ReadFile: %v", err)
			}
			if string(data) != "bytes" {
				t.Errorf("content = %q, want %q", data, "bytes")
			}
		})
	}
}

func TestRoot_Readlink(t *testing.T) {

	dir := t.TempDir()
	writeFixture(t, dir, "target.txt", "data")

	// Create symlink with relative target (required for confined mode).
	if err := os.Symlink("target.txt", filepath.Join(dir, "link.txt")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	roots := allRoots(t, dir)

	for _, tc := range roots {
		t.Run(tc.name, func(t *testing.T) {
			p := tc.root.NewPath("link.txt")
			target, err := tc.root.Readlink(p)
			if err != nil {
				t.Fatalf("Readlink: %v", err)
			}
			if target != "target.txt" {
				t.Errorf("target = %q, want %q", target, "target.txt")
			}
		})
	}
}

// ── Write operations ────────────────────────────────────────────────────────────────────────────────

func TestRoot_MkdirAll(t *testing.T) {

	dir := t.TempDir()
	writableRoots := writableRoots(t, dir)

	for _, tc := range writableRoots {
		t.Run(tc.name, func(t *testing.T) {
			p := tc.root.NewPath(filepath.Join(tc.name, "a", "b", "c"))
			if err := tc.root.MkdirAll(p, 0o755); err != nil {
				t.Fatalf("MkdirAll: %v", err)
			}
			info, err := os.Stat(p.Abs())
			if err != nil {
				t.Fatalf("os.Stat after MkdirAll: %v", err)
			}
			if !info.IsDir() {
				t.Error("expected directory")
			}
		})
	}
}

func TestRoot_OpenFile(t *testing.T) {

	dir := t.TempDir()
	writableRoots := writableRoots(t, dir)

	for _, tc := range writableRoots {
		t.Run(tc.name, func(t *testing.T) {
			p := tc.root.NewPath(tc.name + "_openfile.txt")
			f, err := tc.root.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
			if err != nil {
				t.Fatalf("OpenFile: %v", err)
			}
			if _, err := f.Write([]byte("written")); err != nil {
				t.Fatalf("Write: %v", err)
			}
			f.Close()

			data, err := os.ReadFile(p.Abs())
			if err != nil {
				t.Fatalf("os.ReadFile: %v", err)
			}
			if string(data) != "written" {
				t.Errorf("content = %q, want %q", data, "written")
			}
		})
	}
}

func TestRoot_Remove(t *testing.T) {

	dir := t.TempDir()
	writableRoots := writableRoots(t, dir)

	for _, tc := range writableRoots {
		t.Run(tc.name, func(t *testing.T) {
			name := tc.name + "_remove.txt"
			writeFixture(t, dir, name, "doomed")
			p := tc.root.NewPath(name)

			if err := tc.root.Remove(p); err != nil {
				t.Fatalf("Remove: %v", err)
			}
			if _, err := os.Stat(p.Abs()); !os.IsNotExist(err) {
				t.Errorf("file still exists after Remove")
			}
		})
	}
}

func TestRoot_Rename(t *testing.T) {

	dir := t.TempDir()
	writableRoots := writableRoots(t, dir)

	for _, tc := range writableRoots {
		t.Run(tc.name, func(t *testing.T) {
			oldName := tc.name + "_old.txt"
			newName := tc.name + "_new.txt"
			writeFixture(t, dir, oldName, "moved")

			oldPath := tc.root.NewPath(oldName)
			newPath := tc.root.NewPath(newName)
			if err := tc.root.Rename(oldPath, newPath); err != nil {
				t.Fatalf("Rename: %v", err)
			}

			if _, err := os.Stat(oldPath.Abs()); !os.IsNotExist(err) {
				t.Errorf("old file still exists after Rename")
			}
			data, err := os.ReadFile(newPath.Abs())
			if err != nil {
				t.Fatalf("os.ReadFile: %v", err)
			}
			if string(data) != "moved" {
				t.Errorf("content = %q, want %q", data, "moved")
			}
		})
	}
}

func TestRoot_Symlink(t *testing.T) {

	dir := t.TempDir()
	writeFixture(t, dir, "symtarget.txt", "data")
	writableRoots := writableRoots(t, dir)

	for _, tc := range writableRoots {
		t.Run(tc.name, func(t *testing.T) {
			linkName := tc.name + "_symlink.txt"
			link := tc.root.NewPath(linkName)

			if err := tc.root.Symlink("symtarget.txt", link); err != nil {
				t.Fatalf("Symlink: %v", err)
			}

			target, err := os.Readlink(link.Abs())
			if err != nil {
				t.Fatalf("os.Readlink: %v", err)
			}
			if target != "symtarget.txt" {
				t.Errorf("target = %q, want %q", target, "symtarget.txt")
			}
		})
	}
}

func TestRoot_WriteFile(t *testing.T) {

	dir := t.TempDir()
	writableRoots := writableRoots(t, dir)

	for _, tc := range writableRoots {
		t.Run(tc.name, func(t *testing.T) {
			p := tc.root.NewPath(tc.name + "_writefile.txt")
			if err := tc.root.WriteFile(p, []byte("payload"), 0o644); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}
			data, err := os.ReadFile(p.Abs())
			if err != nil {
				t.Fatalf("os.ReadFile: %v", err)
			}
			if string(data) != "payload" {
				t.Errorf("content = %q, want %q", data, "payload")
			}
		})
	}
}

// ── RootReader write rejection ──────────────────────────────────────────────────────────────────────

func TestRootReader_WritesReturnErrReadOnly(t *testing.T) {

	dir := t.TempDir()
	r := op.NewRootReader(dir)
	p := r.NewPath("file.txt")

	tests := []struct {
		name string
		fn   func() error
	}{
		{"MkdirAll", func() error { return r.MkdirAll(p, 0o755) }},
		{"OpenFile", func() error { _, err := r.OpenFile(p, os.O_WRONLY, 0o644); return err }},
		{"Remove", func() error { return r.Remove(p) }},
		{"Rename", func() error { return r.Rename(p, p) }},
		{"Symlink", func() error { return r.Symlink("target", p) }},
		{"WriteFile", func() error { return r.WriteFile(p, []byte("x"), 0o644) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fn()
			if err != op.ErrReadOnly {
				t.Errorf("got %v, want ErrReadOnly", err)
			}
		})
	}
}

// ── confinedRoot confinement ────────────────────────────────────────────────────────────────────────

func TestConfinedRoot_RejectsTraversal(t *testing.T) {

	dir := t.TempDir()
	r, err := op.NewConfinedRoot(dir)
	if err != nil {
		t.Fatalf("NewConfinedRoot: %v", err)
	}
	defer r.Close()

	// Construct a path that escapes the root.
	p := op.NewPath(dir, "../../etc/passwd")
	_, err = r.Stat(p)
	if err == nil {
		t.Fatal("expected error for traversal path")
	}
}

func TestConfinedRoot_InvalidDir(t *testing.T) {

	_, err := op.NewConfinedRoot("/nonexistent/path/that/does/not/exist")
	if err == nil {
		t.Fatal("expected error for nonexistent directory")
	}
}

// ── helpers ─────────────────────────────────────────────────────────────────────────────────────────

type rootCase struct {
	name string
	root op.Root
}

// allRoots returns all three Root implementations rooted at dir. The confinedRoot is registered for cleanup.
func allRoots(t *testing.T, dir string) []rootCase {

	t.Helper()
	cr, err := op.NewConfinedRoot(dir)
	if err != nil {
		t.Fatalf("NewConfinedRoot: %v", err)
	}
	t.Cleanup(func() { cr.Close() })

	return []rootCase{
		{"RootReader", op.NewRootReader(dir)},
		{"RootReaderWriter", op.NewRootReaderWriter(dir)},
		{"confinedRoot", cr},
	}
}

// writableRoots returns Root implementations that support write operations.
func writableRoots(t *testing.T, dir string) []rootCase {

	t.Helper()
	cr, err := op.NewConfinedRoot(dir)
	if err != nil {
		t.Fatalf("NewConfinedRoot: %v", err)
	}
	t.Cleanup(func() { cr.Close() })

	return []rootCase{
		{"RootReaderWriter", op.NewRootReaderWriter(dir)},
		{"confinedRoot", cr},
	}
}

// writeFixture creates a file under dir with the given content.
func writeFixture(t *testing.T, dir, name, content string) {

	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}
