// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strings"

	"go.starlark.net/starlark"
)

// MethodParams maps Go method names (CamelCase) to Starlark parameter
// name lists. Optional params use the "name?" suffix.
type MethodParams map[string][]string

// ReflectedReceiver wraps a Go struct for immediate-mode Starlark use.
// Exported methods become Starlark builtins; method dispatch, argument
// unpacking, and return value marshaling are handled by reflection.
//
// Optional catalog and stack fields enable resource management
// integration. When set, Resource results are shadowed in the catalog
// after successful dispatch.
type ReflectedReceiver struct {
	Receiver
	providerValue reflect.Value
	methods       map[string]*methodBridge
	attrList      []string
	catalog       *ResourceCatalog // optional; set via SetCatalog
	stack         *RecoveryStack   // optional; set via SetStack
}

// SetCatalog sets the resource catalog for immediate-mode dispatch.
// When set, Resource results are shadowed in the catalog after each
// successful method call.
func (r *ReflectedReceiver) SetCatalog(c *ResourceCatalog) { r.catalog = c }

// SetStack sets the recovery stack for immediate-mode dispatch.
func (r *ReflectedReceiver) SetStack(s *RecoveryStack) { r.stack = s }

type methodBridge struct {
	name   string
	bridge builtinFunc
}

// errorType and noResultType are cached for return-type classification.
var (
	errorType    = reflect.TypeOf((*error)(nil)).Elem()
	noResultType = reflect.TypeOf(NoResult{})
)

// WrapReceiver wraps a Go struct for immediate-mode Starlark use.
// Only methods listed in params are exposed. Compensate* methods are
// excluded automatically.
func WrapReceiver(name string, provider any, params MethodParams) *ReflectedReceiver {
	rv := reflect.ValueOf(provider)
	r := &ReflectedReceiver{
		Receiver:      newReceiver(name),
		providerValue: rv,
		methods:       make(map[string]*methodBridge),
	}

	t := rv.Type()
	for i := range t.NumMethod() {
		m := t.Method(i)
		if !m.IsExported() || strings.HasPrefix(m.Name, "Compensate") {
			continue
		}

		paramNames, ok := params[m.Name]
		if !ok {
			continue
		}

		snakeName := camelToSnake(m.Name)
		bridge := buildMethodBridge(name, rv, m, snakeName, paramNames, r)
		r.methods[snakeName] = &methodBridge{
			name:   snakeName,
			bridge: bridge,
		}
	}

	r.attrList = make([]string, 0, len(r.methods))
	for name := range r.methods {
		r.attrList = append(r.attrList, name)
	}
	sort.Strings(r.attrList)

	return r
}

// Override replaces a method's auto-generated bridge with a custom one.
// Used for methods with unusual signatures (Callable params, variadic
// args, non-zero defaults).
func (r *ReflectedReceiver) Override(name string, fn builtinFunc) {
	r.methods[name] = &methodBridge{
		name:   name,
		bridge: fn,
	}
	// Rebuild attr list if new method added.
	if !slices.Contains(r.attrList, name) {
		r.attrList = append(r.attrList, name)
		sort.Strings(r.attrList)
	}
}

// Attr implements starlark.HasAttrs.
func (r *ReflectedReceiver) Attr(name string) (starlark.Value, error) {
	if m, ok := r.methods[name]; ok {
		return MakeAttr(r.Receiver.name+"."+name, m.bridge), nil
	}
	return nil, NoSuchAttrError(r.Receiver.name, name)
}

// AttrNames implements starlark.HasAttrs.
func (r *ReflectedReceiver) AttrNames() []string {
	return r.attrList
}

// buildMethodBridge creates a builtinFunc that bridges a Go method to Starlark.
// The receiver parameter provides optional catalog/stack integration —
// Resource results are shadowed in the catalog after successful dispatch.
func buildMethodBridge(
	receiverName string,
	providerVal reflect.Value,
	method reflect.Method,
	snakeName string,
	paramNames []string,
	receiver *ReflectedReceiver,
) builtinFunc {
	methodType := method.Type
	numParams := len(paramNames)

	return func(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		// 1. Unpack Starlark args into starlark.Value slots.
		vals := make([]starlark.Value, numParams)
		pairs := make([]any, 0, numParams*2)
		for i, name := range paramNames {
			pairs = append(pairs, name, &vals[i])
		}
		if err := starlark.UnpackArgs(snakeName, args, kwargs, pairs...); err != nil {
			return nil, err
		}

		// 2. Convert Starlark values to Go values via reflection.
		// methodType.In(0) is the receiver; params start at index 1.
		goArgs := make([]reflect.Value, numParams+1)
		goArgs[0] = providerVal

		for i, sv := range vals {
			paramType := methodType.In(i + 1) // skip receiver
			if sv == nil {
				// Optional param not provided; use Go zero value.
				goArgs[i+1] = reflect.Zero(paramType)
				continue
			}
			goVal := reflect.New(paramType).Elem()
			if err := unmarshalValue(sv, goVal); err != nil {
				name := strings.TrimSuffix(paramNames[i], "?")
				return nil, fmt.Errorf("%s.%s: param %s: %w", receiverName, snakeName, name, err)
			}
			goArgs[i+1] = goVal
		}

		// 3. Call the Go method.
		var results []reflect.Value
		if methodType.IsVariadic() {
			results = method.Func.CallSlice(goArgs)
		} else {
			results = method.Func.Call(goArgs)
		}

		// 4. Shadow Resource results in catalog (success path only).
		// The originID is the qualified method name (e.g., "file.copy")
		// since immediate mode has no graph nodes.
		if receiver.catalog != nil && len(results) > 0 {
			lastIdx := len(results) - 1
			isErr := results[lastIdx].Type().Implements(errorType) && !results[lastIdx].IsNil()
			if !isErr && results[0].Type() != noResultType {
				originID := receiverName + "." + snakeName
				shadowResult(results[0].Interface(), receiver.catalog, originID)
			}
		}

		// 5. Classify and marshal return values.
		return classifyReturn(results)
	}
}

// classifyReturn interprets Go method return values for Starlark.
//
// Patterns handled:
//
//	()                         → None
//	(error)                    → None or error
//	(T)                        → marshal(T)
//	(T, error)                 → marshal(T) or error
//	(T, map[string]any, error) → marshal(T), discard undo state, or error
//	(T, *RecoveryStack, error) → marshal(T), discard stack, or error
//	(NoResult, ...)            → None (sentinel for no-output methods)
func classifyReturn(results []reflect.Value) (starlark.Value, error) {
	n := len(results)
	if n == 0 {
		return starlark.None, nil
	}

	// If the last return is an error, consume it.
	if results[n-1].Type().Implements(errorType) {
		if !results[n-1].IsNil() {
			return nil, results[n-1].Interface().(error)
		}
		n--
	}

	if n == 0 {
		return starlark.None, nil
	}

	// NoResult sentinel → starlark.None.
	if results[0].Type() == noResultType {
		return starlark.None, nil
	}

	// Marshal the first non-error return value.
	// Additional returns (compensation state) are discarded in immediate mode.
	return marshalReflect(results[0])
}
