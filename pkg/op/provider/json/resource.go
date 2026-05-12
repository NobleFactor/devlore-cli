// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package json

import (
	"crypto/sha256"
	"encoding"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v5"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// SchemeJSON is the URI scheme for JSON resources.
const SchemeJSON = "json"

// Interface Guard: *Resource implements op.Resource.
var _ op.Resource = (*Resource)(nil)

// Interface Guard: *Resource implements json.Unmarshaler.
var _ json.Unmarshaler = (*Resource)(nil)

// Interface Guard: *Resource implements encoding.TextUnmarshaler.
var _ encoding.TextUnmarshaler = (*Resource)(nil)

// Interface Guard: *Resource implements fmt.Stringer.
var _ fmt.Stringer = (*Resource)(nil)

// Resource represents a parsed JSON document held in memory, identified by the SHA-256 of its canonical form.
//
// Unlike mem.Resource which holds opaque bytes, json.Resource carries a parsed Go value (map[string]any, []any,
// scalars) that can be validated against a JSON Schema or re-encoded without Starlark↔Go round trips.
//
// Identity is content-addressed via canonicalization: the input bytes are parsed with [encoding/json] and
// re-marshaled to produce a canonical byte form (map keys sorted, no whitespace, stable scalar serialization).
// The SHA-256 of those canonical bytes drives the URI-specific (`json:<hex>`) and the Hash field. Two
// semantically equal inputs — `{"a":1,"b":2}` and `{"b":2,"a":1}` — produce identical URIs by construction.
//
// Canonicalization caveats:
//   - Not RFC 8785 (JCS) compliant. Within-Go determinism only; cross-language portability not in scope.
//   - Numbers larger than 2^53 lose precision via float64 round-trip — two distinct large integers can collide.
//   - Object key sort is UTF-8 byte order (Go's [encoding/json] default), not the UTF-16 order JCS specifies.
//     Agrees with JCS for ASCII keys; diverges for the supplementary plane.
type Resource struct {
	op.ResourceBase

	// Data is the canonical JSON bytes (sorted-key, whitespace-free re-marshal of the parsed input). Identity-
	// bearing — SHA-256(Data) is encoded in the URI <specific> as `json:<Hash>`.
	Data []byte `json:"data,omitempty"`

	// Hash is the lowercase hex SHA-256 of Data, identity-bearing. Also encoded in the URI <specific>.
	Hash string `json:"hash,omitempty"`

	// parsed is the decoded Go value, cached at construction for [Resource.Parsed] / [Resource.Validate].
	// Not persisted; rehydration from URI leaves parsed nil.
	parsed any
}

// NewResource constructs a json.Resource and claims production via [op.ResourceCatalog.GetOrCreate].
//
// json.Resource is content-keyed — the URI is `json:<sha256-hex>` derived from the canonical form of the input, so two
// callers with semantically equal inputs produce the same URI and share a single catalog entry. The first caller's
// SiteID stamps producerID; subsequent same-content callers get the existing entry unchanged.
//
// Use NewResource from a producer dispatch context. Use [DiscoverResource] instead when the caller is not claiming
// production (rehydration, the framework's slot-coercion adapter).
//
// Nil-Catalog tolerance: returns the unlinked candidate when no catalog is present.
//
// Parameters:
//   - activationRecord: per-dispatch activation; its Runtime supplies the runtime environment, and its SiteID
//     becomes the catalog entry's producerID. Must be non-nil.
//   - value: raw JSON bytes ([]byte), an [io.Reader] streaming JSON, or a canonical tag URI string. Bytes and
//     streams are parsed + canonicalized during construction; an invalid JSON document errors here.
//
// Returns:
//   - *Resource: canonical catalog entry, or the unlinked candidate when no catalog is present.
//   - error: unsupported value type, JSON parse failure, malformed URI, or identity construction failure.
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
		return nil, fmt.Errorf("json.NewResource: catalog entry for %q is %T, want *json.Resource", candidate.URI(), got)
	}

	return canonical, nil
}

