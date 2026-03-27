// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package bind

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"unicode"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

// region EXPORTED FUNCTIONS

// Marshal converts a Go value to a [starlark.Value].
//
// Structs become [StructValue] with lazy attr dispatch. Primitives, slices, maps, and pointers are handled recursively.
// Values already implementing starlark.Value pass through unchanged.
//
// Parameters:
//   - v: the Go value to convert. Nil returns [starlark.None].
//
// Returns:
//   - starlark.Value: the converted Starlark value.
//   - error: non-nil if v contains an unsupported type (e.g., channels, functions).
func Marshal(v any) (starlark.Value, error) {

	if v == nil {
		return starlark.None, nil
	}
	if sv, ok := v.(starlark.Value); ok {
		return sv, nil
	}
	return marshalReflect(reflect.ValueOf(v))
}

// endregion

// Marshaler is implemented by types that can marshal themselves into a Starlark value.
//
// Check for this interface before walking struct fields via reflection. This is the same pattern as [json.Marshaler].
type Marshaler interface {
	MarshalStarvalue() (starlark.Value, error)
}

// Unmarshaler is implemented by types that can unmarshal a Starlark value into themselves.
//
// Check for this interface before assigning fields via reflection. This is the same pattern as [json.Unmarshaler].
type Unmarshaler interface {
	UnmarshalStarvalue(starlark.Value) error
}

// typeInfo caches struct introspection results for Starlark field and method mapping.
type typeInfo struct {
	typeName string // cached camelToSnake(Type().Name())
	fields   []fieldInfo
	methods  []methodInfo
	attrList []string               // sorted Starlark attribute names (fields + methods)
	byName   map[string]*fieldInfo  // starlark name → field
	byMethod map[string]*methodInfo // starlark name → method
}

// fieldInfo maps a single exported Go struct field to its Starlark name.
type fieldInfo struct {
	index    int
	starName string
	goType   reflect.Type
}

// methodInfo maps an exported Go method to its Starlark name.
type methodInfo struct {
	name       string       // Go method name (CamelCase)
	starName   string       // snake_case Starlark name
	hasError   bool         // true for func() (T, error)
	paramNames []string     // Starlark param names (nil for zero-arg methods)
	numIn      int          // number of parameters (excluding receiver)
	methodType reflect.Type // method signature (set for parameterized methods)
}

// endregion

// region UNEXPORTED FUNCTIONS

