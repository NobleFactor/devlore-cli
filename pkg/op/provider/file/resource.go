// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package file

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

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

// NewResource constructs a file.Resource from a string path.
//
// Parameters:
//   - ctx: execution context.
//   - value: a string file path.
//
// Returns:
//   - Resource: initialized with the given path.
//   - error: if value is not a string.
func NewResource(ctx *op.ExecutionContext, value any) (*Resource, error) {

	path, ok := value.(string)
	if !ok {
		return nil, fmt.Errorf("file.Resource: expected string path, got %T", value)
	}

	sourcePath := ctx.Root.NewPath(path)

	return &Resource{
		ResourceBase: op.NewResourceBase(ctx, "file://"+sourcePath.Abs()),
		SourcePath:   sourcePath,
	}, nil
}

// region EXPORTED METHODS

// region Behaviors

// Exists returns true if the resource has been resolved and the file existed
// at resolve time. An unresolved resource always reports Exists() == false.
func (r *Resource) Exists() bool {
	return !r.ModTime.IsZero()
}

// Refresh re-populates the resource's metadata by performing a fresh stat and re-calculating the checksum. Call after
// any successful physical mutation.
//
// Returns:
//   - error: any stat or read error.
func (r *Resource) Refresh() error {

	root := r.ExecutionContext().Root
	info, err := root.Stat(root.NewPath(r.SourcePath.Abs()))
	if err != nil {
		return err
	}

	return r.refreshWith(info, checksumFile(root, r.SourcePath.Abs()))
}

// RefreshWith updates metadata after a write operation using a known checksum. A stat is still performed to capture
// kernel-assigned identity (Inode, Device).
//
// Parameters:
//   - checksum: pre-computed checksum string.
//
// Returns:
//   - error: any stat error.
func (r *Resource) RefreshWith(checksum string) error {

	root := r.ExecutionContext().Root
	info, err := root.Stat(root.NewPath(r.SourcePath.Abs()))
	if err != nil {
		return err
	}

	return r.refreshWith(info, checksum)
}

// Resolve rebinds the source path to the execution root and populates metadata via stat. The path is canonical from
// construction; rebinding updates Rel for confined I/O under the execution root. If the file does not exist, Resolve
// returns nil and metadata remains empty ([Resource.Exists] returns false).
//
// Returns:
//   - error: any stat error (not-exist is not an error).
func (r *Resource) Resolve() error {

	root := r.ExecutionContext().Root

	r.SourcePath = root.NewPath(r.SourcePath.Abs())

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
		device = uint64(stat.Dev) //nolint:gosec // G115: Dev is platform-specific; overflow is not a practical concern
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

// endregion

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// refreshWith updates the Resource's metadata with the provided information.
func (r *Resource) refreshWith(info os.FileInfo, checksum string) error {
	var inode, device uint64

	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		inode = stat.Ino
		device = uint64(stat.Dev) //nolint:gosec // G115: Dev is platform-specific; overflow is not a practical concern
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
