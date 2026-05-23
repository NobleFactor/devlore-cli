// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package yaml

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
	"gopkg.in/yaml.v3"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// SchemeYAML is the URI scheme for YAML resources.
const SchemeYAML = "yaml"

// Interface Guard: *Resource implements op.Resource.
var _ op.Resource = (*Resource)(nil)

// Interface Guard: *Resource implements json.Unmarshaler.
var _ json.Unmarshaler = (*Resource)(nil)

// Interface Guard: *Resource implements encoding.TextUnmarshaler.
var _ encoding.TextUnmarshaler = (*Resource)(nil)

// Interface Guard: *Resource implements fmt.Stringer.
var _ fmt.Stringer = (*Resource)(nil)

// Resource represents a parsed YAML document held in memory, identified by the SHA-256 of its canonical form.
//
// yaml.Resource is an alternative input rendering of [json.Resource]: YAML input bytes are parsed into a Go value, then
// re-marshaled via [encoding/json] to produce a canonical byte form whose SHA-256 drives identity. Two semantically
// equal documents — whether YAML or JSON, regardless of indentation, key order, or comments — produce identical Hash
// values. The URI scheme stays `yaml:` so the catalog distinguishes the Resource types even when their underlying
// digests collide.
//
// Canonicalization caveats inherit from json.Resource (within-Go determinism only, float64 precision limit, UTF-8 sort
// order). Additionally, YAML-specific features that JSON cannot represent — typed tags (`!!timestamp`, `!!set`),
// anchors/aliases, comments, multi-line scalar styles — are flattened to their plain JSON equivalents during
// canonicalization. If typed-tag preservation becomes a requirement, swap this canonicalizer for a YAML-native one that
// routes through `*yaml.Node` and re-emits canonical YAML.
type Resource struct {
	op.ResourceBase

	// Data is the canonical JSON bytes of the parsed YAML document (sorted-key, whitespace-free). Identity bearing —
	// `SHA-256(Data)` is encoded in the URI <specific> as `yaml:<Hash>`.
	Data []byte `json:"data,omitempty"`

	// Hash is the lowercase hex SHA-256 of Data, identity-bearing. Also encoded in the URI <specific>.
	Hash string `json:"hash,omitempty"`

	// parsed is the decoded Go value, cached at construction for [Resource.Parsed] / [Resource.Validate].
	// Not persisted; rehydration from URI leaves parsed nil.
	parsed any
}

// NewResource constructs a yaml.Resource and claims production via [op.ResourceCatalog.GetOrCreate].
//
// yaml.Resource is content-keyed via canonical-JSON-form digest — two callers with semantically equal YAML inputs (or,
// equivalently, YAML that decodes to the same Go value as some JSON document) produce the same URI and share a single
// catalog entry. The first caller's `Unit.ID()` stamps producerID.
//
// Use [DiscoverResource] instead when the caller is not claiming production.
//
// Nil-Catalog tolerance: returns the unlinked candidate when no catalog is present.
//
// Parameters:
//   - `activationRecord`: per-dispatch activation; its `RuntimeEnvironment` supplies the runtime environment, and
//     its `Unit.ID()` becomes the catalog entry's producerID (empty when `Unit` is nil). Must be non-nil.
//   - `value`: raw YAML bytes ([]byte), an [io.Reader] streaming YAML, or a canonical tag URI string. Bytes and
//     streams are parsed + canonicalized during construction; an invalid YAML document errors here.
//
// Returns:
//   - *Resource: canonical catalog entry, or the unlinked candidate when no catalog is present.
//   - `error`: unsupported value type, YAML parse failure, malformed URI, or identity construction failure.
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
		return nil, fmt.Errorf("yaml.NewResource: catalog entry for %q is %T, want *yaml.Resource", candidate.URI(), got)
	}

	return canonical, nil
}

// DiscoverResource constructs a yaml.Resource and registers it without claiming production.
//
// Used by the framework's resource registry adapter for slot coercion (when starlark supplies a string and the slot
// expects a *yaml.Resource) and by callers holding a reference handle without claiming production. UnmarshalJSON /
// UnmarshalText / UnmarshalYAML rehydration is the canonical use case.
//
// `activationRecord` is required for signature symmetry with [NewResource], but only its `RuntimeEnvironment` is
// consumed — `Unit` is unused since Discover doesn't stamp a producer. Discovery callers commonly construct one
// as `op.NewActivationRecord(nil, nil, runtimeEnvironment)` — both `Graph` and `Unit` nil.
//
// Same value-shape dispatch as [NewResource]: raw YAML bytes, an [io.Reader], or a canonical tag URI string.
//
// Nil-Catalog tolerance: returns the unlinked candidate when no catalog is present.
//
// Parameters:
//   - `activationRecord`: per-dispatch activation; only its `RuntimeEnvironment` is consumed. Must be non-nil with
//     a non-nil `RuntimeEnvironment`.
//   - `value`: raw YAML bytes ([]byte), an [io.Reader], or a canonical tag URI string; same dispatch as
//     [NewResource].
//
// Returns:
//   - *Resource: canonical catalog entry, or the unlinked candidate when no catalog is present.
//   - `error`: unsupported value type, YAML parse failure, malformed URI, or identity construction failure.
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
		return nil, fmt.Errorf("yaml.DiscoverResource: catalog entry for %q is %T, want *yaml.Resource", candidate.URI(), got)
	}

	return canonical, nil
}