// camelToSnake converts a CamelCase identifier to snake_case.
//
// Consecutive uppercase letters are treated as an acronym (e.g., "XMLParser" → "xml_parser").
//
// Parameters:
//   - s: the CamelCase identifier to convert.
//
// Returns:
//   - string: the snake_case equivalent.
func camelToSnake(s string) string {

	runes := []rune(s)
	var b strings.Builder
	b.Grow(len(s) + 4)

	for i, r := range runes {
		if unicode.IsUpper(r) {
			if i > 0 {
				prev := runes[i-1]
				if unicode.IsLower(prev) || unicode.IsDigit(prev) {
					b.WriteRune('_')
				} else if unicode.IsUpper(prev) && i+1 < len(runes) && unicode.IsLower(runes[i+1]) {
					b.WriteRune('_')
				}
			}
			b.WriteRune(unicode.ToLower(r))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// constructResource creates a Resource using the constructor registry.
//
// Returns nil, false if no constructor exists for the given type.
//
// Parameters:
//   - targetType: the [reflect.Type] to construct.
//   - value: the input value to pass to the constructor.
//
// Returns:
//   - Resource: the constructed resource, or nil.
//   - bool: true if construction succeeded.
func constructResource(targetType reflect.Type, value any) (op.Resource, bool) {

	ctor, ok := op.LookupConstructor(targetType)
	if !ok {
		return nil, false
	}

	result, err := ctor(value)
	if err != nil {
		return nil, false
	}

	// The result is typically a value type. Create a temporary pointer to satisfy the Resource interface

	rv := reflect.ValueOf(result)

	if rv.Kind() == reflect.Struct && reflect.PointerTo(rv.Type()).Implements(resourceType) {
		ptr := reflect.New(rv.Type())
		ptr.Elem().Set(rv)
		return ptr.Interface().(op.Resource), true
	}
	if r, ok := result.(op.Resource); ok {
		return r, true
	}

	return nil, false
}

// extractCallable looks up the callable constructor from the registry and extracts the given Starlark function.
//
// The mem package registers a constructor via AnnounceResource that handles extraction and compilation.
//
// Parameters:
//   - fn: the Starlark function to extract.
//   - funcType: the Go func type name for signature validation.
//
// Returns:
//   - CallableResource: the extracted callable resource.
//   - error: non-nil if no extractor is registered or extraction fails.
func extractCallable(fn *starlark.Function, funcType string) (CallableResource, error) {

	ctor, ok := op.LookupConstructor(callableResourceType)
	if !ok {
		return nil, fmt.Errorf("no callable extractor registered (mem package not imported?)")
	}
	result, err := ctor(CallableInput{Fn: fn, FuncType: funcType})
	if err != nil {
		return nil, err
	}
	return result.(CallableResource), nil
}

// stringerType is the [reflect.Type] for the [fmt.Stringer] interface.
var stringerType = reflect.TypeOf((*fmt.Stringer)(nil)).Elem()

// getTypeInfo returns cached struct metadata for the given type.
//
// If t is a pointer type, the element type is used.
//
// Parameters:
//   - t: the [reflect.Type] to introspect (pointer or struct).
//
// Returns:
//   - *typeInfo: the cached field and method metadata.
func getTypeInfo(t reflect.Type) *typeInfo {

	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if cached, ok := typeCache.Load(t); ok {
		return cached.(*typeInfo)
	}

	info := &typeInfo{
		typeName: camelToSnake(t.Name()),
		byName:   make(map[string]*fieldInfo),
		byMethod: make(map[string]*methodInfo),
	}

	// Discover exported fields.
	for i := range t.NumField() {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}

		tag := f.Tag.Get("starlark")
		if tag == "-" {
			continue
		}

		name := tag
		if name == "" {
			name = camelToSnake(f.Name)
		}

		fi := fieldInfo{
			index:    i,
			starName: name,
			goType:   f.Type,
		}
		info.fields = append(info.fields, fi)
		info.byName[name] = &info.fields[len(info.fields)-1]
	}

	// Discover eligible methods.
	discoverMethods(t, info)

	// Build sorted attr list from fields + methods.
	info.attrList = make([]string, 0, len(info.fields)+len(info.methods))
	for _, f := range info.fields {
		info.attrList = append(info.attrList, f.starName)
	}
	for _, m := range info.methods {
		info.attrList = append(info.attrList, m.starName)
	}
	sort.Strings(info.attrList)

	actual, _ := typeCache.LoadOrStore(t, info)
	return actual.(*typeInfo)
}

// classifyMethodReturnOk checks whether a method's return signature is eligible for Starlark exposure.
//
// Parameters:
//   - mt: the method's [reflect.Type].
//
// Returns:
//   - hasError: true when the return pattern is (T, error).
//   - ok: true when the return pattern is eligible.
func classifyMethodReturnOk(mt reflect.Type) (hasError, ok bool) {

	switch mt.NumOut() {
	case 1:
		if mt.Out(0).Implements(errorType) {
			return false, false
		}
		return false, true
	case 2:
		if !mt.Out(1).Implements(errorType) || mt.Out(0).Implements(errorType) {
			return false, false
		}
		return true, true
	default:
		return false, false
	}
}

// discoverMethods populates info.methods and info.byMethod with eligible methods from the pointer type of t.
//
// Zero-arg methods are accepted unconditionally (returns (T) or (T, error)). Methods with parameters are accepted only
// when their type is registered in [typeParamsRegistry] and the method name appears in the registered [MethodParams].
// String() is excluded when the type implements [fmt.Stringer] (reserved for value representation).
//
// Parameters:
//   - t: the struct [reflect.Type] (not a pointer).
//   - info: the typeInfo to populate with discovered methods.
func discoverMethods(t reflect.Type, info *typeInfo) {

	// Ensure resource type params are registered before lookup. ResourceFactory.Init() is lazy; this triggers it so
	// RegisterTypeParams runs before we cache the type's method metadata.

	op.LookupConstructor(t)

	typeParams, _ := lookupTypeParams(t)
	pt := reflect.PointerTo(t)

	for i := range pt.NumMethod() {

		m := pt.Method(i)
		if !m.IsExported() {
			continue
		}

		mt := m.Type
		numIn := mt.NumIn() - 1 // exclude receiver

		// Exclude String() matching fmt.Stringer — reserved for value representation.

		if m.Name == "String" && pt.Implements(stringerType) {
			continue
		}

		hasError, ok := classifyMethodReturnOk(mt)
		if !ok {
			continue
		}

		// Parameterized methods: accept only when registered with matching param count.

		if numIn > 0 {
			paramNames, found := typeParams[m.Name]
			if !found || len(paramNames) != numIn {
				continue
			}
			mi := methodInfo{
				name:       m.Name,
				starName:   camelToSnake(m.Name),
				hasError:   hasError,
				paramNames: paramNames,
				numIn:      numIn,
				methodType: mt,
			}
			info.methods = append(info.methods, mi)
			info.byMethod[mi.starName] = &info.methods[len(info.methods)-1]
			continue
		}

		// Zero-arg methods.

		mi := methodInfo{
			name:     m.Name,
			starName: camelToSnake(m.Name),
			hasError: hasError,
		}
		info.methods = append(info.methods, mi)
		info.byMethod[mi.starName] = &info.methods[len(info.methods)-1]
	}
}

// initCallableSlots finds CallableResource values in slots that target func-typed method parameters, initializes them,
// and replaces the slot value with an adapted Go function.
//
// This runs in Do() before coerceArgs so the standard coercion path sees a directly-assignable func value.
//
// Parameters:
//   - ctx: the execution context (provides the Starlark thread).
//   - slots: the slot map to scan and mutate in place.
//   - methodType: the Go method's [reflect.Type] for parameter introspection.
//   - paramNames: the Starlark parameter names matching the method signature.
//
// Returns:
//   - error: non-nil if callable initialization or adaptation fails.
func initCallableSlots(ctx *op.Context, slots map[string]any, methodType reflect.Type, paramNames []string) error {

	for i, name := range paramNames {
		callable, ok := slots[name].(CallableResource)
		if !ok {
			continue
		}
		paramIdx := i + 1 // skip receiver
		if paramIdx >= methodType.NumIn() {
			continue
		}
		paramType := methodType.In(paramIdx)
		if paramType.Kind() != reflect.Func {
			continue
		}

		thread := ThreadFrom(*ctx)
		if err := callable.Init(thread); err != nil {
			return fmt.Errorf("param %s: init callable: %w", name, err)
		}

		adapted, err := buildCallableFunc(callable.Fn(), thread, paramType)
		if err != nil {
			return fmt.Errorf("param %s: adapt callable: %w", name, err)
		}
		slots[name] = adapted
	}

	return nil
}

// isCallableResource returns true if the value implements CallableResource.
//
// Parameters:
//   - v: the value to check.
//
// Returns:
//   - bool: true if v implements CallableResource.
func isCallableResource(v any) bool {

	_, ok := v.(CallableResource)
	return ok
}

// isFuncType returns true if the [reflect.Type] is a function type.
//
// Parameters:
//   - t: the [reflect.Type] to check.
//
// Returns:
//   - bool: true if t.Kind() is reflect.Func.
func isFuncType(t reflect.Type) bool {

	return t.Kind() == reflect.Func
}

// lookupTypeParams returns the method params for the given struct type.
//
// Parameters:
//   - t: the [reflect.Type] to look up (pointer or struct).
//
// Returns:
//   - MethodParams: the stored params.
//   - bool: true if found.
func lookupTypeParams(t reflect.Type) (MethodParams, bool) {

	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	v, ok := typeParamsRegistry.Load(t)
	if !ok {
		return nil, false
	}
	return v.(MethodParams), true
}

// marshalMap converts a [reflect.Value] map to a starlark.Dict.
//
// Parameters:
//   - rv: the [reflect.Value] of kind Map to convert.
//
// Returns:
//   - starlark.Value: the converted Starlark dict.
//   - error: non-nil if any key or value cannot be marshaled.
func marshalMap(rv reflect.Value) (starlark.Value, error) {

	if rv.IsNil() {
		return starlark.NewDict(0), nil
	}
	dict := starlark.NewDict(rv.Len())
	iter := rv.MapRange()
	for iter.Next() {
		key, err := marshalReflect(iter.Key())
		if err != nil {
			return nil, fmt.Errorf("marshal: map key: %w", err)
		}
		val, err := marshalReflect(iter.Value())
		if err != nil {
			return nil, fmt.Errorf("marshal: map value for %v: %w", iter.Key().Interface(), err)
		}
		if err := dict.SetKey(key, val); err != nil {
			return nil, fmt.Errorf("marshal: dict set: %w", err)
		}
	}
	return dict, nil
}

// marshalReflect converts a [reflect.Value] to a starlark.Value.
//
// Pointer-to-struct types registered in the receiver params registry are wrapped as ExecutingReceivers; all others are
// dispatched by kind.
//
// Parameters:
//   - rv: the [reflect.Value] to convert.
//
// Returns:
//   - starlark.Value: the converted Starlark value.
//   - error: non-nil if rv contains an unsupported type.
func marshalReflect(rv reflect.Value) (starlark.Value, error) {

	// Check receiver params registry for pointer-to-struct types.
	//
	// Registered types get wrapped as ExecutingReceivers (with methods) instead of flattened to field-only structs.

	if rv.Kind() == reflect.Pointer && !rv.IsNil() {
		if entry, ok := lookupReceiverParams(rv.Type()); ok {
			return WrapProviderInExecutingReceiver(entry.factory, rv.Interface()), nil
		}
	}

	for rv.Kind() == reflect.Pointer || rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return starlark.None, nil
		}
		rv = rv.Elem()
	}

	// Check starvalue.Marshaler for non-struct types.
	//
	// Struct types are handled inside the struct case to avoid short-circuiting when Marshaler is promoted from an
	// embedded field (e.g., file.Resource embeds ResourceBase which implements Marshaler — we want the outer struct's
	// exported fields to be marshaled normally, with only the embedded ResourceBase using its Marshaler).

	if rv.Kind() != reflect.Struct && rv.CanInterface() {
		if m, ok := rv.Interface().(Marshaler); ok {
			return m.MarshalStarvalue()
		}
	}

	switch rv.Kind() {
	case reflect.String:
		return starlark.String(rv.String()), nil

	case reflect.Bool:
		return starlark.Bool(rv.Bool()), nil

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return starlark.MakeInt64(rv.Int()), nil

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return starlark.MakeUint64(rv.Uint()), nil

	case reflect.Float32, reflect.Float64:
		return starlark.Float(rv.Float()), nil

	case reflect.Slice:
		if rv.Type().Elem().Kind() == reflect.Uint8 {
			return starlark.Bytes(rv.Bytes()), nil
		}
		return marshalSlice(rv)

	case reflect.Map:
		return marshalMap(rv)

	case reflect.Struct:

		// If the struct has no exported fields, try custom serialization before falling through to marshalStruct.
		//
		// Check starlark.Value first (most fundamental), then starvalue.Marshaler. Structs WITH exported fields go
		// through marshalStruct, where each embedded field gets its own Marshaler check via recursive calls.

		info := getTypeInfo(rv.Type())

		if len(info.fields) == 0 && rv.CanInterface() {
			if sv, ok := rv.Interface().(starlark.Value); ok {
				return sv, nil
			}
			if rv.CanAddr() {
				if sv, ok := rv.Addr().Interface().(starlark.Value); ok {
					return sv, nil
				}
			}
			if m, ok := rv.Interface().(Marshaler); ok {
				return m.MarshalStarvalue()
			}
			if rv.CanAddr() {
				if m, ok := rv.Addr().Interface().(Marshaler); ok {
					return m.MarshalStarvalue()
				}
			}
		}

		// Check receiver params registry for value-type structs.
		//
		// Methods that take arguments are registered here; StructValue only exposes zero-arg methods. Create a pointer
		// so WrapProviderInExecutingReceiver can work with it.

		if entry, ok := lookupReceiverParams(rv.Type()); ok {
			ptr := reflect.New(rv.Type())
			ptr.Elem().Set(rv)
			return WrapProviderInExecutingReceiver(entry.factory, ptr.Interface()), nil
		}

		return marshalStruct(rv)

	default:
		return nil, fmt.Errorf("marshal: unsupported type %s", rv.Type())
	}
}

