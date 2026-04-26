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

	"go.starlark.net/starlark"
	"golang.org/x/exp/mmap"

	"github.com/NobleFactor/devlore-cli/pkg/iox"
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

var (
	byteSliceType = reflect.TypeFor[[]byte]()
	stringType    = reflect.TypeFor[string]()
)

// memArchiveDir is the scoped subdirectory under [op.Root] where mem.Resource content is archived. Every
// resource's SourcePath lives at <Root>/<memArchiveDir>/<uri-suffix>.
const memArchiveDir = ".devlore/mem"

// Resource represents a named in-memory-origin data resource whose content is archived on disk at a location
// derived deterministically from its URI.
//
// The URI is opaque: mem:<content-type>/<namespace>/<name>. It IS the resource's reachability — given the
// URI and the [op.Root] of an [op.ExecutionContext], the on-disk SourcePath is computed as
// <Root>/.devlore/mem/<content-type>/<namespace>/<name>. Two resources with the same URI resolve to the
// same file — named content deduplication by construction.
//
// Content bytes are never held in the Go heap after archival. Consumers read through [Resource.Reader]
// (a mmap-backed [io.ReadCloser]) or via the op.Converter projections to []byte or string.
//
// The content hash is metadata (change detection), not part of the URI. Two resources with the same URI
// but different hashes trigger a catalog shadow.
type Resource struct {
	op.ResourceBase

	// ContentType classifies the content (e.g., "callable", "json", "template", "function") and is the
	// first URI path component. Derivable from URI.
	ContentType string

	// Namespace groups related resources (e.g., "file.Reducer"); second URI path component. May be empty.
	// Derivable from URI.
	Namespace string

	// Name is the specific identifier (e.g., "count_python_files", "config"); third URI path component.
	// May be empty when the URI is content-type-only. Derivable from URI.
	Name string

	// SourcePath is the archive location; derived deterministically from URI during construction and
	// re-derived on unmarshal. Not persisted.
	SourcePath op.Path `json:"-" yaml:"-"`

	// Hash is the SHA-256 of the archived content, populated at archive time. Metadata, not persisted —
	// callers that need the hash post-round-trip recompute via [Resource.Reader] + sha256.
	Hash string `json:"-" yaml:"-"`
}

// ResourceSpec carries identity and payload for constructing a mem.Resource.
//
// ContentType classifies the content (e.g., "callable", "json", "template", "function").
// Namespace is the receiver type or grouping (e.g., "file.Reducer", "Predicate") — empty for non-function resources.
// Name is the specific identifier (e.g., "count_python_files", "config").
// Data is an optional payload — see [NewResource] for accepted shapes.
type ResourceSpec struct {
	ContentType string
	Namespace   string
	Name        string
	Data        any
}

