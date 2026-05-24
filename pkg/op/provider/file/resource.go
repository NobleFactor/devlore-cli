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
	"io/fs"
	"net/url"
	"os"
	"reflect"
	"syscall"

	"github.com/NobleFactor/devlore-cli/pkg/iox"
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// Resource represents a handle to a file on the disk identified by its path.
//
// Resource carries identity only: the URI (derived from the absolute path) and the [op.Path] handle. Runtime-observed
// state — size, mode, mod-time, inode, device, existence — lives on a separate [*Observation] minted by
// [Provider.Observe]; the framework owns observation storage so a buggy provider cannot corrupt the catalog by mutating
// fields on a shared [*Resource] pointer.
type Resource struct {
	op.ResourceBase

	// SourcePath is the canonicalized absolute path on the disk. Set at construction by [buildCandidate] (which routes
	// the input through `RuntimeEnvironment.Root.NewPath`); rebound to the live execution root by [Resource.Resolve]
	// when the run-time root differs from the construction-time root.
	SourcePath op.Path
}

// NewResource constructs a file.Resource and claims production via [op.ResourceCatalog.GetOrCreate].
//
// Use NewResource from a producer dispatch context — typically a provider method that has received an
// [op.ActivationRecord] from the framework. The returned Resource is the canonical catalog entry, stamped with
// `producerID = activationRecord.Unit.ID()` (or empty when `Unit` is nil for non-graph dispatch). Use
// [DiscoverResource] instead when the caller is not claiming production (rehydration, reference handles, scanner-style
// discovery, the framework's slot-coercion adapter).
//
// File internals that need a *Resource without interning (prepareWrite for the backup, helper construction in
// `closestExistingDir` and `resources`, etc.) call the private [buildCandidate] directly.
//
// Nil-Catalog tolerance mirrors [DiscoverResource]: when `activationRecord.RuntimeEnvironment.Catalog` is nil (test
// fixtures, library callers without a runtime), the candidate is returned unlinked.
//
// Parameters:
//   - `activationRecord`: the per-dispatch activation; its `RuntimeEnvironment` carries the runtime environment and
//     its `Unit.ID()` becomes the catalog entry's producerID (empty when `Unit` is nil). Must be non-nil.
//   - `value`: a string file path or file URI.
//
// Returns:
//   - *Resource: the canonical catalog entry (or the unlinked candidate when no catalog is present).
//   - `error`: if `value` is not a string, or the input violates RFC 8089 when in file URI form, or
//     [op.ResourceCatalog.GetOrCreate]'s strict assertions fail.
func NewResource(activationRecord *op.ActivationRecord, value any) (*Resource, error) {

	candidate, err := buildCandidate(activationRecord.RuntimeEnvironment, value)
	if err != nil {
		return nil, err
	}

	if activationRecord.RuntimeEnvironment.Catalog == nil {
		return candidate, nil
	}

	got, err := activationRecord.RuntimeEnvironment.Catalog.GetOrCreate(
		activationRecord, candidate.URI(), func() (op.Resource, error) { return candidate, nil },
	)
	if err != nil {
		return nil, err
	}

	canonical, ok := got.(*Resource)
	if !ok {
		return nil, fmt.Errorf(
			"file.NewResource: catalog entry for %q is %T, want *file.Resource",
			candidate.URI(), got,
		)
	}

	return canonical, nil
}

