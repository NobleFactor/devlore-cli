// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package op

import (
	"encoding/base64"
	"fmt"
	"math"
	"strconv"
)

// region Constants

// The type names a document records for a value in an `any` position, one per Go type the converter produces.
//
// The shape is MongoDB Extended JSON's type wrapper object: a single `$`-prefixed key that names the type,
// with the value as its payload. The name IS the key, so a wrapper is self-identifying at any position
// without consulting a schema -- which is what a decoder needs, since it has no access to declared types.
//
// The names are Go's rather than BSON's. Extended JSON spells these `$numberLong` and `$numberDouble`, which
// would not read to anyone working in this codebase.
const (
	typeNameBool     = "$bool"
	typeNameBytes    = "$bytes"
	typeNameFloat64  = "$float64"
	typeNameInt64    = "$int64"
	typeNameList     = "$list"
	typeNameMap      = "$map"
	typeNameNil      = "$nil"
	typeNameResource = "$resource"
	typeNameString   = "$string"
)

// The payloads a non-finite float carries, which json cannot express as a bare number at all.
//
// Spelled as Canonical Extended JSON spells them rather than as Go's 'g' verb would ("+Inf", "NaN"), since the
// document format is the thing being described here and Extended JSON is its precedent.
const (
	payloadNegativeInfinity = "-Infinity"
	payloadNotANumber       = "NaN"
	payloadPositiveInfinity = "Infinity"
)

// encodeResource records a resource as its catalog id.
//
// The id names one entry in one ledger, which is exactly what a slot must say. A URI is globally meaningful
// and therefore cannot distinguish generations: `ns` maps a URI to whichever generation is CURRENT, while the
// ledger keeps every generation under its own id. A slot recorded by URI re-identifies to the current
// generation on reload, which is #735.
//
// The catalog is reached through the resource itself -- [Resource.RuntimeEnvironment] -- so the write side
// needs nothing threaded into it.
//
// Parameters:
//   - `resource`: the resource occupying the slot.
//
// Returns:
//   - `map[string]any`: the single-key wrapper carrying the catalog id.
//   - `error`: when the resource has no environment or catalog to name it, or the catalog does not hold it.
func encodeResource(resource Resource) (map[string]any, error) {

	runtimeEnvironment := resource.RuntimeEnvironment()
	if runtimeEnvironment == nil || runtimeEnvironment.ResourceCatalog == nil {
		return nil, fmt.Errorf("op.encodeTypeWrapper: %s %q has no catalog to name it",
			typeNameResource, resource.URI())
	}

	_, id := runtimeEnvironment.ResourceCatalog.Resolve(resource)
	if id == "" {
		return nil, fmt.Errorf("op.encodeTypeWrapper: %s %q is not in the catalog",
			typeNameResource, resource.URI())
	}

	return map[string]any{typeNameResource: id}, nil
}

// decodeResource binds the ledger entry a slot named, and only that entry.
//
// Identity only. Existence is NOT checked here: a `Pending` entry -- claimed but not yet produced -- is the
// normal state for a plan whose producing node has not run, and rejecting it would refuse a valid plan for
// describing work it has not done yet (docs/architecture/4-resource-management.md §3). The transition to
// `Active` or `Gone` belongs to the executor's pre-flight pass.
//
// A ledger miss is different in kind: the document names an entry its own ledger does not contain, which no
// amount of waiting repairs. That fails.
//
// Parameters:
//   - `payload`: the decoded payload, a catalog id.
//   - `catalog`: the catalog to look the id up in.
//
// Returns:
//   - `any`: the catalog's entry for the id.
//   - `error`: when there is no catalog, or the ledger does not hold the id.
func decodeResource(payload any, catalog *ResourceCatalog) (any, error) {

	id, isText := payload.(string)
	if !isText {
		return nil, fmt.Errorf("op.decodeTypeWrapper: %s payload is %T, want a catalog id", typeNameResource, payload)
	}

	if catalog == nil {
		return nil, fmt.Errorf("op.decodeTypeWrapper: %s %q needs a catalog to resolve against", typeNameResource, id)
	}

	resource, found := catalog.Lookup(id)
	if !found {
		return nil, fmt.Errorf("op.decodeTypeWrapper: %s %q is not in the ledger", typeNameResource, id)
	}

	return resource, nil
}

// endregion

// region Helpers

