// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package file provides filesystem actions for the operation graph.
package file

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
)

// isSubpath returns true if path is under parent (not equal to).
func isSubpath(path, parent string) bool {
	rel, err := filepath.Rel(parent, path)
	if err != nil {
		return false
	}
	// Must not start with ".." and must not be "."
	return rel != "." && !filepath.IsAbs(rel) && (len(rel) < 2 || rel[:2] != "..")
}

// pruneParents removes empty parent directories up to the boundary.
func pruneParents(path string, prune bool, boundary string) {
	if !prune || boundary == "" {
		return
	}

	boundary = filepath.Clean(boundary)
	dir := filepath.Dir(path)

	for {
		if dir == boundary || !isSubpath(dir, boundary) {
			return
		}
		if err := os.Remove(dir); err != nil {
			return // Not empty or permission error
		}
		dir = filepath.Dir(dir)
	}
}

// checksumBytes computes "sha256:<hex>" for content bytes.
func checksumBytes(data []byte) string {
	h := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(h[:])
}

// checksumFile reads path and returns its "sha256:<hex>" checksum.
// Returns empty string if the file cannot be read (e.g., directory, permission error).
func checksumFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return checksumBytes(data)
}
