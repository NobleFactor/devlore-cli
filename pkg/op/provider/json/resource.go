// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package json

import (
	"crypto/sha256"
	"encoding/hex"
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

// NewResource creates a json.Resource from a value.
//
// Parameters:
//   - ctx: the execution context.
//   - value: expected to be []byte (raw JSON data) or a pre-parsed Go value.
//
// Returns:
//   - *Resource: the initialized resource.
//   - error: if value is not a supported type.
func NewResource(ctx *op.RuntimeEnvironment, value any) (*Resource, error) {

	var data []byte
	var parsed any

	switch v := value.(type) {
	case []byte:
		data = v
	default:
		return nil, fmt.Errorf("json.Resource: expected []byte, got %T", value)
	}

	h := sha256.Sum256(data)
	hash := hex.EncodeToString(h[:])

	base, err := op.NewResourceBase(ctx, SchemeJSON+":"+hash[:12], reflect.TypeFor[*Resource]())
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

// DiscoverResource constructs a json.Resource and registers it with [op.ResourceCatalog.Discover] without
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
		return nil, fmt.Errorf("json.DiscoverResource: catalog entry for %q is %T, want *json.Resource", candidate.URI(), got)
	}
	return canonical, nil
}

// ValidationResult holds the outcome of a JSON Schema validation.
type ValidationResult struct {
	Valid  bool     `json:"valid"  starlark:"valid"`
	Errors []string `json:"errors" starlark:"errors"`
}
