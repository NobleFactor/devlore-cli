// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package mem

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"reflect"
	"strings"

	"golang.org/x/exp/mmap"

	"github.com/NobleFactor/devlore-cli/pkg/iox"
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

var (
	// byteSliceType is the [reflect.Type] for []byte; matched by [Resource.CanConvertTo] and [Resource.ConvertTo].
	byteSliceType = reflect.TypeFor[[]byte]()

	// stringType is the [reflect.Type] for string; matched by [Resource.CanConvertTo] and [Resource.ConvertTo].
	stringType = reflect.TypeFor[string]()
)

// Resource represents a named in-memory-origin data resource archived on disk at a URI-derived path.
//
// The canonical URI is a tag URI of the form
// `tag:devlore.noblefactor.com,2026-01-01:<ns>/<name>#github.com/.../mem.Resource`. The <ns>/<name> portion (the
// <specific>) is the resource's reachability — given <specific> and the [op.Root] of an [op.RuntimeEnvironment], the
// on-disk SourcePath is computed as <Root>/.devlore/mem/resource/<ns>/<name>. Two resources with the same URI resolve
// to the same file — named content deduplication by construction.
//
// Content bytes are never held in the Go heap after archival. Consumers read through [Resource.Reader] (a mmap-backed
// [io.ReadCloser]) or via the [op.SourceConverter] projections to []byte or string.
//
// The content hash is metadata (change detection), not part of the URI. Two resources with the same URI but different
// hashes trigger a catalog shadow.
type Resource struct {
	op.ResourceBase

	// Namespace groups related resources (e.g., "file.Reducer"); first segment of the URI <specific>. It may be empty.
	// Derivable from URI.
	Namespace string

	// Name is the specific identifier (e.g., "count_python_files", "config"); the second segment of the URI <specific>.
	// It may be empty when <specific> is name-only. Derivable from URI.
	Name string

	// Hash is the SHA-256 of the archived content, populated at archive time. Metadata, not persisted. Callers that
	// need the hash post-roundtrip recompute it via [Resource.Reader] + sha256.
	Hash string `json:"-" yaml:"-"`
}

// NewResource constructs a mem.Resource and claims production via [op.ResourceCatalog.GetOrCreate].Read
//
// Use NewResource from a producer dispatch context — typically a provider method that has received an
// [op.ActivationRecord] from the framework. The returned Resource is the canonical catalog entry, stamped with
// `producerID = activationRecord.SiteID`. Use [DiscoverResource] instead when the caller is not claiming production
// (rehydration, reference handles, the framework's slot-coercion adapter).
//
// The URI is computed from value; the on-disk SourcePath is derived from the URI (the URI carries the reachability).
// When value is a [ResourceSpec] with non-nil Data, the content is archived to SourcePath via [writeSpecData] (see
// that function for the accepted Data shapes). The archival write happens during construction regardless of whether
// the catalog later returns a cache hit; two callers with the same URI both write the same content to the same path,
// and the first wins the catalog entry.
//
// Nil-Catalog tolerance: returns the unlinked candidate when no catalog is present.
//
// Parameters:
//   - activationRecord: per-dispatch activation; its Runtime carries the runtime environment (Root must be non-nil
//     when value is a [ResourceSpec] with non-nil Data) and its SiteID becomes the catalog entry's producerID. Must be
//     non-nil.
//   - value: a [ResourceSpec] (creates from spec, archiving Data) or a canonical tag URI string (rehydrates
//     metadata-only).
//
// Returns:
//   - *Resource: canonical catalog entry, or the unlinked candidate when no catalog is present.
//   - error: malformed spec, unsupported Data type, filesystem write failure, or unsupported value type.
func NewResource(activationRecord *op.ActivationRecord, value any) (*Resource, error) {

	candidate, err := buildCandidate(activationRecord.Runtime, value)
	if err != nil {
		return nil, err
	}

	if activationRecord.Runtime.Catalog == nil {
		return candidate, nil
	}

	got, err := activationRecord.Runtime.Catalog.GetOrCreate(activationRecord, candidate.URI(), func() (op.Resource, error) {
		return candidate, nil
	})
	if err != nil {
		return nil, err
	}

	canonical, ok := got.(*Resource)
	if !ok {
		return nil, fmt.Errorf("mem.NewResource: catalog entry for %q is %T, want *mem.Resource", candidate.URI(), got)
	}

	return canonical, nil
}

