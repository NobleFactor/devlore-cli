// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package file

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"reflect"
	"syscall"
	"time"

	"github.com/NobleFactor/devlore-cli/pkg/iox"
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
func NewResource(ctx *op.RuntimeEnvironment, value any) (*Resource, error) {

	path, ok := value.(string)
	if !ok {
		return nil, fmt.Errorf("file.Resource: expected string, got %T", value)
	}

	parsed, err := url.Parse(path)
	if err != nil {
		return nil, fmt.Errorf("file.Resource: invalid input %q: %w", path, err)
	}

	if parsed.Scheme != "" && parsed.Scheme != "file" {
		return nil, fmt.Errorf("file.Resource: expected file scheme, got %q in %q", parsed.Scheme, path)
	}

	if parsed.Scheme == "file" {

		if parsed.User != nil {
			return nil, fmt.Errorf("file.Resource: userinfo not permitted in %q", path)
		}

		if parsed.Host != "" && parsed.Host != "localhost" {
			return nil, fmt.Errorf("file.Resource: unexpected host %q in %q", parsed.Host, path)
		}

		if parsed.RawQuery != "" {
			return nil, fmt.Errorf("file.Resource: query not permitted in %q", path)
		}

		if parsed.Fragment != "" {
			return nil, fmt.Errorf("file.Resource: fragment not permitted in %q", path)
		}

		if parsed.Opaque != "" {
			return nil, fmt.Errorf("file.Resource: opaque form not permitted in %q; use file:///path", path)
		}

		path = parsed.Path
	}

	sourcePath := ctx.Root.NewPath(path)

	base, err := op.NewResourceBase(ctx, "file://"+sourcePath.Abs(), reflect.TypeFor[*Resource]())
	if err != nil {
		return nil, err
	}

	return &Resource{
		ResourceBase: base,
		SourcePath:   sourcePath,
	}, nil
}

// region EXPORTED METHODS

// region State management

// Addressing reports that file.Resource is location-keyed: identity is the path on disk, and bytes at that path
// are mutable. The catalog uses [op.AddressingLocation] semantics — content drift triggers shadow chains, not
// new URIs.
func (r *Resource) Addressing() op.AddressingMode {
	return op.AddressingLocation
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

// Equal reports whether r and other identify the same file resource.
//
// Strict equality: other must be a *file.Resource (not merely an [op.Resource] with the same URI). Once the type
// check passes, URI comparison is delegated to [op.ResourceBase.Equal]. A cross-type URI collision (e.g., a
// file URI embedded in an appnet.Resource) fails at the type check rather than matching spuriously.
//
// Parameters:
//   - other: the value to compare against; may be any, including nil or a non-Resource.
//
// Returns:
//   - bool: true if other is a *file.Resource with the same URI as r.
func (r *Resource) Equal(other any) bool {

	if other == nil {
		return false
	}

	if _, ok := other.(*Resource); !ok {
		return false
	}

	return r.ResourceBase.Equal(other)
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

// Exists returns true if the resource has been resolved and the file existed
// at resolve time. An unresolved resource always reports Exists() == false.
func (r *Resource) Exists() bool {
	return !r.ModTime.IsZero()
}

// String returns a debug-oriented single-line representation of the resource suitable for log lines and IDE
// debug windows.
//
// Returns:
//   - string: "file.Resource{uri=<URI>, exists=<bool>, size=<bytes>, mode=<file-mode>}".
func (r *Resource) String() string {
	return fmt.Sprintf("file.Resource{uri=%s, exists=%t, size=%d, mode=%v}",
		r.URI(), r.Exists(), r.Size, r.Mode)
}

// endregion

// region Behaviors

// Refresh re-populates the resource's stat-derived metadata by performing a fresh stat. Call after any successful
// physical mutation.
//
// Returns:
//   - error: any stat error.
func (r *Resource) Refresh() error {

	root := r.RuntimeEnvironment().Root
	info, err := root.Stat(root.NewPath(r.SourcePath.Abs()))
	if err != nil {
		return err
	}

	return r.refreshWith(info)
}

// Resolve rebinds the source path to the execution root and populates metadata via stat. The path is canonical from
// construction; rebinding updates Rel for confined I/O under the execution root. If the file does not exist, Resolve
// returns nil and metadata remains empty ([Resource.Exists] returns false).
//
// Returns:
//   - error: any stat error (not-exist is not an error).
func (r *Resource) Resolve() error {

	root := r.RuntimeEnvironment().Root

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

	return nil
}

// UnmarshalJSON populates the receiver from a JSON-encoded string (a file path or file URI).
//
// The caller pre-seeds the receiver's embedded [op.ResourceBase] with a valid [op.RuntimeEnvironment] before
// invoking this method; all domain-specific fields are then overwritten by the reconstructed resource.
//
// Parameters:
//   - data: JSON-encoded string containing the resource's URI or path.
//
// Returns:
//   - error: non-nil if the RuntimeEnvironment is missing, the JSON does not decode as a string, or resource
//     construction fails.
func (r *Resource) UnmarshalJSON(data []byte) error {

	if r.RuntimeEnvironment() == nil {
		return errors.New("file.Resource: UnmarshalJSON requires RuntimeEnvironment on receiver")
	}

	var uri string
	if err := json.Unmarshal(data, &uri); err != nil {
		return err
	}

	built, err := NewResource(r.RuntimeEnvironment(), uri)
	if err != nil {
		return err
	}

	*r = *built
	return nil
}

// UnmarshalText populates the receiver from raw UTF-8 bytes containing a file path or file URI.
//
// The caller pre-seeds the receiver's embedded [op.ResourceBase] with a valid [op.RuntimeEnvironment] before
// invoking this method; all domain-specific fields are then overwritten by the reconstructed resource.
//
// Parameters:
//   - text: UTF-8 bytes containing the resource's URI or path.
//
// Returns:
//   - error: non-nil if the RuntimeEnvironment is missing or resource construction fails.
func (r *Resource) UnmarshalText(text []byte) error {

	if r.RuntimeEnvironment() == nil {
		return errors.New("file.Resource: UnmarshalText requires RuntimeEnvironment on receiver")
	}

	built, err := NewResource(r.RuntimeEnvironment(), string(text))
	if err != nil {
		return err
	}

	*r = *built
	return nil
}

// UnmarshalYAML populates the receiver from a YAML scalar (a file path or file URI).
//
// The caller pre-seeds the receiver's embedded [op.ResourceBase] with a valid [op.RuntimeEnvironment] before
// invoking this method; all domain-specific fields are then overwritten by the reconstructed resource.
//
// Parameters:
//   - unmarshal: callback supplied by the YAML decoder that projects the current node into the given target.
//
// Returns:
//   - error: non-nil if the RuntimeEnvironment is missing, the YAML node does not decode as a string, or resource
//     construction fails.
func (r *Resource) UnmarshalYAML(unmarshal func(any) error) error {

	if r.RuntimeEnvironment() == nil {
		return errors.New("file.Resource: UnmarshalYAML requires RuntimeEnvironment on receiver")
	}

	var uri string
	if err := unmarshal(&uri); err != nil {
		return err
	}

	built, err := NewResource(r.RuntimeEnvironment(), uri)
	if err != nil {
		return err
	}

	*r = *built
	return nil
}

// endregion

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// refreshWith updates the Resource's stat-derived metadata.
func (r *Resource) refreshWith(info os.FileInfo) error {
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

	return nil
}

// endregion

// endregion