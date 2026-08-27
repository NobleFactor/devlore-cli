// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package op

import (
	"encoding"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
)

var (
	// resourceInterfaceType is the [reflect.Type] of [Resource], cached for [Convert]'s registered-Resource step.
	resourceInterfaceType = reflect.TypeFor[Resource]()

	// sourceConverterType is the cached [reflect.Type] of [SourceConverter].
	//
	// Used by [typesAreInterconvertible] to test whether a candidate source type opts into the source-side
	// conversion contract.
	sourceConverterType = reflect.TypeFor[SourceConverter]()

	// targetConverterType is the cached [reflect.Type] of [TargetConverter].
	//
	// Used by [typesAreInterconvertible] to test whether a candidate target type opts into the target-side
	// conversion contract.
	targetConverterType = reflect.TypeFor[TargetConverter]()
)

// Convert projects a Go value into the target type via the type-matching cascade.
//
// Convert is the single source of truth for Go↔Go projection in the framework. Every starlark-bridge entry point
// (wrapper extraction, plan-mode slot fill, immediate-mode dispatch) and method-dispatch site ([Method.Invoke]) routes
// through here so type-matching semantics stay in one place. Convert itself is context-blind: graph
// dispatch precedes it with identity resolution at [Method.Invoke]'s seam ([resolveDispatchResource],
// 4-resource-management.md §5.6) — a resource-typed slot value there resolves through the run catalog and
// never reaches the construction steps below.
//
// The cascade:
//
//  1. Identity — value's type is the target type. Return as-is.
//  2. Assignability — value's underlying type is assignable to target ([reflect.Type.AssignableTo]).
//  3. Slice element conversion — both source and target are slices; recurse element-wise.
//  4. Map element conversion — both source and target are maps; recurse key-and-value-wise.
//  5. Source-side opt-in — value implements [SourceConverter] and advertises the target type.
//  6. Registered Resource construction — target implements [Resource], and a constructor is registered in
//     [RuntimeEnvironment.ReceiverRegistry]; the constructor is run with (runtimeEnvironment, value).
//  7. Target-side opt-in — fresh target probe implements [TargetConverter] and advertises the source type.
//  8. Text unmarshal — string source into a target (or its pointer) implementing [encoding.TextUnmarshaler], e.g.
//     [time.Time]; the target reconstructs itself from the text bytes.
//  9. Struct hydration — map source into a struct target; each exported field is filled from the map by its
//     `json`/`yaml` tag (or field name), recursing through Convert.
//  10. Error — no path through the cascade succeeds.
//
// Parameters:
//   - `runtimeEnvironment`: the ambient [RuntimeEnvironment]. Step 6 uses its [Registry] for registered
//     Resource construction.
//   - `value`: the source value to project. `nil` yields the zero value of `target`.
//   - `target`: the [`reflect.Type`] of the desired result.
//
// Returns:
//   - `any`: the projected value, ready to assign to a target of type `target`.
//   - `error`: non-nil if no path through the cascade succeeds.
func Convert(runtimeEnvironment *RuntimeEnvironment, value any, target reflect.Type) (any, error) {

	// Step 0: nil → zero of target.

	if value == nil {
		return reflect.Zero(target).Interface(), nil
	}

	// Step 0.5: a serialized number, read against the type of the field it fills.

	if v, ok, err := tryParseSerializedNumber(value, target); ok {
		return v, err
	}

	// Steps 1-2: identity, the `any` target, and (pointer-dereferenced) assignability/convertibility —
	// see [convertDirect].

	if converted, ok := convertDirect(value, target); ok {
		return converted, nil
	}

	elem := reflect.ValueOf(value)
	for elem.Kind() == reflect.Pointer {
		elem = elem.Elem()
	}

	// Step 3: slice element conversion.

	if v, ok, err := tryConvertSlice(runtimeEnvironment, elem, target); ok {
		return v, err
	}

	// Step 4: map element conversion.

	if v, ok, err := tryConvertMap(runtimeEnvironment, elem, target); ok {
		return v, err
	}

	// Step 5: source-side opt-in.

	if c, ok := value.(SourceConverter); ok && c.CanConvertTo(target) {
		return c.ConvertTo(target)
	}

	// Step 6: registered Resource construction.

	if v, ok, err := tryConstructResource(runtimeEnvironment, value, target); ok {
		return v, err
	}

	// Step 7: target-side opt-in.

	if v, ok, err := tryTargetConverter(value, target); ok {
		return v, err
	}

	// Step 8: text unmarshal (string -> encoding.TextUnmarshaler target, e.g. time.Time).

	if v, ok, err := tryTextUnmarshaler(value, target); ok {
		return v, err
	}

	// Step 9: struct hydration (map -> struct).

	if v, ok, err := tryHydrateStruct(runtimeEnvironment, elem, target); ok {
		return v, err
	}

	// Step 10: not convertible.

	return nil, explainRefusal(value, target)
}

