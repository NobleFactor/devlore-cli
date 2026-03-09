// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// Interface guards.
var (
	_ Root = (*RootReader)(nil)
	_ Root = (*RootReaderWriter)(nil)
	_ Root = (*confinedRoot)(nil)
)

// ErrReadOnly is returned by write operations on a [RootReader].
var ErrReadOnly = errors.New("write operation not available in read-only mode")

// ── Path ─────────────────────────────────────────────────────────────────────────────────────────────────────────────

// Path holds both root-relative and absolute forms of a filesystem path. Created through [Root.NewPath] to guarantee
// both fields are populated. The root field records which root directory Rel is relative to (matching [os.Root.Name]).
// Abs is derived as filepath.Join(root, rel) and is not serialized.
type Path struct {
	root string
	rel  string
	abs  string // derived: filepath.Join(root, rel)
}

// NewPath creates a Path from a root directory and a root-relative path. Abs is derived. Intended for tests and
// deserialization.
//
// Parameters:
//   - root: Root directory that rel is relative to (matches [os.Root.Name])
//   - rel: Root-relative path
//
// Returns:
//   - Path: the constructed path
func NewPath(root, rel string) Path {
	return Path{root: root, rel: rel, abs: filepath.Join(root, rel)}
}

// Abs returns the absolute path used for unconfined I/O, URIs, display, and logging.
func (p Path) Abs() string { return p.abs }

// Rel returns the root-relative path used for confined I/O.
func (p Path) Rel() string { return p.rel }

// Root returns the root directory path that Rel is relative to. Matches [os.Root.Name].
func (p Path) Root() string { return p.root }

// String returns the absolute path.
func (p Path) String() string { return p.abs }

// MarshalJSON serializes the canonical form {root, rel}. Abs is derived on deserialization.
func (p Path) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Root string `json:"root"`
		Rel  string `json:"rel"`
	}{
		Root: p.root,
		Rel:  p.rel,
	})
}

