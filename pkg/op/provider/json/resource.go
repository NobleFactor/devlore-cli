// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package json

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v5"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// SchemeJSON is the URI scheme for JSON resources.
const SchemeJSON = "json"

// Resource represents a parsed JSON document held in memory.
//
// Unlike mem.Resource which holds opaque bytes with a content-type label, json.Resource holds a parsed Go value
// (map[string]any, []any, etc.) that can be validated against a JSON Schema or re-encoded without Starlark↔Go
// roundtrips.
//
// The URI is opaque: json:<hash-prefix>. The hash prefix is the first 12 characters of the SHA-256 of the raw bytes.
type Resource struct {
	op.ResourceBase
	Data   []byte `json:"data,omitempty"` // raw JSON bytes
	Hash   string `json:"hash,omitempty"` // SHA-256 of Data — metadata, NOT part of URI
	parsed any    // decoded Go value — validates/encodes without roundtrip
}

// String returns a compact JSON representation of the resource.
func (r *Resource) String() string { return r.Format(r) }

// buildURI computes the opaque json: URI.
func (r *Resource) buildURI() string {
	if r.Hash != "" {
		return SchemeJSON + ":" + r.Hash[:12]
	}
	return SchemeJSON + ":inline"
}

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

// NewResource creates a json.Resource from raw bytes and a pre-parsed Go value.
func NewResource(data []byte, parsed any) Resource {
	h := sha256.Sum256(data)

	r := Resource{
		Data:   data,
		Hash:   hex.EncodeToString(h[:]),
		parsed: parsed,
	}
	r.SetURI(r.buildURI())
	return r
}

// ResourceFromValue constructs a json.Resource from a string json: URI.
//
// Parameters:
//   - v: expected to be a string in the format "json:<qualifier>"
//
// Returns:
//   - Resource: initialized with the parsed URI
//   - error: if v is not a string or the URI format is invalid
func ResourceFromValue(v any) (Resource, error) {
	s, ok := v.(string)
	if !ok {
		return Resource{}, fmt.Errorf("json.Resource: expected string URI, got %T", v)
	}

	if !strings.HasPrefix(s, SchemeJSON+":") {
		return Resource{}, fmt.Errorf("json.Resource: expected json: URI, got %q", s)
	}

	qualifier := s[len(SchemeJSON+":"):]

	r := Resource{
		Hash: qualifier,
	}
	r.SetURI(s)
	return r, nil
}

// ValidationResult holds the outcome of a JSON Schema validation.
type ValidationResult struct {
	Valid  bool     `json:"valid"  starlark:"valid"`
	Errors []string `json:"errors" starlark:"errors"`
}
