// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package file

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// Regular is the taxonomy variant asserting that its path names a regular file (phase-8 step 23).
//
// The kind is declared intent, never stat-assigned (ruling 1): planning is offline, so the assertion is verified at
// use rather than at construction — [Regular.Digest] and [Regular.Etag] observe the disk with lstat semantics and
// error with a kind mismatch when the entry is anything else (ruling 5e). Identity is the embedded [Resource] (URI +
// SourcePath); runtime-observed metadata lives on [*Observation], exactly as for the base.
type Regular struct {
	Resource
}

// sealedEntry marks Regular as a member of the closed [Entry] set (step 23, slice 4).
func (*Regular) sealedEntry() {}

// NewRegular constructs a [file.Regular] and claims production via [op.ResourceCatalog.GetOrCreate].
//
// Use NewRegular from a producer dispatch context; the returned Regular is the canonical
// catalog entry, stamped with `producerID = unit.ID()` when `unit` is non-nil. A catalog entry already claimed under
// a different kind for the same URI is an error — cross-kind plan conflicts surface at the earliest moment.
// Nil-Catalog tolerance: the candidate is returned unlinked when no catalog is present.
//
// Parameters:
//   - `runtimeEnvironment`: the session runtime environment.
//   - `unit`: the producing [op.ExecutableUnit] whose ID becomes the catalog entry's producerID, or nil for
//     non-graph dispatch (the resulting entry carries an empty producer stamp).
//   - `value`: a string file path or file URI.
//
// Returns:
//   - `*Regular`: the canonical catalog entry (or the unlinked candidate when no catalog is present).
//   - `error`: if `value` is not a string, the input violates RFC 8089 when in file URI form, the catalog's strict
//     assertions fail, or the URI's existing entry is another kind.
func NewRegular(runtimeEnvironment *op.RuntimeEnvironment, unit op.ExecutableUnit, value any) (*Regular, error) {

	base, err := buildCandidateAs(runtimeEnvironment, value, reflect.TypeFor[*Regular]())
	if err != nil {
		return nil, err
	}

	return internEntry(runtimeEnvironment, unit, true, &Regular{Resource: *base})
}

// DiscoverRegular registers a [file.Regular] via [op.ResourceCatalog.Discover] without claiming production.
//
// The discovery counterpart of [NewRegular]: no producer is stamped, so no unit
// reference is taken. Nil-Catalog tolerance returns the unlinked candidate.
//
// Parameters:
//   - `runtimeEnvironment`: the session runtime environment.
//   - `value`: a string file path or file URI.
//
// Returns:
//   - `*Regular`: the canonical catalog entry (or the unlinked candidate when no catalog is present).
//   - `error`: if `value` is not a string, the input violates RFC 8089 when in file URI form, the catalog's strict
//     assertions fail, or the URI's existing entry is another kind.
func DiscoverRegular(runtimeEnvironment *op.RuntimeEnvironment, value any) (*Regular, error) {

	base, err := buildCandidateAs(runtimeEnvironment, value, reflect.TypeFor[*Regular]())
	if err != nil {
		return nil, err
	}

	return internEntry(runtimeEnvironment, nil, false, &Regular{Resource: *base})
}

// region EXPORTED METHODS

// region State management

// Digest returns the honest content hash: sha256 of the file's bytes, streamed (no full-file allocation).
//
// Always fresh: the disk is observed at call time. The entry itself must be a regular file — the kind check uses
// lstat semantics, so a symlink pointing at a regular file is kind symbolic-link, not kind regular — and any other
// observed kind errors with a kind mismatch (step 23, ruling 5e): the plan asserted one kind, the disk shows another.
//
// Returns:
//   - `op.Digest`: sha256 algorithm with 32 raw bytes.
//   - `error`: an lstat error, a kind mismatch, or any read error.
func (r *Regular) Digest() (op.Digest, error) {

	root := r.RuntimeEnvironment().Root

	info, err := root.Lstat(root.NewPath(r.SourcePath.Abs()))
	if err != nil {
		return op.Digest{}, fmt.Errorf("file.Regular: digest lstat %s: %w", r.SourcePath.Abs(), err)
	}

	if !info.Mode().IsRegular() {
		return op.Digest{}, kindMismatchError("file.Regular", r.SourcePath.Abs(), info.Mode())
	}

	digest, err := contentDigest(root, r.SourcePath.Abs())
	if err != nil {
		return op.Digest{}, fmt.Errorf("file.Regular: digest %s: %w", r.SourcePath.Abs(), err)
	}

	return digest, nil
}

// Equal reports whether `r` and `other` identify the same regular-file resource.
//
// Strict equality mirroring [Resource.Equal]: `other` must be a *file.Regular — the same URI held by another kind
// (or by the catch-all base) does not match. Once the type check passes, URI comparison is delegated to
// [op.ResourceBase.Equal].
//
// Parameters:
//   - `other`: the value to compare against; may be `any`, including nil or a non-Regular.
//
// Returns:
//   - `bool`: true if `other` is a *file.Regular with the same URI as `r`.
func (r *Regular) Equal(other any) bool {

	if other == nil {
		return false
	}

	if _, ok := other.(*Regular); !ok {
		return false
	}

	return r.ResourceBase.Equal(other)
}

