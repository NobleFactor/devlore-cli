// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package fsroot_test

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
)

func TestNewPath(t *testing.T) {

	p := fsroot.NewPath("/fsroot", "rel/file.txt")
	if p.Rel() != "rel/file.txt" {
		t.Errorf("Rel() = %q, want %q", p.Rel(), "rel/file.txt")
	}
	// Abs is a local OS-native handle, so its expectation is platform-correct — unlike Rel,
	// whose slash form is the platform-invariant contract asserted verbatim above.
	wantAbs := filepath.FromSlash("/fsroot/rel/file.txt")
	if p.Abs() != wantAbs {
		t.Errorf("Abs() = %q, want %q", p.Abs(), wantAbs)
	}
	if p.Root() != "/fsroot" {
		t.Errorf("Root() = %q, want %q", p.Root(), "/fsroot")
	}
	if p.String() != wantAbs {
		t.Errorf("String() = %q, want %q", p.String(), wantAbs)
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
			wantAbs := filepath.Join(dir, "sub", "file.txt")
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
			if p.Rel() != "a/b.txt" {
				t.Errorf("Rel() = %q, want %q (canonical slash form on every platform)", p.Rel(), "a/b.txt")
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

func TestPath_MarshalJSON(t *testing.T) {

	p := fsroot.NewPath("/project", "src/main.go")
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}

	want := `{"fsroot":"/project","rel":"src/main.go"}`
	if string(data) != want {
		t.Errorf("JSON = %s, want %s", data, want)
	}
}

func TestPath_UnmarshalJSON(t *testing.T) {

	raw := `{"fsroot":"/project","rel":"src/main.go"}`
	var p fsroot.Path
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}

	if p.Root() != "/project" {
		t.Errorf("Root() = %q, want %q", p.Root(), "/project")
	}
	if p.Rel() != "src/main.go" {
		t.Errorf("Rel() = %q, want %q", p.Rel(), "src/main.go")
	}
	if wantAbs := filepath.FromSlash("/project/src/main.go"); p.Abs() != wantAbs {
		t.Errorf("Abs() = %q, want %q", p.Abs(), wantAbs)
	}
}

func TestPath_JSONRoundTrip(t *testing.T) {

	original := fsroot.NewPath("/data", "files/config.yaml")
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var restored fsroot.Path
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
	r := fsroot.OpenUnconfined(dir)
	if err := r.Close(); err != nil {
		t.Errorf("RootReader.Close() = %v", err)
	}

	// RootReaderWriter — Close is a no-op.
	rw := fsroot.OpenWritableUnconfined(dir)
	if err := rw.Close(); err != nil {
		t.Errorf("RootReaderWriter.Close() = %v", err)
	}

	// confinedRoot — Close releases the file descriptor.
	cr, err := fsroot.OpenConfined(dir)
	if err != nil {
		t.Fatalf("fsroot.OpenConfined: %v", err)
	}
	if err := cr.Close(); err != nil {
		t.Errorf("fsroot.confinedRoot.Close() = %v", err)
	}
}

func TestRoot_FS(t *testing.T) {

	dir := t.TempDir()
	writeFixture(t, dir, "fstest.txt", "hello")
	roots := allRoots(t, dir)

	for _, tc := range roots {
		t.Run(tc.name, func(t *testing.T) {
			fileSystem := tc.root.FS()
			data, err := fs.ReadFile(fileSystem, "fstest.txt")
			if err != nil {
				t.Fatalf("fs.ReadFile: %v", err)
			}
			if string(data) != "hello" {
				t.Errorf("content = %q, want %q", data, "hello")
			}
		})
	}
}

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
			defer func() { _ = f.Close() }()

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
			if _, err := f.WriteString("written"); err != nil {
				t.Fatalf("Write: %v", err)
			}
			_ = f.Close()

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

func TestRootReader_WritesReturnErrReadOnly(t *testing.T) {

	dir := t.TempDir()
	r := fsroot.OpenUnconfined(dir)
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
			if !errors.Is(err, errors.ErrUnsupported) {
				t.Errorf("got %v, want ErrReadOnly", err)
			}
		})
	}
}

