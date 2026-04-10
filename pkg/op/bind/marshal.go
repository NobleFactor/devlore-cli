// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package bind

import (
	"fmt"
	"reflect"
	"sort"
	"sync"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

// registry is the receiver type registry, set by [SetRegistry].
//
// Used by [marshalReflect] to look up receiver types for Go structs and wrap them as [Value] instead of field-only
// [StructValue].
var registry *op.ReceiverRegistry

// typeCache stores cached [typeInfo] keyed by [reflect.Type].
var typeCache sync.Map

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

// typeInfo caches struct introspection results for Starlark field mapping.
type typeInfo struct {
	typeName string // cached camelToSnake(Type().Name())
	fields   []fieldInfo
	attrList []string              // sorted Starlark attribute names
	byName   map[string]*fieldInfo // starlark name → field
}

// fieldInfo maps a single exported Go struct field to its Starlark name.
type fieldInfo struct {
	index    int
	starName string
	goType   reflect.Type
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
func camelToSnake(s string) string { return op.CamelToSnake(s) }

// SetRegistry sets the package-level registry used by [marshalReflect].
func SetRegistry(r *op.ReceiverRegistry) { registry = r }

// executionContextFromRegistry returns an ExecutionContext with the package-level registry set.
func executionContextFromRegistry() *op.ExecutionContext {
	return &op.ExecutionContext{Registry: registry}
}

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

	// Build sorted attr list from fields.

	info.attrList = make([]string, 0, len(info.fields))

	for _, f := range info.fields {
		info.attrList = append(info.attrList, f.starName)
	}

	sort.Strings(info.attrList)

	actual, _ := typeCache.LoadOrStore(t, info)
	return actual.(*typeInfo)
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
// Dispatched by kind: primitives, slices, maps, and structs are handled recursively.
//
// Parameters:
//   - rv: the [reflect.Value] to convert.
//
// Returns:
//   - starlark.Value: the converted Starlark value.
//   - error: non-nil if rv contains an unsupported type.
func marshalReflect(rv reflect.Value) (starlark.Value, error) {

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

		// Already a starlark.Value — pass through.

		if rv.CanInterface() {
			if sv, ok := rv.Interface().(starlark.Value); ok {
				return sv, nil
			}
		}
		if rv.CanAddr() {
			if sv, ok := rv.Addr().Interface().(starlark.Value); ok {
				return sv, nil
			}
		}

		// Custom marshaler — let the type handle it.

		if rv.CanInterface() {
			if m, ok := rv.Interface().(Marshaler); ok {
				return m.MarshalStarvalue()
			}
		}
		if rv.CanAddr() {
			if m, ok := rv.Addr().Interface().(Marshaler); ok {
				return m.MarshalStarvalue()
			}
		}

		// Wrap as Value. Ensure we have a pointer for method dispatch.

		var ptr reflect.Value

		if rv.CanAddr() {
			ptr = rv.Addr()
		} else {
			ptr = reflect.New(rv.Type())
			ptr.Elem().Set(rv)
		}

		receiverType := resolveReceiverType(ptr.Type())

		return NewValue(receiverType, op.NewValueBase(executionContextFromRegistry(), ptr)), nil

	default:
		return nil, fmt.Errorf("marshal: unsupported type %s", rv.Type())
	}
}

// resolveReceiverType returns a ReceiverType for the given Go pointer type.
//
// It checks the registry first (for announced types with named parameters). If not found, it derives one via reflection
// with positional parameter names and caches the result.
//
// Parameters:
//   - ptrType: the reflect.Type (must be a pointer to struct).
//
// Returns:
//   - op.ReceiverType: the receiver type descriptor.
func resolveReceiverType(ptrType reflect.Type) op.ReceiverType {

	// Check registry for announced types.

	if registry != nil {
		if rt, ok := registry.TypeByReflection(ptrType); ok {
			return rt
		}
	}

	// Check cache for previously derived types.

	if cached, ok := derivedTypeCache.Load(ptrType); ok {
		return cached.(op.ReceiverType)
	}

	// Derive via reflection — positional parameter names only.

	methodParams := deriveMethodParams(ptrType)
	rt, err := op.NewReceiverType(ptrType, methodParams)

	if err != nil {
		// Fall back to no methods if derivation fails.
		rt, _ = op.NewReceiverType(ptrType, nil)
	}

	derivedTypeCache.Store(ptrType, rt)
	return rt
}

// derivedTypeCache stores ReceiverTypes derived at runtime via reflection.
var derivedTypeCache sync.Map

// deriveMethodParams discovers exported methods on a pointer type and generates positional parameter names.
//
// Only methods with supported return signatures are included (same classification as [op.NewMethod]).
func deriveMethodParams(ptrType reflect.Type) map[string][]string {

	params := make(map[string][]string)

	for i := range ptrType.NumMethod() {
		m := ptrType.Method(i)
		if !m.IsExported() {
			continue
		}

		mt := m.Type
		numIn := mt.NumIn() - 1 // exclude receiver

		// Generate positional names: arg0, arg1, ...
		names := make([]string, numIn)
		for j := range numIn {
			names[j] = fmt.Sprintf("arg%d", j)
		}

		params[m.Name] = names
	}

	return params
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

	// Accept *starlark.Dict (checked first — it implements HasAttrs but needs key-based lookup) or HasAttrs
	// (StructValue, starlarkstruct.Struct). Primitive starlark types (String, Int, Float, Bool, List) implement
	// HasAttrs for built-in methods but are not struct-like. Exclude them to avoid silently creating zero-valued
	// structs when field lookups all fail.

	switch v := sv.(type) {
	case *starlark.Dict:
		return unmarshalDict(v, rv, info)

	case starlark.String, starlark.Int, starlark.Float, starlark.Bool, *starlark.List, starlark.Bytes:
		return fmt.Errorf("unmarshal: expected struct or dict for %s, got %s", rv.Type().Name(), sv.Type())

	case starlark.HasAttrs:
		return unmarshalHasAttrs(v, rv, info)

	default:
		return fmt.Errorf("unmarshal: expected struct or dict, got %s", sv.Type())
	}
}

// unmarshalHasAttrs populates a Go struct from a [starlark.HasAttrs] value.
//
// Fields are matched by name.
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

// unmarshalDict populates a Go struct from a [starlark.Dict].
//
// Fields are matched by name.
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
//
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
		// Pass through Go pointer handles. Known starlark types (List, Dict, StructValue) are handled above; remaining
		// pointers are framework handles (e.g., *Promise) that should flow through as-is.
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

	// Nil starlark value: set to zero.

	if sv == nil {
		rv.Set(reflect.Zero(rv.Type()))
		return nil
	}

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

	// Pass through Go pointer handles: if the starlark.Value is directly assignable to the target type, use it as-is.
	// This handles framework types like *Promise that implement starlark.Value.

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

	// Fast path: if the Starlark value is a StructValue wrapping a Go value whose type matches the target, extract the
	// Go value directly.

	if svs, ok := sv.(*StructValue); ok {
		goElem := svs.goValue.Elem()
		if goElem.Type() == rv.Type() {
			rv.Set(goElem)
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