// explainRefusal renders [Convert]'s step-10 failure, naming the remedy when there is one.
//
// A conversion Go permits but [losesMeaning] refuses lands here with nothing left to say for itself, and
// "int64 value is neither assignable nor convertible to string" is Go's vocabulary for a starlark author's
// mistake. Python answers the same three cases with a sentence that names the fix, and starlark has str(),
// int(), and float() to offer -- so the message points at them.
//
// Anything else keeps the general form: the value's type could not reach the target's, which is all that is
// known about it.
//
// Parameters:
//   - `value`: the value that could not be converted.
//   - `target`: the destination type.
//
// Returns:
//   - `error`: the refusal, with guidance when the case admits any.
func explainRefusal(value any, target reflect.Type) error {

	source := reflect.TypeOf(value)
	if source == nil {
		return fmt.Errorf("%T value is neither assignable nor convertible to %s", value, target)
	}

	sourceKind, targetKind := source.Kind(), target.Kind()

	switch {
	case targetKind == reflect.String && (isSignedInteger(sourceKind) || isUnsignedInteger(sourceKind)):
		return fmt.Errorf("cannot use %s where %s is wanted: write str(x) to render it", source, target)

	case (isSignedInteger(targetKind) || isUnsignedInteger(targetKind)) &&
		(sourceKind == reflect.Float32 || sourceKind == reflect.Float64):
		return fmt.Errorf("a %s cannot be interpreted as %s: write int(x) to truncate it deliberately",
			source, target)

	case (isSignedInteger(sourceKind) || isUnsignedInteger(sourceKind)) &&
		(isSignedInteger(targetKind) || isUnsignedInteger(targetKind)):
		return fmt.Errorf("%v is out of range for %s", value, target)
	}

	return fmt.Errorf("%T value is neither assignable nor convertible to %s", value, target)
}

// convertDirect handles [Convert]'s direct paths — steps 1 through 2.
//
// Step 1: identity. Pointer-equal reflect.Type means the same underlying `*rtype`, so `==` is a single pointer
// comparison and subsumes the assignability identity case without paying for reflect.ValueOf, the deref walk,
// or the Interface() round-trip. Hot path for slot-fill from Parameter.Default (already at p.Type) and for any
// caller-supplied value whose dynamic type already matches target exactly.
//
// Step 1.5: empty interface (`any`) target. Any non-nil value satisfies `any` — return as-is. Crucially,
// SKIP the pointer-deref of step 2: a *T value passed to an `any`-typed target must preserve its pointer
// shape, because callers downstream (e.g., a method whose signature is `[]*T`) need the pointer back. The
// bridge's early-projection path uses this when goReceiver.Project(reflect.TypeFor[any]()) asks for the
// natural Go form of a wrapped instance — *op.Invocation must come back as *op.Invocation, not op.Invocation.
//
// Step 2: assignability with pointer-deref. Dereference pointers so a *T value reaches a T target through the
// underlying assignability rule.
//
// Parameters:
//   - `value`: the non-nil value under conversion.
//   - `target`: the destination type.
//
// Returns:
//   - `any`: the converted value when a direct path applied.
//   - `bool`: true when a direct path applied.
func convertDirect(value any, target reflect.Type) (any, bool) {

	if reflect.TypeOf(value) == target {
		return value, true
	}

	if target.Kind() == reflect.Interface && target.NumMethod() == 0 {
		return value, true
	}

	elem := reflect.ValueOf(value)
	for elem.Kind() == reflect.Pointer {
		elem = elem.Elem()
	}

	if elem.IsValid() {
		if elem.Type().AssignableTo(target) {
			return elem.Interface(), true
		}
		if elem.Type().ConvertibleTo(target) && !losesMeaning(elem, target) {
			return elem.Convert(target).Interface(), true
		}
	}

	if reflect.TypeOf(value).AssignableTo(target) {
		return value, true
	}

	return nil, false
}

