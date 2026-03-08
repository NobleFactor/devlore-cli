// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package file provides filesystem actions for the operation graph.
package file

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
)

// isDirAndNotEmpty checks if the path is a directory that contains at least one entry.
//
// It returns true if it is a directory with contents, false if it's a file, a symlink, or an empty directory. Check
// for existence on error return using errors.Is(err, os.ErrNotExist).
//
// Parameters:
//   - path: The path to check
//
// Returns:
//   - bool: true if the path is a directory with contents, false otherwise
//   - error: If the path cannot be opened; verify the existence on error return using errors.Is(err, os.ErrNotExist).
func isDirAndNotEmpty(path string) (bool, error) {

	f, err := os.Open(path)

	if err != nil {
		// If we can't open it, we can't confirm it's a non-empty dir
		return false, err
	}

	defer f.Close()

	// Check the file info

	fileInfo, err := f.Stat()

	if err != nil {
		return false, err
	}

	// If it's not a directory, the guard doesn't apply (it's "not a non-empty dir")

	if !fileInfo.IsDir() {
		return false, nil
	}

	// Read just one entry and just the name, not all file info as Readdir does

	_, err = f.Readdirnames(1)

	if err != nil {
		if errors.Is(err, io.EOF) {
			return false, nil // the directory is empty
		}
		return false, err // unexpected error
	}

	return true, nil // the directory contains at least 1 element, it's not empty
}

// isSubpath returns true if the give path is under parent (not equal to).
//
// Parameters:
//   - path: The path to check
//   - parent: The parent path
//
// Returns: true if the path is under the parent, false otherwise
func isSubpath(path, parent string) bool {
	rel, err := filepath.Rel(parent, path)
	if err != nil {
		return false
	}
	// Must not start with ".." and must not be "."
	return rel != "." && !filepath.IsAbs(rel) && (len(rel) < 2 || rel[:2] != "..")
}

// checksumBytes computes "sha256:<hex>" for content bytes.
//
// Parameters:
//   - data: Bytes to checksum
//
// Returns: "sha256:<hex>"
func checksumBytes(data []byte) string {
	h := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(h[:])
}

// checksumFile reads a path and returns its "sha256:<hex>" checksum.
//
// Parameters:
//   - path: Absolute path to the file to read
//
// Returns: Empty string if the file cannot be read (e.g., directory, permission error).
func checksumFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return checksumBytes(data)
}
