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

// isDirAndNotEmpty checks if the path is a directory that contains at least one entry.
//
// It returns true if it is a directory with contents, false if it's a file, a symlink, or an empty directory. Check
// for existence on error return using errors.Is(err, os.ErrNotExist).
//
// Parameters:
//   - abs: Absolute path to check
//   - root: OS root for scoped I/O (nil falls back to os.Open)
//
// Returns:
//   - bool: true if the path is a directory with contents, false otherwise
//   - error: If the path cannot be opened; verify the existence on error return using errors.Is(err, os.ErrNotExist).
func isDirAndNotEmpty(abs string, root *os.Root) (bool, error) {

	var f *os.File
	var err error

	if rel := rootRel(root, abs); rel != "" {
		f, err = root.Open(rel)
	} else {
		f, err = os.Open(abs)
	}

	if err != nil {
		return false, err
	}

	defer f.Close()

	fileInfo, err := f.Stat()
	if err != nil {
		return false, err
	}

	if !fileInfo.IsDir() {
		return false, nil
	}

	_, err = f.Readdirnames(1)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

// rootRel computes the root-relative path from an absolute path. Returns empty string when root is nil or the path
// cannot be made relative to root.
//
// Parameters:
//   - root: OS root (nil returns empty)
//   - abs: Absolute path to compute relative to root
//
// Returns:
//   - string: root-relative path, or empty string when root is nil or abs is outside root
func rootRel(root *os.Root, abs string) string {

	if root == nil {
		return ""
	}

	rel, err := filepath.Rel(root.Name(), abs)
	if err != nil || rel == ".." || (len(rel) > 2 && rel[:3] == "../") {
		return ""
	}

	return rel
}