// encodeTypeWrapper wraps a value from an `any` position so the document records the type alongside it.
//
// Every value in an `any` position is wrapped, including those json could have determined on its own. The
// uniformity is what makes a missing wrapper detectable: under a partial rule a bare value is legal, so an
// omission cannot be told from a value that never needed one.
//
// Containers recurse, because an `any` position nested inside one is still an `any` position. A container
// whose element type is DECLARED is not reached by this function at all -- only `any` positions are.
//
// Parameters:
//   - `value`: the value occupying the `any` position.
//
// Returns:
//   - `map[string]any`: the single-key wrapper.
//   - `error`: when `value` has a Go type the document has no name for.
func encodeTypeWrapper(value any) (map[string]any, error) {

	switch v := value.(type) {
	case nil:
		return map[string]any{typeNameNil: nil}, nil
	case bool:
		return map[string]any{typeNameBool: v}, nil
	case string:
		return map[string]any{typeNameString: v}, nil
	case []byte:
		return map[string]any{typeNameBytes: base64.StdEncoding.EncodeToString(v)}, nil
	case int64:
		return map[string]any{typeNameInt64: strconv.FormatInt(v, 10)}, nil
	case float64:
		return map[string]any{typeNameFloat64: encodeFloat64(v)}, nil

	case []any:
		elements := make([]any, 0, len(v))
		for index, element := range v {
			encoded, err := encodeTypeWrapper(element)
			if err != nil {
				return nil, fmt.Errorf("op.encodeTypeWrapper: element %d: %w", index, err)
			}
			elements = append(elements, encoded)
		}
		return map[string]any{typeNameList: elements}, nil

	case Resource:
		return encodeResource(v)

	case map[string]any:
		entries := make(map[string]any, len(v))
		for key, element := range v {
			encoded, err := encodeTypeWrapper(element)
			if err != nil {
				return nil, fmt.Errorf("op.encodeTypeWrapper: key %q: %w", key, err)
			}
			entries[key] = encoded
		}
		return map[string]any{typeNameMap: entries}, nil
	}

	return nil, fmt.Errorf("op.encodeTypeWrapper: no document type name for %T", value)
}

// isTypeWrapper reports whether a decoded document value carries its own type.
//
// Structural, deliberately: a wrapper is a single-key mapping whose key names a type. A reader holding no
// declared type has no other way to tell a wrapper from an author's own single-entry map, and this is the
// property that lets the same check work at any position and at any depth.
//
// An author's map is never mistaken for a wrapper, because in an `any` position it is always the PAYLOAD of
// one -- `{"$map": {...}}` -- so what is examined here is the wrapper around it, not the map itself.
//
// Parameters:
//   - `value`: the decoded document value.
//
// Returns:
//   - `bool`: true when `value` is a type wrapper.
func isTypeWrapper(value any) bool {

	wrapper, isMap := value.(map[string]any)
	if !isMap || len(wrapper) != 1 {
		return false
	}

	for name := range wrapper {
		switch name {
		case typeNameBool, typeNameBytes, typeNameFloat64, typeNameInt64,
			typeNameList, typeNameMap, typeNameNil, typeNameResource, typeNameString:
			return true
		}
	}

	return false
}

// decodeTypeWrapper unwraps a value the document recorded a type for.
//
// The reader never infers. A value that is not a wrapper is refused rather than read for what it looks like:
// the writer knew the field's type and recorded it, so a reader reduced to inspecting a literal's shape is
// reading a document the writer got wrong.
//
// Parameters:
//   - `encoded`: the decoded document value occupying the `any` position.
//   - `catalog`: the catalog a `$resource` id is looked up in; may be nil when none is in play.
//
// Returns:
//   - `any`: the value with its recorded Go type.
//   - `error`: when `encoded` is not a wrapper, or names a type this reader does not know.
func decodeTypeWrapper(encoded any, catalog *ResourceCatalog) (any, error) {

	wrapper, isMap := encoded.(map[string]any)
	if !isMap || len(wrapper) != 1 {
		return nil, fmt.Errorf("op.decodeTypeWrapper: %#v is not a type wrapper; an `any` slot must carry one", encoded)
	}

	for name, payload := range wrapper {
		switch name {
		case typeNameNil:
			return nil, nil
		case typeNameBool:
			return decodeAs[bool](name, payload)
		case typeNameString:
			return decodeAs[string](name, payload)
		case typeNameBytes:
			return decodeBytes(payload)
		case typeNameInt64:
			return decodeInt64(payload)
		case typeNameFloat64:
			return decodeFloat64(payload)
		case typeNameList:
			return decodeList(payload, catalog)
		case typeNameMap:
			return decodeMap(payload, catalog)
		case typeNameResource:
			return decodeResource(payload, catalog)
		}

		return nil, fmt.Errorf("op.decodeTypeWrapper: unknown document type name %q", name)
	}

	return nil, fmt.Errorf("op.decodeTypeWrapper: empty type wrapper")
}

// encodeFloat64 renders a float64 as the text a document carries for it.
//
// The payload is a string, not a bare json number, for three independent reasons: json cannot express a
// non-finite float at all, a bare `42.0` may be renormalized to `42` by any conforming tool, and an integer
// beyond 2^53 loses digits to a reader that parses json numbers as doubles.
//
// Parameters:
//   - `value`: the float to render.
//
// Returns:
//   - `string`: the payload text.
func encodeFloat64(value float64) string {

	switch {
	case math.IsInf(value, 1):
		return payloadPositiveInfinity
	case math.IsInf(value, -1):
		return payloadNegativeInfinity
	case math.IsNaN(value):
		return payloadNotANumber
	}

	// Precision -1 is the shortest text that parses back to exactly this float64.
	return strconv.FormatFloat(value, 'g', -1, 64)
}