// marshalSlice converts a [reflect.Value] slice to a [starlark.List].
//
// Parameters:
//   - rv: the [reflect.Value] of kind Slice to convert.
//
// Returns:
//   - starlark.Value: the converted Starlark list.
//   - error: non-nil if any element cannot be marshaled.
func marshalSlice(rv reflect.Value) (starlark.Value, error) {

	if rv.IsNil() {
		return starlark.NewList(nil), nil
	}
	elems := make([]starlark.Value, rv.Len())
	for i := range rv.Len() {
		val, err := marshalReflect(rv.Index(i))
		if err != nil {
			return nil, fmt.Errorf("marshal: slice index %d: %w", i, err)
		}
		elems[i] = val
	}
	return starlark.NewList(elems), nil
}

// marshalStruct converts a [reflect.Value] struct to a [StructValue] with lazy attr dispatch.
//
// Fields and methods are resolved on access, not at construction.
//
// Parameters:
//   - rv: the [reflect.Value] of kind Struct to convert.
//
// Returns:
//   - starlark.Value: the [StructValue] wrapping the Go struct.
//   - error: always nil (construction cannot fail).
func marshalStruct(rv reflect.Value) (starlark.Value, error) {

	info := getTypeInfo(rv.Type())

	// Ensure we have a pointer so methods (including pointer-receiver) can be called.
	//
	// If the value is not addressable, create a copy.
	var ptr reflect.Value
	if rv.CanAddr() {
		ptr = rv.Addr()
	} else {
		ptr = reflect.New(rv.Type())
		ptr.Elem().Set(rv)
	}

	return &StructValue{
		typeName: info.typeName,
		goValue:  ptr,
		info:     info,
	}, nil
}