// losesMeaning reports whether a conversion Go permits would produce a value the author did not write.
//
// [reflect.Type.ConvertibleTo] answers a question about REPRESENTABILITY, and Go's answer is yes to several
// conversions that silently change meaning: an integer becomes the rune at its code point, a wide integer
// wraps into a narrow one, a negative reinterprets as unsigned, and a float loses its fraction.
//
// Starlark is python-shaped, so the rule here is python's rather than Go's: a cross-category conversion is an
// error whatever the value, and within the integer category the value must be in range. str(), int(), and
// float() exist for an author who means the conversion, and their rendering is correct by construction.
//
//	int -> string     rejected by kind. 65 would become "A". A value check cannot catch this: 65 round-trips
//	                  back to 65, and 0 round-trips through "\x00". Only the kinds tell the truth.
//	float -> int      rejected by kind, INCLUDING an integral float. python rejects [1,2,3][1.0] though 1.0 is
//	                  integral, and a rule that took 3.0 but refused 3.9 is one no author could predict.
//	int -> int        allowed in range, rejected outside it. file.mkdir(mode=0o644) reaches an os.FileMode --
//	                  a uint32 -- so this pair cannot be rejected by kind.
//	int -> float      allowed. python widens implicitly, as in 1 + 2.0.
//
// Parameters:
//   - `elem`: the dereferenced source value.
//   - `target`: the destination type.
//
// Returns:
//   - `bool`: true when the conversion must be refused.
func losesMeaning(elem reflect.Value, target reflect.Type) bool {

	source := elem.Kind()

	// A string target reached from a number. Only integers are ConvertibleTo string in Go -- float and bool
	// already fail earlier -- and an integer arriving here means a rune conversion nobody asked for.
	if target.Kind() == reflect.String && (isSignedInteger(source) || isUnsignedInteger(source)) {
		return true
	}

	// An integer target reached from a float, fractional or not.
	if (isSignedInteger(target.Kind()) || isUnsignedInteger(target.Kind())) &&
		(source == reflect.Float32 || source == reflect.Float64) {
		return true
	}

	// Integer to integer: the one pair whose value decides.
	if (isSignedInteger(source) || isUnsignedInteger(source)) &&
		(isSignedInteger(target.Kind()) || isUnsignedInteger(target.Kind())) {

		return integerLosesValue(elem, target)
	}

	// Float to float: narrowing that overflows silently yields ±Inf. Precision loss does not count -- python
	// packs 0.1 into a single-precision float without complaint and only raises when the value will not fit.
	if (source == reflect.Float32 || source == reflect.Float64) &&
		(target.Kind() == reflect.Float32 || target.Kind() == reflect.Float64) {

		return !math.IsInf(elem.Float(), 0) && math.IsInf(elem.Convert(target).Float(), 0)
	}

	return false
}

// integerLosesValue reports whether an integer-to-integer conversion changes the value.
//
// The obvious test is a round trip: convert, convert back, compare. It catches narrowing and it catches a
// negative reinterpreted as unsigned at a DIFFERENT width -- but it is blind to the same-width case, because
// signed and unsigned of equal width share a bit pattern. uint64(MaxUint64) converts to int64(-1) and back to
// uint64(MaxUint64), comparing equal while the value it denotes has changed entirely.
//
// So sign is asked about directly, and only then the round trip.
//
// Parameters:
//   - `elem`: the dereferenced source value, of an integer kind.
//   - `target`: the destination integer type.
//
// Returns:
//   - `bool`: true when the conversion changes the value.
func integerLosesValue(elem reflect.Value, target reflect.Type) bool {

	source := elem.Kind()

	// A negative can never be an unsigned value, whatever the widths.
	if isSignedInteger(source) && isUnsignedInteger(target.Kind()) && elem.Int() < 0 {
		return true
	}

	// An unsigned value above the signed range wraps to a negative, and at equal width does so invisibly.
	if isUnsignedInteger(source) && isSignedInteger(target.Kind()) && elem.Uint() > math.MaxInt64 {
		return true
	}

	return !elem.Convert(target).Convert(elem.Type()).Equal(elem)
}

// isSignedInteger reports whether kind is one of Go's signed integer kinds.
//
// Parameters:
//   - `kind`: the reflect kind under test.
//
// Returns:
//   - `bool`: true for int through int64.
func isSignedInteger(kind reflect.Kind) bool {
	return kind >= reflect.Int && kind <= reflect.Int64
}