// URI returns the canonical mem: URI for this spec.
//
// Returns:
//   - string: the opaque URI (e.g., "mem:function/file.Reducer/count_python_files").
func (s ResourceSpec) URI() string {

	uri := "mem:" + s.ContentType
	if s.Namespace != "" {
		uri += "/" + s.Namespace
	}
	if s.Name != "" {
		uri += "/" + s.Name
	}
	return uri
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
// Types that don't round-trip losslessly (fmt.Stringer, op.Converter) are rejected to prevent silent data
// loss.
//
// Parameters:
//   - ctx:   execution context; must have a non-nil Root when spec.Data is non-nil.
//   - value: a [ResourceSpec].
//
// Returns:
//   - *Resource: the constructed resource with URI, SourcePath, and (when content was archived) Hash set.
//   - error:     malformed spec, unsupported Data type, or filesystem write error.
func NewResource(ctx *op.ExecutionContext, value any) (*Resource, error) {

	spec, ok := value.(ResourceSpec)
	if !ok {
		return nil, fmt.Errorf("mem.Resource: expected ResourceSpec, got %T", value)
	}

	if spec.ContentType == "" {
		return nil, fmt.Errorf("mem.Resource: empty content type")
	}

	uri := spec.URI()

	sourcePath, err := sourcePathFromURI(ctx.Root, uri)
	if err != nil {
		return nil, fmt.Errorf("mem.Resource: %w", err)
	}

	base, err := op.NewResourceBase(ctx, uri, reflect.TypeFor[*Resource]())
	if err != nil {
		return nil, fmt.Errorf("mem.Resource: %w", err)
	}

	r := &Resource{
		ResourceBase: base,
		ContentType:  spec.ContentType,
		Namespace:    spec.Namespace,
		Name:         spec.Name,
		SourcePath:   sourcePath,
	}

	if spec.Data != nil {
		hash, err := writeSpecData(ctx.Root, sourcePath, spec.Data)
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
func (r *Resource) CanConvert(target reflect.Type) bool {
	return target == byteSliceType || target == stringType
}

// Convert projects the Resource into the requested target Go type.
//
// For []byte: returns the archived content as a byte slice. For string: same, wrapped as a Go string. In
// both cases the mmap is opened fresh, drained, and closed within this call.
func (r *Resource) Convert(target reflect.Type) (any, error) {

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

	abs := r.SourcePath.Abs()
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

// String returns a debug-oriented single-line representation of the Resource.
func (r *Resource) String() string {

	hashPrefix := r.Hash
	if len(hashPrefix) > 12 {
		hashPrefix = hashPrefix[:12]
	}

	return fmt.Sprintf("mem.Resource{uri=%s, sourcePath=%s, hash=%s}",
		r.URI(), r.SourcePath.Abs(), hashPrefix)
}

// UnmarshalJSON populates the receiver from its JSON wire form (a bare URI string).
//
// The caller pre-seeds the receiver's embedded [op.ResourceBase] with a valid [op.ExecutionContext] before
// invoking this method. The URI alone is sufficient to reconstruct the Resource: ContentType, Namespace,
// Name are extracted from the URI path, and SourcePath is computed deterministically from the URI and the
// context's Root.
func (r *Resource) UnmarshalJSON(data []byte) error {

	if r.ExecutionContext() == nil {
		return errors.New("mem.Resource: UnmarshalJSON requires ExecutionContext on receiver")
	}

	var uri string
	if err := json.Unmarshal(data, &uri); err != nil {
		return err
	}

	built, err := newFromURI(r.ExecutionContext(), uri)
	if err != nil {
		return err
	}

	*r = *built
	return nil
}

// UnmarshalStarlark populates the receiver from a [starlark.String] containing the URI.
//
// Same contract as [Resource.UnmarshalJSON]: ExecutionContext must be pre-seeded; URI carries the full
// reachability.
func (r *Resource) UnmarshalStarlark(sv starlark.Value) error {

	if r.ExecutionContext() == nil {
		return errors.New("mem.Resource: UnmarshalStarlark requires ExecutionContext on receiver")
	}

	s, ok := sv.(starlark.String)
	if !ok {
		return fmt.Errorf("mem.Resource: expected starlark.String, got %s", sv.Type())
	}

	built, err := newFromURI(r.ExecutionContext(), string(s))
	if err != nil {
		return err
	}

	*r = *built
	return nil
}

// UnmarshalText populates the receiver from raw UTF-8 bytes containing the URI.
func (r *Resource) UnmarshalText(text []byte) error {

	if r.ExecutionContext() == nil {
		return errors.New("mem.Resource: UnmarshalText requires ExecutionContext on receiver")
	}

	built, err := newFromURI(r.ExecutionContext(), string(text))
	if err != nil {
		return err
	}

	*r = *built
	return nil
}

// UnmarshalYAML populates the receiver from its YAML wire form (a bare URI scalar).
func (r *Resource) UnmarshalYAML(unmarshal func(any) error) error {

	if r.ExecutionContext() == nil {
		return errors.New("mem.Resource: UnmarshalYAML requires ExecutionContext on receiver")
	}

	var uri string
	if err := unmarshal(&uri); err != nil {
		return err
	}

	built, err := newFromURI(r.ExecutionContext(), uri)
	if err != nil {
		return err
	}

	*r = *built
	return nil
}

// endregion

// endregion

// region UNEXPORTED HELPERS

// newFromURI reconstructs a metadata-only mem.Resource from a URI string. No content is archived — callers
// who need content must archive separately. Used by UnmarshalJSON / UnmarshalText / UnmarshalStarlark /
// UnmarshalYAML.
func newFromURI(ctx *op.ExecutionContext, uri string) (*Resource, error) {

	rest, ok := strings.CutPrefix(uri, "mem:")
	if !ok {
		return nil, fmt.Errorf("mem.Resource: invalid URI %q (missing mem: prefix)", uri)
	}

	parts := strings.SplitN(rest, "/", 3)
	if parts[0] == "" {
		return nil, fmt.Errorf("mem.Resource: invalid URI %q (missing content type)", uri)
	}

	sourcePath, err := sourcePathFromURI(ctx.Root, uri)
	if err != nil {
		return nil, err
	}

	base, err := op.NewResourceBase(ctx, uri, reflect.TypeFor[*Resource]())
	if err != nil {
		return nil, err
	}

	r := &Resource{
		ResourceBase: base,
		ContentType:  parts[0],
		SourcePath:   sourcePath,
	}

	if len(parts) >= 2 {
		r.Namespace = parts[1]
	}
	if len(parts) >= 3 {
		r.Name = parts[2]
	}

	return r, nil
}

// sourcePathFromURI derives the on-disk source path for a mem URI under the given Root.
//
// The URI is expected to start with "mem:"; the remainder is treated as a slash-separated relative path
// under <Root>/.devlore/mem/.
func sourcePathFromURI(root op.Root, uri string) (op.Path, error) {

	rest, ok := strings.CutPrefix(uri, "mem:")
	if !ok || rest == "" {
		return op.Path{}, fmt.Errorf("invalid URI %q", uri)
	}

	return root.NewPath(filepath.Join(memArchiveDir, rest)), nil
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
