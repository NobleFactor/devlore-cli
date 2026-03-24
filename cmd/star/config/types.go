// SPDX-License-Identifier: MIT
// Copyright Noble Factor. All rights reserved.

package config

import (
	"fmt"
	"reflect"
	"strings"
	"sync"
)

// typeCache caches generated types by extension path.
var typeCache = struct {
	sync.RWMutex
	types map[string]reflect.Type
}{types: make(map[string]reflect.Type)}

// getOrCreateType returns a cached type or generates a new one.
func getOrCreateType(path string, spec ConfigSpec) reflect.Type {
	typeCache.RLock()
	if typ, ok := typeCache.types[path]; ok {
		typeCache.RUnlock()
		return typ
	}
	typeCache.RUnlock()

	typeCache.Lock()
	defer typeCache.Unlock()

	// Double-check after acquiring write lock
	if typ, ok := typeCache.types[path]; ok {
		return typ
	}

	typ := generateConfigType(spec)
	typeCache.types[path] = typ
	return typ
}

// generateConfigType creates a Go struct type from a ConfigSpec.
// The generated type embeds ConfigElement and has fields matching the spec.
func generateConfigType(spec ConfigSpec) reflect.Type {
	var fields []reflect.StructField

	// Add embedded ConfigElement as first field
	fields = append(fields, reflect.StructField{
		Name:      "ConfigElement",
		Type:      reflect.TypeOf(ConfigElement{}),
		Anonymous: true,
	})

	// Add fields from spec
	for name, typeName := range spec.Fields {
		fieldType := resolveType(typeName, spec.Nested)
		fields = append(fields, reflect.StructField{
			Name: toPascalCase(name),
			Type: fieldType,
			Tag:  reflect.StructTag(fmt.Sprintf(`yaml:"%s" json:"%s"`, name, name)),
		})
	}

	return reflect.StructOf(fields)
}

// resolveType converts a type name string to reflect.Type.
// Handles primitives, slices, maps, and nested struct types.
// The nested parameter contains all type definitions available for resolution,
// including sibling types at the same level.
func resolveType(typeName string, nested map[string]ConfigSpec) reflect.Type {
	switch {
	case typeName == "bool":
		return reflect.TypeOf(true)
	case typeName == "string":
		return reflect.TypeOf("")
	case typeName == "int":
		return reflect.TypeOf(0)
	case typeName == "float64":
		return reflect.TypeOf(0.0)
	case strings.HasPrefix(typeName, "[]"):
		// Slice type: []string, []int, etc.
		elemTypeName := typeName[2:]
		elemType := resolveType(elemTypeName, nested)
		return reflect.SliceOf(elemType)
	case strings.HasPrefix(typeName, "map["):
		// Map type: map[string]Pattern, map[string]interface{}, etc.
		return parseMapType(typeName, nested)
	default:
		// Check if it's a nested type definition
		if nestedSpec, ok := nested[typeName]; ok {
			return generateNestedType(nestedSpec, nested)
		}
		// Unknown type - fall back to interface{}
		return reflect.TypeOf((*interface{})(nil)).Elem()
	}
}

// parseMapType parses a map type string like "map[string]Pattern".
func parseMapType(typeName string, nested map[string]ConfigSpec) reflect.Type {
	// Find the closing bracket for the key type
	keyEnd := strings.Index(typeName, "]")
	if keyEnd == -1 {
		return reflect.TypeOf(map[string]interface{}{})
	}

	keyTypeName := typeName[4:keyEnd]
	valTypeName := typeName[keyEnd+1:]

	keyType := resolveType(keyTypeName, nested)
	valType := resolveType(valTypeName, nested)

	return reflect.MapOf(keyType, valType)
}

// generateNestedType generates a struct type from a nested ConfigSpec.
// Unlike generateConfigType, this does not embed ConfigElement.
// The allNested parameter provides sibling type definitions so that a nested
// type can reference other nested types at the same level (e.g., CommentSchema
// referencing SchemaElement).
func generateNestedType(spec ConfigSpec, allNested map[string]ConfigSpec) reflect.Type {
	var fields []reflect.StructField

	// Merge the type's own nested definitions with the parent scope so
	// sibling types are resolvable.
	merged := allNested
	if len(spec.Nested) > 0 {
		merged = make(map[string]ConfigSpec, len(allNested)+len(spec.Nested))
		for k, v := range allNested {
			merged[k] = v
		}
		for k, v := range spec.Nested {
			merged[k] = v
		}
	}

	for name, typeName := range spec.Fields {
		fieldType := resolveType(typeName, merged)
		fields = append(fields, reflect.StructField{
			Name: toPascalCase(name),
			Type: fieldType,
			Tag:  reflect.StructTag(fmt.Sprintf(`yaml:"%s" json:"%s"`, name, name)),
		})
	}

	return reflect.StructOf(fields)
}

