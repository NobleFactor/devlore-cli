// SPDX-License-Identifier: MIT
// Copyright Noble Factor. All rights reserved.

package config

import (
	"reflect"
	"strings"
	"unicode"
)

// ConfigElement is the base type embedded by all config sections.
// It supports hierarchical composition via Register and navigation via Navigate.
type ConfigElement struct {
	path     string
	children map[string]interface{} // child ConfigElements or config structs
}

// Path returns the dotted path of this element in the config hierarchy.
func (e *ConfigElement) Path() string {
	return e.path
}

// SetPath sets the path of this element. Used during registration.
func (e *ConfigElement) SetPath(path string) {
	e.path = path
}

// Register adds a child to this element.
// The child's path is computed relative to this element's path.
func (e *ConfigElement) Register(name string, child interface{}) {
	if e.children == nil {
		e.children = make(map[string]interface{})
	}

	// Compute child's path
	childPath := name
	if e.path != "" {
		childPath = e.path + "." + name
	}

	// Set the child's path if it has a ConfigElement
	setPath(child, childPath)

	e.children[name] = child
}

// Get retrieves a child by name.
func (e *ConfigElement) Get(name string) interface{} {
	if e.children == nil {
		return nil
	}
	return e.children[name]
}

// Children returns all registered children.
func (e *ConfigElement) Children() map[string]interface{} {
	return e.children
}

// Navigate traverses the config hierarchy by dotted path.
// Returns the element or field value at the given path, or nil if not found.
//
// Examples:
//
//	Navigate("") returns the element itself
//	Navigate("lint") returns the lint child element
//	Navigate("lint.copyright") returns the copyright child of lint
//	Navigate("lint.copyright.enabled") returns the enabled field value
func (e *ConfigElement) Navigate(path string) interface{} {
	if path == "" {
		return e
	}

	parts := strings.Split(path, ".")
	return e.navigateParts(parts)
}

// navigateParts traverses the hierarchy using pre-split path parts.
func (e *ConfigElement) navigateParts(parts []string) interface{} {
	if len(parts) == 0 {
		return e
	}

	current := e
	for i, part := range parts {
		child := current.Get(part)
		if child == nil {
			return nil
		}

		// If this is the last part, return the child
		if i == len(parts)-1 {
			return child
		}

		// Try to continue navigation based on child type
		switch c := child.(type) {
		case *ConfigElement:
			current = c
		default:
			// Child may embed ConfigElement or be a struct with fields
			elem := extractConfigElement(child)
			if elem != nil {
				// Check if the embedded element has the next part as a child
				nextPart := parts[i+1]
				if elem.Get(nextPart) != nil {
					current = elem
					continue
				}
			}

			// Child is a leaf struct - traverse its fields
			return navigateStructFields(child, parts[i+1:])
		}
	}

	return current
}

// navigateStructFields traverses struct fields by name.
func navigateStructFields(obj interface{}, parts []string) interface{} {
	rv := reflect.ValueOf(obj)
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return nil
	}

	for _, part := range parts {
		fieldName := toPascalCase(part)
		field := rv.FieldByName(fieldName)
		if !field.IsValid() {
			return nil
		}
		rv = field
		if rv.Kind() == reflect.Ptr {
			rv = rv.Elem()
		}
	}

	if !rv.IsValid() {
		return nil
	}
	return rv.Interface()
}

// extractConfigElement extracts the embedded ConfigElement from a struct.
func extractConfigElement(obj interface{}) *ConfigElement {
	rv := reflect.ValueOf(obj)
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return nil
	}

	field := rv.FieldByName("ConfigElement")
	if !field.IsValid() || !field.CanAddr() {
		return nil
	}

	if elem, ok := field.Addr().Interface().(*ConfigElement); ok {
		return elem
	}
	return nil
}

// setPath sets the path on a child element.
// Works with *ConfigElement, structs embedding ConfigElement, or structs with SetPath method.
func setPath(child interface{}, path string) {
	// Direct *ConfigElement
	if elem, ok := child.(*ConfigElement); ok {
		elem.path = path
		return
	}

	rv := reflect.ValueOf(child)
	if rv.Kind() != reflect.Ptr {
		return
	}
	rv = rv.Elem()
	if rv.Kind() != reflect.Struct {
		return
	}

	// Check for embedded ConfigElement
	field := rv.FieldByName("ConfigElement")
	if field.IsValid() && field.CanAddr() {
		if elem, ok := field.Addr().Interface().(*ConfigElement); ok {
			elem.path = path
			return
		}
	}

	// Check for SetPath method
	method := reflect.ValueOf(child).MethodByName("SetPath")
	if method.IsValid() {
		method.Call([]reflect.Value{reflect.ValueOf(path)})
	}
}

// toPascalCase converts snake_case or kebab-case to PascalCase.
// Examples: "enabled" -> "Enabled", "skip_mod_tidy" -> "SkipModTidy"
func toPascalCase(s string) string {
	if s == "" {
		return s
	}

	var result strings.Builder
	result.Grow(len(s))

	capitalizeNext := true
	for _, r := range s {
		if r == '_' || r == '-' {
			capitalizeNext = true
			continue
		}
		if capitalizeNext {
			result.WriteRune(unicode.ToUpper(r))
			capitalizeNext = false
		} else {
			result.WriteRune(r)
		}
	}

	return result.String()
}

// toSnakeCase converts PascalCase or camelCase to snake_case.
// Examples: "Enabled" -> "enabled", "SkipModTidy" -> "skip_mod_tidy"
func toSnakeCase(s string) string {
	if s == "" {
		return s
	}

	var result strings.Builder
	result.Grow(len(s) + 4) // Account for underscores

	for i, r := range s {
		if unicode.IsUpper(r) {
			if i > 0 {
				result.WriteRune('_')
			}
			result.WriteRune(unicode.ToLower(r))
		} else {
			result.WriteRune(r)
		}
	}

	return result.String()
}