// isUnsignedInteger reports whether kind is one of Go's unsigned integer kinds.
//
// Uintptr is deliberately excluded: it is an address, not a number an author writes.
//
// Parameters:
//   - `kind`: the reflect kind under test.
//
// Returns:
//   - `bool`: true for uint through uint64.
func isUnsignedInteger(kind reflect.Kind) bool {
	return kind >= reflect.Uint && kind <= reflect.Uint64
}

// tryParseSerializedNumber handles [Convert]'s step 0.5: a [json.Number] reaching a numeric target.
//
// A json document has ONE number type, so unmarshalling into an `any` slot cannot say whether 420 was written
// as an integer or a float. [LoadGraph] decodes with UseNumber so the literal text survives, and the type of
// the field being filled is what decides how to read it -- an fs.FileMode wants an integer, a timeout wants a
// float, and the text answers either without a lossy float64 in between.
//
// This is a PARSE, not a numeric conversion, which is why it sits ahead of the direct paths: json.Number is a
// string kind, and Go will not convert a string to a numeric type at all.
//
// Range is enforced by parsing against the target's bit size, so a value the field cannot hold is an error
// rather than a truncation.
//
// Parameters:
//   - `value`: the value under conversion.
//   - `target`: the destination type.
//
// Returns:
//   - `parsed`: the number read against the field, when this step applied.
//   - `applied`: true when this step applied.
//   - `err`: non-nil when the text does not fit the field.
func tryParseSerializedNumber(value any, target reflect.Type) (parsed any, applied bool, err error) {

	number, isNumber := value.(json.Number)
	if !isNumber {
		return nil, false, nil
	}

	switch target.Kind() {

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		signed, parseErr := strconv.ParseInt(number.String(), 10, target.Bits())
		if parseErr != nil {
			return nil, true, fmt.Errorf("serialized number %s does not fit %s: %w", number, target, parseErr)
		}

		return reflect.ValueOf(signed).Convert(target).Interface(), true, nil

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		unsigned, parseErr := strconv.ParseUint(number.String(), 10, target.Bits())
		if parseErr != nil {
			return nil, true, fmt.Errorf("serialized number %s does not fit %s: %w", number, target, parseErr)
		}

		return reflect.ValueOf(unsigned).Convert(target).Interface(), true, nil

	case reflect.Float32, reflect.Float64:
		floating, parseErr := strconv.ParseFloat(number.String(), target.Bits())
		if parseErr != nil {
			return nil, true, fmt.Errorf("serialized number %s does not fit %s: %w", number, target, parseErr)
		}

		return reflect.ValueOf(floating).Convert(target).Interface(), true, nil
	}

	return nil, false, nil
}

// tryConvertSlice handles [Convert]'s step 3: slice → slice element-wise recursion.
//
// Heterogeneous-shaped sources ([]any from starlark lists) cannot satisfy AssignableTo against typed Go slices
// ([]string, []*file.Resource). Each element is recursed through the full [Convert] cascade so element-level
// conversions compose. Returns (nil, false, nil) when either input is not a slice.
//
// Parameters:
//   - `runtimeEnvironment`: forwarded to the recursive [Convert] calls for env-sensitive steps.
//   - `elem`: the source value as a [reflect.Value] (already pointer-dereferenced by Convert step 2).
//   - `target`: the desired slice target type.
//
// Returns:
//   - `any`: the constructed slice when applicable; nil otherwise.
//   - `bool`: true when this step applied (regardless of error); false when neither input is a slice.
//   - `error`: non-nil when an element conversion failed.
func tryConvertSlice(
	runtimeEnvironment *RuntimeEnvironment,
	elem reflect.Value,
	target reflect.Type,
) (converted any, applied bool, err error) {

	if elem.Kind() != reflect.Slice || target.Kind() != reflect.Slice {
		return nil, false, nil
	}

	n := elem.Len()
	out := reflect.MakeSlice(target, n, n)

	for i := range n {
		converted, err := Convert(runtimeEnvironment, elem.Index(i).Interface(), target.Elem())
		if err != nil {
			return nil, true, fmt.Errorf("slice index %d: %w", i, err)
		}
		out.Index(i).Set(reflect.ValueOf(converted))
	}

	return out.Interface(), true, nil
}