// DiscoverResource constructs a json.Resource and registers it without claiming production.
//
// Used by the framework's resource registry adapter for slot coercion (when starlark supplies a string and the slot
// expects a *json.Resource) and by callers holding a reference handle without claiming production. UnmarshalJSON /
// UnmarshalText / UnmarshalYAML rehydration is the canonical use case.
//
// activationRecord is required for signature symmetry with [NewResource], but only activationRecord.Runtime is
// consumed. SiteID is unused (Discover does not stamp). Discovery callers commonly synthesize an [op.ActivationRecord]
// with empty SiteID and only Runtime set: `&op.ActivationRecord{Runtime: runtimeEnvironment}`.
//
// Same value-shape dispatch as [NewResource]: raw JSON bytes, an [io.Reader], or a canonical tag URI string.
//
// Nil-Catalog tolerance: returns the unlinked candidate when no catalog is present.
//
// Parameters:
//   - activationRecord: per-dispatch activation; only its Runtime is consumed. Must be non-nil with a non-nil
//     Runtime.
//   - value: raw JSON bytes ([]byte), an [io.Reader], or a canonical tag URI string; same dispatch as
//     [NewResource].
//
// Returns:
//   - *Resource: canonical catalog entry, or the unlinked candidate when no catalog is present.
//   - error: unsupported value type, JSON parse failure, malformed URI, or identity construction failure.
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
		return nil, fmt.Errorf("json.DiscoverResource: catalog entry for %q is %T, want *json.Resource", candidate.URI(), got)
	}

	return canonical, nil
}

// buildCandidate returns an unlinked *Resource for value.
//
// []byte values are parsed and re-marshaled to canonical form; the canonical bytes are stored as Data, hashed
// for identity, and the parsed Go value is cached. [io.Reader] values are drained and routed through the
// []byte path. String values are treated as canonical tag URIs, and the hash is extracted from the URI's
// <specific> for metadata-only rehydration (Data and parsed are left empty). Resource catalog interaction is
// the caller's concern, not this function's. See [NewResource] and [DiscoverResource].
//
// Parameters:
//   - runtimeEnvironment: runtime environment threaded into the produced [op.ResourceBase].
//   - value: []byte (raw JSON), [io.Reader] (streaming JSON), or string (canonical tag URI).
//
// Returns:
//   - *Resource: unlinked candidate.
//   - error: unsupported value type, JSON parse failure, malformed URI, URI <specific> not in `json:<hex>` form,
//     or [op.ResourceBase] construction failure.
func buildCandidate(runtimeEnvironment *op.RuntimeEnvironment, value any) (*Resource, error) {

	switch v := value.(type) {

	case []byte:
		return newFromBytes(runtimeEnvironment, v)

	case io.Reader:
		return newFromReader(runtimeEnvironment, v)

	case string:
		return newFromURI(runtimeEnvironment, v)

	default:
		return nil, fmt.Errorf("json.Resource: expected []byte, io.Reader, or URI string, got %T", value)
	}
}

// newFromBytes parses, canonicalizes, hashes, and builds a *Resource from raw JSON bytes.
func newFromBytes(runtimeEnvironment *op.RuntimeEnvironment, data []byte) (*Resource, error) {

	canonical, parsed, err := canonicalize(data)
	if err != nil {
		return nil, err
	}

	sum := sha256.Sum256(canonical)
	hash := hex.EncodeToString(sum[:])

	base, err := op.NewResourceBase(runtimeEnvironment, SchemeJSON+":"+hash, reflect.TypeFor[*Resource]())
	if err != nil {
		return nil, err
	}

	return &Resource{
		ResourceBase: base,
		Data:         canonical,
		Hash:         hash,
		parsed:       parsed,
	}, nil
}

// newFromReader drains a stream and forwards to [newFromBytes] for parse + canonicalization.
//
// Canonicalization requires the full document in memory (sorted keys, stable re-marshal), so there is no
// stream-while-hashing fast path the way mem.Resource has — the reader must be fully drained before the parser
// runs. The drain cost is unavoidable for any content-addressed JSON form.
//
// Parameters:
//   - runtimeEnvironment: runtime environment threaded into the produced [op.ResourceBase].
//   - reader: source of payload bytes; drained completely via [io.ReadAll].
//
// Returns:
//   - *Resource: candidate produced by [newFromBytes] over the drained bytes.
//   - error: any error from [io.ReadAll] or from [newFromBytes].
func newFromReader(runtimeEnvironment *op.RuntimeEnvironment, reader io.Reader) (*Resource, error) {

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("json.Resource: read stream: %w", err)
	}

	return newFromBytes(runtimeEnvironment, data)
}

