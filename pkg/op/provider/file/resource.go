// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package file

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// ResourceFromValue constructs a file.Resource from a string path.
//
// Parameters:
//   - v: expected to be a string file path
//
// Returns:
//   - Resource: initialized with the given path
//   - error: if v is not a string
func ResourceFromValue(v any) (Resource, error) {

	s, ok := v.(string)
	if !ok {
		return Resource{}, fmt.Errorf("file.Resource: expected string path, got %T", v)
	}
	return NewResource(s), nil
}

// Resource represents a handle to data that can be streamed.
type Resource struct {
	op.ResourceBase
	SourcePath op.Path
	Inode      uint64
	Device     uint64
	Size       int64
	Mode       os.FileMode
	ModTime    time.Time
	Checksum   string
}

// NewResource creates a [Resource] with the given source path. The constructor
// is pure computation — no I/O, no error. Metadata (size, mode, checksum)
// is populated later by [Resource.Resolve].
func NewResource(path string) Resource {
	r := Resource{SourcePath: op.NewPath("", path)}
	r.SetURI(r.buildURI())
	return r
}

// region EXPORTED METHODS

// region Behaviors

// Exists returns true if the resource has been resolved and the file existed
// at resolve time. An unresolved resource always reports Exists() == false.
func (r *Resource) Exists() bool {
	return !r.ModTime.IsZero()
}

// Refresh re-populates the resource's metadata by performing a fresh stat and re-calculating the checksum. Call after
// any successful physical mutation. I/O is scoped through [op.Root].
//
// Parameters:
//   - root: [op.Root] for scoped I/O
//
// Returns:
//   - error: any stat or read error
func (r *Resource) Refresh(root op.Root) error {

	info, err := root.Stat(root.NewPath(r.SourcePath.Abs()))
	if err != nil {
		return err
	}

	return r.refreshWith(info, checksumFile(root, r.SourcePath.Abs()), info.Size())
}

// RefreshWith updates metadata after a write operation using a known checksum and size. A stat is still performed to
// capture kernel-assigned identity (Inode, Device). I/O is scoped through [op.Root].
//
// Parameters:
//   - root: [op.Root] for scoped I/O
//   - checksum: Pre-computed checksum string
//   - size: Known file size in bytes
//
// Returns:
//   - error: any stat error
func (r *Resource) RefreshWith(root op.Root, checksum string, size int64) error {

	info, err := root.Stat(root.NewPath(r.SourcePath.Abs()))
	if err != nil {
		return err
	}

	return r.refreshWith(info, checksum, size)
}

// Resolve populates the resource's metadata by canonicalizing the path and performing a stat. If the file does not
// exist, Resolve returns nil and metadata remains empty ([Resource.Exists] returns false). Other stat errors are
// returned. I/O is scoped through [op.Root] and SourcePath.Rel is populated.
//
// Parameters:
//   - root: [op.Root] for scoped I/O
//
// Returns:
//   - error: any stat error (not-exist is not an error)
func (r *Resource) Resolve(root op.Root) error {

	abs, err := filepath.Abs(r.SourcePath.Abs())
	if err == nil {
		r.SourcePath = root.NewPath(abs)
		r.SetURI(r.buildURI())
	}

	info, err := root.Stat(r.SourcePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("failed to stat: %w", err)
	}

	var inode, device uint64
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		inode = stat.Ino
		device = uint64(stat.Dev)
	}

	r.Inode = inode
	r.Device = device
	r.Size = info.Size()
	r.Mode = info.Mode()
	r.ModTime = info.ModTime()

	r.Checksum = checksumFile(root, r.SourcePath.Abs())

	return nil
}

// String returns a compact JSON representation of the resource.
func (r *Resource) String() string { return r.Format(r) }

// WriteTo allows the Resource to be streamed directly to any io.Writer. I/O is scoped through [op.Root].
//
// For efficiency, it uses [io.Copy] which automatically attempts a zero-copy syscall before falling back to a 32KB
// buffer.
//
// Parameters:
//   - root: [op.Root] for scoped I/O
//   - writer: io.Writer to write to
//
// Returns:
//   - int64: number of bytes written
//   - error: any error that occurred during writing
func (r *Resource) WriteTo(root op.Root, writer io.Writer) (int64, error) {

	f, err := root.Open(root.NewPath(r.SourcePath.Abs()))
	if err != nil {
		return 0, err
	}
	defer f.Close()

	return io.Copy(writer, f)
}

// endregion

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// buildURI computes the canonical file:// URI from SourcePath.
func (r *Resource) buildURI() string {
	return "file://" + r.SourcePath.Abs()
}

// refreshWith updates the Resource's metadata with the provided information.
func (r *Resource) refreshWith(info os.FileInfo, checksum string, size int64) error {

	var inode, device uint64

	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		inode = stat.Ino
		device = uint64(stat.Dev)
	}

	r.Inode = inode
	r.Device = device
	r.Size = info.Size()
	r.Mode = info.Mode()
	r.ModTime = info.ModTime()
	r.Checksum = checksum

	return nil
}

// endregion

// endregion

// Tombstone holds file-specific compensation state.
//
// The embedded [op.TombstoneBase] carries the affected [Resource] whose identity is preserved — SourcePath always
// reflects the file's true home.
type Tombstone struct {
	op.TombstoneBase

	// RecoveryID records where the data was temporarily moved during the operation (backup, recovery site, or move
	// destination). An empty RecoveryID means no prior data existed to recover.
	RecoveryID string
}