// unmarshal populates a Go value from a starlark.Value.
//
// The target must be a non-nil pointer. For *any targets, native Go types (string, int, bool, float64, nil, []any,
// map[string]any) are returned. For struct targets, starlarkstruct.Struct fields are matched by name.
//
// Parameters:
//   - sv: the Starlark value to convert.
//   - target: a non-nil pointer to the Go value to populate.
//
// Returns:
//   - error: non-nil if the conversion fails or target is not a pointer.
func unmarshal(sv starlark.Value, target any) error {

	rv := reflect.ValueOf(target)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return fmt.Errorf("unmarshal: target must be a non-nil pointer, got %T", target)
	}
	return unmarshalValue(sv, rv.Elem())
}

// unmarshalDictToAny converts a starlark.Dict to a map[string]any.
//
// Parameters:
//   - dict: the Starlark dict to convert.
//
// Returns:
//   - map[string]any: the native Go map.
//   - error: non-nil if any key is not a string or any value cannot be converted.
func unmarshalDictToAny(dict *starlark.Dict) (map[string]any, error) {

	result := make(map[string]any, dict.Len())
	for _, item := range dict.Items() {
		key, ok := starlark.AsString(item[0])
		if !ok {
			return nil, fmt.Errorf("dict key must be string, got %s", item[0].Type())
		}
		val, err := unmarshalToAny(item[1])
		if err != nil {
			return nil, fmt.Errorf("dict key %q: %w", key, err)
		}
		result[key] = val
	}
	return result, nil
}

