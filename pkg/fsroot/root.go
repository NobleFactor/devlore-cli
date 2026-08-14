// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

// Package fsroot provides scoped filesystem roots.
//
// All provider I/O flows through the [Root] interface, confining reads and writes to a directory
// tree. Three implementations serve the three lifecycles: confined roots for execution, read-only
// roots for planning, and writable unconfined roots for tests.
package fsroot

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/NobleFactor/devlore-cli/pkg/assert"
)

// Interface guards.
var (
	_ Root = (*confinedRoot)(nil)
	_ Root = (*scratchRoot)(nil)
	_ Root = (*unconfinedRootReader)(nil)
	_ Root = (*unconfinedRootReaderWriter)(nil)
)

// errReadOnly is returned by write operations on a [unconfinedRootReader]. It wraps the standard
// [errors.ErrUnsupported] sentinel so callers can test for it with `errors.Is(err, errors.ErrUnsupported)` without
// depending on this package.
var errReadOnly = fmt.Errorf("write operation not available in read-only mode: %w", errors.ErrUnsupported)

// Root provides scoped filesystem operations. All path arguments are [Path] values created through [Root.NewPath].
//
// Three concrete implementations provide different access modes:
//
//   - [OpenConfined] wraps [*os.Root] for OS-enforced confinement (execution)
//   - [OpenUnconfined] delegates to os.* for unconfined read-only access (planning)
//   - [OpenWritableUnconfined] delegates to os.* for unconfined read-write access (testing)
//
// [Open] constructs any of the three from a [Mode] value; [OpenScratch] constructs a confined root
// at a temporary directory that removes itself on [Root.Close].
//
// The method set mirrors [*os.Root] in full, so code that knows the standard library's root knows
// this one. Every filesystem mutation in the repository is expected to flow through this interface;
// a direct os.* call must carry a `// Confinement:` comment stating why the root cannot serve it.
type Root interface {
	Chmod(p Path, mode os.FileMode) error
	Chown(p Path, uid, gid int) error
	Chtimes(p Path, atime, mtime time.Time) error
	Close() error
	Create(p Path) (*os.File, error)
	FS() fs.FS
	Lchown(p Path, uid, gid int) error
	Link(oldPath, newPath Path) error
	Lstat(p Path) (fs.FileInfo, error)
	Mkdir(p Path, perm os.FileMode) error
	MkdirAll(p Path, perm os.FileMode) error
	Name() string
	NewPath(path string) Path
	Open(p Path) (*os.File, error)
	OpenFile(p Path, flag int, perm os.FileMode) (*os.File, error)
	OpenRoot(p Path) (Root, error)
	ReadFile(p Path) ([]byte, error)
	Readlink(p Path) (string, error)
	Remove(p Path) error
	RemoveAll(p Path) error
	Rename(oldPath, newPath Path) error
	Stat(p Path) (fs.FileInfo, error)
	Symlink(target string, link Path) error
	WriteFile(p Path, data []byte, perm os.FileMode) error
}

// Mode selects which [Root] implementation [Open] constructs.
//
// The zero value is [ModeConfined], so a caller that never sets a mode gets OS-enforced confinement
// rather than silently widened access.
type Mode int

const (

	// ModeConfined selects the OS-enforced confined implementation ([OpenConfined]).
	ModeConfined Mode = iota

	// ModeUnconfined selects the unconfined read-only implementation ([OpenUnconfined]).
	ModeUnconfined

	// ModeWritableUnconfined selects the unconfined read-write implementation ([OpenWritableUnconfined]).
	ModeWritableUnconfined
)

