// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Interface guards.
var (
	_ Root = (*confinedRoot)(nil)
	_ Root = (*unconfinedRootReader)(nil)
	_ Root = (*unconfinedRootReaderWriter)(nil)
)

// errReadOnly is returned by write operations on a [unconfinedRootReader].
var errReadOnly = errors.New("write operation not available in read-only mode")

// Root provides scoped filesystem operations. All path arguments are [Path] values created through [Root.NewPath].
//
// Three concrete implementations provide different access modes:
//
//   - [NewConfinedRoot] wraps [*os.Root] for OS-enforced confinement (execution)
//   - [NewRootReader] delegates to os.* for unconfined read-only access (planning)
//   - [NewRootReaderWriter] delegates to os.* for unconfined read-write access (testing)
type Root interface {
	Close() error
	FS() fs.FS
	Lstat(p Path) (fs.FileInfo, error)
	MkdirAll(p Path, perm os.FileMode) error
	Name() string
	NewPath(path string) Path
	Open(p Path) (*os.File, error)
	OpenFile(p Path, flag int, perm os.FileMode) (*os.File, error)
	ReadFile(p Path) ([]byte, error)
	Readlink(p Path) (string, error)
	Remove(p Path) error
	Rename(oldPath, newPath Path) error
	Stat(p Path) (fs.FileInfo, error)
	Symlink(target string, link Path) error
	WriteFile(p Path, data []byte, perm os.FileMode) error
}

// rootBase holds the root directory path shared by unconfined implementations.ap a
type rootBase struct {
	name string
}

// region EXPORTED METHODS

// region State management

// FS returns a [fs.FS] rooted at the base directory.
//
// Returns:
//   - `fs.FS`: a read-only filesystem view rooted at the base directory.
func (b *rootBase) FS() fs.FS { return os.DirFS(b.name) }

// Name returns the base directory path. Matches [os.Root.Name].
//
// Returns:
//   - `string`: the base directory path.
func (b *rootBase) Name() string { return b.name }

// endregion

// region Behaviors

// Close releases the root. Unconfined roots hold no OS handle, so this is a no-op.
//
// Returns:
//   - `error`: always nil.
func (b *rootBase) Close() error { return nil }

// NewPath builds a [Path] from an input path, resolved against the base directory.
//
// Parameters:
//   - `path`: the input path, absolute or relative to the base directory.
//
// Returns:
//   - `Path`: the constructed path with both rel and abs populated.
func (b *rootBase) NewPath(path string) Path { return makePath(b.name, path) }

// endregion

// endregion

// confinedRoot wraps [*os.Root] for OS-enforced confinement.
//
// All I/O is confined to the root directory by the kernel. Symlinks cannot escape, path traversal is blocked.
type confinedRoot struct {
	inner *os.Root
}

// NewConfinedRoot opens an OS-enforced confined [Root] at dir.
//
// Parameters:
//   - `dir`: the directory to confine all I/O within.
//
// Returns:
//   - `Root`: a confined root backed by [*os.Root].
//   - `error`: any error from [os.OpenRoot].
func NewConfinedRoot(dir string) (Root, error) {

	r, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	return &confinedRoot{inner: r}, nil
}

// region EXPORTED METHODS

// region State management

// FS returns a [fs.FS] rooted at the confined directory.
//
// Returns:
//   - `fs.FS`: a filesystem view rooted at the confined directory.
func (r *confinedRoot) FS() fs.FS { return r.inner.FS() }

// Name returns the confined root directory path. Matches [os.Root.Name].
//
// Returns:
//   - `string`: the confined root directory path.
func (r *confinedRoot) Name() string { return r.inner.Name() }

// endregion

// region Behaviors

// Close releases the underlying [*os.Root] handle.
//
// Returns:
//   - `error`: any error from [os.Root.Close].
func (r *confinedRoot) Close() error { return r.inner.Close() }

// Lstat returns file info for the path without following a terminal symlink, confined to the root.
//
// Parameters:
//   - `p`: the target path.
//
// Returns:
//   - `fs.FileInfo`: the file info.
//   - `error`: any error from [os.Root.Lstat].
func (r *confinedRoot) Lstat(p Path) (fs.FileInfo, error) { return r.inner.Lstat(p.rel) }