// unmarshalListToAny converts a starlark.List to a native Go slice.
//
// Returns a typed Go slice ([]string, []int, []bool, []float64, [][]byte) for
// homogeneous scalar lists, []any otherwise.
//
// Parameters:
//   - list: the Starlark list to convert.
//
// Returns:
//   - any: the native Go slice ([]string, []int, []bool, []float64, [][]byte, or []any).
//   - error: non-nil if any element cannot be converted.
func unmarshalListToAny(list *starlark.List) (any, error) {

	n := list.Len()
	if n == 0 {
		return []any{}, nil
	}

	// Check whether all elements share the same Starlark type.

	firstType := reflect.TypeOf(list.Index(0))
	homogeneous := true

	for i := 1; i < n; i++ {
		if reflect.TypeOf(list.Index(i)) != firstType {
			homogeneous = false
			break
		}
	}

	// Build a typed Go slice for homogeneous scalar lists.

	if homogeneous {

		switch list.Index(0).(type) {

		case starlark.String:
			result := make([]string, n)
			for i := range n {
				result[i] = string(list.Index(i).(starlark.String))
			}
			return result, nil

		case starlark.Int:
			result := make([]int, n)
			for i := range n {
				v, ok := list.Index(i).(starlark.Int).Int64()
				if !ok {
					return nil, fmt.Errorf("list index %d: int value out of range", i)
				}
				result[i] = int(v)
			}
			return result, nil

		case starlark.Bool:
			result := make([]bool, n)
			for i := range n {
				result[i] = bool(list.Index(i).(starlark.Bool))
			}
			return result, nil

		case starlark.Float:
			result := make([]float64, n)
			for i := range n {
				result[i] = float64(list.Index(i).(starlark.Float))
			}
			return result, nil

		case starlark.Bytes:
			result := make([][]byte, n)
			for i := range n {
				result[i] = []byte(list.Index(i).(starlark.Bytes))
			}
			return result, nil
		}
	}

	result := make([]any, n)

	for i := range n {
		val, err := unmarshalToAny(list.Index(i))
		if err != nil {
			return nil, fmt.Errorf("list index %d: %w", i, err)
		}
		result[i] = val
	}

	return result, nil
}

