// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package mem

import (
	"crypto/sha256"
	"encoding"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"golang.org/x/exp/mmap"

	"github.com/NobleFactor/devlore-cli/pkg/iox"
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

var (
	byteSliceType = reflect.TypeFor[[]byte]()
	stringType    = reflect.TypeFor[string]()
)

// Resource represents a named in-memory-origin data resource whose content is archived on disk at a location
// derived deterministically from its URI.
//
// The canonical URI is a tag URI of the form
// tag:devlore.noblefactor.com,2026-01-01:<ns>/<name>#github.com/.../mem.Resource. The <ns>/<name> portion
// (the <specific>) is the resource's reachability — given <specific> and the [op.Root] of an
// [op.RuntimeEnvironment], the on-disk SourcePath is computed as <Root>/.devlore/mem/resource/<ns>/<name>.
// Two resources with the same URI resolve to the same file — named content deduplication by construction.
//
// Content bytes are never held in the Go heap after archival. Consumers read through [Resource.Reader]
// (a mmap-backed [io.ReadCloser]) or via the op.SourceConverter projections to []byte or string.
//
// The content hash is metadata (change detection), not part of the URI. Two resources with the same URI
// but different hashes trigger a catalog shadow.
type Resource struct {
	op.ResourceBase

	// Namespace groups related resources (e.g., "file.Reducer"); first segment of the URI <specific>.
	// May be empty. Derivable from URI.
	Namespace string

	// Name is the specific identifier (e.g., "count_python_files", "config"); second segment of the URI
	// <specific>. May be empty when <specific> is name-only. Derivable from URI.
	Name string

	// Hash is the SHA-256 of the archived content, populated at archive time. Metadata, not persisted —
	// callers that need the hash post-round-trip recompute via [Resource.Reader] + sha256.
	Hash string `json:"-" yaml:"-"`
}

// NewResource constructs a mem.Resource from a [ResourceSpec].
//
// The URI is computed from the spec; the on-disk SourcePath is derived from the URI (the URI carries the
// reachability). If spec.Data is non-nil, its content is written to SourcePath via one of the accepted
// full-fidelity shapes (first-match-wins dispatch):
//
//  1. nil                                 — metadata-only; SourcePath is set but the file is not created.
//  2. [io.Reader]                         — streamed to SourcePath via [io.Copy]; hash computed in-flight
//     via [io.TeeReader] into a SHA-256 hasher. No in-memory buffering.
//  3. []byte                              — written via [op.Root.WriteFile].
//  4. string                              — []byte(s) → written.
//  5. interface{ Bytes() []byte }         — v.Bytes() → written.
//  6. [encoding.BinaryMarshaler]          — MarshalBinary → written.
//  7. [encoding.TextMarshaler]            — MarshalText → written.
//
// Types that don't round-trip losslessly (fmt.Stringer, op.SourceConverter) are rejected to prevent silent data
// loss.
//
// Parameters:
//   - ctx:   execution context; must have a non-nil Root when spec.Data is non-nil.
//   - value: a [ResourceSpec].
//
// Returns:
//   - *Resource: the constructed resource with URI, SourcePath, and (when content was archived) Hash set.
//   - error:     malformed spec, unsupported Data type, or filesystem write error.
func NewResource(ctx *op.RuntimeEnvironment, value any) (*Resource, error) {

	switch v := value.(type) {

	case ResourceSpec:
		return newFromSpec(ctx, v)

	case string:
		return newFromURI(ctx, v)

	default:
		return nil, fmt.Errorf("mem.Resource: expected ResourceSpec or URI string, got %T", value)
	}
}

// DiscoverResource constructs a mem.Resource and registers it with [op.ResourceCatalog.Discover] without
// claiming production. Used by the framework's resource registry adapter for slot coercion. activationRecord
// is required for signature symmetry with the production-claim path; only activationRecord.Runtime is consumed.
// SiteID is unused (Discover doesn't stamp). Nil-Catalog tolerance returns the unlinked candidate.
func DiscoverResource(activationRecord *op.ActivationRecord, value any) (*Resource, error) {
	candidate, err := NewResource(activationRecord.Runtime, value)
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

// newFromSpec constructs a fully-populated mem.Resource from a [ResourceSpec], archiving spec.Data when present.
func newFromSpec(ctx *op.RuntimeEnvironment, spec ResourceSpec) (*Resource, error) {

	if spec.Namespace == "" && spec.Name == "" {
		return nil, fmt.Errorf("mem.Resource: spec must have non-empty Namespace or Name")
	}

	base, err := op.NewResourceBase(ctx, spec.Specific(), reflect.TypeFor[*Resource]())
	if err != nil {
		return nil, fmt.Errorf("mem.Resource: %w", err)
	}

	r := &Resource{
		ResourceBase: base,
		Namespace:    spec.Namespace,
		Name:         spec.Name,
	}

	if spec.Data != nil {
		hash, err := writeSpecData(ctx.Root, r.SourcePath(), spec.Data)
		if err != nil {
			return nil, fmt.Errorf("mem.Resource: %w", err)
		}
		r.Hash = hash
	}

	return r, nil
}

// region EXPORTED METHODS

// region Behaviors

// CanConvert reports whether this Resource can project to the given target Go type.
//
// Supports []byte and string — both read the archived content through a memory-mapped view. Overrides
// [op.ResourceBase.CanConvert]'s URI-as-string baseline because mem.Resource's string projection means
// content-as-text, not URI.
func (r *Resource) CanConvertTo(target reflect.Type) bool {
	return target == byteSliceType || target == stringType
}

// Convert projects the Resource into the requested target Go type.
//
// For []byte: returns the archived content as a byte slice. For string: same, wrapped as a Go string. In
// both cases the mmap is opened fresh, drained, and closed within this call.
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

// Equal reports whether r and other identify the same mem resource.
//
// Strict equality: other must be a *mem.Resource (not merely an [op.Resource] with the same URI). Once the
// type check passes, URI comparison is delegated to [op.ResourceBase.Equal].
func (r *Resource) Equal(other any) bool {

	if other == nil {
		return false
	}

	if _, ok := other.(*Resource); !ok {
		return false
	}

	return r.ResourceBase.Equal(other)
}

// Reader opens a fresh memory-mapped view of the archived content and returns an [io.ReadCloser] over it.
//
// The caller must Close the reader — Close munmaps the underlying file. Each call opens a new mmap.
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
// The path follows 13.0(c)'s per-type formula: <Root>/.devlore/<last-pkg-segment>/<lowercase(TypeName)>/<specific>,
// where <last-pkg-segment> and <TypeName> are derived from the URI fragment (the canonical Go type id) and
// <specific> is the reachability identity. For mem.Resource that resolves to .devlore/mem/resource/<ns>/<name>;
// embedders inherit this method automatically and their distinct typeID drives a distinct subdirectory
// (e.g., function.Resource → .devlore/function/resource/<ns>/<name>).
//
// Returns the zero op.Path when the Resource has no [op.RuntimeEnvironment] (e.g., constructed without a context).
func (r *Resource) SourcePath() op.Path {

	env := r.RuntimeEnvironment()
	if env == nil || env.Root == nil {
		return op.Path{}
	}

	pkg, typeName := splitTypeID(r.ResourceType())
	return env.Root.NewPath(filepath.Join(".devlore", pkg, strings.ToLower(typeName), r.ReachabilityURI()))
}

// String returns a debug-oriented single-line representation of the Resource.
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
// The caller pre-seeds the receiver's embedded [op.ResourceBase] with a valid [op.RuntimeEnvironment] before
// invoking this method. The URI alone is sufficient to reconstruct the Resource: Namespace and Name are
// extracted from the URI <specific>, and SourcePath is computed deterministically from the URI and the
// context's Root.
func (r *Resource) UnmarshalJSON(data []byte) error {

	if r.RuntimeEnvironment() == nil {
		return errors.New("mem.Resource: UnmarshalJSON requires RuntimeEnvironment on receiver")
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

// UnmarshalText populates the receiver from raw UTF-8 bytes containing the URI.
func (r *Resource) UnmarshalText(text []byte) error {

	if r.RuntimeEnvironment() == nil {
		return errors.New("mem.Resource: UnmarshalText requires RuntimeEnvironment on receiver")
	}

	built, err := NewResource(r.RuntimeEnvironment(), string(text))
	if err != nil {
		return err
	}

	*r = *built
	return nil
}

// UnmarshalYAML populates the receiver from its YAML wire form (a bare URI scalar).
func (r *Resource) UnmarshalYAML(unmarshal func(any) error) error {

	if r.RuntimeEnvironment() == nil {
		return errors.New("mem.Resource: UnmarshalYAML requires RuntimeEnvironment on receiver")
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

// region UNEXPORTED HELPERS

// newFromURI reconstructs a metadata-only mem.Resource from a canonical tag URI string. No content is
// archived — callers who need content must archive separately. Used by UnmarshalJSON / UnmarshalText /
// UnmarshalYAML.
func newFromURI(ctx *op.RuntimeEnvironment, uri string) (*Resource, error) {

	specific, _, err := op.ExtractTagSpecific(uri)
	if err != nil {
		return nil, fmt.Errorf("mem.Resource: %w", err)
	}

	if specific == "" {
		return nil, fmt.Errorf("mem.Resource: cannot reconstruct from deferred URI %q", uri)
	}

	base, err := op.NewResourceBase(ctx, specific, reflect.TypeFor[*Resource]())
	if err != nil {
		return nil, err
	}

	r := &Resource{
		ResourceBase: base,
	}

	// Specific shape is "<ns>/<name>" or "<single>". A single segment populates Name (the leaf).
	if ns, name, ok := strings.Cut(specific, "/"); ok {
		r.Namespace = ns
		r.Name = name
	} else {
		r.Name = specific
	}

	return r, nil
}

// splitTypeID splits a canonical Go type id of the form "<pkgPath>.<TypeName>" into (<last-pkg-segment>, <TypeName>).
// Example: "github.com/NobleFactor/devlore-cli/pkg/op/provider/mem.Resource" → ("mem", "Resource").
func splitTypeID(typeID string) (pkg, typeName string) {

	dot := strings.LastIndex(typeID, ".")
	if dot < 0 {
		return "", typeID
	}

	typeName = typeID[dot+1:]
	left := typeID[:dot]

	if slash := strings.LastIndex(left, "/"); slash >= 0 {
		pkg = left[slash+1:]
	} else {
		pkg = left
	}
	return pkg, typeName
}

// writeSpecData archives spec.Data to sourcePath under root. Returns the SHA-256 hex of the written
// content. See [NewResource] for the accepted Data shapes.
func writeSpecData(root op.Root, sourcePath op.Path, data any) (string, error) {

	parentRel := filepath.Dir(sourcePath.Rel())
	parentPath := root.NewPath(parentRel)
	if err := root.MkdirAll(parentPath, 0o700); err != nil {
		return "", fmt.Errorf("create parent dir: %w", err)
	}

	switch v := data.(type) {

	case io.Reader:
		return writeStream(root, sourcePath, v)

	case []byte:
		return writeBytes(root, sourcePath, v)

	case string:
		return writeBytes(root, sourcePath, []byte(v))

	case interface{ Bytes() []byte }:
		return writeBytes(root, sourcePath, v.Bytes())

	case encoding.BinaryMarshaler:
		b, err := v.MarshalBinary()
		if err != nil {
			return "", fmt.Errorf("MarshalBinary: %w", err)
		}
		return writeBytes(root, sourcePath, b)

	case encoding.TextMarshaler:
		b, err := v.MarshalText()
		if err != nil {
			return "", fmt.Errorf("MarshalText: %w", err)
		}
		return writeBytes(root, sourcePath, b)

	default:
		return "", fmt.Errorf("unsupported Data type %T; want nil, io.Reader, []byte, string, Bytes() []byte, encoding.BinaryMarshaler, or encoding.TextMarshaler", v)
	}
}

// writeBytes writes data to sourcePath and returns the SHA-256 hex.
func writeBytes(root op.Root, sourcePath op.Path, data []byte) (string, error) {

	if err := root.WriteFile(sourcePath, data, 0o600); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}

	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:]), nil
}

// writeStream drains reader into sourcePath via io.Copy, computing SHA-256 in-flight via io.TeeReader.
func writeStream(root op.Root, sourcePath op.Path, reader io.Reader) (_ string, err error) {

	f, err := root.OpenFile(sourcePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return "", fmt.Errorf("open file: %w", err)
	}
	defer iox.Close(&err, f)

	h := sha256.New()
	teed := io.TeeReader(reader, h)

	if _, err := io.Copy(f, teed); err != nil {
		return "", fmt.Errorf("copy: %w", err)
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// endregion

// resourceReader bundles a mmap handle with a SectionReader over its full range.
type resourceReader struct {
	mmap    *mmap.ReaderAt
	section *io.SectionReader
}

func (r *resourceReader) Read(p []byte) (int, error) { return r.section.Read(p) }
func (r *resourceReader) Close() error               { return r.mmap.Close() }