// newConfigInstance creates an instance of a generated type and populates defaults.
func newConfigInstance(typ reflect.Type, defaults map[string]interface{}) interface{} {
	ptr := reflect.New(typ)
	instance := ptr.Elem()

	for name, val := range defaults {
		fieldName := toPascalCase(name)
		field := instance.FieldByName(fieldName)
		if !field.IsValid() || !field.CanSet() {
			continue
		}
		setFieldValue(field, val)
	}

	return ptr.Interface()
}

// setFieldValue sets a reflect.Value from an interface{} value.
// Recursively handles maps and nested structs.
func setFieldValue(field reflect.Value, val interface{}) {
	if val == nil {
		return
	}

	switch v := val.(type) {
	case map[string]interface{}:
		setMapOrStructValue(field, v)
	case []interface{}:
		setSliceValue(field, v)
	case []string:
		setStringSliceValue(field, v)
	default:
		// Primitive value
		setPrimitiveValue(field, val)
	}
}

// setMapOrStructValue sets a map or struct field from a map.
func setMapOrStructValue(field reflect.Value, m map[string]interface{}) {
	switch field.Kind() {
	case reflect.Map:
		// Map field - create map and populate
		mapType := field.Type()
		if field.IsNil() {
			field.Set(reflect.MakeMap(mapType))
		}
		for k, mv := range m {
			keyVal := reflect.ValueOf(k)
			elemVal := reflect.New(mapType.Elem()).Elem()
			if mvMap, ok := mv.(map[string]interface{}); ok {
				setStructFields(elemVal, mvMap)
			} else {
				setPrimitiveValue(elemVal, mv)
			}
			field.SetMapIndex(keyVal, elemVal)
		}
	case reflect.Struct:
		// Nested struct
		setStructFields(field, m)
	case reflect.Interface:
		// Interface{} field - just set the map directly
		field.Set(reflect.ValueOf(m))
	}
}

// setSliceValue sets a slice field from []interface{}.
func setSliceValue(field reflect.Value, items []interface{}) {
	if field.Kind() != reflect.Slice {
		return
	}

	sliceType := field.Type()
	newSlice := reflect.MakeSlice(sliceType, len(items), len(items))

	for i, item := range items {
		elemVal := newSlice.Index(i)
		if itemMap, ok := item.(map[string]interface{}); ok && elemVal.Kind() == reflect.Struct {
			setStructFields(elemVal, itemMap)
		} else {
			setPrimitiveValue(elemVal, item)
		}
	}

	field.Set(newSlice)
}

// setStringSliceValue sets a []string field.
func setStringSliceValue(field reflect.Value, items []string) {
	if field.Kind() != reflect.Slice {
		return
	}
	field.Set(reflect.ValueOf(items))
}

// setPrimitiveValue sets a primitive field value with type conversion.
func setPrimitiveValue(field reflect.Value, val interface{}) {
	if !field.CanSet() {
		return
	}

	// Handle type conversions
	rv := reflect.ValueOf(val)
	if !rv.IsValid() {
		return
	}

	// Direct assignment if types match
	if rv.Type().AssignableTo(field.Type()) {
		field.Set(rv)
		return
	}

	// Try conversion
	if rv.Type().ConvertibleTo(field.Type()) {
		field.Set(rv.Convert(field.Type()))
		return
	}

	// Special case: YAML often parses numbers as float64
	switch field.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if f, ok := val.(float64); ok {
			field.SetInt(int64(f))
		} else if i, ok := val.(int); ok {
			field.SetInt(int64(i))
		}
	case reflect.Float32, reflect.Float64:
		if i, ok := val.(int); ok {
			field.SetFloat(float64(i))
		}
	case reflect.Bool:
		if b, ok := val.(bool); ok {
			field.SetBool(b)
		}
	case reflect.String:
		if s, ok := val.(string); ok {
			field.SetString(s)
		}
	}
}

// setStructFields populates struct fields from a map.
func setStructFields(structVal reflect.Value, values map[string]interface{}) {
	if structVal.Kind() == reflect.Ptr {
		structVal = structVal.Elem()
	}
	if structVal.Kind() != reflect.Struct {
		return
	}

	for name, val := range values {
		fieldName := toPascalCase(name)
		field := structVal.FieldByName(fieldName)
		if field.IsValid() && field.CanSet() {
			setFieldValue(field, val)
		}
	}
}

// ClearTypeCache clears the type cache. Useful for testing.
func ClearTypeCache() {
	typeCache.Lock()
	defer typeCache.Unlock()
	typeCache.types = make(map[string]reflect.Type)
}