// DiscoverResource constructs a mem.Resource and registers it without claiming production.
//
// Used by the framework's resource registry adapter for slot coercion (when starlark supplies a string URI and the slot
// expects a *mem.Resource) and by callers holding a reference handle without claiming production. UnmarshalJSON /
// UnmarshalText / UnmarshalYAML rehydration is the canonical use case.
//
// An activationRecord is required for signature symmetry with [NewResource], but only activationRecord.Runtime is
// consumed. SiteID is unused (Discover does not stamp). Discovery callers commonly synthesize an [op.ActivationRecord]
// with empty SiteID and only Runtime set: `&op.ActivationRecord{Runtime: runtimeEnvironment}`.
//
// Same value-shape semantics as [NewResource]: [ResourceSpec] creates from spec (with potential Data archival); string
// rehydrates metadata-only.
//
// Nil-Catalog tolerance: returns the unlinked candidate when no catalog is present.
//
// Parameters:
//   - activationRecord: per-dispatch activation; only its Runtime is consumed. Must be non-nil with a non-nil Runtime.
//   - value: a [ResourceSpec] or a canonical tag URI string; same dispatch as [NewResource].
//
// Returns:
//   - *Resource: canonical catalog entry, or the unlinked candidate when no catalog is present.
//   - error: malformed spec, unsupported Data type, filesystem write failure, or unsupported value type.
func DiscoverResource(activationRecord *op.ActivationRecord, value any) (*Resource, error) {

	candidate, err := buildCandidate(activationRecord.Runtime, value)
	if err != nil {
		return nil, err
	}

	if activationRecord.Runtime.Catalog == nil {
		return candidate, nil
	}

	got, err := activationRecord.Runtime.Catalog.Discover(candidate.URI(), func() (op.Resource, error) {
		return candidate, nil
	})
	if err != nil {
		return nil, err
	}

	canonical, ok := got.(*Resource)
	if !ok {
		return nil, fmt.Errorf("mem.DiscoverResource: catalog entry for %q is %T, want *mem.Resource", candidate.URI(), got)
	}

	return canonical, nil
}

// region EXPORTED METHODS

// region Behaviors

// CanConvertTo reports whether this Resource can project to the given target Go type.
//
// Supports []byte and string — both read the archived content through a memory-mapped view. Overrides
// [op.ResourceBase.CanConvertTo]'s URI-as-string baseline because mem.Resource's string projection means
// content-as-text, not URI.
//
// Parameters:
//   - target: destination Go type the caller wants to project the Resource into.
//
// Returns:
//   - bool: true when target is []byte or string; false otherwise.
func (r *Resource) CanConvertTo(target reflect.Type) bool {
	return target == byteSliceType || target == stringType
}

// ConvertTo projects the mem.Resource into the requested target Go type.
//
// Supports []byte and string. Both read the archived content through a fresh memory-mapped view that is opened,
// drained, and closed within this call.
//
// Parameters:
//   - target: destination Go type — must be []byte or string.
//
// Returns:
//   - any: projected value ([]byte or string).
//   - error: unrecognized target type, missing source path, or read failure.
func (r *Resource) ConvertTo(target reflect.Type) (any, error) {

	if target != byteSliceType && target != stringType {
		return nil, fmt.Errorf("mem.Resource: cannot convert to %s", target)
	}

	rc, err := r.Reader()
	if err != nil {
		return nil, err
	}
	defer iox.Close(&err, rc)

	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("mem.Resource: read archived content: %w", err)
	}

	if target == stringType {
		return string(data), nil
	}
	return data, nil
}

// Equal reports whether r and other identify the same mem.Resource.
//
// Strict equality: the other must be a *mem.Resource (not merely an [op.Resource] with the same URI). Once the type
// check passes, URI comparison is delegated to [op.ResourceBase.Equal].
//
// Parameters:
//   - other: candidate value to compare against; nil or any non-*mem.Resource value returns false.
//
// Returns:
//   - bool: true when other is a *mem.Resource with the same URI as r.
func (r *Resource) Equal(other any) bool {

	if other == nil {
		return false
	}

	if _, ok := other.(*Resource); !ok {
		return false
	}

	return r.ResourceBase.Equal(other)
}

// Reader opens a fresh memory-mapped view of the archived content.
//
// Each call opens a new mmap. The caller must Close the returned reader — Close munmaps the underlying file.
//
// Returns:
//   - io.ReadCloser: reader over the full archived content; Close releases the mmap.
//   - error: missing SourcePath, or mmap failure.
func (r *Resource) Reader() (io.ReadCloser, error) {

	abs := r.SourcePath().Abs()
	if abs == "" {
		return nil, errors.New("mem.Resource: no SourcePath")
	}

	m, err := mmap.Open(abs)
	if err != nil {
		return nil, fmt.Errorf("mem.Resource: mmap %s: %w", abs, err)
	}

	return &resourceReader{
		mmap:    m,
		section: io.NewSectionReader(m, 0, int64(m.Len())),
	}, nil
}

// SourcePath returns the on-disk archive path for this Resource under the runtime environment's [op.Root].
//
// The path follows the per-type formula <Root>/.devlore/<last-pkg-segment>/<lowercase(TypeName)>/<specific>, where
// <last-pkg-segment> and <TypeName> are derived from the URI fragment (the canonical Go type id) and <specific> is the
// reachability identity. For mem.Resource that resolves to .devlore/mem/resource/<ns>/<name>; embedders inherit this
// method automatically and their distinct typeID drives a distinct subdirectory (e.g., function.Resource →
// .devlore/function/resource/<ns>/<name>).
//
// Returns:
//   - op.Path: canonical archive path, or the zero op.Path when the Resource has no [op.RuntimeEnvironment] or no Root.
func (r *Resource) SourcePath() op.Path {

	env := r.RuntimeEnvironment()
	if env == nil || env.Root == nil {
		return op.Path{}
	}

	pkg, typeName := splitTypeID(r.ResourceType())
	return env.Root.NewPath(filepath.Join(".devlore", pkg, strings.ToLower(typeName), r.ReachabilityURI()))
}