// unmarshalMap converts a starlark.Dict into a typed Go map via reflection.
//
// Parameters:
//   - dict: the Starlark dict to convert.
//   - rv: the [reflect.Value] of kind Map to populate.
//
// Returns:
//   - error: non-nil if any key or value cannot be unmarshaled.
func unmarshalMap(dict *starlark.Dict, rv reflect.Value) error {

	m := reflect.MakeMapWithSize(rv.Type(), dict.Len())
	keyType := rv.Type().Key()
	valType := rv.Type().Elem()

	for _, item := range dict.Items() {
		key := reflect.New(keyType).Elem()
		if err := unmarshalValue(item[0], key); err != nil {
			return fmt.Errorf("dict key: %w", err)
		}
		val := reflect.New(valType).Elem()
		if err := unmarshalValue(item[1], val); err != nil {
			return fmt.Errorf("dict value: %w", err)
		}
		m.SetMapIndex(key, val)
	}

	rv.Set(m)
	return nil
}

// unmarshalSlice converts a starlark.List into a typed Go slice via reflection.
//
// Parameters:
//   - list: the Starlark list to convert.
//   - rv: the [reflect.Value] of kind Slice to populate.
//
// Returns:
//   - error: non-nil if any element cannot be unmarshaled.
func unmarshalSlice(list *starlark.List, rv reflect.Value) error {

	n := list.Len()
	slice := reflect.MakeSlice(rv.Type(), n, n)
	for i := range n {
		if err := unmarshalValue(list.Index(i), slice.Index(i)); err != nil {
			return fmt.Errorf("list index %d: %w", i, err)
		}
	}
	rv.Set(slice)
	return nil
}

// unmarshalStruct converts a [StructValue], starlarkstruct.Struct, or starlark.Dict
// into a typed Go struct via reflection. Fields are matched by Starlark name.
//
// Parameters:
//   - sv: the Starlark value (must be *StructValue, *starlarkstruct.Struct, or *starlark.Dict).
//   - rv: the [reflect.Value] of kind Struct to populate.
//
// Returns:
//   - error: non-nil if sv is an unsupported type, or if any field fails.
func unmarshalStruct(sv starlark.Value, rv reflect.Value) error {

	info := getTypeInfo(rv.Type())

	// Accept *starlark.Dict (checked first — it implements HasAttrs but
	// needs key-based lookup) or HasAttrs (StructValue, starlarkstruct.Struct).
	switch v := sv.(type) {
	case *starlark.Dict:
		return unmarshalDict(v, rv, info)

	case starlark.HasAttrs:
		return unmarshalHasAttrs(v, rv, info)

	default:
		return fmt.Errorf("unmarshal: expected struct or dict, got %s", sv.Type())
	}
}

// unmarshalHasAttrs populates a Go struct from a [starlark.HasAttrs] value
// (e.g., [StructValue] or starlarkstruct.Struct). Fields are matched by name.
//
// Parameters:
//   - v: the Starlark value with named attributes.
//   - rv: the [reflect.Value] of kind Struct to populate.
//   - info: the cached type metadata for the target struct.
//
// Returns:
//   - error: non-nil if any field fails to unmarshal.
func unmarshalHasAttrs(v starlark.HasAttrs, rv reflect.Value, info *typeInfo) error {

	for i := range info.fields {
		fi := &info.fields[i]
		attr, err := v.Attr(fi.starName)
		if err != nil {
			continue // Field not present; leave zero.
		}
		if err := unmarshalValue(attr, rv.Field(fi.index)); err != nil {
			return fmt.Errorf("field %s: %w", fi.starName, err)
		}
	}
	return nil
}