// DiscoverResource registers a file.Resource via [op.ResourceCatalog.Discover] without claiming production.
//
// Used by the framework's resource registry adapter for slot coercion (when starlark supplies a string and the slot
// expects a *file.Resource), and by callers that hold a reference handle without claiming to have produced the
// underlying file (UnmarshalJSON/Text/YAML rehydration, WalkTree's per-entry construction, scanner-style preflight
// passes).
//
// `activationRecord` is required for signature symmetry with [NewResource], but only its `RuntimeEnvironment` is
// consumed — `Unit` is unused since Discover doesn't stamp a producer. Discovery callers commonly construct one as
// `op.NewActivationRecord(nil, nil, ctx)` — both `Graph` and `Unit` nil.
//
// Nil-Catalog tolerance: returns the unlinked candidate when no catalog is present.
//
// Parameters:
//   - `activationRecord`: the per-dispatch activation; only its `RuntimeEnvironment` is consumed (Discover does not
//     stamp a producer). Must be non-nil.
//   - `value`: a string file path or file URI.
//
// Returns:
//   - *Resource: the canonical catalog entry (or the unlinked candidate when no catalog is present).
//   - `error`: if `value` is not a string, or the input violates RFC 8089 when in file URI form, or
//     [op.ResourceCatalog.Discover]'s strict assertions fail.
func DiscoverResource(activationRecord *op.ActivationRecord, value any) (*Resource, error) {

	candidate, err := buildCandidate(activationRecord.RuntimeEnvironment, value)
	if err != nil {
		return nil, err
	}

	if activationRecord.RuntimeEnvironment.Catalog == nil {
		return candidate, nil
	}

	got, err := activationRecord.RuntimeEnvironment.Catalog.Discover(candidate.URI(), func() (op.Resource, error) {
		return candidate, nil
	})
	if err != nil {
		return nil, err
	}

	canonical, ok := got.(*Resource)
	if !ok {
		return nil, fmt.Errorf("file.DiscoverResource: catalog entry for %q is %T, want *file.Resource", candidate.URI(), got)
	}

	return canonical, nil
}

