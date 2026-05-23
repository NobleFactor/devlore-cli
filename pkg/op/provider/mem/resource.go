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

// Resource represents an in-memory-origin data resource archived on disk at a content-addressed path.
//
// The canonical URI is a tag URI of the form
// `tag:devlore.noblefactor.com,2026-01-01:<algo>:<hex>#github.com/.../mem.Resource`, where `<algo>:<hex>` is the
// SHA-256 of the archived bytes. Identity is the digest: two resources built from the same bytes resolve to the same
// URI and the same on-disk path — content deduplication by construction.
//
// The on-disk path follows a sharded CAS layout:
// `<Root>/.devlore/mem/resource/<algo>/<hex[0:2]>/<hex>`. The 2-character prefix shard keeps any single directory
// bounded as content grows. Embedders inherit the formula via their distinct typeID.
//
// Content bytes are never held in the Go heap after archival. Consumers read through [Resource.Reader] (a mmap-backed
// [io.ReadCloser]) or via the [op.SourceConverter] projections to []byte or string.
type Resource struct {
	op.ResourceBase

	// Hash is the lowercase hex SHA-256 of the archived content. Identity-bearing — also encoded in the URI's
	// <specific> portion as `sha256:<Hash>`. Populated by both construction (post-hash) and rehydration (parsed
	// from URI). Not persisted in marshaled wire form because the URI carries the same value.
	Hash string `json:"-" yaml:"-"`
}

