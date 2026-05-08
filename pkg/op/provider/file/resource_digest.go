// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package file

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"syscall"

	"github.com/NobleFactor/devlore-cli/pkg/iox"
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// region EXPORTED METHODS

// region Behaviors

// Addressing reports that file.Resource is location-keyed: identity is the path on disk, and bytes at that path
// are mutable. The catalog uses [op.AddressingLocation] semantics — content drift triggers shadow chains, not
// new URIs.
func (r *Resource) Addressing() op.AddressingMode {
	return op.AddressingLocation
}

// Etag returns a cheap stat-derived change-detection token: sha256 of (size, mtime_ns, ino) packed
// little-endian. Always fresh — stats the file at call time, ignoring any resolved-time fields cached on the
// Resource. The catalog uses Etag as a cheap signal; mismatch triggers a full [Resource.Digest] comparison.
//
// Returns:
//   - string: lowercase hex sha256 of the packed stat tuple.
//   - error: any stat error (file gone, permission denied, etc.).
func (r *Resource) Etag() (string, error) {

	root := r.RuntimeEnvironment().Root

	info, err := root.Stat(root.NewPath(r.SourcePath.Abs()))
	if err != nil {
		return "", fmt.Errorf("file.Resource: etag stat %s: %w", r.SourcePath.Abs(), err)
	}

	var inode uint64
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		inode = stat.Ino
	}

	var buf [24]byte
	binary.LittleEndian.PutUint64(buf[0:8], uint64(info.Size())) //nolint:gosec // file sizes are non-negative
	binary.LittleEndian.PutUint64(buf[8:16], uint64(info.ModTime().UnixNano()))
	binary.LittleEndian.PutUint64(buf[16:24], inode)

	h := sha256.Sum256(buf[:])
	return hex.EncodeToString(h[:]), nil
}

// Digest returns the honest content hash: sha256 of the file's bytes, streamed (no full-file allocation).
// Always fresh — opens and reads the file at call time. Errors with [op.ErrUnimplemented] for directories;
// the catch-all file.Resource pre-dates the taxonomic split into Regular / Directory / Link variants and
// directory hashing requires a Merkle-root scheme deferred until that split (step 22).
//
// Returns:
//   - op.Digest: sha256 algorithm with 32 raw bytes.
//   - error: a stat error, [op.ErrUnimplemented] for directories, or any read error.
func (r *Resource) Digest() (op.Digest, error) {

	root := r.RuntimeEnvironment().Root
	path := root.NewPath(r.SourcePath.Abs())

	info, err := root.Stat(path)
	if err != nil {
		return op.Digest{}, fmt.Errorf("file.Resource: digest stat %s: %w", r.SourcePath.Abs(), err)
	}

	if info.IsDir() {
		return op.Digest{}, fmt.Errorf("file.Resource: digest of directory %s: %w", r.SourcePath.Abs(), op.ErrUnimplemented)
	}

	f, err := root.Open(path)
	if err != nil {
		return op.Digest{}, fmt.Errorf("file.Resource: digest open %s: %w", r.SourcePath.Abs(), err)
	}
	defer iox.Close(&err, f)

	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return op.Digest{}, fmt.Errorf("file.Resource: digest read %s: %w", r.SourcePath.Abs(), err)
	}

	return op.Digest{Algorithm: "sha256", Bytes: h.Sum(nil)}, nil
}

// endregion

// endregion