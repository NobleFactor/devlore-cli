// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package recovery

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

const recoveryDir = ".devlore/recovery"

// Site manages archival and restoration of resources within the authority
// boundary. All operations use zero-copy renames for files and byte
// serialization for data. The recovery directory is .devlore/recovery/
// within the base directory.
type Site struct {
	baseDir string
}

// NewSite creates a Site rooted at the given base directory.
func NewSite(baseDir string) *Site {
	return &Site{baseDir: baseDir}
}

// ArchiveData writes bytes to a file in the recovery directory.
// Returns the recovery path for tombstone storage.
func (s *Site) ArchiveData(data []byte) (string, error) {

	dir := filepath.Join(s.baseDir, recoveryDir)

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create recovery directory: %w", err)
	}

	recoveryPath := filepath.Join(dir, uuid.New().String())

	if err := os.WriteFile(recoveryPath, data, 0o600); err != nil {
		return "", err
	}

	return recoveryPath, nil
}

// ArchiveFile moves a file to recovery via zero-copy rename. No data is
// copied — the file's directory entry is relocated. Returns the recovery
// path for tombstone storage.
func (s *Site) ArchiveFile(path string) (string, error) {

	dir := filepath.Join(s.baseDir, recoveryDir)

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create recovery directory: %w", err)
	}

	recoveryPath := filepath.Join(dir, uuid.New().String())

	if err := os.Rename(path, recoveryPath); err != nil {
		return "", err
	}

	return recoveryPath, nil
}

// RestoreData reads bytes back from a file in the recovery directory.
func (s *Site) RestoreData(recoveryPath string) ([]byte, error) {

	data, err := os.ReadFile(recoveryPath)
	if err != nil {
		return nil, fmt.Errorf("read recovery data: %w", err)
	}

	return data, nil
}

// RestoreFile moves a file back from recovery via zero-copy rename. No
// data is copied — the directory entry is relocated back. The parent
// directory of originalPath is recreated if it was pruned after archival.
func (s *Site) RestoreFile(originalPath, recoveryPath string) error {

	if originalPath == "" || recoveryPath == "" {
		return fmt.Errorf("invalid recovery state: missing path metadata")
	}

	if _, err := os.Lstat(recoveryPath); errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("recovery source not found: %s", recoveryPath)
	}

	if err := os.MkdirAll(filepath.Dir(originalPath), 0o755); err != nil {
		return fmt.Errorf("recreate parent directory: %w", err)
	}

	if err := os.Rename(recoveryPath, originalPath); err != nil {
		return fmt.Errorf("restore from recovery: %w", err)
	}

	return nil
}