// NewResource constructs a *Resource and claims production via [op.ResourceCatalog.GetOrCreate].
//
// Use NewResource from a producer dispatch context — typically a provider method that has received an
// [op.ActivationRecord] from the framework. The returned Resource is the canonical catalog entry, stamped with
// `producerID = activationRecord.Unit.ID()` (or empty when `Unit` is nil for non-graph dispatch). Use
// [DiscoverResource] instead when the caller is not claiming production (rehydration, reference handles, the
// framework's slot-coercion adapter).
//
// Identity is the SHA-256 of the archived bytes. The on-disk SourcePath is derived from that digest. When `value`
// is []byte the content is hashed in memory and written directly. When `value` is an [io.Reader] the content is
// streamed through a TeeReader into a staging file, hashed in flight, then renamed onto the canonical path. When
// `value` is a string URI the Resource is rehydrated metadata-only (no archival; the URI alone carries the digest).
//
// Two callers with the same content produce the same URI; the first to reach the catalog wins the entry. The second
// caller's write overwrites the canonical path with byte-identical content.
//
// Nil-Catalog tolerance: returns the unlinked candidate when no catalog is present.
//
// Parameters:
//   - `activationRecord`: per-dispatch activation; its `RuntimeEnvironment` supplies the runtime environment
//     (`Root` must be non-nil when `value` is []byte or [io.Reader]) and its `Unit.ID()` becomes the catalog
//     entry's producerID (empty when `Unit` is nil). Must be non-nil.
//   - `value`: []byte (in-memory archival), [io.Reader] (stream archival), or a canonical tag URI string
//     (metadata-only rehydration).
//
// Returns:
//   - *Resource: canonical catalog entry, or the unlinked candidate when no catalog is present.
//   - `error`: unsupported value type, filesystem write failure, malformed URI, or identity construction failure.
func NewResource(activationRecord *op.ActivationRecord, value any) (*Resource, error) {

	candidate, err := buildCandidate(activationRecord.RuntimeEnvironment, value)
	if err != nil {
		return nil, err
	}

	if activationRecord.RuntimeEnvironment.Catalog == nil {
		return candidate, nil
	}

	got, err := activationRecord.RuntimeEnvironment.Catalog.GetOrCreate(activationRecord, candidate.URI(), func() (op.Resource, error) {
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

// DiscoverResource constructs a *Resource and registers it without claiming production.
//
// Used by the framework's resource registry adapter for slot coercion (when starlark supplies a string URI and the slot
// expects a *mem.Resource) and by callers holding a reference handle without claiming production. UnmarshalJSON /
// UnmarshalText / UnmarshalYAML rehydration is the canonical use case.
//
// An `activationRecord` is required for signature symmetry with [NewResource], but only its `RuntimeEnvironment`
// is consumed — `Unit` is unused since Discover doesn't stamp a producer. Discovery callers commonly construct
// one as `op.NewActivationRecord(nil, nil, runtimeEnvironment)` — both `Graph` and `Unit` nil.
//
// Same value-shape dispatch as [NewResource]: []byte / [io.Reader] archive content; string rehydrates metadata-only.
//
// Nil-Catalog tolerance: returns the unlinked candidate when no catalog is present.
//
// Parameters:
//   - `activationRecord`: per-dispatch activation; only its `RuntimeEnvironment` is consumed. Must be non-nil with
//     a non-nil `RuntimeEnvironment`.
//   - `value`: []byte, [io.Reader], or a canonical tag URI string; same dispatch as [NewResource].
//
// Returns:
//   - *Resource: canonical catalog entry, or the unlinked candidate when no catalog is present.
//   - `error`: unsupported value type, filesystem write failure, malformed URI, or identity construction failure.
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
		return nil, fmt.Errorf("mem.DiscoverResource: catalog entry for %q is %T, want *mem.Resource", candidate.URI(), got)
	}

	return canonical, nil
}

// region EXPORTED METHODS

// region Behaviors

// Addressing reports that mem.Resource is content-addressed.
//
// Overrides [op.ResourceBase.Addressing]'s [op.AddressingUnknown] default. The boot-discipline check in
// pkg/op/addressing_test.go relies on every announced Resource type returning a non-Unknown mode here.
//
// Returns:
//   - op.AddressingMode: [op.AddressingContent] — identity is the SHA-256 of the archived bytes.
func (r *Resource) Addressing() op.AddressingMode {
	return op.AddressingContent
}

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

// ConvertTo projects the Resource into the requested target Go type.
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

// Digest returns the content digest of the archived bytes.
//
// The SHA-256 was computed during construction (or parsed from the URI on rehydration) and stamped on
// [Resource.Hash]. This method reassembles the canonical `sha256:<hex>` form via [op.ParseDigest], yielding the
// strict [op.Digest] shape with Algorithm = "sha256" and Bytes = the raw 32-byte digest. Overrides
// [op.ResourceBase.Digest]'s [op.ErrUnimplemented] default.
//
// Returns:
//   - op.Digest: {Algorithm: "sha256", Bytes: decoded Hash}.
//   - error: non-nil if Hash is malformed; should not occur post-construction or post-rehydration.
func (r *Resource) Digest() (op.Digest, error) {
	return op.ParseDigest("sha256:" + r.Hash)
}

// Equal reports whether r and other identify the same mem.Resource.
//
// Strict equality: other must be a *mem.Resource (not merely an [op.Resource] with the same URI). Once the type check
// passes, URI comparison is delegated to [op.ResourceBase.Equal].
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
// The path follows the CAS sharded formula
// `<Root>/.devlore/<last-pkg-segment>/<lowercase(TypeName)>/<algo>/<hex[0:2]>/<hex>`, where `<last-pkg-segment>` and
// `<TypeName>` are derived from the URI fragment (the canonical Go type id) and `<algo>:<hex>` is parsed from the
// URI's <specific> portion. The 2-character prefix shard keeps any single directory bounded as content grows.
// Embedders inherit this method automatically; their distinct typeID drives a distinct subdirectory
// (e.g., function.Resource → `.devlore/function/resource/<algo>/<hex[0:2]>/<hex>`).
//
// Returns:
//   - op.Path: canonical archive path, or the zero op.Path when the Resource has no [op.RuntimeEnvironment], no Root,
//     or a <specific> that is not in `<algo>:<hex>` form.
func (r *Resource) SourcePath() op.Path {

	runtimeEnvironment := r.RuntimeEnvironment()
	if runtimeEnvironment == nil || runtimeEnvironment.Root == nil {
		return op.Path{}
	}

	algo, hexPart, ok := strings.Cut(r.ReachabilityURI(), ":")
	if !ok {
		return op.Path{}
	}

	shard := hexPart
	if len(shard) >= 2 {
		shard = hexPart[0:2]
	}

	pkg, typeName := splitTypeID(r.ResourceType())
	return runtimeEnvironment.Root.NewPath(filepath.Join(".devlore", pkg, strings.ToLower(typeName), algo, shard, hexPart))
}

// String returns the compact JSON encoding of the Resource for debug output.
//
// Delegates to [op.ResourceBase.Format] per the project Go style guideline that String() of every concrete Resource
// type calls r.Format(r).
//
// Returns:
//   - string: the compact JSON encoding of r.
func (r *Resource) String() string {
	return r.Format(r)
}

// UnmarshalJSON populates the receiver from its JSON wire form (a bare URI string).
//
// The caller pre-seeds the receiver's embedded [op.ResourceBase] with a valid [op.RuntimeEnvironment] before invoking
// this method. The URI alone is sufficient to reconstruct the Resource: Hash is parsed from the URI's <specific>
// portion, and SourcePath is computed deterministically from the URI and the runtime environment's Root.
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

	built, err := DiscoverResource(op.NewActivationRecord(nil, nil, r.RuntimeEnvironment()), uri)
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

	built, err := DiscoverResource(op.NewActivationRecord(nil, nil, r.RuntimeEnvironment()), string(text))
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

	built, err := DiscoverResource(op.NewActivationRecord(nil, nil, r.RuntimeEnvironment()), uri)
	if err != nil {
		return err
	}

	*r = *built
	return nil
}

// endregion

// endregion

// region Auxiliary Types

// resourceReader bundles a [mmap.ReaderAt] handle with an [io.SectionReader] over its full range so that Read drains
// through the mmap and Close releases it.
type resourceReader struct {

	// mmap is the underlying memory map; held so Close can unmap it.
	mmap *mmap.ReaderAt

	// section is an [io.SectionReader] over the full range of mmap, used for Read.
	section *io.SectionReader
}

// Close releases the underlying memory map.
//
// Returns:
//   - error: any error returned by [mmap.ReaderAt.Close].
func (r *resourceReader) Close() error {
	return r.mmap.Close()
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

// endregion