// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

// Package fsroot provides scoped filesystem roots.
//
// All provider I/O flows through the [Dir] interface, confining reads and writes to a directory
// tree. Three implementations serve the three lifecycles: confined roots for execution, read-only
// roots for planning, and writable unconfined roots for tests.
package fsroot

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/NobleFactor/devlore-cli/pkg/assert"
)

// Interface guards.
var (
	_ Dir = (*confinedRoot)(nil)
	_ Dir = (*scratchRoot)(nil)
	_ Dir = (*unconfinedRootReader)(nil)
	_ Dir = (*unconfinedRootReaderWriter)(nil)
)

// errReadOnly is returned by write operations on a [unconfinedRootReader]. It wraps the standard
// [errors.ErrUnsupported] sentinel so callers can test for it with `errors.Is(err, errors.ErrUnsupported)` without
// depending on this package.
var errReadOnly = fmt.Errorf("write operation not available in read-only mode: %w", errors.ErrUnsupported)

// errTempPatternSeparator is returned when a temporary-name pattern contains a path separator, which would let a
// caller redirect the created object through the name instead of the directory argument. It wraps
// [fs.ErrInvalid] so callers can test for it without depending on this package.
var errTempPatternSeparator = fmt.Errorf("temporary name pattern contains a path separator: %w", fs.ErrInvalid)

// maxTempAttempts bounds the collision retries in [createTempIn] and [mkdirTempIn], matching [os.CreateTemp].
const maxTempAttempts = 10000

// Permission-class masks for the three classes a Unix mode names.
//
// Go supplies no symbol for these. [fs.ModePerm] covers all nine bits at once, and syscall's S_IRWXU, S_IRWXG
// and S_IRWXO are unix-only, so they cannot appear in code that also builds for windows.
//
// They exist because the two rules that matter are one character apart as literals — `perm&0o007` excludes
// only other, `perm&0o077` excludes group as well — and that is how this package's enforcement gate and its own
// documentation came to disagree about which one it implements. Named, the rules stop resembling each other:
// `perm&PermOther == 0` and `perm&(PermGroup|PermOther) == 0` cannot be misread for one another.
//
// Migrating the existing call sites onto these, and ruling which of the two the enforcement gate should mean,
// is tracked separately.
const (

	// PermOwner masks the owner's read, write, and execute bits.
	PermOwner os.FileMode = 0o700

	// PermGroup masks the group's read, write, and execute bits.
	PermGroup os.FileMode = 0o070

	// PermOther masks other's — the world's — read, write, and execute bits.
	PermOther os.FileMode = 0o007
)

// Dir provides scoped filesystem operations. All path arguments are [Path] values created through [Dir.NewPath].
//
// Three concrete implementations provide different access modes:
//
//   - [OpenConfined] wraps [*os.Dir] for OS-enforced confinement (execution)
//   - [OpenUnconfined] delegates to os.* for unconfined read-only access (planning)
//   - [OpenWritableUnconfined] delegates to os.* for unconfined read-write access (testing)
//
// [Open] constructs any of the three from a [Mode] value; [OpenScratch] constructs a confined root
// at a temporary directory that removes itself on [Dir.Close].
//
// The method set mirrors [*os.Dir] in full, so code that knows the standard library's root knows
// this one. Every filesystem mutation in the repository is expected to flow through this interface;
// a direct os.* call must carry a `// Confinement:` comment stating why the root cannot serve it.
//
// [Dir.CreateTemp] and [Dir.MkdirTemp] go beyond the mirror: [*os.Dir] has no equivalent, and
// [os.CreateTemp] cannot be confined to a root. Choosing between a scratch root and this one is the
// whole of the decision — use the session's scratch unless the bytes must end up in this tree
// atomically, since a rename out of scratch can cross a device boundary and degrade to a copy.
type Dir interface {
	Chmod(p Path, mode os.FileMode) error
	Chown(p Path, uid, gid int) error
	Chtimes(p Path, atime, mtime time.Time) error
	Close() error
	Create(p Path) (*os.File, error)
	CreateTemp(dir Path, pattern string) (*os.File, Path, error)
	FS() fs.FS
	Lchown(p Path, uid, gid int) error
	Link(oldPath, newPath Path) error
	Lstat(p Path) (fs.FileInfo, error)
	Mkdir(p Path, perm os.FileMode) error
	MkdirAll(p Path, perm os.FileMode) error
	MkdirTemp(dir Path, pattern string) (Path, error)
	Name() string
	NewPath(name ...string) Path
	Open(p Path) (*os.File, error)
	OpenFile(p Path, flag int, perm os.FileMode) (*os.File, error)
	OpenRoot(p Path) (Dir, error)
	ReadFile(p Path) ([]byte, error)
	Readlink(p Path) (string, error)
	Remove(p Path) error
	RemoveAll(p Path) error
	Rename(oldPath, newPath Path) error
	Stat(p Path) (fs.FileInfo, error)
	Symlink(target string, link Path) error
	WriteFile(p Path, data []byte, perm os.FileMode) error
}