// buildCandidate returns an unlinked *Resource for value.
//
// []byte values are parsed as YAML and re-marshaled to canonical JSON form; the canonical bytes are stored as
// Data, hashed for identity, and the parsed Go value is cached. [io.Reader] values are drained and routed
// through the []byte path. String values are treated as canonical tag URIs, and the hash is extracted from
// the URI's <specific> for metadata-only rehydration (Data and parsed are left empty). Resource catalog
// interaction is the caller's concern, not this function's. See [NewResource] and [DiscoverResource].
//
// Parameters:
//   - runtimeEnvironment: runtime environment threaded into the produced [op.ResourceBase].
//   - value: []byte (raw YAML, will be canonicalized) or string (canonical tag URI).
//
// Returns:
//   - *Resource: unlinked candidate.
//   - error: unsupported value type, YAML parse failure, malformed URI, URI <specific> not in `yaml:<hex>`
//     form, or [op.ResourceBase] construction failure.
func buildCandidate(runtimeEnvironment *op.RuntimeEnvironment, value any) (*Resource, error) {

	switch v := value.(type) {

	case []byte:
		return newFromBytes(runtimeEnvironment, v)

	case io.Reader:
		return newFromReader(runtimeEnvironment, v)

	case string:
		return newFromURI(runtimeEnvironment, v)

	default:
		return nil, fmt.Errorf("yaml.Resource: expected []byte, io.Reader, or URI string, got %T", value)
	}
}

// newFromBytes parses YAML, canonicalizes through JSON, hashes, and builds a *Resource from raw YAML bytes.
func newFromBytes(runtimeEnvironment *op.RuntimeEnvironment, data []byte) (*Resource, error) {

	canonical, parsed, err := canonicalize(data)
	if err != nil {
		return nil, err
	}

	sum := sha256.Sum256(canonical)
	hash := hex.EncodeToString(sum[:])

	base, err := op.NewResourceBase(runtimeEnvironment, SchemeYAML+":"+hash, reflect.TypeFor[*Resource]())
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
// Canonicalization requires the full document in memory (YAML → Go value → canonical JSON re-marshal), so
// there is no stream-while-hashing fast path the way mem.Resource has — the reader must be fully drained
// before the YAML parser runs.
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
		return nil, fmt.Errorf("yaml.Resource: read stream: %w", err)
	}

	return newFromBytes(runtimeEnvironment, data)
}