func TestConfinedRoot_RejectsTraversal(t *testing.T) {

	dir := t.TempDir()

	r, err := fsroot.OpenConfined(dir)
	if err != nil {
		t.Fatalf("NewConfinedRoot: %v", err)
	}
	defer func() { _ = r.Close() }()

	// Construct a path that escapes the fsroot.

	p := fsroot.NewPath(dir, "../../etc/passwd")

	_, err = r.Stat(p)
	if err == nil {
		t.Fatal("expected error for traversal path")
	}
}

func TestConfinedRoot_InvalidDir(t *testing.T) {

	_, err := fsroot.OpenConfined("/nonexistent/path/that/does/not/exist")
	if err == nil {
		t.Fatal("expected error for nonexistent directory")
	}
}

type rootCase struct {
	name string
	root fsroot.Dir
}

func TestOpen_ModeConfined(t *testing.T) {

	dir := t.TempDir()

	root, err := fsroot.Open(dir, fsroot.ModeConfined)
	if err != nil {
		t.Fatalf("Open(ModeConfined): %v", err)
	}

	p := root.NewPath("probe.txt")
	if err := root.WriteFile(p, []byte("confined"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := root.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Only the confined implementation holds a live OS handle, so failure after Close is the
	// behavioral discriminator for the dispatch.
	if _, err := root.ReadFile(p); err == nil {
		t.Fatal("ReadFile after Close = nil error, want failure (confined root holds a live handle)")
	}
}

func TestOpen_ModeUnconfined(t *testing.T) {

	dir := t.TempDir()
	writeFixture(t, dir, "fixture.txt", "content")

	root, err := fsroot.Open(dir, fsroot.ModeUnconfined)
	if err != nil {
		t.Fatalf("Open(ModeUnconfined): %v", err)
	}

	if _, err := root.ReadFile(root.NewPath("fixture.txt")); err != nil {
		t.Errorf("ReadFile: %v", err)
	}

	err = root.WriteFile(root.NewPath("probe.txt"), []byte("x"), 0o644)
	if !errors.Is(err, errors.ErrUnsupported) {
		t.Errorf("WriteFile = %v, want errors.ErrUnsupported (read-only mode)", err)
	}
}

func TestOpen_ModeWritableUnconfined(t *testing.T) {

	dir := t.TempDir()

	root, err := fsroot.Open(dir, fsroot.ModeWritableUnconfined)
	if err != nil {
		t.Fatalf("Open(ModeWritableUnconfined): %v", err)
	}

	p := root.NewPath("probe.txt")
	if err := root.WriteFile(p, []byte("writable"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := root.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Unconfined roots are handle-free: Close is a no-op and operations keep working.
	if _, err := root.ReadFile(p); err != nil {
		t.Errorf("ReadFile after Close = %v, want nil (unconfined roots are handle-free)", err)
	}
}

func TestOpen_ConfinedMissingDirFails(t *testing.T) {

	missing := filepath.Join(t.TempDir(), "absent")

	if _, err := fsroot.Open(missing, fsroot.ModeConfined); err == nil {
		t.Fatal("Open(ModeConfined) on a missing directory = nil error, want failure")
	}
}

func TestOpen_ZeroValueModeIsConfined(t *testing.T) {

	var mode fsroot.Mode
	if mode != fsroot.ModeConfined {
		t.Fatalf("zero-value Mode = %d, want ModeConfined (%d)", mode, fsroot.ModeConfined)
	}
}

func TestParity_WritableRootsImplementEveryMutation(t *testing.T) {

	dir := t.TempDir()

	for _, tc := range writableRoots(t, dir) {
		t.Run(tc.name, func(t *testing.T) {

			base := tc.root.NewPath(tc.name)
			if err := tc.root.Mkdir(base, 0o750); err != nil {
				t.Fatalf("Mkdir: %v", err)
			}

			file := tc.root.NewPath(filepath.Join(tc.name, "created.txt"))

			handle, err := tc.root.Create(file)
			if err != nil {
				t.Fatalf("Create: %v", err)
			}
			if _, err := handle.WriteString("parity"); err != nil {
				t.Fatalf("write: %v", err)
			}
			if err := handle.Close(); err != nil {
				t.Fatalf("close: %v", err)
			}

			if err := tc.root.Chmod(file, 0o600); err != nil {
				t.Fatalf("Chmod: %v", err)
			}

			stamp := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
			if err := tc.root.Chtimes(file, stamp, stamp); err != nil {
				t.Fatalf("Chtimes: %v", err)
			}
			info, err := tc.root.Stat(file)
			if err != nil {
				t.Fatalf("Stat: %v", err)
			}
			if !info.ModTime().UTC().Equal(stamp) {
				t.Errorf("ModTime = %v, want %v", info.ModTime().UTC(), stamp)
			}

			link := tc.root.NewPath(filepath.Join(tc.name, "hard.txt"))
			if err := tc.root.Link(file, link); err != nil {
				t.Fatalf("Link: %v", err)
			}
			if _, err := tc.root.Stat(link); err != nil {
				t.Errorf("Stat(hard link): %v", err)
			}

			// Chown/Lchown to the current identity is a no-op on Unix and unsupported on Windows;
			// either outcome is correct, so only a panic or a wrong-platform success would fail here.
			chownErr := tc.root.Chown(file, -1, -1)
			if runtime.GOOS == "windows" && chownErr == nil {
				t.Error("Chown on Windows returned nil; want the os-level unsupported error")
			}
			if runtime.GOOS != "windows" && chownErr != nil {
				t.Errorf("Chown(-1,-1) = %v, want nil (no-op)", chownErr)
			}
			if err := tc.root.Lchown(file, -1, -1); err != nil && runtime.GOOS != "windows" {
				t.Errorf("Lchown(-1,-1) = %v, want nil (no-op)", err)
			}

			sub, err := tc.root.OpenRoot(base)
			if err != nil {
				t.Fatalf("OpenRoot: %v", err)
			}
			if _, err := sub.ReadFile(sub.NewPath("created.txt")); err != nil {
				t.Errorf("sub-root ReadFile: %v", err)
			}
			if err := sub.Close(); err != nil {
				t.Errorf("sub-root Close: %v", err)
			}

			if err := tc.root.RemoveAll(base); err != nil {
				t.Fatalf("RemoveAll: %v", err)
			}
			if _, err := tc.root.Stat(base); err == nil {
				t.Error("Stat after RemoveAll = nil error; want the tree gone")
			}
		})
	}
}

// TestCreateTemp_UniqueNamesHonorThePatternAndStayInTheDirectory proves the two temp constructors behave like
// [os.CreateTemp] and [os.MkdirTemp] while staying inside the root: distinct names under the same pattern, the
// prefix and suffix around the random component, and the private modes that carry Windows enforcement.
func TestCreateTemp_UniqueNamesHonorThePatternAndStayInTheDirectory(t *testing.T) {

	dir := t.TempDir()

	for _, tc := range writableRoots(t, dir) {
		t.Run(tc.name, func(t *testing.T) {

			under := tc.root.NewPath(tc.name)
			if err := tc.root.Mkdir(under, 0o750); err != nil {
				t.Fatalf("Mkdir: %v", err)
			}

			first, firstPath, err := tc.root.CreateTemp(under, "spool-*.zip")
			if err != nil {
				t.Fatalf("CreateTemp: %v", err)
			}
			t.Cleanup(func() { _ = first.Close() })

			second, secondPath, err := tc.root.CreateTemp(under, "spool-*.zip")
			if err != nil {
				t.Fatalf("CreateTemp (second): %v", err)
			}
			t.Cleanup(func() { _ = second.Close() })

			if firstPath.Rel() == secondPath.Rel() {
				t.Fatalf("both CreateTemp calls returned %q; concurrent callers would collide", firstPath.Rel())
			}

			for _, p := range []fsroot.Path{firstPath, secondPath} {
				base := path.Base(p.Rel())
				if !strings.HasPrefix(base, "spool-") || !strings.HasSuffix(base, ".zip") {
					t.Errorf("name %q does not wrap the random part in the pattern's prefix and suffix", base)
				}
				if parent := path.Dir(p.Rel()); parent != under.Rel() {
					t.Errorf("created under %q, want %q", parent, under.Rel())
				}
				if _, err := tc.root.Stat(p); err != nil {
					t.Errorf("Stat(%s): %v", p.Rel(), err)
				}
			}

			// 0600/0700 match os.CreateTemp/os.MkdirTemp, and are private — which is what makes a temporary
			// file DACL-protected on Windows without the caller asking. Mode bits are a Unix subject.
			if runtime.GOOS != "windows" {
				info, statErr := tc.root.Stat(firstPath)
				if statErr != nil {
					t.Fatalf("Stat: %v", statErr)
				}
				if got := info.Mode().Perm(); got != 0o600 {
					t.Errorf("CreateTemp mode = %v, want 0600", got)
				}
			}

			tempDir, err := tc.root.MkdirTemp(under, "stage-*")
			if err != nil {
				t.Fatalf("MkdirTemp: %v", err)
			}
			info, err := tc.root.Stat(tempDir)
			if err != nil {
				t.Fatalf("Stat(MkdirTemp): %v", err)
			}
			if !info.IsDir() {
				t.Error("MkdirTemp did not create a directory")
			}
			if runtime.GOOS != "windows" {
				if got := info.Mode().Perm(); got != 0o700 {
					t.Errorf("MkdirTemp mode = %v, want 0700", got)
				}
			}
		})
	}
}

// TestCreateTemp_PatternWithSeparatorIsRefused proves the pattern cannot redirect the created object out of the
// directory argument — the one way a name could otherwise act as a path.
func TestCreateTemp_PatternWithSeparatorIsRefused(t *testing.T) {

	root := fsroot.OpenWritableUnconfined(t.TempDir())
	under := root.NewPath(".")

	if _, _, err := root.CreateTemp(under, "../escape-*"); !errors.Is(err, fs.ErrInvalid) {
		t.Errorf("CreateTemp with a separator = %v, want fs.ErrInvalid", err)
	}
	if _, err := root.MkdirTemp(under, "sub/escape-*"); !errors.Is(err, fs.ErrInvalid) {
		t.Errorf("MkdirTemp with a separator = %v, want fs.ErrInvalid", err)
	}
}

// TestCreateTemp_ScratchRootKeepsItsTempFilesInsideTheTree proves a scratch root's temp files are removed with it,
// which is the property that makes scratch the default home for transient bytes.
func TestCreateTemp_ScratchRootKeepsItsTempFilesInsideTheTree(t *testing.T) {

	scratch, err := fsroot.OpenScratch("fsroot-scratch-temp-*")
	if err != nil {
		t.Fatalf("fsroot.OpenScratch: %v", err)
	}

	file, created, err := scratch.CreateTemp(scratch.NewPath("."), "spool-*")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	absolute := created.Abs()
	if _, err := os.Stat(absolute); err != nil {
		t.Fatalf("temp file missing before Close: %v", err)
	}

	if err := scratch.Close(); err != nil {
		t.Fatalf("scratch Close: %v", err)
	}
	if _, err := os.Stat(absolute); !os.IsNotExist(err) {
		t.Errorf("Stat after scratch Close = %v, want the file gone with the tree", err)
	}
}

func TestParity_ReadOnlyRootRefusesEveryMutation(t *testing.T) {

	dir := t.TempDir()
	writeFixture(t, dir, "fixture.txt", "content")

	root := fsroot.OpenUnconfined(dir)
	p := root.NewPath("fixture.txt")

	if _, err := root.Create(p); !errors.Is(err, errors.ErrUnsupported) {
		t.Errorf("Create = %v, want ErrUnsupported", err)
	}
	if _, _, err := root.CreateTemp(root.NewPath("."), "temp-*"); !errors.Is(err, errors.ErrUnsupported) {
		t.Errorf("CreateTemp = %v, want ErrUnsupported", err)
	}

	for name, err := range map[string]error{
		"Chmod":     root.Chmod(p, 0o600),
		"Chown":     root.Chown(p, -1, -1),
		"Chtimes":   root.Chtimes(p, time.Now(), time.Now()),
		"Lchown":    root.Lchown(p, -1, -1),
		"Link":      root.Link(p, root.NewPath("link.txt")),
		"Mkdir":     root.Mkdir(root.NewPath("sub"), 0o750),
		"MkdirTemp": mkdirTempError(root),
		"RemoveAll": root.RemoveAll(p),
	} {
		if !errors.Is(err, errors.ErrUnsupported) {
			t.Errorf("%s = %v, want ErrUnsupported", name, err)
		}
	}

	// OpenRoot is a read, so it succeeds — and its sub-root inherits read-only.
	sub, err := root.OpenRoot(root.NewPath("."))
	if err != nil {
		t.Fatalf("OpenRoot: %v", err)
	}
	if err := sub.RemoveAll(sub.NewPath("fixture.txt")); !errors.Is(err, errors.ErrUnsupported) {
		t.Errorf("sub-root RemoveAll = %v, want ErrUnsupported (mode is inherited)", err)
	}
}

func TestOpenScratch_ClosesAndRemovesTheTree(t *testing.T) {

	scratch, err := fsroot.OpenScratch("fsroot-scratch-*")
	if err != nil {
		t.Fatalf("OpenScratch: %v", err)
	}

	dir := scratch.Name()
	if dir == "" {
		t.Fatal("Name() = empty; want the temporary directory")
	}

	if err := scratch.WriteFile(scratch.NewPath("spool.bin"), []byte("scratch"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := scratch.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// The contract that makes scratch worth having: Close removes the tree, not just the handle.
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("Stat(%s) after Close = %v; want the tree removed", dir, err)
	}
}

func TestOpenScratch_IsConfined(t *testing.T) {

	scratch, err := fsroot.OpenScratch("fsroot-scratch-*")
	if err != nil {
		t.Fatalf("OpenScratch: %v", err)
	}
	t.Cleanup(func() { _ = scratch.Close() })

	// Scratch is a confined root, so an escaping path is refused rather than followed.
	if err := scratch.WriteFile(scratch.NewPath("../escaped.txt"), []byte("x"), 0o600); err == nil {
		t.Error("WriteFile(../escaped.txt) = nil error; want the confinement refusal")
	}
}

// allRoots returns all three Root implementations rooted at dir. The confinedRoot is registered for cleanup.
func allRoots(t *testing.T, dir string) []rootCase {

	t.Helper()

	cr, err := fsroot.OpenConfined(dir)
	if err != nil {
		t.Fatalf("fsroot.OpenConfined: %v", err)
	}

	t.Cleanup(func() { _ = cr.Close() })

	return []rootCase{
		{"RootReader", fsroot.OpenUnconfined(dir)},
		{"RootReaderWriter", fsroot.OpenWritableUnconfined(dir)},
		{"confinedRoot", cr},
	}
}

// writableRoots returns Root implementations that support write operations.
// mkdirTempError returns only MkdirTemp's error, so the read-only refusal table can hold it beside the
// single-value mutations.
func mkdirTempError(root fsroot.Dir) error {

	_, err := root.MkdirTemp(root.NewPath("."), "temp-*")
	return err
}

func writableRoots(t *testing.T, dir string) []rootCase {

	t.Helper()
	cr, err := fsroot.OpenConfined(dir)
	if err != nil {
		t.Fatalf("fsroot.OpenConfined: %v", err)
	}
	t.Cleanup(func() { _ = cr.Close() })

	return []rootCase{
		{"RootReaderWriter", fsroot.OpenWritableUnconfined(dir)},
		{"confinedRoot", cr},
	}
}

// writeFixture creates a file under dir with the given content.
func writeFixture(t *testing.T, dir, name, content string) {

	t.Helper()
	absolute := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(absolute, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}
