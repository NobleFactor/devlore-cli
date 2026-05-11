// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"

	"github.com/google/uuid"

	"github.com/NobleFactor/devlore-cli/pkg/iox"
)

const recoveryDir = ".devlore/recovery"

// ErrRecoverySourceNotFound is returned by [RecoverySite.RestoreFile] when no archive exists for the supplied
// recoveryID.
//
// Compensation paths use this sentinel to distinguish "the receipt was committed but no archive was made"
// (a forward action that created new state without displacing existing state) from genuine restore failures.
// In the former case the caller silently treats the missing archive as a no-op; in the latter it propagates.
var ErrRecoverySourceNotFound = errors.New("recovery source not found")

// RecoverySite manages archival and restoration of resources within the authority boundary.
//
// All operations use zero-copy renames for files and byte serialization for data. The recovery directory is
// .devlore/recovery/ within the op.Root authority boundary. All I/O goes through RuntimeEnvironment.Root.
type RecoverySite struct {
	ctx *RuntimeEnvironment
}

// NewRecoverySite creates a RecoverySite with the given RuntimeEnvironment.
//
// The RuntimeEnvironment must have a non-nil Root.
func NewRecoverySite(runtimeEnvironment *RuntimeEnvironment) *RecoverySite {
	return &RecoverySite{ctx: runtimeEnvironment}
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

	id := uuid.Must(uuid.NewV7()).String()
	recoveryPath := recoveryDir + "/" + id

	if err := s.ctx.Root.WriteFile(s.ctx.Root.NewPath(recoveryPath), data, 0o600); err != nil {
		return "", err
	}

	return id, nil
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

	id := uuid.Must(uuid.NewV7()).String()
	recoveryPath := recoveryDir + "/" + id

	if err := s.ctx.Root.Rename(p, s.ctx.Root.NewPath(recoveryPath)); err != nil {
		return "", err
	}

	return id, nil
}

// ArchiveStream copies a reader into the recovery directory chunk-by-chunk.
//
// Use when the content source is an [io.Reader] — e.g., an HTTP response body — and should not be buffered
// fully in memory. The reader is drained via [io.Copy] into a freshly created file at
// .devlore/recovery/<uuid>. Returns the opaque recovery ID for tombstone storage or later consumption via
// memory-mapped access.
//
// Parameters:
//   - r: source reader; drained until EOF.
//
// Returns:
//   - string: opaque recovery ID for tombstone storage.
//   - error:  any error from recovery-directory creation, file creation, or the copy.
func (s *RecoverySite) ArchiveStream(r io.Reader) (_ string, err error) {

	if err := s.ctx.Root.MkdirAll(s.ctx.Root.NewPath(recoveryDir), 0o700); err != nil {
		return "", fmt.Errorf("create recovery directory: %w", err)
	}

	id := uuid.Must(uuid.NewV7()).String()
	recoveryPath := recoveryDir + "/" + id

	f, err := s.ctx.Root.OpenFile(s.ctx.Root.NewPath(recoveryPath), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return "", fmt.Errorf("create recovery file: %w", err)
	}
	defer iox.Close(&err, f)

	if _, err := io.Copy(f, r); err != nil {
		return "", fmt.Errorf("stream to recovery: %w", err)
	}

	return id, nil
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

	recoveryPath := recoveryDir + "/" + recoveryID
	data, err := s.ctx.Root.ReadFile(s.ctx.Root.NewPath(recoveryPath))
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

	recoveryPath := recoveryDir + "/" + recoveryID
	recPath := s.ctx.Root.NewPath(recoveryPath)

	if _, err := s.ctx.Root.Lstat(recPath); errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("%w: %s", ErrRecoverySourceNotFound, recoveryID)
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
