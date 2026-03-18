// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"fmt"
	"reflect"
	"strings"

	"go.starlark.net/starlark"
)

// StructValue wraps a Go struct for Starlark with lazy attr dispatch.
// Fields are marshaled on access; eligible methods are called on access.
// This is an internal implementation detail of [Marshal] — Go programmers
// do not construct StructValue directly.
type StructValue struct {
	typeName string
	goValue  reflect.Value // pointer to the Go struct (always addressable)
	info     *typeInfo
}

// region EXPORTED METHODS

// region Behaviors

// Attr implements [starlark.HasAttrs]. Fields are resolved first, then methods.
// Field values are marshaled on each access. Methods are called on each access
// and their return values marshaled.
//
// Parameters:
//   - name: the attribute name to look up.
//
// Returns:
//   - starlark.Value: the marshaled field value or method result.
//   - error: non-nil if the attribute does not exist or marshaling/method call fails.
func (s *StructValue) Attr(name string) (starlark.Value, error) {

	// Fields first.
	if fi, ok := s.info.byName[name]; ok {
		return marshalReflect(s.goValue.Elem().Field(fi.index))
	}

	// Methods second.
	if mi, ok := s.info.byMethod[name]; ok {
		return s.callMethod(mi)
	}

	return nil, NoSuchAttrError(s.typeName, name)
}

// AttrNames implements [starlark.HasAttrs].
//
// Returns:
//   - []string: sorted list of field and method names.
func (s *StructValue) AttrNames() []string {

	return s.info.attrList
}

// Freeze implements [starlark.Value]. No-op — the Go struct is not mutated from Starlark.
func (s *StructValue) Freeze() {}

// Hash implements [starlark.Value]. StructValue is unhashable.
//
// Returns:
//   - uint32: unused.
//   - error: always non-nil.
func (s *StructValue) Hash() (uint32, error) {

	return 0, fmt.Errorf("unhashable: %s", s.typeName)
}

// String implements [starlark.Value]. If the Go type implements [fmt.Stringer],
// its String() method is used. Otherwise, formats as type_name(field = val, ...).
//
// Returns:
//   - string: the string representation.
func (s *StructValue) String() string {

	// Delegate to fmt.Stringer if available.
	if stringer, ok := s.goValue.Interface().(fmt.Stringer); ok {
		return stringer.String()
	}

	var b strings.Builder
	b.WriteString(s.typeName)
	b.WriteByte('(')
	for i, name := range s.info.attrList {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(name)
		b.WriteString(" = ")
		v, err := s.Attr(name)
		if err != nil {
			b.WriteString("<error>")
		} else {
			b.WriteString(v.String())
		}
	}
	b.WriteByte(')')
	return b.String()
}

// Truth implements [starlark.Value].
//
// Returns:
//   - starlark.Bool: always True.
func (s *StructValue) Truth() starlark.Bool {

	return starlark.True
}

// Type implements [starlark.Value].
//
// Returns:
//   - string: the snake_case type name.
func (s *StructValue) Type() string {

	return s.typeName
}

// endregion

// endregion

// region UNEXPORTED METHODS

// callMethod invokes a zero-arg Go method and marshals its return value.
//
// Parameters:
//   - mi: the method metadata.
//
// Returns:
//   - starlark.Value: the marshaled return value.
//   - error: non-nil if the method returns an error or marshaling fails.
func (s *StructValue) callMethod(mi *methodInfo) (starlark.Value, error) {

	results := s.goValue.MethodByName(mi.name).Call(nil)

	if mi.hasError && !results[1].IsNil() {
		return nil, fmt.Errorf("%s.%s: %w", s.typeName, mi.starName, results[1].Interface().(error))
	}

	sv, err := marshalReflect(results[0])
	if err != nil {
		return nil, fmt.Errorf("%s.%s: %w", s.typeName, mi.starName, err)
	}
	return sv, nil
}

// endregion
