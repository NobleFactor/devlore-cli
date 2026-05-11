// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package json

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v5"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// SchemeJSON is the URI scheme for JSON resources.
const SchemeJSON = "json"

// Resource represents a parsed JSON document held in memory.
//
// Unlike mem.Resource which holds opaque bytes with a content-type label, json.Resource holds a parsed Go value
// (map[string]any, []any, etc.) that can be validated against a JSON Schema or re-encoded without Starlark↔Go round
// trips.
//
// The URI is opaque: `json:<hash-prefix>`. The hash prefix is the first 12 characters of the SHA-256 of the raw bytes.
type Resource struct {
	op.ResourceBase
	Data   []byte `json:"data,omitempty"` // raw JSON bytes
	Hash   string `json:"hash,omitempty"` // SHA-256 of Data — metadata, NOT part of URI
	parsed any    // decoded Go value — validates/encodes without round trip
}

// String returns a compact JSON representation of the resource.
func (r *Resource) String() string { return r.Format(r) }

// Parsed returns the decoded Go value. The value is cached from the initial parse.
func (r *Resource) Parsed() any {
	return r.parsed
}

// Validate checks the parsed document against a JSON Schema.
//
// The schema is compiled from schemaJSON (a JSON string containing a valid JSON Schema document). Validation operates
// on the internal Go representation — no re-serialization is needed.
//
// Parameters:
//   - schemaJSON: a JSON string containing the JSON Schema to validate against
//
// Returns:
//   - ValidationResult: the validation outcome with Valid bool and Errors []string
//   - error: schema compilation errors (NOT validation errors — those go in ValidationResult.Errors)
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

// NewResource constructs a json.Resource and claims production via [op.ResourceCatalog.GetOrCreate].
//
// json.Resource is content-keyed — the URI is `json:<sha256-prefix>` derived from the raw bytes — so two
// callers with the same input produce the same URI and share a single canonical entry. The first caller's
// SiteID stamps producerID; subsequent same-content callers get the existing entry unchanged.
//
// Use NewResource from a producer dispatch context (typically [Provider.Parse]). The returned Resource is
// the canonical catalog entry. Use [DiscoverResource] instead when the caller is not claiming production
// (rehydration, the framework's slot-coercion adapter).
//
// Nil-Catalog tolerance: returns the unlinked candidate when no catalog is present.
//
// Parameters:
//   - activationRecord: the per-dispatch activation; its `Runtime` carries the runtime environment and its
//     `SiteID` becomes the catalog entry's producerID. Must be non-nil.
//   - value: raw JSON bytes ([]byte). Parsed during construction; an invalid JSON document errors here.
//
// Returns:
//   - *Resource: the canonical catalog entry (or the unlinked candidate when no catalog is present).
//   - error: if value is not []byte, the JSON does not parse, or [op.ResourceCatalog.GetOrCreate]'s strict
//     assertions fail.
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

// DiscoverResource constructs a json.Resource and registers it with [op.ResourceCatalog.Discover] without
// claiming production. Used by the framework's resource registry adapter for slot coercion. activationRecord
// is required for signature symmetry with [NewResource], but only activationRecord.Runtime is consumed.
// SiteID is unused (Discover doesn't stamp). Discovery callers commonly synthesize an [op.ActivationRecord]
// with empty SiteID and only Runtime set: `&op.ActivationRecord{Runtime: ctx}`.
//
// Nil-Catalog tolerance: returns the unlinked candidate when no catalog is present.
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

// buildCandidate validates value as []byte, parses the JSON, computes the SHA-256, and constructs a
// *Resource without touching the catalog. Shared by [NewResource] and [DiscoverResource]. The parse
// happens during construction — every json.Resource carries a valid parsed Go value (`r.parsed`); an
// invalid JSON document errors here rather than producing a half-initialized Resource.
func buildCandidate(runtimeEnvironment *op.RuntimeEnvironment, value any) (*Resource, error) {

	data, ok := value.([]byte)
	if !ok {
		return nil, fmt.Errorf("json.Resource: expected []byte, got %T", value)
	}

	var parsed any
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("json parse: %w", err)
	}

	h := sha256.Sum256(data)
	hash := hex.EncodeToString(h[:])

	base, err := op.NewResourceBase(runtimeEnvironment, SchemeJSON+":"+hash[:12], reflect.TypeFor[*Resource]())
	if err != nil {
		return nil, err
	}

	return &Resource{
		ResourceBase: base,
		Data:         data,
		Hash:         hash,
		parsed:       parsed,
	}, nil
}

// ValidationResult holds the outcome of a JSON Schema validation.
type ValidationResult struct {
	Valid  bool     `json:"valid"  starlark:"valid"`
	Errors []string `json:"errors" starlark:"errors"`
}