// tryConvertMap handles [Convert]'s step 4: map → map key-and-value recursion.
//
// Heterogeneous-shaped sources (map[any]any, map[string]any from starlark dictionaries) cannot satisfy AssignableTo
// against typed Go maps. Keys and values are recursed through the full [Convert] cascade so map-element conversions
// compose. Returns (nil, false, nil) when either input is not a map.
//
// Parameters:
//   - `runtimeEnvironment`: forwarded to the recursive [Convert] calls for env-sensitive steps.
//   - `elem`: the source value as a [reflect.Value] (already pointer-dereferenced by Convert step 2).
//   - `target`: the desired map target type.
//
// Returns:
//   - `any`: the constructed map when applicable; nil otherwise.
//   - `bool`: true when this step applied (regardless of error); false when neither input is a map.
//   - `error`: non-nil when a key or value conversion failed.
func tryConvertMap(
	runtimeEnvironment *RuntimeEnvironment,
	elem reflect.Value,
	target reflect.Type,
) (converted any, applied bool, err error) {

	if elem.Kind() != reflect.Map || target.Kind() != reflect.Map {
		return nil, false, nil
	}

	out := reflect.MakeMapWithSize(target, elem.Len())

	iter := elem.MapRange()
	for iter.Next() {

		convertedKey, err := Convert(runtimeEnvironment, iter.Key().Interface(), target.Key())
		if err != nil {
			return nil, true, fmt.Errorf("map key %v: %w", iter.Key().Interface(), err)
		}

		convertedValue, err := Convert(runtimeEnvironment, iter.Value().Interface(), target.Elem())
		if err != nil {
			return nil, true, fmt.Errorf("map value for %v: %w", iter.Key().Interface(), err)
		}

		out.SetMapIndex(reflect.ValueOf(convertedKey), reflect.ValueOf(convertedValue))
	}

	return out.Interface(), true, nil
}

// tryConstructResource handles [Convert]'s step 6: registered Resource construction.
//
// This step serves plan-time claiming (an authored string minting its pending entry — "plan time converts
// and claims", §9 item 6) and immediate mode (a session constructing from a path). Graph dispatch never
// reaches it: resource-typed slot values resolve by identity at [Method.Invoke]'s seam
// ([resolveDispatchResource], 4-resource-management.md §5.6), where a run-catalog miss refuses instead of
// falling through to construction here.
//
// Tried before [TargetConverter] (step 7) so Resources with both a registered constructor and a [TargetConverter]
// opt-in use the env-aware canonical path at dispatch: the registered constructor receives the full
// [RuntimeEnvironment] (catalog, root, registry, etc.) and can produce a fully-canonicalized Resource.
// [TargetConverter] (step 7) is reached only when no registered constructor applies — env-less library callers, tests,
// or non-Resource target types — and serves as the framework's plan-time convertibility probe via
// [typesAreInterconvertible]. Resources without a registered constructor still get the [TargetConverter] path;
// Resources with one always prefer the canonical.
//
// Returns (nil, false, nil) when `target` is not a Resource type or no [RuntimeEnvironment.ReceiverRegistry] is
// available.
//
// Parameters:
//   - `runtimeEnvironment`: provides [RuntimeEnvironment.ReceiverRegistry] for type lookup and is passed to the
//     registered constructor.
//   - `value`: the source value to project.
//   - `target`: the Resource target type.
//
// Returns:
//   - `any`: the constructed Resource when applicable; nil otherwise.
//   - `bool`: true when this step applied (regardless of error).
//   - `error`: non-nil when registry lookup, type assertion, or constructor execution fails.
func tryConstructResource(
	runtimeEnvironment *RuntimeEnvironment,
	value any,
	target reflect.Type,
) (constructed any, applied bool, err error) {

	if !target.Implements(resourceInterfaceType) || runtimeEnvironment == nil {
		return nil, false, nil
	}

	// An interface target constructs through its designated claim type (§5.7 rule 6): the planner
	// substitutes at the claiming seam, and immediate mode arrives here instead, so the designation is
	// consulted from both. One registry, one answer — an interface means the same claim whichever seam
	// the value came through.
	if target.Kind() == reflect.Interface {
		if mint, designated := resourceMintFor(target); designated {
			target = mint
		}
	}

	// Resources are typically announced under the value type (file.Resource), but the parameter type is the pointer
	// (*file.Resource). Try the pointer-or-element fallback the registry's other lookups use.
	rt, ok := ReceiverRegistry().TypeByReflection(target)
	if !ok && target.Kind() == reflect.Pointer {
		rt, ok = ReceiverRegistry().TypeByReflection(target.Elem())
	}
	if !ok && target.Kind() != reflect.Pointer {
		rt, ok = ReceiverRegistry().TypeByReflection(reflect.PointerTo(target))
	}
	if !ok {
		return nil, true, fmt.Errorf("resource type %s not registered — must be announced via op.AnnounceResource", target)
	}

	rrt, isResourceReceiverType := rt.(ResourceReceiverType)
	if !isResourceReceiverType {
		return nil, true, fmt.Errorf("type %s registered as %T, not as ResourceReceiverType", target, rt)
	}

	v, err := rrt.Construct()(runtimeEnvironment, value)
	if err != nil {
		return nil, true, fmt.Errorf("construct %s from %T: %w", target, value, err)
	}

	return v, true, nil
}