// newFromURI rehydrates a metadata-only *Resource from a canonical tag URI.
//
// The URI's <specific> must be `json:<hex>` with hex being a full 64-character lowercase SHA-256. Data and parsed are
// left empty — callers that need the content must reconstruct via [NewResource]([]byte).
func newFromURI(runtimeEnvironment *op.RuntimeEnvironment, uri string) (*Resource, error) {

	specific, _, err := op.ExtractTagSpecific(uri)
	if err != nil {
		return nil, fmt.Errorf("json.Resource: %w", err)
	}

	if specific == "" {
		return nil, fmt.Errorf("json.Resource: cannot reconstruct from deferred URI %q", uri)
	}

	hashPart, ok := strings.CutPrefix(specific, SchemeJSON+":")
	if !ok {
		return nil, fmt.Errorf("json.Resource: URI specific %q is not in json:<hex> form", specific)
	}
	if _, err := hex.DecodeString(hashPart); err != nil {
		return nil, fmt.Errorf("json.Resource: invalid digest hex %q: %w", hashPart, err)
	}

	base, err := op.NewResourceBase(runtimeEnvironment, specific, reflect.TypeFor[*Resource]())
	if err != nil {
		return nil, err
	}

	return &Resource{
		ResourceBase: base,
		Hash:         hashPart,
	}, nil
}

// canonicalize parses input bytes as JSON and re-marshals them to a canonical form.
//
// Go's [encoding/json] handles most of the work: object keys are sorted lexicographically by byte, whitespace outside
// strings is dropped, and scalar serialization is stable. See the [Resource] type doc for the known limitations (UTF-8
// vs. UTF-16 sort order, large-integer precision).
//
// Parameters:
//   - data: raw JSON bytes.
//
// Returns:
//   - []byte: canonical JSON bytes.
//   - any: the decoded Go value (map[string]any / []any / scalar), cached for [Resource.Parsed].
//   - error: parse failure or re-marshal failure.
func canonicalize(data []byte) ([]byte, any, error) {

	var parsed any
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, nil, fmt.Errorf("json parse: %w", err)
	}

	canonical, err := json.Marshal(parsed)
	if err != nil {
		return nil, nil, fmt.Errorf("json canonicalize: %w", err)
	}

	return canonical, parsed, nil
}

// region EXPORTED METHODS

// region State management

// Addressing reports that json.Resource is content-addressed.
//
// Overrides [op.ResourceBase.Addressing]'s [op.AddressingUnknown] default.
//
// Returns:
//   - op.AddressingMode: [op.AddressingContent] — identity is the SHA-256 of the canonical JSON bytes.
func (r *Resource) Addressing() op.AddressingMode {
	return op.AddressingContent
}

// Digest returns the content digest of the canonical bytes.
//
// The SHA-256 was computed during construction (or parsed from the URI on rehydration) and stamped on Hash. Reassembles
// the canonical `sha256:<hex>` form via [op.ParseDigest], producing the strict [op.Digest] shape. Overrides
// [op.ResourceBase.Digest]'s [op.ErrUnimplemented] default.
//
// Returns:
//   - op.Digest: {Algorithm: "sha256", Bytes: decoded Hash}.
//   - error: non-nil if Hash is malformed; should not occur post-construction or post-rehydration.
func (r *Resource) Digest() (op.Digest, error) {
	return op.ParseDigest("sha256:" + r.Hash)
}

// Equal reports whether r and other identify the same json.Resource.
//
// Strict equality: the `other` must be a *json.Resource. URI comparison is delegated to [op.ResourceBase.Equal].
//
// Parameters:
//   - other: candidate value; nil or any non-*json.Resource value returns false.
//
// Returns:
//   - bool: true when other is a *json.Resource with the same URI as r.
func (r *Resource) Equal(other any) bool {

	if other == nil {
		return false
	}

	if _, ok := other.(*Resource); !ok {
		return false
	}

	return r.ResourceBase.Equal(other)
}

// Parsed returns the decoded Go value cached during construction.
//
// Returns nil when the Resource was rehydrated from a URI (parsed is not reconstructed from URI alone. Call
// [NewResource]([]byte) to reparse from canonical bytes if needed).
//
// Returns:
//   - any: the parsed Go value (map[string]any / []any / scalar), or nil for URI-rehydrated Resources.
func (r *Resource) Parsed() any {
	return r.parsed
}