// MkdirAll creates the directory at the path along with any necessary parents, confined to the root.
//
// Parameters:
//   - `p`: the directory path.
//   - `perm`: the permission bits for created directories.
//
// Returns:
//   - `error`: any error from [os.Root.MkdirAll].
func (r *confinedRoot) MkdirAll(p Path, perm os.FileMode) error { return r.inner.MkdirAll(p.rel, perm) }

// NewPath builds a [Path] from an input path, resolved against the confined root directory.
//
// Parameters:
//   - `path`: the input path, absolute or relative to the root directory.
//
// Returns:
//   - `Path`: the constructed path with both rel and abs populated.
func (r *confinedRoot) NewPath(path string) Path { return makePath(r.inner.Name(), path) }

// Open opens the path for reading, confined to the root.
//
// Parameters:
//   - `p`: the target path.
//
// Returns:
//   - `*os.File`: the opened file.
//   - `error`: any error from [os.Root.Open].
func (r *confinedRoot) Open(p Path) (*os.File, error) { return r.inner.Open(p.rel) }

// OpenFile opens the path with the given flags and permissions, confined to the root.
//
// Parameters:
//   - `p`: the target path.
//   - `flag`: the [os.OpenFile] flags.
//   - `perm`: the permission bits applied on creation.
//
// Returns:
//   - `*os.File`: the opened file.
//   - `error`: any error from [os.Root.OpenFile].
func (r *confinedRoot) OpenFile(p Path, flag int, perm os.FileMode) (*os.File, error) {
	return r.inner.OpenFile(p.rel, flag, perm)
}

// ReadFile reads the entire contents of the path, confined to the root.
//
// Parameters:
//   - `p`: the target path.
//
// Returns:
//   - `[]byte`: the file contents.
//   - `error`: any error from [os.Root.ReadFile].
func (r *confinedRoot) ReadFile(p Path) ([]byte, error) { return r.inner.ReadFile(p.rel) }

// Readlink returns the destination of the symbolic link at the path, confined to the root.
//
// Parameters:
//   - `p`: the symlink path.
//
// Returns:
//   - `string`: the link destination.
//   - `error`: any error from [os.Root.Readlink].
func (r *confinedRoot) Readlink(p Path) (string, error) { return r.inner.Readlink(p.rel) }

// Remove deletes the file or empty directory at the path, confined to the root.
//
// Parameters:
//   - `p`: the target path.
//
// Returns:
//   - `error`: any error from [os.Root.Remove].
func (r *confinedRoot) Remove(p Path) error { return r.inner.Remove(p.rel) }

// Rename moves oldPath to newPath, confined to the root.
//
// Parameters:
//   - `oldPath`: the source path.
//   - `newPath`: the destination path.
//
// Returns:
//   - `error`: any error from [os.Root.Rename].
func (r *confinedRoot) Rename(oldPath, newPath Path) error {
	return r.inner.Rename(oldPath.rel, newPath.rel)
}

// Stat returns file info for the path, following symlinks, confined to the root.
//
// Parameters:
//   - `p`: the target path.
//
// Returns:
//   - `fs.FileInfo`: the file info.
//   - `error`: any error from [os.Root.Stat].
func (r *confinedRoot) Stat(p Path) (fs.FileInfo, error) { return r.inner.Stat(p.rel) }

// Symlink creates a symbolic link at link pointing to target, confined to the root.
//
// Parameters:
//   - `target`: the link destination.
//   - `link`: the path at which to create the link.
//
// Returns:
//   - `error`: any error from [os.Root.Symlink].
func (r *confinedRoot) Symlink(target string, link Path) error {
	return r.inner.Symlink(target, link.rel)
}

// WriteFile writes data to the path, creating or truncating it, confined to the root.
//
// Parameters:
//   - `p`: the target path.
//   - `data`: the bytes to write.
//   - `perm`: the permission bits applied on creation.
//
// Returns:
//   - `error`: any error from [os.Root.WriteFile].
func (r *confinedRoot) WriteFile(p Path, data []byte, perm os.FileMode) error {
	return r.inner.WriteFile(p.rel, data, perm)
}

// endregion

// endregion

