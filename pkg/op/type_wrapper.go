// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package op

import (
	"encoding/base64"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
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
	typeNameBool    = "$bool"
	typeNameBytes   = "$bytes"
	typeNameFloat64 = "$float64"
	typeNameInt64   = "$int64"
	typeNameList    = "$list"
	typeNameMap     = "$map"
	// typeNamePrefix marks a document type name; a single-key mapping whose key carries it is an envelope.
	typeNamePrefix = "$"

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
// A cataloged resource names itself (ruled 2026-08-30). The catalog is the stamper, not the namer: it writes
// the id onto the [ResourceBase] when the resource is cataloged, and every later reader asks the resource. The
// write side therefore needs no catalog -- the environment's is nil by design once [NewGraph] has taken
// ownership, and asking it to `Resolve` would intern into a ledger a serializer was only meant to read.
//
// Parameters:
//   - `resource`: the resource occupying the slot.
//
// Returns:
//   - `map[string]any`: the single-key wrapper carrying the catalog id.
//   - `error`: when the resource was never cataloged, which is a defect upstream, not a case to serialize.
func encodeResource(resource Resource) (map[string]any, error) {

	id := resource.ID()
	if id == "" {
		return nil, fmt.Errorf("op.encodeTypeWrapper: %s %q is not cataloged", typeNameResource, resource.URI())
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
		// No catalog at this seam -- a receipt or a recovery stack decoding on its own. The id is kept, typed, and
		// resolved by [resolveRecordedResource] once the run's catalog is in hand.
		return recordedResourceID(id), nil
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
	case recordedResourceID:
		// A resource a trace recorded and a resume decoded with no catalog: an id already, re-emitted as one.
		return map[string]any{typeNameResource: string(v)}, nil

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

	return encodeReflected(value)
}

// encodeReflected is [encodeTypeWrapper]'s reflective step for Go's natural shapes (#712 decision 12). The typed
// cases above name the document's own types; this names everything else a caller legitimately holds. Every integer
// kind is a `$int64`; a `float32` widens exactly to `$float64`; a slice or array of any element type is a `$list` and
// a map with string keys a `$map`, elements enveloped recursively; a named bool or string is its kind. Anything else
// -- a struct, a channel, a function, a `uint64` beyond `MaxInt64` -- has no name, and that is the error.
//
// Parameters:
//   - `value`: a non-nil value none of the typed cases matched.
//
// Returns:
//   - `map[string]any`: the single-key wrapper.
//   - `error`: when `value` has a Go type the document has no name for.
func encodeReflected(value any) (map[string]any, error) {

	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Bool:
		return encodeTypeWrapper(reflected.Bool())
	case reflect.String:
		return encodeTypeWrapper(reflected.String())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return encodeTypeWrapper(reflected.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		if reflected.Uint() > math.MaxInt64 {
			return nil, fmt.Errorf("op.encodeTypeWrapper: %d exceeds the document's integer range", reflected.Uint())
		}
		return map[string]any{typeNameInt64: strconv.FormatUint(reflected.Uint(), 10)}, nil
	case reflect.Float32, reflect.Float64:
		return encodeTypeWrapper(reflected.Float())
	case reflect.Slice, reflect.Array:
		elements := make([]any, reflected.Len())
		for index := range elements {
			elements[index] = reflected.Index(index).Interface()
		}
		return encodeTypeWrapper(elements)
	case reflect.Map:
		if reflected.Type().Key().Kind() != reflect.String {
			return nil, fmt.Errorf("op.encodeTypeWrapper: no document type name for %T: map keys must be strings", value)
		}
		entries := make(map[string]any, reflected.Len())
		iterator := reflected.MapRange()
		for iterator.Next() {
			entries[iterator.Key().String()] = iterator.Value().Interface()
		}
		return encodeTypeWrapper(entries)
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

// unknownEnvelopeName reports whether a decoded value has an envelope's shape -- a single-key mapping whose key
// carries the `$` prefix -- but names a type this reader does not know, so a reader can refuse it by name rather
// than as an anonymous bare map (#712 phase 4).
//
// Parameters:
//   - `value`: the decoded document value.
//
// Returns:
//   - `string`: the unknown type name.
//   - `bool`: true when `value` is shaped like an envelope and [isTypeWrapper] is false for it.
func unknownEnvelopeName(value any) (string, bool) {

	wrapper, isMap := value.(map[string]any)
	if !isMap || len(wrapper) != 1 || isTypeWrapper(value) {
		return "", false
	}
	for name := range wrapper {
		if strings.HasPrefix(name, typeNamePrefix) {
			return name, true
		}
	}
	return "", false
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

// recordedResourceID is a `$resource` envelope's payload decoded where no catalog was available: a receipt or a
// recovery stack unmarshaling on its own. It is the catalog id, typed so [resolveRecordedResource] can tell it from
// a string result, and resolved by [ResourceCatalog.Lookup] at rehydration -- never by URI (#735).
type recordedResourceID string

// envelopeRecorded records a receipt or stack value for a document.
//
// The rule is the plan's (#712): a value with no declared type carries its own. A receipt's `result_type` is a
// declaration for a typed product -- the reader retypes it through the registry -- so a value the envelope has no
// name for (a provider's own result struct) stays bare, as today. A value the envelope does name (a scalar, bytes, a
// list, a map, a resource) is enveloped, because `result_type` is empty or names a type the registry cannot rebuild
// for exactly those, and a bare number would come back as a float64.
//
// Parameters:
//   - `value`: the recorded value.
//
// Returns:
//   - `any`: the envelope, or `value` unchanged when the envelope has no name for it.
func envelopeRecorded(value any) any {
	if value == nil {
		return nil
	}
	enveloped, err := encodeTypeWrapper(value)
	if err != nil {
		return value
	}
	return enveloped
}

// envelopeRecordedSlots applies [envelopeRecorded] to each slot value; the keys are slot names and stay bare.
//
// Parameters:
//   - `slots`: the recorded slot values, possibly nil.
//
// Returns:
//   - `map[string]any`: the same keys with enveloped values, or nil for nil.
func envelopeRecordedSlots(slots map[string]any) map[string]any {
	if slots == nil {
		return nil
	}
	out := make(map[string]any, len(slots))
	for name, value := range slots {
		out[name] = envelopeRecorded(value)
	}
	return out
}

// unwrapRecorded reads a recorded value back: an envelope is decoded (with no catalog, so a `$resource` becomes a
// [recordedResourceID]); anything else is the bare, declared-type form and passes through to the registry's retyping.
//
// Parameters:
//   - `value`: the document value.
//
// Returns:
//   - `any`: the decoded value.
//   - `error`: a malformed envelope.
func unwrapRecorded(value any) (any, error) {
	if !isTypeWrapper(value) {
		return value, nil
	}
	return decodeTypeWrapper(value, nil)
}

// unwrapRecordedSlots applies [unwrapRecorded] to each slot value.
//
// Parameters:
//   - `slots`: the document slot values, possibly nil.
//
// Returns:
//   - `map[string]any`: the decoded values under the same keys, or nil for nil.
//   - `error`: the first malformed envelope, naming its slot.
func unwrapRecordedSlots(slots map[string]any) (map[string]any, error) {
	if slots == nil {
		return nil, nil
	}
	out := make(map[string]any, len(slots))
	for name, value := range slots {
		decoded, err := unwrapRecorded(value)
		if err != nil {
			return nil, fmt.Errorf("slot %q: %w", name, err)
		}
		out[name] = decoded
	}
	return out, nil
}

// envelopeAnnotations envelopes each annotation value strictly (#712 phase 3, item 3). An annotation has no declared
// type, so every value carries its own, and a value the document has no name for is the error, naming its key. The
// keys are annotation names and stay bare.
//
// Parameters:
//   - `annotations`: the annotation values, possibly nil.
//
// Returns:
//   - `map[string]any`: the same keys with enveloped values; nil for nil.
//   - `error`: the first value the document has no name for, naming its key.
func envelopeAnnotations(annotations map[string]any) (map[string]any, error) {

	if annotations == nil {
		return nil, nil
	}
	out := make(map[string]any, len(annotations))
	for name, value := range annotations {
		enveloped, err := encodeTypeWrapper(value)
		if err != nil {
			return nil, fmt.Errorf("annotation %q: %w", name, err)
		}
		out[name] = enveloped
	}
	return out, nil
}

// unwrapAnnotations decodes each annotation value strictly: a bare value is an error naming its key, there being no
// declared type to read it against, and a `$resource` decodes to a [recordedResourceID], there being no catalog at
// this seam.
//
// Parameters:
//   - `annotations`: the document's annotation values, possibly nil.
//
// Returns:
//   - `map[string]any`: the decoded values under the same keys; nil for nil.
//   - `error`: the first bare or malformed value, naming its key.
func unwrapAnnotations(annotations map[string]any) (map[string]any, error) {

	if annotations == nil {
		return nil, nil
	}
	out := make(map[string]any, len(annotations))
	for name, value := range annotations {
		decoded, err := decodeTypeWrapper(value, nil)
		if err != nil {
			return nil, fmt.Errorf("annotation %q: %w", name, err)
		}
		out[name] = decoded
	}
	return out, nil
}