// tryTargetConverter handles [Convert]'s step 7: target-side opt-in.
//
// Probe must be a *target or target-as-already-pointer, since converter methods conventionally sit on the pointer
// receiver. Reached after step 6, so registered-Resource canonicalization always wins at dispatch when env is
// available; step 7 fires for RuntimeEnvironment-les callers, non-Resource target types, and Resources whose registry
// entry is missing.
//
// Returns (nil, false, nil) when the target probe does not implement [TargetConverter] or declines to
// absorb `value`'s type via [TargetConverter.CanConvertFrom].
//
// Parameters:
//   - `value`: the source value to project.
//   - `target`: the desired target type.
//
// Returns:
//   - `any`: the constructed value when applicable; nil otherwise.
//   - `bool`: true when this step applied; false when the target does not opt into [TargetConverter] for
//     `value`'s type.
//   - `error`: non-nil when [TargetConverter.ConvertFrom] fails.
func tryTargetConverter(value any, target reflect.Type) (converted any, applied bool, err error) {

	var probe any
	if target.Kind() == reflect.Pointer {
		probe = reflect.New(target.Elem()).Interface()
	} else {
		probe = reflect.New(target).Interface()
	}

	t, ok := probe.(TargetConverter)
	if !ok || !t.CanConvertFrom(reflect.TypeOf(value)) {
		return nil, false, nil
	}

	v, err := t.ConvertFrom(value)
	return v, true, err
}

// tryTextUnmarshaler handles [Convert]'s step 8: a string source into a text-absorbing target.
//
// A value serialized through [encoding.TextMarshaler] (e.g. [time.Time] → RFC 3339) reloads as a plain string; a target
// — or its pointer — implementing [encoding.TextUnmarshaler] reconstructs itself from those bytes. Returns
// (nil, false, nil) when the source is not a string or the target does not absorb text.
//
// Parameters:
//   - `value`: the source value; only a string applies.
//   - `target`: the desired target type.
//
// Returns:
//   - `any`: the unmarshaled value (target kind, or its pointer when the target is a pointer); nil otherwise.
//   - `bool`: true when this step applied (regardless of error); false when it does not apply.
//   - `error`: non-nil when [encoding.TextUnmarshaler.UnmarshalText] fails.
func tryTextUnmarshaler(value any, target reflect.Type) (unmarshaled any, applied bool, err error) {

	text, isString := value.(string)
	if !isString {
		return nil, false, nil
	}

	concrete := target
	if concrete.Kind() == reflect.Pointer {
		concrete = concrete.Elem()
	}

	pointer := reflect.New(concrete)
	unmarshaler, ok := pointer.Interface().(encoding.TextUnmarshaler)
	if !ok {
		return nil, false, nil
	}

	if err := unmarshaler.UnmarshalText([]byte(text)); err != nil {
		return nil, true, fmt.Errorf("unmarshal text into %s: %w", target, err)
	}

	if target.Kind() == reflect.Pointer {
		return pointer.Interface(), true, nil
	}
	return pointer.Elem().Interface(), true, nil
}

