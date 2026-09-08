// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package op

import (
	"encoding"
	"fmt"
	"reflect"
)

// frameworkSlotTypes declares the slots the framework reads by name off a unit's bindings, and their types.
//
// A unit's bindings hold two kinds of slot: the parameters its method declares, and these, which no method declares
// because the framework itself consumes them -- `on_missing` is read by [unitMissingResourcePolicy], `claim` by scoped
// pre-flight. Without this table the serializer's "undeclared, therefore any" rule enveloped a
// [MissingResourcePolicy], which has no envelope name, and the reader had nothing to parse a bare `ignore` back
// against (ruled 2026-09-06, docs/plans/fix/712-any-slot-types.md).
//
// Both seams consult it: [slotCarriesItsType] treats a framework slot as declared, and [readAgainstField] parses a
// bare value through the declared type.
var frameworkSlotTypes = map[string]reflect.Type{
	"claim":      reflect.TypeFor[Resource](),
	"on_missing": reflect.TypeFor[MissingResourcePolicy](),
}

// frameworkSlotType returns the framework-declared type of a slot, when the framework declares one.
//
// Parameters:
//   - `name`: the slot name.
//
// Returns:
//   - `reflect.Type`: the declared type, or nil.
//   - `bool`: true when `name` is a framework-owned slot.
func frameworkSlotType(name string) (reflect.Type, bool) {
	declared, ok := frameworkSlotTypes[name]
	return declared, ok
}

// readFrameworkSlot parses a bare document value back through a framework slot's declared type.
//
// The declared types marshal as text (a policy writes its name), so a string reads back through
// [encoding.TextUnmarshaler]. Anything else is an error naming the slot and the type: no guessing (#712 phase 2).
//
// Parameters:
//   - `value`: the decoded document value.
//   - `declared`: the slot's framework-declared type.
//
// Returns:
//   - `any`: the typed value.
//   - `error`: a value that is not text, a declared type without a text form, or text the type refuses.
func readFrameworkSlot(value any, declared reflect.Type) (any, error) {

	text, isString := value.(string)
	if !isString {
		return nil, fmt.Errorf("declared %s by the framework, and the document holds a %T, not its text form", declared, value)
	}

	target := reflect.New(declared)
	unmarshaler, ok := target.Interface().(encoding.TextUnmarshaler)
	if !ok {
		return nil, fmt.Errorf("declared %s by the framework, a type with no text form to read %q back through", declared, text)
	}
	if err := unmarshaler.UnmarshalText([]byte(text)); err != nil {
		return nil, err
	}

	return target.Elem().Interface(), nil
}
