// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/google/uuid"
)

const recoveryDir = ".devlore/recovery"

// RecoverySite manages archival and restoration of resources within the authority boundary.
//
// All operations use zero-copy renames for files and byte serialization for data. The recovery directory is
// .devlore/recovery/ within the op.Root authority boundary. All I/O goes through ExecutionContext.Root.
type RecoverySite struct {
	ctx *ExecutionContext
}

// NewRecoverySite creates a RecoverySite with the given ExecutionContext.
//
// The ExecutionContext must have a non-nil Root.
func NewRecoverySite(ctx *ExecutionContext) *RecoverySite {
	return &RecoverySite{ctx: ctx}
}

// --- Published methods ---

// ArchiveData writes bytes to a file in the recovery directory.
//
// Parameters:
//   - data: Bytes to archive
//
// Returns:
//   - string: opaque recovery ID for tombstone storage
//   - error: any write error
func (s *RecoverySite) ArchiveData(data []byte) (string, error) {

	if err := s.ctx.Root.MkdirAll(s.ctx.Root.NewPath(recoveryDir), 0o700); err != nil {
		return "", fmt.Errorf("create recovery directory: %w", err)
	}

	recoveryID := recoveryDir + "/" + uuid.New().String()

	if err := s.ctx.Root.WriteFile(s.ctx.Root.NewPath(recoveryID), data, 0o600); err != nil {
		return "", err
	}

	return recoveryID, nil
}

// ArchiveFile moves a file to recovery via zero-copy rename.
//
// No data is copied — the file's directory entry is relocated. Takes [Path] for the user-facing location. Returns an
// opaque recovery ID for tombstone storage.
//
// Parameters:
//   - p: Path of the file to archive
//
// Returns:
//   - string: opaque recovery ID for tombstone storage
//   - error: any rename error
func (s *RecoverySite) ArchiveFile(p Path) (string, error) {

	// Normalize: ensure the Path has root context for confined I/O. Resources created via coercion
	// (NewResource → op.NewPath("", abs)) carry root="" and rel=<absolute>, which os.Root rejects.
	p = s.ctx.Root.NewPath(p.Abs())

	if err := s.ctx.Root.MkdirAll(s.ctx.Root.NewPath(recoveryDir), 0o700); err != nil {
		return "", fmt.Errorf("create recovery directory: %w", err)
	}

	recoveryID := recoveryDir + "/" + uuid.New().String()

	if err := s.ctx.Root.Rename(p, s.ctx.Root.NewPath(recoveryID)); err != nil {
		return "", err
	}

	return recoveryID, nil
}

// RestoreData reads bytes back from a file in the recovery directory.
//
// Parameters:
//   - recoveryID: Opaque recovery ID returned by ArchiveData
//
// Returns:
//   - []byte: archived data
//   - error: any read error
func (s *RecoverySite) RestoreData(recoveryID string) ([]byte, error) {

	data, err := s.ctx.Root.ReadFile(s.ctx.Root.NewPath(recoveryID))
	if err != nil {
		return nil, fmt.Errorf("read recovery data: %w", err)
	}

	return data, nil
}

// RestoreFile moves a file back from recovery via zero-copy rename.
//
// No data is copied — the directory entry is relocated back. The parent directory of original is recreated if it was
// pruned after archival.
//
// Parameters:
//   - original: Path where the file should be restored
//   - recoveryID: Opaque recovery ID returned by ArchiveFile
//
// Returns:
//   - error: any rename error
func (s *RecoverySite) RestoreFile(original Path, recoveryID string) error {

	// Normalize: ensure the Path has root context for confined I/O.
	original = s.ctx.Root.NewPath(original.Abs())

	if original.Rel() == "" || recoveryID == "" {
		return fmt.Errorf("invalid recovery state: missing path metadata")
	}

	recPath := s.ctx.Root.NewPath(recoveryID)

	if _, err := s.ctx.Root.Lstat(recPath); errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("recovery source not found: %s", recoveryID)
	}

	parentDir := recoveryParentDir(original.Rel())
	if err := s.ctx.Root.MkdirAll(s.ctx.Root.NewPath(parentDir), 0o755); err != nil {
		return fmt.Errorf("recreate parent directory: %w", err)
	}

	if err := s.ctx.Root.Rename(recPath, original); err != nil {
		return fmt.Errorf("restore from recovery: %w", err)
	}

	return nil
}

// recoveryParentDir returns the parent directory of a root-relative path.
//
// Uses simple string splitting to avoid filepath.Dir's absolute path normalization.
func recoveryParentDir(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[:i]
		}
	}
	return "."
}