// newFromURI rehydrates a metadata-only *Resource from a canonical tag URI.
//
// The URI's <specific> must be `yaml:<hex>` with hex being a full 64-character lowercase SHA-256. Data and
// parsed are left empty — callers that need the content must reconstruct via [NewResource]([]byte).
func newFromURI(runtimeEnvironment *op.RuntimeEnvironment, uri string) (*Resource, error) {

	specific, _, err := op.ExtractTagSpecific(uri)
	if err != nil {
		return nil, fmt.Errorf("yaml.Resource: %w", err)
	}

	if specific == "" {
		return nil, fmt.Errorf("yaml.Resource: cannot reconstruct from deferred URI %q", uri)
	}

	hashPart, ok := strings.CutPrefix(specific, SchemeYAML+":")
	if !ok {
		return nil, fmt.Errorf("yaml.Resource: URI specific %q is not in yaml:<hex> form", specific)
	}
	if _, err := hex.DecodeString(hashPart); err != nil {
		return nil, fmt.Errorf("yaml.Resource: invalid digest hex %q: %w", hashPart, err)
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

// canonicalize parses YAML input bytes and re-marshals them through [encoding/json] to a canonical byte form.
//
// YAML decodes into a Go value; [encoding/json] then serializes that value with sorted map keys and no whitespace. The
// result is byte-identical to what [json.Resource]'s canonicalize would produce for a semantically equal JSON input.
// See the [Resource] type doc for limitations.
//
// Parameters:
//   - data: raw YAML bytes.
//
// Returns:
//   - []byte: canonical JSON bytes of the YAML document's Go-value form.
//   - any: the decoded Go value (map[string]any / []any / scalar), cached for [Resource.Parsed].
//   - error: YAML parse failure, JSON re-marshal failure, or normalization failure.
func canonicalize(data []byte) ([]byte, any, error) {

	var parsed any
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		return nil, nil, fmt.Errorf("yaml parse: %w", err)
	}

	// yaml.v3 may produce map[interface{}]interface{} for non-string keys; round-trip through JSON to normalize into
	// pure map[string]any, mirroring the shape json.Resource works with.
	normalized, err := normalizeForJSON(parsed)
	if err != nil {
		return nil, nil, fmt.Errorf("yaml normalize: %w", err)
	}

	canonical, err := json.Marshal(normalized)
	if err != nil {
		return nil, nil, fmt.Errorf("yaml canonicalize: %w", err)
	}

	return canonical, normalized, nil
}

// normalizeForJSON coerces a YAML-decoded value into a JSON-marshal-compatible shape.
//
// yaml.v3 sometimes produces map[interface{}]interface{} for non-string-keyed maps, which json.Marshal cannot handle
// directly. Round-tripping the value through json.Marshal + json.Unmarshal flattens those maps to map[string]any
// (string keys, the only JSON-legal key shape) while preserving all scalar and array values.
//
// Parameters:
//   - v: the value produced by [yaml.Unmarshal] into `any`.
//
// Returns:
//   - any: the same value with all map keys coerced to strings (via Go's json round-trip).
//   - error: any error from json.Marshal or json.Unmarshal.
func normalizeForJSON(v any) (any, error) {

	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}

	var result any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// region EXPORTED METHODS

// region State management

// Addressing reports that yaml.Resource is content-addressed.
//
// Overrides [op.ResourceBase.Addressing]'s [op.AddressingUnknown] default.
//
// Returns:
//   - op.AddressingMode: [op.AddressingContent] — identity is the SHA-256 of the canonical JSON form.
func (r *Resource) Addressing() op.AddressingMode {
	return op.AddressingContent
}

// Digest returns the content digest of the canonical bytes.
//
// The SHA-256 was computed during construction (or parsed from the URI on rehydration) and stamped on Hash.
// Overrides [op.ResourceBase.Digest]'s [op.ErrUnimplemented] default.
//
// Returns:
//   - op.Digest: {Algorithm: "sha256", Bytes: decoded Hash}.
//   - error: non-nil if Hash is malformed; should not occur post-construction or post-rehydration.
func (r *Resource) Digest() (op.Digest, error) {
	return op.ParseDigest("sha256:" + r.Hash)
}

// Equal reports whether r and other identify the same yaml.Resource.
//
// Strict equality: other must be a *yaml.Resource. URI comparison is delegated to [op.ResourceBase.Equal].
//
// Parameters:
//   - other: candidate value; nil or any non-*yaml.Resource value returns false.
//
// Returns:
//   - bool: true when other is a *yaml.Resource with the same URI as r.
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
// Returns nil when the Resource was rehydrated from a URI alone.
//
// Returns:
//   - any: the parsed Go value, or nil for URI-rehydrated Resources.
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

// Resolve is a no-op for yaml resources — identity is the canonical content; nothing to probe at resolve time.
//
// Returns:
//   - error: nil under normal conditions.
func (r *Resource) Resolve() error {
	return nil
}

// UnmarshalJSON populates the receiver from its JSON wire form (a bare URI string).
//
// The caller pre-seeds the receiver's embedded [op.ResourceBase] with a valid [op.RuntimeEnvironment] before
// invocation. The URI alone reconstructs the Resource metadata; Data and parsed are left empty.
//
// Parameters:
//   - data: JSON bytes encoding a single bare URI string.
//
// Returns:
//   - error: missing RuntimeEnvironment on receiver, malformed JSON, or rehydration failure.
func (r *Resource) UnmarshalJSON(data []byte) error {

	if r.RuntimeEnvironment() == nil {
		return errors.New("yaml.Resource: UnmarshalJSON requires RuntimeEnvironment on receiver")
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
// Same prerequisites and semantics as [Resource.UnmarshalJSON].
//
// Parameters:
//   - text: UTF-8 bytes containing the canonical tag URI.
//
// Returns:
//   - error: missing RuntimeEnvironment on receiver, or rehydration failure.
func (r *Resource) UnmarshalText(text []byte) error {

	if r.RuntimeEnvironment() == nil {
		return errors.New("yaml.Resource: UnmarshalText requires RuntimeEnvironment on receiver")
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
// Same prerequisites and semantics as [Resource.UnmarshalJSON].
//
// Parameters:
//   - unmarshal: the YAML decode hook supplied by the YAML library; called with a *string target.
//
// Returns:
//   - error: missing RuntimeEnvironment on receiver, decode failure, or rehydration failure.
func (r *Resource) UnmarshalYAML(unmarshal func(any) error) error {

	if r.RuntimeEnvironment() == nil {
		return errors.New("yaml.Resource: UnmarshalYAML requires RuntimeEnvironment on receiver")
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

// Validate checks the parsed document against a JSON Schema.
//
// YAML documents validate against JSON Schema because the canonical form is JSON; the cached parsed Go value has
// already been normalized to JSON-compatible shapes during construction.
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
		return ValidationResult{}, fmt.Errorf("yaml validate: add schema: %w", err)
	}

	schema, err := compiler.Compile("schema.json")
	if err != nil {
		return ValidationResult{}, fmt.Errorf("yaml validate: compile schema: %w", err)
	}

	if err := schema.Validate(r.parsed); err != nil {
		var ve *jsonschema.ValidationError
		if !errors.As(err, &ve) {
			return ValidationResult{}, fmt.Errorf("yaml validate: %w", err)
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