// UnmarshalJSON deserializes {root, rel} and derives Abs.
func (p *Path) UnmarshalJSON(data []byte) error {

	var wire struct {
		Root string `json:"root"`
		Rel  string `json:"rel"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	p.root = wire.Root
	p.rel = wire.Rel
	p.abs = filepath.Join(wire.Root, wire.Rel)
	return nil
}

// ── Root ─────────────────────────────────────────────────────────────────────────────────────────────────────────────

// Root provides scoped filesystem operations. All path arguments are [Path] values created through [Root.NewPath].
// Three concrete implementations provide different access modes:
//
//   - [NewConfinedRoot] wraps [*os.Root] for OS-enforced confinement (execution)
//   - [NewRootReader] delegates to os.* for unconfined read-only access (planning)
//   - [NewRootReaderWriter] delegates to os.* for unconfined read-write access (testing)
type Root interface {

	// Read

	Stat(p Path) (fs.FileInfo, error)
	Lstat(p Path) (fs.FileInfo, error)
	Open(p Path) (*os.File, error)
	ReadFile(p Path) ([]byte, error)
	Readlink(p Path) (string, error)
	FS() fs.FS

	// Write

	OpenFile(p Path, flag int, perm os.FileMode) (*os.File, error)
	WriteFile(p Path, data []byte, perm os.FileMode) error
	MkdirAll(p Path, perm os.FileMode) error
	Remove(p Path) error
	Rename(oldPath, newPath Path) error
	Symlink(target string, link Path) error

	// Factory

	NewPath(path string) Path

	// Lifecycle

	Name() string
	Close() error
}

// ── rootBase ─────────────────────────────────────────────────────────────────────────────────────────────────────────

// rootBase holds the root directory path shared by unconfined implementations.
type rootBase struct {
	name string
}

func (b *rootBase) Name() string             { return b.name }
func (b *rootBase) Close() error             { return nil }
func (b *rootBase) FS() fs.FS                { return os.DirFS(b.name) }
func (b *rootBase) NewPath(path string) Path { return makePath(b.name, path) }

// ── RootReader ───────────────────────────────────────────────────────────────────────────────────────────────────────

// RootReader provides unconfined, read-only filesystem access. Write operations return [ErrReadOnly]. Used during
// planning when providers need to inspect source files without mutation capability.
type RootReader struct {
	rootBase
}

// NewRootReader creates a read-only [Root] at dir. Write operations return [ErrReadOnly].
//
// Parameters:
//   - dir: Base directory for all path resolution
//
// Returns:
//   - Root: read-only, unconfined root
func NewRootReader(dir string) Root {
	return &RootReader{rootBase{name: dir}}
}

// region EXPORTED METHODS

// region Behaviors

// Read

func (r *RootReader) Lstat(p Path) (fs.FileInfo, error) { return os.Lstat(p.abs) }
func (r *RootReader) Open(p Path) (*os.File, error)     { return os.Open(p.abs) }
func (r *RootReader) ReadFile(p Path) ([]byte, error)   { return os.ReadFile(p.abs) }
func (r *RootReader) Readlink(p Path) (string, error)   { return os.Readlink(p.abs) }
func (r *RootReader) Stat(p Path) (fs.FileInfo, error)  { return os.Stat(p.abs) }

// Write — return ErrReadOnly.

func (r *RootReader) MkdirAll(Path, os.FileMode) error          { return ErrReadOnly }
func (r *RootReader) Remove(Path) error                         { return ErrReadOnly }
func (r *RootReader) Rename(_, _ Path) error                    { return ErrReadOnly }
func (r *RootReader) Symlink(_ string, _ Path) error            { return ErrReadOnly }
func (r *RootReader) WriteFile(Path, []byte, os.FileMode) error { return ErrReadOnly }

func (r *RootReader) OpenFile(Path, int, os.FileMode) (*os.File, error) {
	return nil, ErrReadOnly
}

// endregion

// endregion

// ── RootReaderWriter ─────────────────────────────────────────────────────────────────────────────────────────────────

// RootReaderWriter provides unconfined, read-write filesystem access. Reads are inherited from [RootReader]. Write
// operations delegate to os.* without OS-level confinement.
type RootReaderWriter struct {
	RootReader
}

// NewRootReaderWriter creates a read-write [Root] at dir without OS-level confinement.
//
// Parameters:
//   - dir: Base directory for all path resolution
//
// Returns:
//   - Root: read-write, unconfined root
func NewRootReaderWriter(dir string) Root {
	return &RootReaderWriter{RootReader{rootBase{name: dir}}}
}

// region EXPORTED METHODS

// region Behaviors

// Write — override RootReader stubs with os.* delegates.

func (r *RootReaderWriter) MkdirAll(p Path, perm os.FileMode) error { return os.MkdirAll(p.abs, perm) }
func (r *RootReaderWriter) Remove(p Path) error                     { return os.Remove(p.abs) }

func (r *RootReaderWriter) OpenFile(p Path, flag int, perm os.FileMode) (*os.File, error) {
	return os.OpenFile(p.abs, flag, perm)
}

func (r *RootReaderWriter) Rename(oldPath, newPath Path) error {
	return os.Rename(oldPath.abs, newPath.abs)
}

func (r *RootReaderWriter) Symlink(target string, link Path) error {
	return os.Symlink(target, link.abs)
}

func (r *RootReaderWriter) WriteFile(p Path, data []byte, perm os.FileMode) error {
	return os.WriteFile(p.abs, data, perm)
}

// endregion

// endregion

// ── confinedRoot ─────────────────────────────────────────────────────────────────────────────────────────────────────

// confinedRoot wraps [*os.Root] for OS-enforced confinement. All I/O is confined to the root directory by the kernel.
// Symlinks cannot escape, path traversal is blocked.
type confinedRoot struct {
	inner *os.Root
}

// NewConfinedRoot opens an OS-enforced confined [Root] at dir.
//
// Parameters:
//   - dir: Directory to confine all I/O within
//
// Returns:
//   - Root: confined root backed by [*os.Root]
//   - error: any error from [os.OpenRoot]
func NewConfinedRoot(dir string) (Root, error) {

	r, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	return &confinedRoot{inner: r}, nil
}

func (r *confinedRoot) Name() string             { return r.inner.Name() }
func (r *confinedRoot) Close() error             { return r.inner.Close() }
func (r *confinedRoot) FS() fs.FS                { return r.inner.FS() }
func (r *confinedRoot) NewPath(path string) Path { return makePath(r.inner.Name(), path) }

// Read — delegate to os.Root using p.rel.

func (r *confinedRoot) Lstat(p Path) (fs.FileInfo, error) { return r.inner.Lstat(p.rel) }
func (r *confinedRoot) Open(p Path) (*os.File, error)     { return r.inner.Open(p.rel) }
func (r *confinedRoot) ReadFile(p Path) ([]byte, error)   { return r.inner.ReadFile(p.rel) }
func (r *confinedRoot) Readlink(p Path) (string, error)   { return r.inner.Readlink(p.rel) }
func (r *confinedRoot) Stat(p Path) (fs.FileInfo, error)  { return r.inner.Stat(p.rel) }

// Write — delegate to os.Root using p.rel.

func (r *confinedRoot) MkdirAll(p Path, perm os.FileMode) error { return r.inner.MkdirAll(p.rel, perm) }
func (r *confinedRoot) Remove(p Path) error                     { return r.inner.Remove(p.rel) }

func (r *confinedRoot) OpenFile(p Path, flag int, perm os.FileMode) (*os.File, error) {
	return r.inner.OpenFile(p.rel, flag, perm)
}

func (r *confinedRoot) Rename(oldPath, newPath Path) error {
	return r.inner.Rename(oldPath.rel, newPath.rel)
}

func (r *confinedRoot) Symlink(target string, link Path) error {
	return r.inner.Symlink(target, link.rel)
}

func (r *confinedRoot) WriteFile(p Path, data []byte, perm os.FileMode) error {
	return r.inner.WriteFile(p.rel, data, perm)
}

// ── helpers ──────────────────────────────────────────────────────────────────────────────────────────────────────────

// makePath computes a [Path] from a root directory name and an input path. Absolute inputs compute Rel via
// [filepath.Rel] (may contain ../ prefixes for paths outside root — valid in unconfined mode, rejected by [*os.Root]
// in confined mode). Relative inputs compute Abs via [filepath.Join].
//
// Parameters:
//   - rootName: Root directory path
//   - path: Input path (absolute or relative)
//
// Returns:
//   - Path: the constructed path with both rel and abs populated
func makePath(rootName, path string) Path {

	if filepath.IsAbs(path) {
		rel, _ := filepath.Rel(rootName, path)
		return Path{root: rootName, rel: rel, abs: filepath.Clean(path)}
	}
	return Path{
		root: rootName,
		rel:  filepath.Clean(path),
		abs:  filepath.Join(rootName, path),
	}
}