// Mode selects which [Dir] implementation [Open] constructs.
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

// Open opens a [Dir] at dir in the given [Mode].
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
func Open(dir string, mode Mode) (Dir, error) {

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

// OpenScratch opens a confined [Dir] at a newly created temporary directory that removes itself.
//
// Scratch is not an escape from confinement — it is its own confined tree with a self-destroying
// lifetime. Process scratch (spool files, staging trees) belongs here rather than in a direct
// [os.CreateTemp] call, so that scratch I/O flows through the same seam as every other mutation and
// inherits its platform behavior.
//
// IMPORTANT: [Dir.Close] on a scratch root does two things — it releases the handle AND removes
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
func OpenScratch(pattern string) (Dir, error) {

	dir, err := os.MkdirTemp("", pattern)
	if err != nil {
		return nil, fmt.Errorf("fsroot.OpenScratch: create temporary directory: %w", err)
	}

	confined, err := OpenConfined(dir)
	if err != nil {
		return nil, errors.Join(
			fmt.Errorf("fsroot.OpenScratch: confine %s: %w", dir, err), os.RemoveAll(dir))
	}

	return &scratchRoot{Dir: confined, dir: dir}, nil
}

// rootBase holds the root directory path shared by all implementations.
type rootBase struct {
	name string
}

// scratchRoot is a confined [Dir] over a temporary directory it owns and removes on Close.
//
// Every method other than Close is the embedded confined root's; only the lifetime differs.
type scratchRoot struct {
	Dir

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
func (r *scratchRoot) Close() error { return errors.Join(r.Dir.Close(), os.RemoveAll(r.dir)) }

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
func (b *rootBase) NewPath(name ...string) Path { return makePath(b.name, name) }

// endregion

// endregion

// confinedRoot wraps [*os.Root] for OS-enforced confinement.
//
// All I/O is confined to the root directory by the kernel. Symlinks cannot escape, path traversal is blocked.
type confinedRoot struct {
	inner *os.Root
}

// OpenConfined opens an OS-enforced confined [Dir] at dir.
//
// Parameters:
//   - `dir`: the directory to confine all I/O within.
//
// Returns:
//   - `Root`: a confined root backed by [*os.Root].
//   - `error`: any error from [os.OpenRoot].
func OpenConfined(dir string) (Dir, error) {

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
// On Windows a restrictive mode is additionally enforced as an access-control list, because
// [os.Root.Chmod] there toggles only the read-only attribute (issue #405).
//
// Returns:
//   - `error`: any error from [os.Root.Chmod], or from the Windows enforcement pass.
func (r *confinedRoot) Chmod(p Path, mode os.FileMode) error {

	if err := r.inner.Chmod(p.rel, mode); err != nil {
		return err
	}

	info, err := r.inner.Stat(p.rel)
	if err != nil {
		return err
	}

	return applyMode(p.abs, mode, info.IsDir())
}

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
//   - `error`: any error from [os.Root.Mkdir], or from the Windows enforcement pass.
func (r *confinedRoot) Mkdir(p Path, perm os.FileMode) error {

	if err := r.inner.Mkdir(p.rel, perm); err != nil {
		return err
	}

	return applyMode(p.abs, perm, true)
}

// CreateTemp creates a uniquely named file under `dir`, confined to the root.
//
// Parameters:
//   - `dir`: the directory within the root to create the file under; use `NewPath(".")` for the root itself.
//   - `pattern`: the name pattern; the random string replaces the last `*`, or is appended when there is none.
//
// Returns:
//   - `*os.File`: the open file, owned by the caller.
//   - `Path`: the created file's path.
//   - `error`: any error from [createTempIn].
func (r *confinedRoot) CreateTemp(dir Path, pattern string) (*os.File, Path, error) {
	return createTempIn(r, dir, pattern)
}

// MkdirTemp creates a uniquely named directory under `dir`, confined to the root.
//
// Parameters:
//   - `dir`: the directory within the root to create the directory under; use `NewPath(".")` for the root itself.
//   - `pattern`: the name pattern; the random string replaces the last `*`, or is appended when there is none.
//
// Returns:
//   - `Path`: the created directory's path.
//   - `error`: any error from [mkdirTempIn].
func (r *confinedRoot) MkdirTemp(dir Path, pattern string) (Path, error) {
	return mkdirTempIn(r, dir, pattern)
}

// MkdirAll creates the directory at the path along with any necessary parents, confined to the root.
//
// Parameters:
//   - `p`: the directory path.
//   - `perm`: the permission bits for created directories.
//
// Returns:
//   - `error`: any error from [os.Root.MkdirAll], or from the Windows enforcement pass.
func (r *confinedRoot) MkdirAll(p Path, perm os.FileMode) error {

	if err := r.inner.MkdirAll(p.rel, perm); err != nil {
		return err
	}

	return applyMode(p.abs, perm, true)
}

// NewPath builds a [Path] from an input path, resolved against the confined root directory.
//
// Parameters:
//   - `path`: the input path, absolute or relative to the root directory.
//
// Returns:
//   - `Path`: the constructed path with both rel and abs populated.
func (r *confinedRoot) NewPath(name ...string) Path { return makePath(r.inner.Name(), name) }

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

	file, err := r.inner.OpenFile(p.rel, flag, perm)
	if err != nil {
		return nil, err
	}

	if flag&os.O_CREATE != 0 {
		if modeErr := applyMode(p.abs, perm, false); modeErr != nil {
			return nil, errors.Join(modeErr, file.Close())
		}
	}

	return file, nil
}

// OpenRoot opens the directory at the path as its own [Dir], confined to this root.
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
func (r *confinedRoot) OpenRoot(p Path) (Dir, error) {

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
//   - `error`: any error from [os.Root.WriteFile], or from the Windows enforcement pass.
func (r *confinedRoot) WriteFile(p Path, data []byte, perm os.FileMode) error {

	if err := r.inner.WriteFile(p.rel, data, perm); err != nil {
		return err
	}

	return applyMode(p.abs, perm, false)
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

// OpenUnconfined creates a read-only [Dir] at dir. Write operations return [errReadOnly].
//
// Parameters:
//   - `dir`: the base directory for all path resolution.
//
// Returns:
//   - `Root`: a read-only, unconfined root.
func OpenUnconfined(dir string) Dir {
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

// CreateTemp is unavailable in read-only mode.
//
// Returns:
//   - `*os.File`: always nil.
//   - `Path`: always the zero Path.
//   - `error`: always [errReadOnly].
func (r *unconfinedRootReader) CreateTemp(Path, string) (*os.File, Path, error) {
	return nil, Path{}, errReadOnly
}

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

// MkdirTemp is unavailable in read-only mode.
//
// Returns:
//   - `Path`: always the zero Path.
//   - `error`: always [errReadOnly].
func (r *unconfinedRootReader) MkdirTemp(Path, string) (Path, error) { return Path{}, errReadOnly }

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

// OpenRoot opens the directory at the path as its own read-only [Dir].
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
func (r *unconfinedRootReader) OpenRoot(p Path) (Dir, error) { return OpenUnconfined(p.abs), nil }

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

// OpenWritableUnconfined creates a read-write [Dir] at dir without OS-level confinement.
//
// Parameters:
//   - `dir`: the base directory for all path resolution.
//
// Returns:
//   - `Root`: a read-write, unconfined root.
func OpenWritableUnconfined(dir string) Dir {
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
// On Windows a restrictive mode is additionally enforced as an access-control list, because
// [os.Chmod] there toggles only the read-only attribute (issue #405).
//
// Returns:
//   - `error`: any error from [os.Chmod], or from the Windows enforcement pass.
func (r *unconfinedRootReaderWriter) Chmod(p Path, mode os.FileMode) error {

	if err := os.Chmod(p.abs, mode); err != nil {
		return err
	}

	info, err := os.Stat(p.abs)
	if err != nil {
		return err
	}

	return applyMode(p.abs, mode, info.IsDir())
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
//   - `error`: any error from [os.Mkdir], or from the Windows enforcement pass.
func (r *unconfinedRootReaderWriter) Mkdir(p Path, perm os.FileMode) error {

	if err := os.Mkdir(p.abs, perm); err != nil {
		return err
	}

	return applyMode(p.abs, perm, true)
}

// CreateTemp creates a uniquely named file under `dir`.
//
// Parameters:
//   - `dir`: the directory to create the file under; use `NewPath(".")` for the root directory itself.
//   - `pattern`: the name pattern; the random string replaces the last `*`, or is appended when there is none.
//
// Returns:
//   - `*os.File`: the open file, owned by the caller.
//   - `Path`: the created file's path.
//   - `error`: any error from [createTempIn].
func (r *unconfinedRootReaderWriter) CreateTemp(dir Path, pattern string) (*os.File, Path, error) {
	return createTempIn(r, dir, pattern)
}

// MkdirTemp creates a uniquely named directory under `dir`.
//
// Parameters:
//   - `dir`: the directory to create the directory under; use `NewPath(".")` for the root directory itself.
//   - `pattern`: the name pattern; the random string replaces the last `*`, or is appended when there is none.
//
// Returns:
//   - `Path`: the created directory's path.
//   - `error`: any error from [mkdirTempIn].
func (r *unconfinedRootReaderWriter) MkdirTemp(dir Path, pattern string) (Path, error) {
	return mkdirTempIn(r, dir, pattern)
}

// MkdirAll creates the directory at the path along with any necessary parents.
//
// Parameters:
//   - `p`: the directory path.
//   - `perm`: the permission bits for created directories.
//
// Returns:
//   - `error`: any error from [os.MkdirAll], or from the Windows enforcement pass.
func (r *unconfinedRootReaderWriter) MkdirAll(p Path, perm os.FileMode) error {

	if err := os.MkdirAll(p.abs, perm); err != nil {
		return err
	}

	return applyMode(p.abs, perm, true)
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

	file, err := os.OpenFile(p.abs, flag, perm)
	if err != nil {
		return nil, err
	}

	if flag&os.O_CREATE != 0 {
		if modeErr := applyMode(p.abs, perm, false); modeErr != nil {
			return nil, errors.Join(modeErr, file.Close())
		}
	}

	return file, nil
}

// OpenRoot opens the directory at the path as its own read-write [Dir].
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
func (r *unconfinedRootReaderWriter) OpenRoot(p Path) (Dir, error) {
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
//   - `error`: any error from [os.WriteFile], or from the Windows enforcement pass.
func (r *unconfinedRootReaderWriter) WriteFile(p Path, data []byte, perm os.FileMode) error {

	if err := os.WriteFile(p.abs, data, perm); err != nil {
		return err
	}

	return applyMode(p.abs, perm, false)
}

// endregion

// endregion

// region SUPPORTING TYPES

// Path holds both root-relative and absolute forms of a filesystem path.
//
// Created through [Dir.NewPath] to guarantee both fields are populated. The root field records which root directory
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

// isPrivateMode reports whether `perm` denies all access to other.
//
// This is the one distinction Windows can honor faithfully (issue #405), and the boundary is
// deliberately "other", not "group and other". We can express *other is excluded*; we have no group
// principal to grant to, so the group bit is inexpressible and collapses into the owner. 0o600,
// 0o640 and 0o660 therefore all become owner-only on Windows.
//
// The alternative — treating 0o640 as non-private and leaving it to inherit — fails **open**: a
// file its author deliberately restricted would become readable by everyone. Cygwin can preserve
// the group triad only because it maintains a POSIX-group-to-SID mapping and emits non-canonical
// ACLs its own documentation calls non-interoperable with Windows; Win32-OpenSSH, facing this exact
// problem for private keys, has no group axis at all and requires owner + SYSTEM + Administrators.
//
// Parameters:
//   - `perm`: the permission bits the caller requested.
//
// Returns:
//   - `bool`: true when other is granted nothing.
func isPrivateMode(perm os.FileMode) bool { return perm.Perm()&0o007 == 0 }

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
func makePath(rootName string, name []string) Path {

	// Joined here so no caller has to: a path within a root is named by its elements, and
	// `NewPath(a, b)` is the joiner leaking to every call site.
	path := filepath.Join(name...)

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

// createTempIn creates a uniquely named file under `dir`, mirroring [os.CreateTemp] inside a root.
//
// The file is created through `r`'s own [Dir.OpenFile] with `O_CREATE|O_EXCL`, so confinement and the Windows
// enforcement pass both apply exactly as they do to any other create. Mode 0600 matches [os.CreateTemp] and is
// private, which means a temporary file receives a protected DACL on Windows without the caller asking.
//
// Parameters:
//   - `r`: the root the file is created in.
//   - `dir`: the directory within `r` to create the file under; use `r.NewPath(".")` for the root itself.
//   - `pattern`: the name pattern; the random string replaces the last `*`, or is appended when there is none.
//
// Returns:
//   - `*os.File`: the open file, positioned at offset zero and owned by the caller.
//   - `Path`: the created file's path, for the rename or removal the caller owes it.
//   - `error`: [errTempPatternSeparator] when the pattern contains a path separator, [fs.ErrExist] when a unique
//     name could not be found, or any error from [Dir.OpenFile].
func createTempIn(r Dir, dir Path, pattern string) (*os.File, Path, error) {

	prefix, suffix, err := tempNameParts(pattern)
	if err != nil {
		return nil, Path{}, err
	}

	for range maxTempAttempts {

		candidate := r.NewPath(dir.Rel(), prefix+nextTempName()+suffix)

		file, openErr := r.OpenFile(candidate, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
		if openErr == nil {
			return file, candidate, nil
		}
		if !errors.Is(openErr, fs.ErrExist) {
			return nil, Path{}, openErr
		}
	}

	return nil, Path{}, fmt.Errorf("fsroot.CreateTemp %s: %w", filepath.Join(dir.Rel(), prefix+"*"+suffix), fs.ErrExist)
}

// mkdirTempIn creates a uniquely named directory under `dir`, mirroring [os.MkdirTemp] inside a root.
//
// The directory is created through `r`'s own [Dir.Mkdir], so confinement and the Windows enforcement pass apply.
// Mode 0700 matches [os.MkdirTemp] and is private, so the directory is DACL-protected on Windows.
//
// Parameters:
//   - `r`: the root the directory is created in.
//   - `dir`: the directory within `r` to create the new directory under; use `r.NewPath(".")` for the root itself.
//   - `pattern`: the name pattern; the random string replaces the last `*`, or is appended when there is none.
//
// Returns:
//   - `Path`: the created directory's path, for the removal the caller owes it.
//   - `error`: [errTempPatternSeparator] when the pattern contains a path separator, [fs.ErrExist] when a unique
//     name could not be found, or any error from [Dir.Mkdir].
func mkdirTempIn(r Dir, dir Path, pattern string) (Path, error) {

	prefix, suffix, err := tempNameParts(pattern)
	if err != nil {
		return Path{}, err
	}

	for range maxTempAttempts {

		candidate := r.NewPath(dir.Rel(), prefix+nextTempName()+suffix)

		mkdirErr := r.Mkdir(candidate, 0o700)
		if mkdirErr == nil {
			return candidate, nil
		}
		if !errors.Is(mkdirErr, fs.ErrExist) {
			return Path{}, mkdirErr
		}
	}

	return Path{}, fmt.Errorf("fsroot.MkdirTemp %s: %w", filepath.Join(dir.Rel(), prefix+"*"+suffix), fs.ErrExist)
}

// nextTempName returns a random component for a temporary name.
//
// Uniqueness is not this function's job: both callers create with O_EXCL and retry on collision, exactly as
// [os.CreateTemp] does, so a repeat costs an attempt rather than a wrong result. The source is [rand.Text] rather
// than a pseudo-random generator because a predictable temporary name in a directory an attacker can also write
// to invites a symlink race, and the cost of the strong source here is nothing.
//
// Returns:
//   - `string`: the random component.
func nextTempName() string { return rand.Text() }

// tempNameParts splits a temporary-name pattern around its last `*`.
//
// Parameters:
//   - `pattern`: the name pattern.
//
// Returns:
//   - `string`: the part before the `*`, or the whole pattern when it has none.
//   - `string`: the part after the `*`, or empty.
//   - `error`: [errTempPatternSeparator] when the pattern contains a path separator, which would let a caller
//     escape `dir` through the name rather than the path.
func tempNameParts(pattern string) (prefix, suffix string, err error) {

	if strings.ContainsAny(pattern, `/\`) {
		return "", "", fmt.Errorf("fsroot: pattern %q: %w", pattern, errTempPatternSeparator)
	}

	if position := strings.LastIndex(pattern, "*"); position >= 0 {
		return pattern[:position], pattern[position+1:], nil
	}

	return pattern, "", nil
}

// endregion