// decodeAs reads a payload the codec already decoded to the wanted Go type.
//
// Parameters:
//   - `name`: the type name, for the error message.
//   - `payload`: the decoded payload.
//
// Returns:
//   - `any`: the payload.
//   - `error`: when the payload is not of the wanted type.
func decodeAs[T any](name string, payload any) (any, error) {

	typed, ok := payload.(T)
	if !ok {
		return nil, fmt.Errorf("op.decodeTypeWrapper: %s payload is %T, want %T", name, payload, *new(T))
	}

	return typed, nil
}

// decodeBytes reads a base64 payload back to the bytes that were written.
//
// Parameters:
//   - `payload`: the decoded payload, a base64 string.
//
// Returns:
//   - `any`: the `[]byte`.
//   - `error`: when the payload is not a string, or not valid base64.
func decodeBytes(payload any) (any, error) {

	text, isText := payload.(string)
	if !isText {
		return nil, fmt.Errorf("op.decodeTypeWrapper: %s payload is %T, want a base64 string", typeNameBytes, payload)
	}

	decoded, err := base64.StdEncoding.DecodeString(text)
	if err != nil {
		return nil, fmt.Errorf("op.decodeTypeWrapper: %s payload is not base64: %w", typeNameBytes, err)
	}

	return decoded, nil
}

// decodeInt64 reads an integer payload with every digit it was written with.
//
// Parameters:
//   - `payload`: the decoded payload, a decimal string.
//
// Returns:
//   - `any`: the `int64`.
//   - `error`: when the payload is not a string, or does not parse.
func decodeInt64(payload any) (any, error) {

	text, isText := payload.(string)
	if !isText {
		return nil, fmt.Errorf("op.decodeTypeWrapper: %s payload is %T, want a decimal string", typeNameInt64, payload)
	}

	value, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("op.decodeTypeWrapper: %s payload %q: %w", typeNameInt64, text, err)
	}

	return value, nil
}

// decodeFloat64 reads a float payload, including the non-finite values json cannot express.
//
// Parameters:
//   - `payload`: the decoded payload, a decimal string or a non-finite name.
//
// Returns:
//   - `any`: the `float64`.
//   - `error`: when the payload is not a string, or does not parse.
func decodeFloat64(payload any) (any, error) {

	text, isText := payload.(string)
	if !isText {
		return nil, fmt.Errorf("op.decodeTypeWrapper: %s payload is %T, want a decimal string", typeNameFloat64, payload)
	}

	switch text {
	case payloadPositiveInfinity:
		return math.Inf(1), nil
	case payloadNegativeInfinity:
		return math.Inf(-1), nil
	case payloadNotANumber:
		return math.NaN(), nil
	}

	value, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return nil, fmt.Errorf("op.decodeTypeWrapper: %s payload %q: %w", typeNameFloat64, text, err)
	}

	return value, nil
}

// decodeList reads a list payload, unwrapping each element.
//
// Parameters:
//   - `payload`: the decoded payload, a sequence of wrappers.
//
// Returns:
//   - `any`: the `[]any`.
//   - `error`: when the payload is not a sequence, or an element does not unwrap.
func decodeList(payload any, catalog *ResourceCatalog) (any, error) {

	encoded, isList := payload.([]any)
	if !isList {
		return nil, fmt.Errorf("op.decodeTypeWrapper: %s payload is %T, want a sequence", typeNameList, payload)
	}

	elements := make([]any, 0, len(encoded))
	for index, element := range encoded {
		decoded, err := decodeTypeWrapper(element, catalog)
		if err != nil {
			return nil, fmt.Errorf("op.decodeTypeWrapper: %s element %d: %w", typeNameList, index, err)
		}
		elements = append(elements, decoded)
	}

	return elements, nil
}

// decodeMap reads a map payload, unwrapping each value.
//
// Parameters:
//   - `payload`: the decoded payload, a mapping of wrappers.
//
// Returns:
//   - `any`: the `map[string]any`.
//   - `error`: when the payload is not a mapping, or a value does not unwrap.
func decodeMap(payload any, catalog *ResourceCatalog) (any, error) {

	encoded, isMap := payload.(map[string]any)
	if !isMap {
		return nil, fmt.Errorf("op.decodeTypeWrapper: %s payload is %T, want a mapping", typeNameMap, payload)
	}

	entries := make(map[string]any, len(encoded))
	for key, element := range encoded {
		decoded, err := decodeTypeWrapper(element, catalog)
		if err != nil {
			return nil, fmt.Errorf("op.decodeTypeWrapper: %s key %q: %w", typeNameMap, key, err)
		}
		entries[key] = decoded
	}

	return entries, nil
}

// endregion