// unmarshalDict populates a Go struct from a [starlark.Dict]. Fields are matched by name.
//
// Parameters:
//   - dict: the Starlark dict to read from.
//   - rv: the [reflect.Value] of kind Struct to populate.
//   - info: the cached type metadata for the target struct.
//
// Returns:
//   - error: non-nil if any field fails to unmarshal.
func unmarshalDict(dict *starlark.Dict, rv reflect.Value, info *typeInfo) error {

	for i := range info.fields {
		fi := &info.fields[i]
		val, found, err := dict.Get(starlark.String(fi.starName))
		if err != nil {
			return fmt.Errorf("field %s: %w", fi.starName, err)
		}
		if !found {
			continue
		}
		if err := unmarshalValue(val, rv.Field(fi.index)); err != nil {
			return fmt.Errorf("field %s: %w", fi.starName, err)
		}
	}
	return nil
}

// unmarshalAttrsToAny converts any [starlark.HasAttrs] value to a map[string]any.
// Works for both [StructValue] and starlarkstruct.Struct.
//
// Parameters:
//   - s: the Starlark value with named attributes.
//
// Returns:
//   - map[string]any: the native Go map keyed by attribute names.
//   - error: non-nil if any attribute cannot be converted.
func unmarshalAttrsToAny(s starlark.HasAttrs) (map[string]any, error) {

	names := s.AttrNames()
	result := make(map[string]any, len(names))
	for _, name := range names {
		v, err := s.Attr(name)
		if err != nil {
			return nil, fmt.Errorf("struct attr %q: %w", name, err)
		}
		val, err := unmarshalToAny(v)
		if err != nil {
			return nil, fmt.Errorf("struct attr %q: %w", name, err)
		}
		result[name] = val
	}
	return result, nil
}

// unmarshalToAny converts a [starlark.Value] to a native Go value without a specific target type.
//
// Returns string, int, bool, float64, nil, []any (or []string for homogeneous string lists),
// or map[string]any.
//
// Parameters:
//   - sv: the Starlark value to convert.
//
// Returns:
//   - any: the native Go value.
//   - error: non-nil if sv contains an unsupported Starlark type.
func unmarshalToAny(sv starlark.Value) (any, error) {

	switch v := sv.(type) {
	case starlark.NoneType:
		return nil, nil
	case starlark.String:
		return string(v), nil
	case starlark.Int:
		i, ok := v.Int64()
		if !ok {
			return nil, fmt.Errorf("unmarshal: int value out of range")
		}
		return int(i), nil
	case starlark.Bool:
		return bool(v), nil
	case starlark.Float:
		return float64(v), nil
	case starlark.Bytes:
		return []byte(v), nil
	case *starlark.List:
		return unmarshalListToAny(v)
	case *starlark.Dict:
		return unmarshalDictToAny(v)
	case *StructValue:
		return unmarshalAttrsToAny(v)
	case *starlarkstruct.Struct:
		return unmarshalAttrsToAny(v)
	default:
		// Pass through Go pointer handles. Known starlark types (List, Dict,
		// StructValue) are handled above; remaining pointers are framework
		// handles (e.g., *Promise) that should flow through as-is.
		if reflect.TypeOf(sv).Kind() == reflect.Pointer {
			return sv, nil
		}
		return nil, fmt.Errorf("unmarshal: unsupported starlark type %s", sv.Type())
	}
}

