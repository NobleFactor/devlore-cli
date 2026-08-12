// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package config

import (
	"reflect"
)

// Accessor provides typed access to config struct fields via reflection.
// It wraps a reflect.Value and provides type-safe accessor methods.
type Accessor struct {
	v reflect.Value
}

// NewAccessor creates a Accessor for the given value.
// The value should be a struct or pointer to struct.
func NewAccessor(v interface{}) *Accessor {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer {
		rv = rv.Elem()
	}
	return &Accessor{v: rv}
}

// IsValid returns true if the accessor wraps a valid value.
func (a *Accessor) IsValid() bool {
	return a.v.IsValid()
}

// Bool returns the bool value of the named field.
// Returns false if the field doesn't exist or isn't a bool.
func (a *Accessor) Bool(name string) bool {
	field := a.field(name)
	if !field.IsValid() || field.Kind() != reflect.Bool {
		return false
	}
	return field.Bool()
}

// BoolOr returns the bool value of the named field, or the default if not set.
func (a *Accessor) BoolOr(name string, defaultVal bool) bool {
	field := a.field(name)
	if !field.IsValid() || field.Kind() != reflect.Bool {
		return defaultVal
	}
	return field.Bool()
}

// String returns the string value of the named field.
// Returns empty string if the field doesn't exist or isn't a string.
func (a *Accessor) String(name string) string {
	field := a.field(name)
	if !field.IsValid() || field.Kind() != reflect.String {
		return ""
	}
	return field.String()
}

// StringOr returns the string value of the named field, or the default if not set.
func (a *Accessor) StringOr(name, defaultVal string) string {
	field := a.field(name)
	if !field.IsValid() || field.Kind() != reflect.String {
		return defaultVal
	}
	s := field.String()
	if s == "" {
		return defaultVal
	}
	return s
}

// Int returns the int value of the named field.
// Returns 0 if the field doesn't exist or isn't an integer type.
func (a *Accessor) Int(name string) int {
	field := a.field(name)
	if !field.IsValid() {
		return 0
	}
	switch field.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return int(field.Int())
	default:
		return 0
	}
}

// IntOr returns the int value of the named field, or the default if not set.
func (a *Accessor) IntOr(name string, defaultVal int) int {
	field := a.field(name)
	if !field.IsValid() {
		return defaultVal
	}
	switch field.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return int(field.Int())
	default:
		return defaultVal
	}
}

// Float returns the float64 value of the named field.
// Returns 0 if the field doesn't exist or isn't a float type.
func (a *Accessor) Float(name string) float64 {
	field := a.field(name)
	if !field.IsValid() {
		return 0
	}
	switch field.Kind() {
	case reflect.Float32, reflect.Float64:
		return field.Float()
	default:
		return 0
	}
}

// StringSlice returns the []string value of the named field.
// Returns nil if the field doesn't exist or isn't a string slice.
func (a *Accessor) StringSlice(name string) []string {
	field := a.field(name)
	if !field.IsValid() || field.Kind() != reflect.Slice {
		return nil
	}
	// Check if it's a []string
	if field.Type().Elem().Kind() != reflect.String {
		return nil
	}
	// Convert to []string
	result := make([]string, field.Len())
	for i := 0; i < field.Len(); i++ {
		result[i] = field.Index(i).String()
	}
	return result
}

// StringSliceOr returns the []string value, or the default if not set.
func (a *Accessor) StringSliceOr(name string, defaultVal []string) []string {
	result := a.StringSlice(name)
	if result == nil {
		return defaultVal
	}
	return result
}

// Map returns the map[string]interface{} value of the named field.
// Returns nil if the field doesn't exist or isn't a map.
func (a *Accessor) Map(name string) map[string]interface{} {
	field := a.field(name)
	if !field.IsValid() || field.Kind() != reflect.Map {
		return nil
	}

	result := make(map[string]interface{})
	iter := field.MapRange()
	for iter.Next() {
		key := iter.Key()
		if key.Kind() == reflect.String {
			result[key.String()] = iter.Value().Interface()
		}
	}
	return result
}

// Struct returns a Accessor for the named nested struct field.
// Returns an invalid accessor if the field doesn't exist or isn't a struct.
func (a *Accessor) Struct(name string) *Accessor {
	field := a.field(name)
	if !field.IsValid() {
		return &Accessor{}
	}
	if field.Kind() == reflect.Pointer {
		field = field.Elem()
	}
	if field.Kind() != reflect.Struct {
		return &Accessor{}
	}
	return &Accessor{v: field}
}

// Get returns the raw value of the named field as interface{}.
func (a *Accessor) Get(name string) interface{} {
	field := a.field(name)
	if !field.IsValid() {
		return nil
	}
	return field.Interface()
}

// Has returns true if the named field exists.
func (a *Accessor) Has(name string) bool {
	return a.field(name).IsValid()
}

// Fields returns all field names in the struct.
func (a *Accessor) Fields() []string {
	if !a.v.IsValid() || a.v.Kind() != reflect.Struct {
		return nil
	}

	t := a.v.Type()
	var names []string
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
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

// field returns the reflect.Value for the named field.
func (a *Accessor) field(name string) reflect.Value {
	if !a.v.IsValid() || a.v.Kind() != reflect.Struct {
		return reflect.Value{}
	}

	fieldName := toPascalCase(name)
	field := a.v.FieldByName(fieldName)
	return field
}

// Raw returns the underlying reflect.Value.
func (a *Accessor) Raw() reflect.Value {
	return a.v
}