// Open opens a [Root] at dir in the given [Mode].
//
// The single mode-dispatched constructor. [ModeConfined] routes to [OpenConfined] — the only branch
// that can fail — while the unconfined modes construct handle-free roots that cannot.
//
// Parameters:
//   - `dir`: the directory to anchor the root at.
//   - `mode`: the [Mode] selecting the implementation.
//
// Returns:
//   - `Root`: the constructed root.
//   - `error`: any error from [OpenConfined] when mode is [ModeConfined]; nil for the unconfined modes.
func Open(dir string, mode Mode) (Root, error) {

	switch mode {

	case ModeConfined:
		return OpenConfined(dir)

	case ModeUnconfined:
		return OpenUnconfined(dir), nil

	case ModeWritableUnconfined:
		return OpenWritableUnconfined(dir), nil

	default:
		assert.Unreachablef("fsroot.Open: unknown mode %d", mode)
		return nil, nil
	}
}

// OpenScratch opens a confined [Root] at a newly created temporary directory that removes itself.
//
// Scratch is not an escape from confinement — it is its own confined tree with a self-destroying
// lifetime. Process scratch (spool files, staging trees) belongs here rather than in a direct
// [os.CreateTemp] call, so that scratch I/O flows through the same seam as every other mutation and
// inherits its platform behavior.
//
// IMPORTANT: [Root.Close] on a scratch root does two things — it releases the handle AND removes
// the directory tree. Closing early destroys the contents. This overload is deliberate: it makes
// cleanup impossible to forget, which a separate discard method would not.
//
// Parameters:
//   - `pattern`: the [os.MkdirTemp] name pattern; a `*` in the pattern is replaced by a random
//     string, otherwise the random string is appended.
//
// Returns:
//   - `Root`: a confined root anchored at the new temporary directory.
//   - `error`: any error from [os.MkdirTemp] or [OpenConfined]. On an [OpenConfined] failure the
//     temporary directory is removed before returning, so no tree is orphaned.
func OpenScratch(pattern string) (Root, error) {

	dir, err := os.MkdirTemp("", pattern)
	if err != nil {
		return nil, fmt.Errorf("fsroot.OpenScratch: create temporary directory: %w", err)
	}

	confined, err := OpenConfined(dir)
	if err != nil {
		return nil, errors.Join(
			fmt.Errorf("fsroot.OpenScratch: confine %s: %w", dir, err), os.RemoveAll(dir))
	}

	return &scratchRoot{Root: confined, dir: dir}, nil
}

// rootBase holds the root directory path shared by all implementations.
type rootBase struct {
	name string
}

// scratchRoot is a confined [Root] over a temporary directory it owns and removes on Close.
//
// Every method other than Close is the embedded confined root's; only the lifetime differs.
type scratchRoot struct {
	Root

	// dir is the temporary directory this root owns, removed by Close.
	dir string
}

// region EXPORTED METHODS

// region Behaviors

// Close releases the confined root's handle and removes the temporary directory tree.
//
// Both run even when the first fails, and their errors are joined — a failed handle close must not
// leave the tree behind, and a failed removal must still be reported. On Windows the removal cannot
// succeed while any file inside the tree is still open, so a leaked file handle surfaces here as a
// removal error rather than as a silent orphan.
//
// Returns:
//   - `error`: the joined close and removal errors, or nil.
func (r *scratchRoot) Close() error { return errors.Join(r.Root.Close(), os.RemoveAll(r.dir)) }

// endregion

// endregion

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