// tryHydrateStruct handles [Convert]'s step 9: a map source into a struct target.
//
// A struct serializes to an object and reloads as `map[string]any` — the codec drops the Go type. Given the target
// struct type (on restore, the source's produced type id; otherwise the consumer's slot), this rebuilds it: each
// exported field is filled from the map by its `json`/`yaml` tag (or field name), recursing every field value through
// [Convert] so nested structs, resources, slices, and maps compose. Returns (nil, false, nil) when the source is not a
// string-keyed map or the target (after one pointer deref) is not a struct.
//
// Parameters:
//   - `runtimeEnvironment`: forwarded to the recursive [Convert] calls for env-sensitive field types (resources).
//   - `elem`: the source value as a [reflect.Value] (pointer-dereferenced by [Convert] step 2).
//   - `target`: the desired struct or *struct target type.
//
// Returns:
//   - `any`: the populated struct (or its pointer when the target is a pointer); nil otherwise.
//   - `bool`: true when this step applied (regardless of error); false when it does not apply.
//   - `error`: non-nil when a field conversion fails.
func tryHydrateStruct(
	runtimeEnvironment *RuntimeEnvironment, elem reflect.Value, target reflect.Type,
) (hydrated any, applied bool, err error) {

	concrete := target
	if concrete.Kind() == reflect.Pointer {
		concrete = concrete.Elem()
	}

	if elem.Kind() != reflect.Map || elem.Type().Key().Kind() != reflect.String || concrete.Kind() != reflect.Struct {
		return nil, false, nil
	}

	out := reflect.New(concrete).Elem()

	for i := range concrete.NumField() {

		field := concrete.Field(i)
		if !field.IsExported() {
			continue
		}

		fieldValue, ok := structFieldFromMap(elem, field)
		if !ok {
			continue
		}

		converted, err := Convert(runtimeEnvironment, fieldValue.Interface(), field.Type)
		if err != nil {
			return nil, true, fmt.Errorf("field %s: %w", field.Name, err)
		}

		convertedValue := reflect.ValueOf(converted)
		if !convertedValue.IsValid() {
			continue
		}
		out.Field(i).Set(convertedValue)
	}

	if target.Kind() == reflect.Pointer {
		pointer := reflect.New(concrete)
		pointer.Elem().Set(out)
		return pointer.Interface(), true, nil
	}
	return out.Interface(), true, nil
}

// structFieldFromMap finds a struct field's value in a decoded object map, trying the field's `json` tag, `yaml` tag,
// then its Go name — whichever key the codec wrote.
//
// Parameters:
//   - `elem`: the source map (string-keyed).
//   - `field`: the struct field being filled.
//
// Returns:
//   - `reflect.Value`: the matching map value; the zero Value when no candidate key is present.
//   - `bool`: true when a candidate key matched.
func structFieldFromMap(elem reflect.Value, field reflect.StructField) (reflect.Value, bool) {

	for _, key := range fieldKeys(field) {
		if value := elem.MapIndex(reflect.ValueOf(key)); value.IsValid() {
			return value, true
		}
	}
	return reflect.Value{}, false
}

// fieldKeys returns the candidate object keys for a struct field: its `json` and `yaml` tag names (the part before the
// first comma) and its Go field name.
//
// Parameters:
//   - `field`: the struct field.
//
// Returns:
//   - `[]string`: the candidate keys, tag names first.
func fieldKeys(field reflect.StructField) []string {

	keys := make([]string, 0, 3)
	for _, tag := range []string{"json", "yaml"} {
		if name, _, _ := strings.Cut(field.Tag.Get(tag), ","); name != "" && name != "-" {
			keys = append(keys, name)
		}
	}
	return append(keys, field.Name)
}