// unconfinedRootReader provides unconfined, read-only filesystem access.
//
// Write operations return [errReadOnly]. Used during planning when providers need to inspect source files without
// mutation capability.
type unconfinedRootReader struct {
	rootBase
}

// NewRootReader creates a read-only [Root] at dir. Write operations return [errReadOnly].
//
// Parameters:
//   - `dir`: the base directory for all path resolution.
//
// Returns:
//   - `Root`: a read-only, unconfined root.
func NewRootReader(dir string) Root {
	return &unconfinedRootReader{rootBase{name: dir}}
}

// region EXPORTED METHODS

// region Behaviors

// Lstat returns file info for the path without following a terminal symlink.
//
// Parameters:
//   - `p`: the target path.
//
// Returns:
//   - `fs.FileInfo`: the file info.
//   - `error`: any error from [os.Lstat].
func (r *unconfinedRootReader) Lstat(p Path) (fs.FileInfo, error) { return os.Lstat(p.abs) }

// MkdirAll is unavailable in read-only mode.
//
// Returns:
//   - `error`: always [errReadOnly].
func (r *unconfinedRootReader) MkdirAll(Path, os.FileMode) error { return errReadOnly }

// Open opens the path for reading.
//
// Parameters:
//   - `p`: the target path.
//
// Returns:
//   - `*os.File`: the opened file.
//   - `error`: any error from [os.Open].
func (r *unconfinedRootReader) Open(p Path) (*os.File, error) { return os.Open(p.abs) }

// OpenFile is unavailable in read-only mode.
//
// Returns:
//   - `*os.File`: always nil.
//   - `error`: always [errReadOnly].
func (r *unconfinedRootReader) OpenFile(Path, int, os.FileMode) (*os.File, error) {
	return nil, errReadOnly
}

// ReadFile reads the entire contents of the path.
//
// Parameters:
//   - `p`: the target path.
//
// Returns:
//   - `[]byte`: the file contents.
//   - `error`: any error from [os.ReadFile].
func (r *unconfinedRootReader) ReadFile(p Path) ([]byte, error) { return os.ReadFile(p.abs) }

// Readlink returns the destination of the symbolic link at the path.
//
// Parameters:
//   - `p`: the symlink path.
//
// Returns:
//   - `string`: the link destination.
//   - `error`: any error from [os.Readlink].
func (r *unconfinedRootReader) Readlink(p Path) (string, error) { return os.Readlink(p.abs) }

// Remove is unavailable in read-only mode.
//
// Returns:
//   - `error`: always [errReadOnly].
func (r *unconfinedRootReader) Remove(Path) error { return errReadOnly }

// Rename is unavailable in read-only mode.
//
// Returns:
//   - `error`: always [errReadOnly].
func (r *unconfinedRootReader) Rename(_, _ Path) error { return errReadOnly }

// Stat returns file info for the path, following symlinks.
//
// Parameters:
//   - `p`: the target path.
//
// Returns:
//   - `fs.FileInfo`: the file info.
//   - `error`: any error from [os.Stat].
func (r *unconfinedRootReader) Stat(p Path) (fs.FileInfo, error) { return os.Stat(p.abs) }

// Symlink is unavailable in read-only mode.
//
// Returns:
//   - `error`: always [errReadOnly].
func (r *unconfinedRootReader) Symlink(_ string, _ Path) error { return errReadOnly }

// WriteFile is unavailable in read-only mode.
//
// Returns:
//   - `error`: always [errReadOnly].
func (r *unconfinedRootReader) WriteFile(Path, []byte, os.FileMode) error { return errReadOnly }

// endregion

// endregion

// unconfinedRootReaderWriter provides unconfined, read-write filesystem access.
//
// Reads are inherited from [unconfinedRootReader]. Write operations delegate to os.* without OS-level confinement.
type unconfinedRootReaderWriter struct {
	unconfinedRootReader
}

// NewRootReaderWriter creates a read-write [Root] at dir without OS-level confinement.
//
// Parameters:
//   - `dir`: the base directory for all path resolution.
//
// Returns:
//   - `Root`: a read-write, unconfined root.
func NewRootReaderWriter(dir string) Root {
	return &unconfinedRootReaderWriter{unconfinedRootReader{rootBase{name: dir}}}
}

