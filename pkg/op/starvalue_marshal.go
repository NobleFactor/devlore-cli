// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"unicode"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/NobleFactor/devlore-cli/pkg/op/starvalue"
)

// --- Constructor registry ---

// constructorRegistry maps reflect.Type → func(any) (any, error).
// Types like Blob register a constructor so the reflection bridge can
// construct them from simpler Starlark representations (e.g. string → Blob).
var constructorRegistry sync.Map

// RegisterConstructor registers a function that constructs a Go value
// from a simpler representation (e.g., string → Blob via NewBlob).
func RegisterConstructor[T any](fn func(any) (T, error)) {
	t := reflect.TypeOf((*T)(nil)).Elem()
	constructorRegistry.Store(t, func(v any) (any, error) {
		return fn(v)
	})
}

// planTimeConstructorRegistry maps reflect.Type → func(any) (any, error).
// Plan-time constructors create URI-only resources with no I/O.
// Used by buildPlannedBridge for catalog resolution at plan time.
var planTimeConstructorRegistry sync.Map

// RegisterPlanTimeConstructor registers a function that constructs a
// URI-only resource from a simpler representation (e.g., string → Resource).
// Unlike RegisterConstructor, the plan-time constructor must not perform I/O.
func RegisterPlanTimeConstructor[T any](fn func(any) (T, error)) {
	t := reflect.TypeOf((*T)(nil)).Elem()
	planTimeConstructorRegistry.Store(t, func(v any) (any, error) {
		return fn(v)
	})
}

// constructPlanTimeResource creates a Resource for plan-time catalog
// resolution using the plan-time constructor registry. Returns nil, false
// if no plan-time constructor exists for the given type.
func constructPlanTimeResource(targetType reflect.Type, value any) (Resource, bool) {
	ctor, ok := planTimeConstructorRegistry.Load(targetType)
	if !ok {
		return nil, false
	}
	result, err := ctor.(func(any) (any, error))(value)
	if err != nil {
		return nil, false
	}

	// The result is typically a value type. Create a temporary pointer
	// to satisfy the Resource interface (pointer receivers).
	rv := reflect.ValueOf(result)
	if rv.Kind() == reflect.Struct && reflect.PointerTo(rv.Type()).Implements(resourceType) {
		ptr := reflect.New(rv.Type())
		ptr.Elem().Set(rv)
		return ptr.Interface().(Resource), true
	}
	if r, ok := result.(Resource); ok {
		return r, true
	}
	return nil, false
}

// --- Receiver params registry ---

// receiverParamsRegistry maps reflect.Type (struct, not pointer) to
// receiverEntry. When marshalReflect encounters a pointer to a registered
// type, it calls WrapReceiver instead of flattening to fields.
var receiverParamsRegistry sync.Map

type receiverEntry struct {
	name   string
	params MethodParams
}

// RegisterReceiverParams registers a Go struct type as a Starlark receiver.
// When marshalReflect encounters *T, it wraps it with WrapReceiver using
// the given name and params instead of flattening to a field-only struct.
func RegisterReceiverParams[T any](name string, params MethodParams) {
	t := reflect.TypeOf((*T)(nil)).Elem()
	receiverParamsRegistry.Store(t, receiverEntry{name: name, params: params})
}

// --- Type cache ---

// typeCache stores struct introspection results, keyed by reflect.Type.
// Computed once per type, concurrent-safe, amortized O(1) lookups.
var typeCache sync.Map

type typeInfo struct {
	fields   []fieldInfo
	attrList []string              // sorted Starlark attribute names
	byName   map[string]*fieldInfo // starlark name → field
}

type fieldInfo struct {
	index    int
	starName string
	goType   reflect.Type
}

// getTypeInfo returns cached struct metadata for the given type.
// If t is a pointer type, the element type is used.
func getTypeInfo(t reflect.Type) *typeInfo {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if cached, ok := typeCache.Load(t); ok {
		return cached.(*typeInfo)
	}

	info := &typeInfo{
		byName: make(map[string]*fieldInfo),
	}

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

	info.attrList = make([]string, len(info.fields))
	for i, f := range info.fields {
		info.attrList[i] = f.starName
	}
	sort.Strings(info.attrList)

	actual, _ := typeCache.LoadOrStore(t, info)
	return actual.(*typeInfo)
}

// --- CamelCase → snake_case ---

// camelToSnake converts a CamelCase identifier to snake_case.
// Consecutive uppercase letters are treated as an acronym (e.g., "XMLParser" → "xml_parser").
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

// --- Marshal (Go → Starlark) ---

// marshal converts a Go value to a starlark.Value.
// Structs become starlarkstruct.Struct. Primitives, slices, maps, and
// pointers are handled recursively. Values already implementing
// starlark.Value pass through unchanged.
func marshal(v any) (starlark.Value, error) {
	if v == nil {
		return starlark.None, nil
	}
	if sv, ok := v.(starlark.Value); ok {
		return sv, nil
	}
	return marshalReflect(reflect.ValueOf(v))
}