// String returns a debug-oriented single-line representation of the Resource.
//
// Format: `mem.Resource{uri=..., sourcePath=..., hash=...}`. Hash is truncated to its first 12 hex characters when
// longer.
//
// Returns:
//   - string: debug-oriented single-line representation.
func (r *Resource) String() string {

	hashPrefix := r.Hash
	if len(hashPrefix) > 12 {
		hashPrefix = hashPrefix[:12]
	}

	return fmt.Sprintf("mem.Resource{uri=%s, sourcePath=%s, hash=%s}",
		r.URI(), r.SourcePath().Abs(), hashPrefix)
}

// UnmarshalJSON populates the receiver from its JSON wire form (a bare URI string).
//
// The caller pre-seeds the receiver's embedded [op.ResourceBase] with a valid [op.RuntimeEnvironment] before invoking
// this method. The URI alone is sufficient to reconstruct the Resource: Namespace and Name are extracted from the URI
// <specific>, and SourcePath is computed deterministically from the URI and the runtime environment's Root.
//
// Parameters:
//   - data: JSON bytes encoding a single bare URI string.
//
// Returns:
//   - error: missing RuntimeEnvironment on receiver, malformed JSON, or rehydration failure.
func (r *Resource) UnmarshalJSON(data []byte) error {

	if r.RuntimeEnvironment() == nil {
		return errors.New("mem.Resource: UnmarshalJSON requires RuntimeEnvironment on receiver")
	}

	var uri string
	if err := json.Unmarshal(data, &uri); err != nil {
		return err
	}

	built, err := DiscoverResource(&op.ActivationRecord{Runtime: r.RuntimeEnvironment()}, uri)
	if err != nil {
		return err
	}

	*r = *built
	return nil
}

// UnmarshalText populates the receiver from raw UTF-8 bytes containing the URI.
//
// Same prerequisites and semantics as [Resource.UnmarshalJSON]; the receiver's [op.RuntimeEnvironment] must be set
// before invocation.
//
// Parameters:
//   - text: UTF-8 bytes containing the canonical tag URI.
//
// Returns:
//   - error: missing RuntimeEnvironment on receiver, or rehydration failure.
func (r *Resource) UnmarshalText(text []byte) error {

	if r.RuntimeEnvironment() == nil {
		return errors.New("mem.Resource: UnmarshalText requires RuntimeEnvironment on receiver")
	}

	built, err := DiscoverResource(&op.ActivationRecord{Runtime: r.RuntimeEnvironment()}, string(text))
	if err != nil {
		return err
	}

	*r = *built
	return nil
}

// UnmarshalYAML populates the receiver from its YAML wire form (a bare URI scalar).
//
// Same prerequisites and semantics as [Resource.UnmarshalJSON]; the receiver's [op.RuntimeEnvironment] must be set
// before invocation.
//
// Parameters:
//   - unmarshal: yaml decode hook supplied by the YAML library; called with a *string target.
//
// Returns:
//   - error: missing RuntimeEnvironment on receiver, decode failure, or rehydration failure.
func (r *Resource) UnmarshalYAML(unmarshal func(any) error) error {

	if r.RuntimeEnvironment() == nil {
		return errors.New("mem.Resource: UnmarshalYAML requires RuntimeEnvironment on receiver")
	}

	var uri string
	if err := unmarshal(&uri); err != nil {
		return err
	}

	built, err := DiscoverResource(&op.ActivationRecord{Runtime: r.RuntimeEnvironment()}, uri)
	if err != nil {
		return err
	}

	*r = *built
	return nil
}

// endregion

// endregion

// region AUXILIARY TYPES

// resourceReader bundles a [mmap.ReaderAt] handle with an [io.SectionReader] over its full range.
type resourceReader struct {

	// mmap is the underlying memory map; held so Close can unmap it.
	mmap *mmap.ReaderAt

	// section is an io.SectionReader over the full range of mmap, used for Read.
	section *io.SectionReader
}

// Read reads up to len(p) bytes from the underlying [io.SectionReader] into p.
//
// Parameters:
//   - p: destination buffer.
//
// Returns:
//   - int: number of bytes read.
//   - error: any error returned by [io.SectionReader.Read]; [io.EOF] at end of content.
func (r *resourceReader) Read(p []byte) (int, error) {
	return r.section.Read(p)
}

// Close releases the underlying memory map.
//
// Returns:
//   - error: any error returned by [mmap.ReaderAt.Close].
func (r *resourceReader) Close() error {
	return r.mmap.Close()
}

// endregion