// Etag returns the inexpensive stat-derived change-detection token for the regular file.
//
// Always fresh: the disk is observed at call time with lstat semantics, and a kind other than regular file errors
// with a kind mismatch (step 23, ruling 5e). The token is the shared stat-tuple form: a sha256 of
// (size, mtime_ns, ino) packed little-endian, encoded as lowercase hex.
//
// Returns:
//   - `string`: lowercase hex sha256 of the packed stat tuple.
//   - `error`: an lstat error or a kind mismatch.
func (r *Regular) Etag() (string, error) {

	root := r.RuntimeEnvironment().Root

	info, err := root.Lstat(root.NewPath(r.SourcePath.Abs()))
	if err != nil {
		return "", fmt.Errorf("file.Regular: etag lstat %s: %w", r.SourcePath.Abs(), err)
	}

	if !info.Mode().IsRegular() {
		return "", kindMismatchError("file.Regular", r.SourcePath.Abs(), info.Mode())
	}

	return statTupleEtag(info), nil
}

// String returns a debug-oriented single-line representation of the regular-file resource.
//
// Returns:
//   - `string`: `file.Regular{uri=<URI>, source_path=<path>}`.
func (r *Regular) String() string {
	return fmt.Sprintf("file.Regular{uri=%s, source_path=%s}", r.URI(), r.SourcePath.Abs())
}

// endregion

// region Behaviors

// CanConvertFrom reports whether `source` can be projected into a [*Regular] via [Regular.ConvertFrom].
//
// The variant's own probe for the framework's [op.TargetConverter] contract — defined directly (not promoted from
// the embedded base) because the cheap-probe contract calls it against a nil-or-zero `*Regular` receiver, and a
// promoted method would dereference the nil receiver to reach the embedded base. Today's accepted source shape is
// `string`, interpreted as a filesystem path under the active fsroot.
//
// Parameters:
//   - `source`: the candidate source type to test.
//
// Returns:
//   - `bool`: true when `source` is `string`.
func (*Regular) CanConvertFrom(source reflect.Type) bool {

	return source != nil && source.Kind() == reflect.String
}

// ConvertFrom projects `value` into a fresh [*Regular].
//
// Mirrors [Resource.ConvertFrom]: the returned value carries the path under SourcePath but is NOT catalog-interned
// at this layer; receiving provider methods intern via their own [NewRegular]/[DiscoverRegular] path.
//
// Parameters:
//   - `value`: the source value; must be `string`.
//
// Returns:
//   - `any`: the constructed unlinked [*Regular].
//   - `error`: non-nil when `value` is not a `string`.
func (*Regular) ConvertFrom(value any) (any, error) {

	str, ok := value.(string)
	if !ok {
		return nil, fmt.Errorf("file.Regular.ConvertFrom: source must be string, got %T", value)
	}

	return &Regular{Resource: Resource{SourcePath: fsroot.NewPath("", str)}}, nil
}

// UnmarshalJSON populates the receiver from a JSON-encoded string (a file path or file URI).
//
// The caller pre-seeds the receiver's embedded [op.ResourceBase] with a valid [op.RuntimeEnvironment] before
// invoking this method; the whole receiver is then overwritten by the reconstructed variant — defined directly so
// rehydration rebuilds a [*Regular], never a half-filled embedded base.
//
// Parameters:
//   - `data`: JSON-encoded string containing the resource's URI or path.
//
// Returns:
//   - `error`: non-nil if the RuntimeEnvironment is missing, the JSON does not decode as a string, or resource
//     construction fails.
func (r *Regular) UnmarshalJSON(data []byte) error {

	if r.RuntimeEnvironment() == nil {
		return errors.New("file.Regular: UnmarshalJSON requires RuntimeEnvironment on receiver")
	}

	var uri string

	if err := json.Unmarshal(data, &uri); err != nil {
		return err
	}

	built, err := DiscoverRegular(r.RuntimeEnvironment(), uri)
	if err != nil {
		return err
	}

	*r = *built
	return nil
}

// UnmarshalText populates the receiver from raw UTF-8 bytes containing a file path or file URI.
//
// The caller pre-seeds the receiver's embedded [op.ResourceBase] with a valid [op.RuntimeEnvironment] before
// invoking this method; the whole receiver is then overwritten by the reconstructed variant.
//
// Parameters:
//   - `text`: UTF-8 bytes containing the resource's URI or path.
//
// Returns:
//   - `error`: non-nil if the RuntimeEnvironment is missing or resource construction fails.
func (r *Regular) UnmarshalText(text []byte) error {

	if r.RuntimeEnvironment() == nil {
		return errors.New("file.Regular: UnmarshalText requires RuntimeEnvironment on receiver")
	}

	built, err := DiscoverRegular(r.RuntimeEnvironment(), string(text))
	if err != nil {
		return err
	}

	*r = *built
	return nil
}

// UnmarshalYAML populates the receiver from a YAML scalar (a file path or file URI).
//
// The caller pre-seeds the receiver's embedded [op.ResourceBase] with a valid [op.RuntimeEnvironment] before
// invoking this method; the whole receiver is then overwritten by the reconstructed variant.
//
// Parameters:
//   - `unmarshal`: callback supplied by the YAML decoder that projects the current node into the given target.
//
// Returns:
//   - `error`: non-nil if the RuntimeEnvironment is missing, the YAML node does not decode as a string, or resource
//     construction fails.
func (r *Regular) UnmarshalYAML(unmarshal func(any) error) error {

	if r.RuntimeEnvironment() == nil {
		return errors.New("file.Regular: UnmarshalYAML requires RuntimeEnvironment on receiver")
	}

	var uri string

	if err := unmarshal(&uri); err != nil {
		return err
	}

	built, err := DiscoverRegular(r.RuntimeEnvironment(), uri)
	if err != nil {
		return err
	}

	*r = *built
	return nil
}

// endregion

// endregion