// OpenConfined opens an OS-enforced confined [Root] at dir.
//
// Parameters:
//   - `dir`: the directory to confine all I/O within.
//
// Returns:
//   - `Root`: a confined root backed by [*os.Root].
//   - `error`: any error from [os.OpenRoot].
func OpenConfined(dir string) (Root, error) {

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

// Chmod changes the mode of the path, confined to the root.
//
// Parameters:
//   - `p`: the target path.
//   - `mode`: the permission bits to apply.
//
// Returns:
//   - `error`: any error from [os.Root.Chmod].
func (r *confinedRoot) Chmod(p Path, mode os.FileMode) error { return r.inner.Chmod(p.rel, mode) }

// Chown changes the numeric owner and group of the path, confined to the root.
//
// Unsupported on Windows, where the underlying syscall always fails; that is [os]'s behavior and is
// surfaced rather than masked.
//
// Parameters:
//   - `p`: the target path.
//   - `uid`: the owner id, or -1 to leave unchanged.
//   - `gid`: the group id, or -1 to leave unchanged.
//
// Returns:
//   - `error`: any error from [os.Root.Chown].
func (r *confinedRoot) Chown(p Path, uid, gid int) error { return r.inner.Chown(p.rel, uid, gid) }

// Chtimes changes the access and modification times of the path, confined to the root.
//
// Parameters:
//   - `p`: the target path.
//   - `atime`: the access time to set.
//   - `mtime`: the modification time to set.
//
// Returns:
//   - `error`: any error from [os.Root.Chtimes].
func (r *confinedRoot) Chtimes(p Path, atime, mtime time.Time) error {
	return r.inner.Chtimes(p.rel, atime, mtime)
}

// Close releases the underlying [*os.Root] handle.
//
// Returns:
//   - `error`: any error from [os.Root.Close].
func (r *confinedRoot) Close() error { return r.inner.Close() }

// Create creates or truncates the path for reading and writing, confined to the root.
//
// The created file carries mode 0o666 before umask — an unrestricted file by definition. Never use
// Create for sensitive content; use [confinedRoot.OpenFile] with an explicit restrictive mode.
//
// Parameters:
//   - `p`: the target path.
//
// Returns:
//   - `*os.File`: the created file, opened read-write.
//   - `error`: any error from [os.Root.Create].
func (r *confinedRoot) Create(p Path) (*os.File, error) { return r.inner.Create(p.rel) }

// Lchown changes owner and group without following a terminal symlink, confined to the root.
//
// Unsupported on Windows, where the underlying syscall always fails.
//
// Parameters:
//   - `p`: the target path.
//   - `uid`: the owner id, or -1 to leave unchanged.
//   - `gid`: the group id, or -1 to leave unchanged.
//
// Returns:
//   - `error`: any error from [os.Root.Lchown].
func (r *confinedRoot) Lchown(p Path, uid, gid int) error { return r.inner.Lchown(p.rel, uid, gid) }

// Link creates a hard link at newPath pointing to oldPath, confined to the root.
//
// Both endpoints are [Path] values because a hard link, unlike a symlink, must resolve inside the
// root at creation time.
//
// Parameters:
//   - `oldPath`: the existing file the link points to.
//   - `newPath`: the path at which to create the link.
//
// Returns:
//   - `error`: any error from [os.Root.Link].
func (r *confinedRoot) Link(oldPath, newPath Path) error {
	return r.inner.Link(oldPath.rel, newPath.rel)
}

// Lstat returns file info for the path without following a terminal symlink, confined to the root.
//
// Parameters:
//   - `p`: the target path.
//
// Returns:
//   - `fs.FileInfo`: the file info.
//   - `error`: any error from [os.Root.Lstat].
func (r *confinedRoot) Lstat(p Path) (fs.FileInfo, error) { return r.inner.Lstat(p.rel) }

// Mkdir creates the directory at the path, confined to the root.
//
// Parents must already exist; use [confinedRoot.MkdirAll] otherwise.
//
// Parameters:
//   - `p`: the directory path.
//   - `perm`: the permission bits for the created directory.
//
// Returns:
//   - `error`: any error from [os.Root.Mkdir].
func (r *confinedRoot) Mkdir(p Path, perm os.FileMode) error { return r.inner.Mkdir(p.rel, perm) }

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

// OpenRoot opens the directory at the path as its own [Root], confined to this root.
//
// The sub-root inherits this root's access mode: a confined root yields a confined sub-root. The
// caller owns the returned root and must Close it.
//
// Parameters:
//   - `p`: the directory to open as a root.
//
// Returns:
//   - `Root`: the sub-root, confined to `p`.
//   - `error`: any error from [os.Root.OpenRoot].
func (r *confinedRoot) OpenRoot(p Path) (Root, error) {

	inner, err := r.inner.OpenRoot(p.rel)
	if err != nil {
		return nil, err
	}

	return &confinedRoot{inner: inner}, nil
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

// RemoveAll removes the path and any children it contains, confined to the root.
//
// Traversal is performed by [os.Root.RemoveAll], so confinement holds for every descendant without
// this package walking the tree itself.
//
// Parameters:
//   - `p`: the target path; a missing path is not an error.
//
// Returns:
//   - `error`: any error from [os.Root.RemoveAll].
func (r *confinedRoot) RemoveAll(p Path) error { return r.inner.RemoveAll(p.rel) }

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

// OpenUnconfined creates a read-only [Root] at dir. Write operations return [errReadOnly].
//
// Parameters:
//   - `dir`: the base directory for all path resolution.
//
// Returns:
//   - `Root`: a read-only, unconfined root.
func OpenUnconfined(dir string) Root {
	return &unconfinedRootReader{rootBase{name: dir}}
}

// region EXPORTED METHODS

// region Behaviors

// Chmod is unavailable in read-only mode.
//
// Returns:
//   - `error`: always [errReadOnly].
func (r *unconfinedRootReader) Chmod(Path, os.FileMode) error { return errReadOnly }

// Chown is unavailable in read-only mode.
//
// Returns:
//   - `error`: always [errReadOnly].
func (r *unconfinedRootReader) Chown(Path, int, int) error { return errReadOnly }

// Chtimes is unavailable in read-only mode.
//
// Returns:
//   - `error`: always [errReadOnly].
func (r *unconfinedRootReader) Chtimes(Path, time.Time, time.Time) error { return errReadOnly }

// Create is unavailable in read-only mode.
//
// Returns:
//   - `*os.File`: always nil.
//   - `error`: always [errReadOnly].
func (r *unconfinedRootReader) Create(Path) (*os.File, error) { return nil, errReadOnly }

// Lchown is unavailable in read-only mode.
//
// Returns:
//   - `error`: always [errReadOnly].
func (r *unconfinedRootReader) Lchown(Path, int, int) error { return errReadOnly }

// Link is unavailable in read-only mode.
//
// Returns:
//   - `error`: always [errReadOnly].
func (r *unconfinedRootReader) Link(_, _ Path) error { return errReadOnly }

// Lstat returns file info for the path without following a terminal symlink.
//
// Parameters:
//   - `p`: the target path.
//
// Returns:
//   - `fs.FileInfo`: the file info.
//   - `error`: any error from [os.Lstat].
func (r *unconfinedRootReader) Lstat(p Path) (fs.FileInfo, error) { return os.Lstat(p.abs) }

// Mkdir is unavailable in read-only mode.
//
// Returns:
//   - `error`: always [errReadOnly].
func (r *unconfinedRootReader) Mkdir(Path, os.FileMode) error { return errReadOnly }

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

// OpenRoot opens the directory at the path as its own read-only [Root].
//
// The sub-root inherits this root's access mode, so it is read-only too. Unconfined roots hold no
// handle, so this cannot fail.
//
// Parameters:
//   - `p`: the directory to open as a root.
//
// Returns:
//   - `Root`: a read-only, unconfined root anchored at `p`.
//   - `error`: always nil.
func (r *unconfinedRootReader) OpenRoot(p Path) (Root, error) { return OpenUnconfined(p.abs), nil }

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

// RemoveAll is unavailable in read-only mode.
//
// Returns:
//   - `error`: always [errReadOnly].
func (r *unconfinedRootReader) RemoveAll(Path) error { return errReadOnly }

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

// OpenWritableUnconfined creates a read-write [Root] at dir without OS-level confinement.
//
// Parameters:
//   - `dir`: the base directory for all path resolution.
//
// Returns:
//   - `Root`: a read-write, unconfined root.
func OpenWritableUnconfined(dir string) Root {
	return &unconfinedRootReaderWriter{unconfinedRootReader{rootBase{name: dir}}}
}

// region EXPORTED METHODS

// region Behaviors

// Chmod changes the mode of the path.
//
// Parameters:
//   - `p`: the target path.
//   - `mode`: the permission bits to apply.
//
// Returns:
//   - `error`: any error from [os.Chmod].
func (r *unconfinedRootReaderWriter) Chmod(p Path, mode os.FileMode) error {
	return os.Chmod(p.abs, mode)
}

// Chown changes the numeric owner and group of the path.
//
// Unsupported on Windows, where the underlying syscall always fails.
//
// Parameters:
//   - `p`: the target path.
//   - `uid`: the owner id, or -1 to leave unchanged.
//   - `gid`: the group id, or -1 to leave unchanged.
//
// Returns:
//   - `error`: any error from [os.Chown].
func (r *unconfinedRootReaderWriter) Chown(p Path, uid, gid int) error {
	return os.Chown(p.abs, uid, gid)
}

// Chtimes changes the access and modification times of the path.
//
// Parameters:
//   - `p`: the target path.
//   - `atime`: the access time to set.
//   - `mtime`: the modification time to set.
//
// Returns:
//   - `error`: any error from [os.Chtimes].
func (r *unconfinedRootReaderWriter) Chtimes(p Path, atime, mtime time.Time) error {
	return os.Chtimes(p.abs, atime, mtime)
}

// Create creates or truncates the path for reading and writing.
//
// The created file carries mode 0o666 before umask — an unrestricted file by definition. Never use
// Create for sensitive content; use [unconfinedRootReaderWriter.OpenFile] with an explicit
// restrictive mode.
//
// Parameters:
//   - `p`: the target path.
//
// Returns:
//   - `*os.File`: the created file, opened read-write.
//   - `error`: any error from [os.Create].
func (r *unconfinedRootReaderWriter) Create(p Path) (*os.File, error) { return os.Create(p.abs) }

// Lchown changes the numeric owner and group of the path without following a terminal symlink.
//
// Unsupported on Windows, where the underlying syscall always fails.
//
// Parameters:
//   - `p`: the target path.
//   - `uid`: the owner id, or -1 to leave unchanged.
//   - `gid`: the group id, or -1 to leave unchanged.
//
// Returns:
//   - `error`: any error from [os.Lchown].
func (r *unconfinedRootReaderWriter) Lchown(p Path, uid, gid int) error {
	return os.Lchown(p.abs, uid, gid)
}

// Link creates a hard link at newPath pointing to oldPath.
//
// Parameters:
//   - `oldPath`: the existing file the link points to.
//   - `newPath`: the path at which to create the link.
//
// Returns:
//   - `error`: any error from [os.Link].
func (r *unconfinedRootReaderWriter) Link(oldPath, newPath Path) error {
	return os.Link(oldPath.abs, newPath.abs)
}

// Mkdir creates the directory at the path.
//
// Parents must already exist; use [unconfinedRootReaderWriter.MkdirAll] otherwise.
//
// Parameters:
//   - `p`: the directory path.
//   - `perm`: the permission bits for the created directory.
//
// Returns:
//   - `error`: any error from [os.Mkdir].
func (r *unconfinedRootReaderWriter) Mkdir(p Path, perm os.FileMode) error {
	return os.Mkdir(p.abs, perm)
}

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

// OpenRoot opens the directory at the path as its own read-write [Root].
//
// The sub-root inherits this root's access mode, so it is read-write and unconfined too. Unconfined
// roots hold no handle, so this cannot fail.
//
// Parameters:
//   - `p`: the directory to open as a root.
//
// Returns:
//   - `Root`: a read-write, unconfined root anchored at `p`.
//   - `error`: always nil.
func (r *unconfinedRootReaderWriter) OpenRoot(p Path) (Root, error) {
	return OpenWritableUnconfined(p.abs), nil
}

// Remove deletes the file or empty directory at the path.
//
// Parameters:
//   - `p`: the target path.
//
// Returns:
//   - `error`: any error from [os.Remove].
func (r *unconfinedRootReaderWriter) Remove(p Path) error { return os.Remove(p.abs) }

// RemoveAll removes the path and any children it contains.
//
// Parameters:
//   - `p`: the target path; a missing path is not an error.
//
// Returns:
//   - `error`: any error from [os.RemoveAll].
func (r *unconfinedRootReaderWriter) RemoveAll(p Path) error { return os.RemoveAll(p.abs) }

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
// Abs is derived via [filepath.Join]. Rel is stored in canonical slash form regardless of the input's separators —
// rel is the half that serializes, and equal logical paths must produce equal document bytes on every platform (the
// same rule the Merkle-root digest follows). Intended for tests and deserialization.
//
// Parameters:
//   - `root`: the root directory that `rel` is relative to (matches [os.Root.Name]).
//   - `rel`: the root-relative path; any separator form.
//
// Returns:
//   - `Path`: the constructed path, with `rel` canonicalized to slash form.
func NewPath(root, rel string) Path {
	return Path{root: root, rel: filepath.ToSlash(rel), abs: filepath.Join(root, rel)}
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
// Always canonical slash form, on every platform: rel is the half that serializes, so equal logical paths produce
// equal document bytes and checksums everywhere, and it feeds [io/fs] APIs whose contract requires slash paths.
// Convert with [filepath.FromSlash] only where an OS-native rel is genuinely needed; direct filesystem I/O flows
// through [Path.Abs], which stays OS-native.
//
// Returns:
//   - `string`: the root-relative path, slash-separated.
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
		Root string `json:"fsroot"`
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
		Root string `yaml:"fsroot"`
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
		Root string `json:"fsroot"`
		Rel  string `json:"rel"`
	}

	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}

	// ToSlash enforces the canonical-rel invariant on arrival: a document written by a pre-fix
	// Windows build carries native separators, and decode is the boundary that normalizes them.
	p.root = decoded.Root
	p.rel = filepath.ToSlash(decoded.Rel)
	p.abs = filepath.Join(decoded.Root, filepath.FromSlash(decoded.Rel))

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
		Root string `yaml:"fsroot"`
		Rel  string `yaml:"rel"`
	}

	if err := value.Decode(&decoded); err != nil {
		return err
	}

	// ToSlash enforces the canonical-rel invariant on arrival — same boundary rule as UnmarshalJSON.
	p.root = decoded.Root
	p.rel = filepath.ToSlash(decoded.Rel)
	p.abs = filepath.Join(decoded.Root, filepath.FromSlash(decoded.Rel))

	return nil
}

// endregion

// endregion

// endregion

// region HELPER FUNCTIONS

// makePath computes a [Path] from a root directory name and an input path.
//
// Absolute inputs compute Rel via [filepath.Rel] (may contain ../ prefixes for paths outside root — valid in
// unconfined mode, rejected by [*os.Root] in confined mode). Relative inputs compute Abs via [filepath.Join]. Rel is
// stored in canonical slash form on every platform: it is the half that serializes, and equal logical paths must
// produce equal document bytes everywhere. Abs stays OS-native and carries all direct filesystem I/O.
//
// Parameters:
//   - `rootName`: the root directory path.
//   - `path`: the input path, absolute or relative.
//
// Returns:
//   - `Path`: the constructed path with `rel` in slash form and `abs` OS-native.
func makePath(rootName, path string) Path {

	if filepath.IsAbs(path) {
		rel := assert.Must(filepath.Rel(rootName, path))
		return Path{root: rootName, rel: filepath.ToSlash(rel), abs: filepath.Clean(path)}
	}
	return Path{
		root: rootName,
		rel:  filepath.ToSlash(filepath.Clean(path)),
		abs:  filepath.Join(rootName, path),
	}
}

// endregion
