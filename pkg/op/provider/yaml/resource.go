// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package yaml

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

// SchemeYAML is the URI scheme for YAML resources.
const SchemeYAML = "yaml"

// Resource represents a parsed YAML document held in memory.
//
// Like json.Resource, yaml.Resource holds a parsed Go value that can be validated against a JSON Schema or re-encoded
// without Starlark↔Go roundtrips. JSON Schema validation works because YAML is a superset of JSON — the decoded Go
// representation (map[string]any, []any, etc.) is the same structure that JSON Schema operates on.
//
// The URI is opaque: yaml:<hash-prefix>. The hash prefix is the first 12 characters of the SHA-256 of the raw bytes.
type Resource struct {
	op.ResourceBase
	Data   []byte `json:"data,omitempty"` // raw YAML bytes
	Hash   string `json:"hash,omitempty"` // SHA-256 of Data — metadata, NOT part of URI
	parsed any    // decoded Go value — validates/encodes without roundtrip
}

// String returns a compact JSON representation of the resource.
func (r *Resource) String() string { return r.Format(r) }

// Parsed returns the decoded Go value. The value is cached from the initial parse.
func (r *Resource) Parsed() any {
	return r.parsed
}

// Validate checks the parsed document against a JSON Schema.
//
// YAML documents are validated against JSON Schema because the decoded Go representation (map[string]any, []any) is
// structurally identical to decoded JSON. The schema is compiled from schemaJSON (a JSON string containing a valid
// JSON Schema document).
//
// Parameters:
//   - schemaJSON: a JSON string containing the JSON Schema to validate against
//
// Returns:
//   - ValidationResult: the validation outcome with Valid bool and Errors []string
//   - error: schema compilation errors (NOT validation errors — those go in ValidationResult.Errors)
func (r *Resource) Validate(schemaJSON string) (ValidationResult, error) {
	// YAML v3 decodes to map[string]any, but nested structures may still contain interface{} keys.
	// Normalize to pure map[string]any by round-tripping through JSON.
	normalized, err := normalizeForSchema(r.parsed)
	if err != nil {
		return ValidationResult{}, fmt.Errorf("yaml validate: normalize: %w", err)
	}

	compiler := jsonschema.NewCompiler()

	if err := compiler.AddResource("schema.json", strings.NewReader(schemaJSON)); err != nil {
		return ValidationResult{}, fmt.Errorf("yaml validate: add schema: %w", err)
	}

	schema, err := compiler.Compile("schema.json")
	if err != nil {
		return ValidationResult{}, fmt.Errorf("yaml validate: compile schema: %w", err)
	}

	if err := schema.Validate(normalized); err != nil {
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

// normalizeForSchema converts any remaining map[interface{}]interface{} to map[string]any via JSON round-trip.
func normalizeForSchema(v any) (any, error) {
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

// NewResource creates a yaml.Resource from a value.
//
// Parameters:
//   - ctx: the execution context.
//   - value: expected to be []byte (raw YAML data).
//
// Returns:
//   - *Resource: the initialized resource.
//   - error: if value is not a supported type.
func NewResource(ctx *op.ExecutionContext, value any) (*Resource, error) {

	var data []byte

	switch v := value.(type) {
	case []byte:
		data = v
	default:
		return nil, fmt.Errorf("yaml.Resource: expected []byte, got %T", value)
	}

	checksum := sha256.Sum256(data)
	hash := hex.EncodeToString(checksum[:])

	base, err := op.NewResourceBase(ctx, SchemeYAML+":"+hash[:12], reflect.TypeFor[*Resource]())
	if err != nil {
		return nil, err
	}

	r := &Resource{
		ResourceBase: base,
		Data:         data,
		Hash:         hash,
	}

	return r, nil
}

// ValidationResult holds the outcome of a JSON Schema validation.
type ValidationResult struct {
	Valid  bool     `json:"valid"  starlark:"valid"`
	Errors []string `json:"errors" starlark:"errors"`
}