// region EXPORTED METHODS

// region Behaviors

// MkdirAll creates the directory at the path along with any necessary parents.
//
// Parameters:
//   - `p`: the directory path.
//   - `perm`: the permission bits for created directories.
//
// Returns:
//   - `error`: any error from [os.MkdirAll].
func (r *unconfinedRootReaderWriter) MkdirAll(p Path, perm os.FileMode) error {
	return os.MkdirAll(p.abs, perm)
}

// OpenFile opens the path with the given flags and permissions.
//
// Parameters:
//   - `p`: the target path.
//   - `flag`: the [os.OpenFile] flags.
//   - `perm`: the permission bits applied on creation.
//
// Returns:
//   - `*os.File`: the opened file.
//   - `error`: any error from [os.OpenFile].
func (r *unconfinedRootReaderWriter) OpenFile(p Path, flag int, perm os.FileMode) (*os.File, error) {
	return os.OpenFile(p.abs, flag, perm)
}

// Remove deletes the file or empty directory at the path.
//
// Parameters:
//   - `p`: the target path.
//
// Returns:
//   - `error`: any error from [os.Remove].
func (r *unconfinedRootReaderWriter) Remove(p Path) error { return os.Remove(p.abs) }

// Rename moves oldPath to newPath.
//
// Parameters:
//   - `oldPath`: the source path.
//   - `newPath`: the destination path.
//
// Returns:
//   - `error`: any error from [os.Rename].
func (r *unconfinedRootReaderWriter) Rename(oldPath, newPath Path) error {
	return os.Rename(oldPath.abs, newPath.abs)
}

// Symlink creates a symbolic link at link pointing to target.
//
// Parameters:
//   - `target`: the link destination.
//   - `link`: the path at which to create the link.
//
// Returns:
//   - `error`: any error from [os.Symlink].
func (r *unconfinedRootReaderWriter) Symlink(target string, link Path) error {
	return os.Symlink(target, link.abs)
}

// WriteFile writes data to the path, creating or truncating it.
//
// Parameters:
//   - `p`: the target path.
//   - `data`: the bytes to write.
//   - `perm`: the permission bits applied on creation.
//
// Returns:
//   - `error`: any error from [os.WriteFile].
func (r *unconfinedRootReaderWriter) WriteFile(p Path, data []byte, perm os.FileMode) error {
	return os.WriteFile(p.abs, data, perm)
}

// endregion

// endregion

// region SUPPORTING TYPES

// Path holds both root-relative and absolute forms of a filesystem path.
//
// Created through [Root.NewPath] to guarantee both fields are populated. The root field records which root directory
// Rel is relative to (matching [os.Root.Name]). Abs is derived as filepath.Join(root, rel) and is not serialized.
//
// noinspection GoMixedReceiverTypes
type Path struct {
	root string
	rel  string
	abs  string // derived: filepath.Join(root, rel)
}

// NewPath creates a [Path] from a root directory and a root-relative path.
//
// Abs is derived via [filepath.Join]. Intended for tests and deserialization.
//
// Parameters:
//   - `root`: the root directory that `rel` is relative to (matches [os.Root.Name]).
//   - `rel`: the root-relative path.
//
// Returns:
//   - `Path`: the constructed path.
func NewPath(root, rel string) Path {
	return Path{root: root, rel: rel, abs: filepath.Join(root, rel)}
}

// region EXPORTED METHODS

// region State management

// Abs returns the absolute path used for unconfined I/O, URIs, display, and logging.
//
// Returns:
//   - `string`: the absolute path.
func (p Path) Abs() string { return p.abs }

// Rel returns the root-relative path used for confined I/O.
//
// Returns:
//   - `string`: the root-relative path.
func (p Path) Rel() string { return p.rel }

// Root returns the root directory path that Rel is relative to. Matches [os.Root.Name].
//
// Returns:
//   - `string`: the root directory path.
func (p Path) Root() string { return p.root }

// String returns the absolute path.
//
// Returns:
//   - `string`: the absolute path.
func (p Path) String() string { return p.abs }

// endregion

// region Behaviors