// buildCandidate validates value, parses any file URI per RFC 8089, and constructs a [file.Resource].
//
// This function does not touch the resource catalog. It is shared by [file.NewResource], [file.DiscoverResource], and
// internal helpers that need a Resource without interning (e.g., `prepareWrite` when backing up an existing target
// before overwriting it).
//
// Parameters:
//   - `runtimeEnvironment`: the session's runtime environment; supplies `Root` for path canonicalization and is
//     embedded via [op.NewResourceBase].
//   - `value`: an `any` carrying a string file path or file URI; other dynamic types are rejected.
//
// Returns:
//   - *Resource: the constructed candidate. Not interned in the catalog — callers ([NewResource] / [DiscoverResource])
//     route it through [op.ResourceCatalog] themselves.
//   - `error`: non-nil if `value` is not a string, the input violates RFC 8089 when in file URI form (non-file scheme,
//     userinfo, non-localhost host, query, fragment, or opaque form), or [op.NewResourceBase] fails.
func buildCandidate(runtimeEnvironment *op.RuntimeEnvironment, value any) (resource *Resource, err error) {

	path, ok := value.(string)
	if !ok {
		return nil, fmt.Errorf("file.Resource: expected string, got %T", value)
	}

	var parsed *url.URL

	parsed, err = url.Parse(path)
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

	sourcePath := runtimeEnvironment.Root.NewPath(path)
	var base op.ResourceBase

	base, err = op.NewResourceBase(runtimeEnvironment, "file://"+sourcePath.Abs(), reflect.TypeFor[*Resource]())
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

// Addressing reports that file.Resource is location-keyed.
//
// Identity is the path on the disk, and bytes at that path are mutable. The catalog uses [op.AddressingLocation]
// semantics. Content drift triggers shadow chains, not new URIs.
//
// Returns:
//   - `AddressingMode`: always [op.AddressingLocation].
func (r *Resource) Addressing() op.AddressingMode {
	return op.AddressingLocation
}

// Digest returns the honest content hash: sha256 of the file's bytes, streamed (no full-file allocation).
//
// Always fresh: opens and reads the file at call time. Errors with [op.ErrUnimplemented] for directories; the catch-all
// file.Resource pre-dates the taxonomic split into Regular / Directory / Link variants, and directory hashing requires
// a [Merkle-root scheme] deferred until that split (step 22).
//
// Returns:
//   - `op.Digest`: sha256 algorithm with 32 raw bytes.
//   - `error`: a stat error, [op.ErrUnimplemented] for directories, or any read error.
//
// [Merkle-root scheme]: https://en.wikipedia.org/wiki/Merkle_signature_scheme
func (r *Resource) Digest() (digest op.Digest, err error) {

	root := r.RuntimeEnvironment().Root
	path := root.NewPath(r.SourcePath.Abs())

	var info fs.FileInfo

	info, err = root.Stat(path)
	if err != nil {
		return op.Digest{}, fmt.Errorf("file.Resource: digest stat %s: %w", r.SourcePath.Abs(), err)
	}

	if info.IsDir() {
		return op.Digest{}, fmt.Errorf("file.Resource: digest of directory %s: %w", r.SourcePath.Abs(), op.ErrUnimplemented)
	}

	var f fs.File

	f, err = root.Open(path)
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

// Equal reports whether `r` and `other` identify the same file resource.
//
// Strict equality: `other` must be a *file.Resource (not merely an [op.Resource] with the same URI). Once the type
// check passes, URI comparison is delegated to [op.ResourceBase.Equal]. A cross-type URI collision (e.g., a file URI
// embedded in an appnet.Resource) fails at the type check rather than matching spuriously.
//
// Parameters:
//   - `other`: the value to compare against; may be `any`, including nil or a non-Resource.
//
// Returns:
//   - `bool`: true if `other` is a *file.Resource with the same URI as `r`.
func (r *Resource) Equal(other any) bool {

	if other == nil {
		return false
	}

	if _, ok := other.(*Resource); !ok {
		return false
	}

	return r.ResourceBase.Equal(other)
}

// Etag returns an inexpensive stat-derived change-detection token.
//
// Always fresh: stats the file at call time. The catalog uses Etag as an inexpensive signal that triggers a full
// [Resource.Digest] comparison. It is a sha256 of (size, mtime_ns, ino) packed into a little-endian byte array encoded
// as a lowercase hex string.
//
// Returns:
//   - `string`: lowercase hex sha256 of the packed stat tuple.
//   - `error`: any stat error (file gone, permission denied, etc.).
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

// Exists reports whether the file exists on disk at the time of the call.
//
// Self-stat: performs a fresh stat at every call rather than reading any cached field. For richer metadata (size, mode,
// mod-time, etc.) call [Provider.Observe] which returns a [*Observation].
//
// Returns:
//   - `bool`: true when the file exists; false when the stat returns [os.ErrNotExist] or any other error.
func (r *Resource) Exists() bool {
	root := r.RuntimeEnvironment().Root
	_, err := root.Stat(root.NewPath(r.SourcePath.Abs()))
	return err == nil
}

// IsDir reports whether the file at this resource's path is a directory at the time of the call.
//
// Self-stat. Returns false for any stat error (not-exist, permission denied, etc.) — callers that need to distinguish
// "missing" from "not a directory" should call [Provider.Observe] and check `obs.Exists` and `obs.Mode.IsDir()`
// separately.
//
// Returns:
//   - `bool`: true when the file exists and is a directory; false otherwise.
func (r *Resource) IsDir() bool {

	root := r.RuntimeEnvironment().Root
	info, err := root.Stat(root.NewPath(r.SourcePath.Abs()))

	if err != nil {
		return false
	}

	return info.IsDir()
}

// String returns a debug-oriented single-line representation of the resource.
//
// Suitable for log lines and IDE debug windows. Identity-only — observation-shaped data (size, mode, mod-time) is not
// on the Resource. Use [Provider.Observe] to capture observation values and log those alongside the Resource when
// needed.
//
// Returns:
//   - `string`: `file.Resource{uri=<URI>, source_path=<path>}`.
func (r *Resource) String() string {
	return fmt.Sprintf("file.Resource{uri=%s, source_path=%s}", r.URI(), r.SourcePath.Abs())
}

// endregion

// region Behaviors

// Resolve rebinds the source path to the execution root and verifies the file exists.
//
// The path is canonical from construction; rebinding updates Rel for confined I/O under the execution root. If the
// file does not exist, Resolve returns nil — existence is observation, not identity, and `not-exist` is a valid
// observation outcome. Other stat failures (permission denied, I/O error) surface as errors.
//
// Resolve does not populate any observation-shaped metadata on the Resource. Callers that need metadata call
// [Provider.Observe] to get an [Observation] value the framework can catalog.
//
// Returns:
//   - `error`: any stat error other than not-exist.
func (r *Resource) Resolve() error {

	root := r.RuntimeEnvironment().Root

	r.SourcePath = root.NewPath(r.SourcePath.Abs())

	_, err := root.Stat(r.SourcePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("failed to stat: %w", err)
	}

	return nil
}

// UnmarshalJSON populates the receiver from a JSON-encoded string (a file path or file URI).
//
// The caller pre-seeds the receiver's embedded [op.ResourceBase] with a valid [op.RuntimeEnvironment] before invoking
// this method; all domain-specific fields are then overwritten by the reconstructed resource.
//
// Parameters:
//   - `data`: JSON-encoded string containing the resource's URI or path.
//
// Returns:
//   - `error`: non-nil if the RuntimeEnvironment is missing, the JSON does not decode as a string, or resource
//     construction fails.
func (r *Resource) UnmarshalJSON(data []byte) error {

	if r.RuntimeEnvironment() == nil {
		return errors.New("file.Resource: UnmarshalJSON requires RuntimeEnvironment on receiver")
	}

	var uri string

	if err := json.Unmarshal(data, &uri); err != nil {
		return err
	}

	built, err := DiscoverResource(op.NewActivationRecord(nil, nil, r.RuntimeEnvironment()), uri)
	if err != nil {
		return err
	}

	*r = *built
	return nil
}

// UnmarshalText populates the receiver from raw UTF-8 bytes containing a file path or file URI.
//
// The caller pre-seeds the receiver's embedded [op.ResourceBase] with a valid [op.RuntimeEnvironment] before invoking
// this method; all domain-specific fields are then overwritten by the reconstructed resource.
//
// Parameters:
//   - `text`: UTF-8 bytes containing the resource's URI or path.
//
// Returns:
//   - `error`: non-nil if the RuntimeEnvironment is missing or resource construction fails.
func (r *Resource) UnmarshalText(text []byte) error {

	if r.RuntimeEnvironment() == nil {
		return errors.New("file.Resource: UnmarshalText requires RuntimeEnvironment on receiver")
	}

	built, err := DiscoverResource(op.NewActivationRecord(nil, nil, r.RuntimeEnvironment()), string(text))
	if err != nil {
		return err
	}

	*r = *built
	return nil
}

// UnmarshalYAML populates the receiver from a YAML scalar (a file path or file URI).
//
// The caller pre-seeds the receiver's embedded [op.ResourceBase] with a valid [op.RuntimeEnvironment] before invoking
// this method; all domain-specific fields are then overwritten by the reconstructed resource.
//
// Parameters:
//   - `unmarshal`: callback supplied by the YAML decoder that projects the current node into the given target.
//
// Returns:
//   - `error`: non-nil if the RuntimeEnvironment is missing, the YAML node does not decode as a string, or resource
//     construction fails.
func (r *Resource) UnmarshalYAML(unmarshal func(any) error) error {

	if r.RuntimeEnvironment() == nil {
		return errors.New("file.Resource: UnmarshalYAML requires RuntimeEnvironment on receiver")
	}

	var uri string

	if err := unmarshal(&uri); err != nil {
		return err
	}

	built, err := DiscoverResource(op.NewActivationRecord(nil, nil, r.RuntimeEnvironment()), uri)
	if err != nil {
		return err
	}

	*r = *built
	return nil
}

// endregion

// endregion