func marshalReflect(rv reflect.Value) (starlark.Value, error) {
	// Check receiver params registry for pointer-to-struct types.
	// Registered types get wrapped as ReflectedReceivers (with methods)
	// instead of flattened to field-only structs.
	if rv.Kind() == reflect.Pointer && !rv.IsNil() {
		if entry, ok := receiverParamsRegistry.Load(rv.Type().Elem()); ok {
			e := entry.(receiverEntry)
			return WrapReceiver(e.name, rv.Interface(), e.params), nil
		}
	}

	for rv.Kind() == reflect.Pointer || rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return starlark.None, nil
		}
		rv = rv.Elem()
	}

	// Check starvalue.Marshaler for non-struct types. Struct types are
	// handled inside the struct case to avoid short-circuiting when
	// Marshaler is promoted from an embedded field (e.g., file.Resource
	// embeds ResourceBase which implements Marshaler — we want the outer
	// struct's exported fields to be marshaled normally, with only the
	// embedded ResourceBase using its Marshaler).
	if rv.Kind() != reflect.Struct && rv.CanInterface() {
		if m, ok := rv.Interface().(starvalue.Marshaler); ok {
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
		// If the struct has no exported fields but implements Marshaler,
		// use custom serialization (e.g., ResourceBase with private fields).
		// Structs WITH exported fields go through marshalStruct, where each
		// embedded field gets its own Marshaler check via recursive calls.
		info := getTypeInfo(rv.Type())
		if len(info.fields) == 0 && rv.CanInterface() {
			if m, ok := rv.Interface().(starvalue.Marshaler); ok {
				return m.MarshalStarvalue()
			}
		}
		return marshalStruct(rv)

	default:
		return nil, fmt.Errorf("marshal: unsupported type %s", rv.Type())
	}
}

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

func marshalStruct(rv reflect.Value) (starlark.Value, error) {
	info := getTypeInfo(rv.Type())
	dict := make(starlark.StringDict, len(info.fields))
	for i := range info.fields {
		fi := &info.fields[i]
		val, err := marshalReflect(rv.Field(fi.index))
		if err != nil {
			return nil, fmt.Errorf("marshal: field %s: %w", fi.starName, err)
		}
		dict[fi.starName] = val
	}
	typeName := camelToSnake(rv.Type().Name())
	return starlarkstruct.FromStringDict(starlark.String(typeName), dict), nil
}

// --- Unmarshal (Starlark → Go) ---

// unmarshal populates a Go value from a starlark.Value.
// target must be a non-nil pointer. For *any targets, native Go types
// (string, int, bool, float64, nil, []any, map[string]any) are returned.
// For struct targets, starlarkstruct.Struct fields are matched by name.
func unmarshal(sv starlark.Value, target any) error {
	rv := reflect.ValueOf(target)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return fmt.Errorf("unmarshal: target must be a non-nil pointer, got %T", target)
	}
	return unmarshalValue(sv, rv.Elem())
}

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

	// Dereference/allocate pointers.
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			rv.Set(reflect.New(rv.Type().Elem()))
		}
		rv = rv.Elem()
	}

	// Constructor registry: build complex Go types from simpler Starlark values.
	// If the Starlark value is already a struct whose type name matches the
	// target Go type, skip the constructor and unmarshal fields directly.
	if ctor, ok := constructorRegistry.Load(rv.Type()); ok {
		alreadyTarget := false
		if ss, ok := sv.(*starlarkstruct.Struct); ok {
			if name, ok := ss.Constructor().(starlark.String); ok {
				alreadyTarget = string(name) == camelToSnake(rv.Type().Name())
			}
		}
		if !alreadyTarget {
			native, err := unmarshalToAny(sv)
			if err != nil {
				return err
			}
			val, err := ctor.(func(any) (any, error))(native)
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

// unmarshalToAny converts a Starlark value to a native Go value without
// a specific target type. Returns string, int, bool, float64, nil,
// []any (or []string for homogeneous string lists), or map[string]any.
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
	case *starlarkstruct.Struct:
		return unmarshalStructToAny(v)
	default:
		return nil, fmt.Errorf("unmarshal: unsupported starlark type %s", sv.Type())
	}
}

func unmarshalListToAny(list *starlark.List) (any, error) {
	n := list.Len()
	if n == 0 {
		return []string{}, nil
	}

	// Try homogeneous []string first.
	allStrings := true
	for i := range n {
		if _, ok := list.Index(i).(starlark.String); !ok {
			allStrings = false
			break
		}
	}

	if allStrings {
		result := make([]string, n)
		for i := range n {
			result[i] = string(list.Index(i).(starlark.String))
		}
		return result, nil
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

func unmarshalStructToAny(s *starlarkstruct.Struct) (map[string]any, error) {
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

func unmarshalStruct(sv starlark.Value, rv reflect.Value) error {
	info := getTypeInfo(rv.Type())

	// Accept starlarkstruct.Struct or *starlark.Dict.
	switch v := sv.(type) {
	case *starlarkstruct.Struct:
		for i := range info.fields {
			fi := &info.fields[i]
			attr, err := v.Attr(fi.starName)
			if err != nil {
				continue // Field not present in Starlark struct; leave zero.
			}
			if err := unmarshalValue(attr, rv.Field(fi.index)); err != nil {
				return fmt.Errorf("field %s: %w", fi.starName, err)
			}
		}
		return nil

	case *starlark.Dict:
		for i := range info.fields {
			fi := &info.fields[i]
			val, found, err := v.Get(starlark.String(fi.starName))
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

	default:
		return fmt.Errorf("unmarshal: expected struct or dict, got %s", sv.Type())
	}
}