// MarshalJSON serializes the canonical form {root, rel}. Abs is derived on deserialization.
//
// Returns:
//   - `[]byte`: the JSON encoding of the {root, rel} form.
//   - `error`: any error returned by [json.Marshal].
func (p Path) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Root string `json:"root"`
		Rel  string `json:"rel"`
	}{
		Root: p.root,
		Rel:  p.rel,
	})
}

// MarshalYAML serializes the canonical form {root, rel}. Abs is derived on deserialization.
//
// Returns:
//   - `any`: the {root, rel} form for the YAML encoder to serialize.
//   - `error`: always nil; present to satisfy the [yaml.Marshaler] interface.
func (p Path) MarshalYAML() (any, error) {
	return struct {
		Root string `yaml:"root"`
		Rel  string `yaml:"rel"`
	}{
		Root: p.root,
		Rel:  p.rel,
	}, nil
}

// UnmarshalJSON deserializes {root, rel} and derives Abs.
//
// Pointer receiver is required by the [json.Unmarshaler] contract — the method must mutate the receiver to populate
// fields from the JSON bytes. All other Path methods use value receivers since Path is an immutable value type.
//
// Implementation note: A pointer receiver is required by the [json.Unmarshaler] contract. The method mutates the
// receiver in place to populate `root`, `rel`, and `abs` from the encoded document, so a value receiver would fill a
// discarded copy.
//
// This is the deliberate exception to Path's value-receiver convention. The getters and the Marshal methods use value
// receivers so the value type [Path] — not just [*Path] — satisfies [json.Marshaler] / [yaml.Marshaler] and its
// accessors stay callable on non-addressable values (map elements, function returns). Unmarshaling always targets an
// addressable variable (json.Unmarshal(data, &p)), so the pointer receiver is safe. The resulting value/pointer mix is
// intentional, which is why the mixed-receivers inspection is suppressed.
//
// Parameters:
//   - `data`: the JSON bytes to decode.
//
// Returns:
//   - `error`: non-nil if the JSON is malformed.
func (p *Path) UnmarshalJSON(data []byte) error {

	var decoded struct {
		Root string `json:"root"`
		Rel  string `json:"rel"`
	}

	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}

	p.root = decoded.Root
	p.rel = decoded.Rel
	p.abs = filepath.Join(decoded.Root, decoded.Rel)

	return nil
}

// UnmarshalYAML deserializes {root, rel} and derives Abs.
//
// Implementation note: A pointer receiver is required by the [json.Unmarshaler] contract. The method mutates the
// receiver in place to populate `root`, `rel`, and `abs` from the encoded document, so a value receiver would fill a
// discarded copy.
//
// This is the deliberate exception to Path's value-receiver convention. The getters and the Marshal methods use value
// receivers so the value type [Path] — not just [*Path] — satisfies [json.Marshaler] / [yaml.Marshaler] and its
// accessors stay callable on non-addressable values (map elements, function returns). Unmarshaling always targets an
// addressable variable (json.Unmarshal(data, &p)), so the pointer receiver is safe. The resulting value/pointer mix is
// intentional, which is why the mixed-receivers inspection is suppressed.
//
// Parameters:
//   - `value`: the YAML node to decode.
//
// Returns:
//   - `error`: non-nil if the YAML is malformed.
func (p *Path) UnmarshalYAML(value *yaml.Node) error {

	var decoded struct {
		Root string `yaml:"root"`
		Rel  string `yaml:"rel"`
	}

	if err := value.Decode(&decoded); err != nil {
		return err
	}

	p.root = decoded.Root
	p.rel = decoded.Rel
	p.abs = filepath.Join(decoded.Root, decoded.Rel)

	return nil
}

// endregion

// endregion

// endregion

// region HELPERS

// makePath computes a [Path] from a root directory name and an input path.
//
// Absolute inputs compute Rel via [filepath.Rel] (may contain ../ prefixes for paths outside root — valid in
// unconfined mode, rejected by [*os.Root] in confined mode). Relative inputs compute Abs via [filepath.Join].
//
// Parameters:
//   - `rootName`: the root directory path.
//   - `path`: the input path, absolute or relative.
//
// Returns:
//   - `Path`: the constructed path with both rel and abs populated.
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

// endregion