// unmarshalValue recursively converts a Starlark value into a [reflect.Value] target.
//
// Parameters:
//   - sv: the Starlark value to convert.
//   - rv: the [reflect.Value] to populate (must be settable).
//
// Returns:
//   - error: non-nil if the conversion fails.
func unmarshalValue(sv starlark.Value, rv reflect.Value) error {

	// Handle *any target: generic conversion.
	if rv.Kind() == reflect.Interface {
		val, err := unmarshalToAny(sv)
		if err != nil {
			return err
		}
		if val == nil {
			rv.Set(reflect.Zero(rv.Type()))
		} else {
			rv.Set(reflect.ValueOf(val))
		}
		return nil
	}

	// Handle None → zero value for pointer types, error for non-pointers.
	if _, ok := sv.(starlark.NoneType); ok {
		if rv.Kind() == reflect.Pointer {
			rv.Set(reflect.Zero(rv.Type()))
			return nil
		}
		rv.Set(reflect.Zero(rv.Type()))
		return nil
	}

	// Pass through Go pointer handles: if the starlark.Value is directly
	// assignable to the target type, use it as-is. This handles framework
	// types like *Promise that implement starlark.Value.
	if reflect.TypeOf(sv).AssignableTo(rv.Type()) {
		rv.Set(reflect.ValueOf(sv))
		return nil
	}

	// Dereference/allocate pointers.
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			rv.Set(reflect.New(rv.Type().Elem()))
		}
		rv = rv.Elem()
	}

	// Fast path: if the Starlark value is a StructValue wrapping a Go value
	// whose type matches the target, extract the Go value directly.
	if svs, ok := sv.(*StructValue); ok {
		goElem := svs.goValue.Elem()
		if goElem.Type() == rv.Type() {
			rv.Set(goElem)
			return nil
		}
	}

	// Constructor registry: build complex Go types from simpler Starlark values.
	// If the Starlark value is already a struct whose type name matches the
	// target Go type, skip the constructor and unmarshal fields directly.
	if ctor, ok := op.LookupConstructor(rv.Type()); ok {
		alreadyTarget := false
		if ss, ok := sv.(*starlarkstruct.Struct); ok {
			if name, ok := ss.Constructor().(starlark.String); ok {
				alreadyTarget = string(name) == camelToSnake(rv.Type().Name())
			}
		}
		if svs, ok := sv.(*StructValue); ok {
			alreadyTarget = svs.typeName == camelToSnake(rv.Type().Name())
		}
		if !alreadyTarget {
			native, err := unmarshalToAny(sv)
			if err != nil {
				return err
			}
			val, err := ctor(native)
			if err != nil {
				return err
			}
			rv.Set(reflect.ValueOf(val))
			return nil
		}
	}

	switch rv.Kind() {
	case reflect.String:
		s, ok := starlark.AsString(sv)
		if !ok {
			return fmt.Errorf("unmarshal: expected string, got %s", sv.Type())
		}
		rv.SetString(s)
		return nil

	case reflect.Bool:
		b, ok := sv.(starlark.Bool)
		if !ok {
			return fmt.Errorf("unmarshal: expected bool, got %s", sv.Type())
		}
		rv.SetBool(bool(b))
		return nil

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		si, ok := sv.(starlark.Int)
		if !ok {
			return fmt.Errorf("unmarshal: expected int, got %s", sv.Type())
		}
		i, ok := si.Int64()
		if !ok {
			return fmt.Errorf("unmarshal: int value out of range")
		}
		rv.SetInt(i)
		return nil

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		si, ok := sv.(starlark.Int)
		if !ok {
			return fmt.Errorf("unmarshal: expected int, got %s", sv.Type())
		}
		u, ok := si.Uint64()
		if !ok {
			return fmt.Errorf("unmarshal: uint value out of range")
		}
		rv.SetUint(u)
		return nil

	case reflect.Float32, reflect.Float64:
		switch v := sv.(type) {
		case starlark.Float:
			rv.SetFloat(float64(v))
		case starlark.Int:
			i, ok := v.Int64()
			if !ok {
				return fmt.Errorf("unmarshal: int value out of range for float")
			}
			rv.SetFloat(float64(i))
		default:
			return fmt.Errorf("unmarshal: expected float or int, got %s", sv.Type())
		}
		return nil

	case reflect.Slice:
		if rv.Type().Elem().Kind() == reflect.Uint8 {
			b, ok := sv.(starlark.Bytes)
			if !ok {
				return fmt.Errorf("unmarshal: expected bytes, got %s", sv.Type())
			}
			rv.SetBytes([]byte(b))
			return nil
		}
		list, ok := sv.(*starlark.List)
		if !ok {
			return fmt.Errorf("unmarshal: expected list, got %s", sv.Type())
		}
		return unmarshalSlice(list, rv)

	case reflect.Map:
		dict, ok := sv.(*starlark.Dict)
		if !ok {
			return fmt.Errorf("unmarshal: expected dict, got %s", sv.Type())
		}
		return unmarshalMap(dict, rv)

	case reflect.Struct:
		return unmarshalStruct(sv, rv)

	default:
		return fmt.Errorf("unmarshal: unsupported target type %s", rv.Type())
	}
}

// endregion