// String returns the compact JSON encoding of the Resource for debug output. Delegates to
// [op.ResourceBase.Format].
//
// Returns:
//   - string: the compact JSON encoding of r.
func (r *Resource) String() string {
	return r.Format(r)
}

// endregion

// region Behaviors

// Resolve is a no-op for json resources — identity is the canonical content; nothing to probe at resolve time.
//
// Returns:
//   - error: nil under normal conditions.
func (r *Resource) Resolve() error {
	return nil
}

// UnmarshalJSON populates the receiver from its JSON wire form (a bare URI string).
//
// The caller pre-seeds the receiver's embedded [op.ResourceBase] with a valid [op.RuntimeEnvironment] before
// invocation. The URI alone reconstructs the Resource metadata; Data and parsed are left empty — call
// [NewResource]([]byte) if the canonical bytes are needed.
//
// Parameters:
//   - data: JSON bytes encoding a single bare URI string.
//
// Returns:
//   - error: missing RuntimeEnvironment on receiver, malformed JSON, or rehydration failure.
func (r *Resource) UnmarshalJSON(data []byte) error {

	if r.RuntimeEnvironment() == nil {
		return errors.New("json.Resource: UnmarshalJSON requires RuntimeEnvironment on receiver")
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
// Same prerequisites and semantics as [Resource.UnmarshalJSON].
//
// Parameters:
//   - text: UTF-8 bytes containing the canonical tag URI.
//
// Returns:
//   - error: missing RuntimeEnvironment on receiver, or rehydration failure.
func (r *Resource) UnmarshalText(text []byte) error {

	if r.RuntimeEnvironment() == nil {
		return errors.New("json.Resource: UnmarshalText requires RuntimeEnvironment on receiver")
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
// Same prerequisites and semantics as [Resource.UnmarshalJSON].
//
// Parameters:
//   - unmarshal: yaml decode hook supplied by the YAML library; called with a *string target.
//
// Returns:
//   - error: missing RuntimeEnvironment on receiver, decode failure, or rehydration failure.
func (r *Resource) UnmarshalYAML(unmarshal func(any) error) error {

	if r.RuntimeEnvironment() == nil {
		return errors.New("json.Resource: UnmarshalYAML requires RuntimeEnvironment on receiver")
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

// Validate checks the parsed document against a JSON Schema.
//
// Operates on the cached parsed Go value — no re-serialization needed. Returns an error only on schema compilation
// failures; validation outcomes are returned in the [ValidationResult].
//
// Parameters:
//   - schemaJSON: a JSON string containing the JSON Schema to validate against.
//
// Returns:
//   - ValidationResult: the validation outcome with Valid bool and Errors []string.
//   - error: schema compilation errors (NOT validation errors — those go in ValidationResult.Errors).
func (r *Resource) Validate(schemaJSON string) (ValidationResult, error) {

	compiler := jsonschema.NewCompiler()

	if err := compiler.AddResource("schema.json", strings.NewReader(schemaJSON)); err != nil {
		return ValidationResult{}, fmt.Errorf("json validate: add schema: %w", err)
	}

	schema, err := compiler.Compile("schema.json")
	if err != nil {
		return ValidationResult{}, fmt.Errorf("json validate: compile schema: %w", err)
	}

	if err := schema.Validate(r.parsed); err != nil {
		var ve *jsonschema.ValidationError
		if !errors.As(err, &ve) {
			return ValidationResult{}, fmt.Errorf("json validate: %w", err)
		}

		basic := ve.BasicOutput()
		var errs []string
		for _, e := range basic.Errors {
			if e.Error != "" {
				errs = append(errs, e.Error)
			}
		}

		return ValidationResult{Valid: false, Errors: errs}, nil
	}

	return ValidationResult{Valid: true}, nil
}

// endregion

// endregion

// region Auxiliary Types

// ValidationResult holds the outcome of a JSON Schema validation.
type ValidationResult struct {

	// Valid is true when the document conforms to the schema.
	Valid bool `json:"valid"  starlark:"valid"`

	// Errors is the list of validation error messages; empty when Valid is true.
	Errors []string `json:"errors" starlark:"errors"`
}

// endregion
