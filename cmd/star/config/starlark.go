// SPDX-License-Identifier: MIT
// Copyright Noble Factor. All rights reserved.

package config

import (
	"errors"
	"fmt"
	"reflect"

	"go.starlark.net/starlark"
)

// ===========================================================================
// Value: Reflection-based Starlark adapter for dynamic config types
// ===========================================================================
//
// Value wraps any Go struct (including those generated at runtime via
// reflect.StructOf) and implements starlark.HasAttrs for attribute access.
// This allows dynamically-generated extension configs to be accessed in
// Starlark scripts using dot notation: cfg.lint.copyright.enabled

// Value wraps a Go struct for Starlark attribute access.
// It implements starlark.Value and starlark.HasAttrs.
type Value struct {
	elem interface{} // any struct or pointer to struct
}

// Ensure Value implements the required interfaces.
var (
	_ starlark.Value    = (*Value)(nil)
	_ starlark.HasAttrs = (*Value)(nil)
)

// WrapAsStarlarkValue wraps any Go value for Starlark access.
// Returns a Value that provides attribute access via reflection.
func WrapAsStarlarkValue(v interface{}) *Value {
	return &Value{elem: v}
}

// String returns a string representation of the Value.
func (v *Value) String() string {
	if v.elem == nil {
		return "config(nil)"
	}
	return fmt.Sprintf("config(%T)", v.elem)
}

// Type returns the Starlark type name.
func (v *Value) Type() string {
	return "config"
}

// Freeze makes the Value immutable. This is a no-op since Go structs
// don't have a concept of mutability that matches Starlark's.
func (v *Value) Freeze() {}

// Truth returns the Starlark truth value. Value is always truthy.
func (v *Value) Truth() starlark.Bool {
	return starlark.True
}

// Hash returns a hash for the Value. Config values are not hashable.
func (v *Value) Hash() (uint32, error) {
	return 0, errors.New("config is not hashable")
}

// Attr returns the value of the named attribute.
// Implements starlark.HasAttrs.
func (v *Value) Attr(name string) (starlark.Value, error) {
	if v.elem == nil {
		return nil, starlark.NoSuchAttrError(fmt.Sprintf("config has no .%s attribute", name))
	}

	// Try to use Element interface if available
	if elem := getConfigElement(v.elem); elem != nil {
		if child := elem.Get(name); child != nil {
			return goToStarlarkReflect(child)
		}
	}

	rv := reflect.ValueOf(v.elem)
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}

	// Look for struct field
	if rv.Kind() == reflect.Struct {
		fieldName := toPascalCase(name)
		field := rv.FieldByName(fieldName)
		if field.IsValid() {
			return goToStarlarkReflect(field.Interface())
		}
	}

	return nil, starlark.NoSuchAttrError(fmt.Sprintf("config has no .%s attribute", name))
}

// getConfigElement extracts the Element from various types.
func getConfigElement(v interface{}) *Element {
	if v == nil {
		return nil
	}

	// Direct *Element
	if elem, ok := v.(*Element); ok {
		return elem
	}

	// extensionsConfig has embedded Element
	if cfg, ok := v.(*extensionsConfig); ok {
		return &cfg.Element
	}

	// Try to extract via reflection for other types
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return nil
	}

	// Look for embedded Element field
	elemField := rv.FieldByName("Element")
	if !elemField.IsValid() {
		return nil
	}

	// Get the address of the embedded field if possible
	if elemField.CanAddr() {
		if elem, ok := elemField.Addr().Interface().(*Element); ok {
			return elem
		}
	}

	return nil
}

// AttrNames returns the names of all available attributes.
// Implements starlark.HasAttrs.
func (v *Value) AttrNames() []string {
	if v.elem == nil {
		return nil
	}

	var names []string

	// Add Element children
	if elem := getConfigElement(v.elem); elem != nil {
		for name := range elem.Children() {
			names = append(names, name)
		}
	}

	rv := reflect.ValueOf(v.elem)
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return names
	}

	// Add struct fields
	rt := rv.Type()
	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)
		if !field.IsExported() {
			continue
		}
		// Skip embedded Element
		if field.Anonymous && field.Name == "Element" {
			continue
		}
		names = append(names, toSnakeCase(field.Name))
	}

	return names
}

// goToStarlarkReflect converts a Go value to a Starlark value using reflection.
// This handles runtime-generated types that can't be matched in type switches.
func goToStarlarkReflect(v interface{}) (starlark.Value, error) {
	if v == nil {
		return starlark.None, nil
	}

	rv := reflect.ValueOf(v)
	return reflectToStarlark(rv)
}

// reflectToStarlark converts a reflect.Value to a Starlark value.
func reflectToStarlark(rv reflect.Value) (starlark.Value, error) {
	// Handle interface{}
	if rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return starlark.None, nil
		}
		rv = rv.Elem()
	}

	// Handle pointers - wrap pointer to struct as Value to preserve reference
	if rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return starlark.None, nil
		}
		// If it's a pointer to a struct, wrap it directly (preserve pointer)
		if rv.Elem().Kind() == reflect.Struct {
			return WrapAsStarlarkValue(rv.Interface()), nil
		}
		// For other pointer types, dereference
		rv = rv.Elem()
	}

	switch rv.Kind() {
	case reflect.Bool:
		return starlark.Bool(rv.Bool()), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return starlark.MakeInt64(rv.Int()), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return starlark.MakeUint64(rv.Uint()), nil
	case reflect.Float32, reflect.Float64:
		return starlark.Float(rv.Float()), nil
	case reflect.String:
		return starlark.String(rv.String()), nil
	case reflect.Slice:
		return sliceToStarlark(rv)
	case reflect.Map:
		return mapToStarlarkReflect(rv)
	case reflect.Struct:
		// Wrap struct as Value - try to get addressable version
		if rv.CanAddr() {
			return WrapAsStarlarkValue(rv.Addr().Interface()), nil
		}
		return WrapAsStarlarkValue(rv.Interface()), nil
	default:
		return starlark.None, nil
	}
}

// sliceToStarlark converts a slice to a Starlark list.
func sliceToStarlark(rv reflect.Value) (starlark.Value, error) {
	n := rv.Len()
	items := make([]starlark.Value, n)
	for i := 0; i < n; i++ {
		item, err := reflectToStarlark(rv.Index(i))
		if err != nil {
			return nil, err
		}
		items[i] = item
	}
	return starlark.NewList(items), nil
}

// mapToStarlarkReflect converts a map to a Starlark dict using reflection.
func mapToStarlarkReflect(rv reflect.Value) (starlark.Value, error) {
	dict := starlark.NewDict(rv.Len())
	iter := rv.MapRange()
	for iter.Next() {
		key, err := reflectToStarlark(iter.Key())
		if err != nil {
			return nil, err
		}
		val, err := reflectToStarlark(iter.Value())
		if err != nil {
			return nil, err
		}
		if err := dict.SetKey(key, val); err != nil {
			return nil, err
		}
	}
	return dict, nil
}