// typesAreInterconvertible reports whether a value of type `a` can fill a slot typed `b` or vice versa.
//
// Symmetrically tests both directions via any of the purely reflective paths of the [Convert] cascade.
//
// Used by [Subgraph.mergeBubbled] (the bubble-up parameter-consistency check), so the same-named-variable-across-
// differently-typed-slots case is not treated as a hard collision when a registered conversion bridges the two
// types. Slot-fill is *defined* as the conversion site; the plan-time check honors the same contract the dispatch-
// time cascade does.
//
// Paths probed (mirroring [Convert] steps 1, 2, 5, 6):
//
//  1. Identity — `a == b`.
//  2. Assignability — `a.AssignableTo(b)` or `b.AssignableTo(a)`.
//  3. Source-side opt-in — a zero-value of `a` implements [SourceConverter] and `CanConvertTo(b)` returns true,
//     or symmetrically for `b → a`.
//  4. Target-side opt-in — a fresh probe of `b` implements [TargetConverter] and `CanConvertFrom(a)` returns true,
//     or symmetrically for `b ← a`.
//
// Paths NOT probed: slice / map element-wise recursion (require a concrete value) and registered-Resource construction
// (requires a [RuntimeEnvironment] handle). Providers whose Resource types want plan-time type-compatibility honored
// for non-Resource sources opt in by implementing [TargetConverter] on the Resource type — the framework then wires
// both the plan-time consistency check (via this function) and the dispatch-time slot-fill (via [Convert] step 6)
// uniformly.
//
// Both [SourceConverter.CanConvertTo] and [TargetConverter.CanConvertFrom] are part of the cheap-probe contract:
// callers MUST be safe on a zero-value receiver (no field dereference), because this function calls them against nil
// pointers and zero structs to determine the existence of a conversion path without producing a value.
//
// Parameters:
//   - `a`: one of the two types to test.
//   - `b`: the other type.
//
// Returns:
//   - `bool`: true if at least one of the probed paths reports interconvertibility in either direction.
func typesAreInterconvertible(a, b reflect.Type) bool {

	if a == nil || b == nil {
		return false
	}

	if a == b {
		return true
	}

	if a.AssignableTo(b) || b.AssignableTo(a) {
		return true
	}

	if sourceSideAdvertises(a, b) || sourceSideAdvertises(b, a) {
		return true
	}

	if targetSideAdvertises(b, a) || targetSideAdvertises(a, b) {
		return true
	}

	return false
}

// sourceSideAdvertises reports whether `source` opts into [SourceConverter] for `target`.
//
// Probes both the value form of `source` (when methods sit on a value receiver) and the pointer form (when methods sit
// on `*source`, the conventional Go choice). When the type implements [SourceConverter], [SourceConverter.CanConvertTo]
// is called on the probe to confirm `target` is an advertised destination.
//
// Pointer-type sources allocate a fresh zero-value via [reflect.New](source.Elem()) — never a nil pointer — so methods
// promoted through embedded structs (e.g., [op.ResourceBase] on a Resource type) can access the embedded field without
// dereferencing nil.
//
// Parameters:
//   - `source`: the candidate source type whose [SourceConverter] opt-in is being probed.
//   - `target`: the destination type the source must advertise convertibility to.
//
// Returns:
//   - `bool`: true when a probe of `source` (or its pointer form) implements [SourceConverter] and reports
//     `CanConvertTo(target)`; false otherwise.
func sourceSideAdvertises(source, target reflect.Type) bool {

	if source.Implements(sourceConverterType) {

		var probe any
		if source.Kind() == reflect.Pointer {
			probe = reflect.New(source.Elem()).Interface()
		} else {
			probe = reflect.Zero(source).Interface()
		}

		if c, ok := probe.(SourceConverter); ok {
			return c.CanConvertTo(target)
		}
	}

	if source.Kind() != reflect.Pointer {
		ptrSource := reflect.PointerTo(source)
		if ptrSource.Implements(sourceConverterType) {
			probe := reflect.New(source).Interface()
			if c, ok := probe.(SourceConverter); ok {
				return c.CanConvertTo(target)
			}
		}
	}

	return false
}

// targetSideAdvertises reports whether `target` opts into [TargetConverter] for `source`.
//
// Mirrors [Convert] step 6's probe construction: when `target` is `*T`, the probe is `*T`; when `target` is a
// non-pointer `T`, the probe is `*T` (TargetConverter methods conventionally sit on the pointer receiver).
// [TargetConverter.CanConvertFrom] is then called on the probe to confirm `source` is an absorbable type.
//
// Parameters:
//   - `source`: the candidate source type the target must advertise convertibility from.
//   - `target`: the destination type whose [TargetConverter] opt-in is being probed.
//
// Returns:
//   - `bool`: true when a fresh probe of `target` (or its pointer form) implements [TargetConverter] and
//     reports `CanConvertFrom(source)`; false otherwise.
func targetSideAdvertises(source, target reflect.Type) bool {

	var probeType reflect.Type
	if target.Kind() == reflect.Pointer {
		probeType = target
	} else {
		probeType = reflect.PointerTo(target)
	}

	if !probeType.Implements(targetConverterType) {
		return false
	}

	var probe any
	if target.Kind() == reflect.Pointer {
		probe = reflect.New(target.Elem()).Interface()
	} else {
		probe = reflect.New(target).Interface()
	}

	t, ok := probe.(TargetConverter)
	if !ok {
		return false
	}

	return t.CanConvertFrom(source)
}
