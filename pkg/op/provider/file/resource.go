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

func init() {
	op.RegisterConstructor(func(v any) (Resource, error) {
		s, ok := v.(string)
		if !ok {
			return Resource{}, fmt.Errorf("file.Resource: expected string path, got %T", v)
		}
		return NewResource(s), nil
	})
}

// SourcePath holds both the root-relative and absolute forms of a file path.
// Rel is used for all I/O through os.Root; Abs is used for URIs, display, and logging.
type SourcePath struct {
	Rel string // root-relative path — used for scoped I/O through os.Root
	Abs string // absolute path — used for URIs, display, logging
}

// String returns the absolute path.
func (sp SourcePath) String() string { return sp.Abs }

// Resource represents a handle to data that can be streamed.
type Resource struct {
	op.ResourceBase
	SourcePath SourcePath
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
	r := Resource{SourcePath: SourcePath{Abs: path}}
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

// Reader returns an io.ReadCloser for reading the file resource's data from its source path.
//
// The caller is responsible for closing the returned reader.
//
// Parameters:
//   - none
//
// Returns:
//   - io.ReadCloser: an io.ReadCloser for reading the file resource's data
//   - error: any error that occurred during opening
func (r *Resource) Reader() (io.ReadCloser, error) {
	return os.Open(r.SourcePath.Abs)
}

// Refresh re-populates the resource's metadata by performing a fresh stat and re-calculating the checksum. Call after
// any successful physical mutation. When root is non-nil, I/O is scoped through os.Root.
//
// Parameters:
//   - root: OS root for scoped I/O (nil falls back to direct os.* calls)
//
// Returns:
//   - error: any stat or read error
func (r *Resource) Refresh(root *os.Root) error {

	var info os.FileInfo
	var err error

	if rel := rootRel(root, r.SourcePath.Abs); rel != "" {
		info, err = root.Stat(rel)
	} else {
		info, err = os.Stat(r.SourcePath.Abs)
	}

	if err != nil {
		return err
	}

	var checksum string
	if rel := rootRel(root, r.SourcePath.Abs); rel != "" {
		if data, readErr := root.ReadFile(rel); readErr == nil {
			checksum = checksumBytes(data)
		}
	} else {
		checksum = checksumFile(r.SourcePath.Abs)
	}

	return r.refreshWith(info, checksum, info.Size())
}

// RefreshWith updates metadata after a write operation using a known checksum and size. A stat is still performed to
// capture kernel-assigned identity (Inode, Device). When root is non-nil, I/O is scoped through os.Root.
//
// Parameters:
//   - root: OS root for scoped I/O (nil falls back to direct os.* calls)
//   - checksum: Pre-computed checksum string
//   - size: Known file size in bytes
//
// Returns:
//   - error: any stat error
func (r *Resource) RefreshWith(root *os.Root, checksum string, size int64) error {

	var info os.FileInfo
	var err error

	if rel := rootRel(root, r.SourcePath.Abs); rel != "" {
		info, err = root.Stat(rel)
	} else {
		info, err = os.Stat(r.SourcePath.Abs)
	}

	if err != nil {
		return err
	}

	return r.refreshWith(info, checksum, size)
}

// Resolve populates the resource's metadata by canonicalizing the path and performing a stat. If the file does not
// exist, Resolve returns nil and metadata remains empty ([Resource.Exists] returns false). Other stat errors are
// returned. When root is non-nil, I/O is scoped through os.Root and SourcePath.Rel is populated.
//
// Parameters:
//   - root: OS root for scoped I/O (nil falls back to direct os.* calls)
//
// Returns:
//   - error: any stat error (not-exist is not an error)
func (r *Resource) Resolve(root *os.Root) error {

	abs, err := filepath.Abs(r.SourcePath.Abs)
	if err == nil {
		r.SourcePath.Abs = filepath.Clean(abs)
		r.SetURI(r.buildURI())
	}

	// Compute root-relative path when root is available.
	if root != nil {
		if rel, relErr := filepath.Rel(root.Name(), r.SourcePath.Abs); relErr == nil {
			r.SourcePath.Rel = rel
		}
	}

	var info os.FileInfo
	if rel := rootRel(root, r.SourcePath.Abs); rel != "" {
		info, err = root.Stat(rel)
	} else {
		info, err = os.Stat(r.SourcePath.Abs)
	}

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

	// Compute checksum using root-scoped I/O when available.
	if rel := rootRel(root, r.SourcePath.Abs); rel != "" {
		if data, readErr := root.ReadFile(rel); readErr == nil {
			r.Checksum = checksumBytes(data)
		}
	} else {
		r.Checksum = checksumFile(r.SourcePath.Abs)
	}

	return nil
}

// String returns a compact JSON representation of the resource.
func (r *Resource) String() string { return r.Format(r) }

// WriteTo allows the Resource to be streamed directly to any io.Writer.
//
// For efficiency, it uses [io.Copy] which automatically attempts a zero-copy syscall before falling back to a 32KB
// buffer.
//
// Parameters:
//   - writer: io.Writer to write to
//
// Returns:
//   - int64: number of bytes written
//   - error: any error that occurred during writing
func (r *Resource) WriteTo(writer io.Writer) (int64, error) {

	f, err := os.Open(r.SourcePath.Abs)

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
	return "file://" + r.SourcePath.Abs
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
// The embedded [op.TombstoneBase] carries the affected [Resource] whose identity is preserved — SourcePath.Abs always
// reflects the file's true home.
type Tombstone struct {
	op.TombstoneBase

	// RecoveryPath records where the data was temporarily moved during the operation (backup, recovery site, or move
	// destination). An empty RecoveryPath means no prior data existed to recover.
	RecoveryPath string
}
