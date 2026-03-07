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

// WrapReceiver wraps a Go struct for immediate-mode Starlark use.
// Params are looked up from the receiver params registry (populated by
// RegisterReflectedActions during InitAll). Only methods listed in the
// registered params are exposed. Compensate* methods are excluded.
func WrapReceiver(name string, provider any) *ReflectedReceiver {
	entry, ok := lookupReceiverParams(reflect.TypeOf(provider))
	if !ok {
		panic(fmt.Sprintf("WrapReceiver(%s): no params registered — was RegisterReflectedActions called?", name))
	}

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

		paramNames, ok := entry.params[m.Name]
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
//
// Variadic parameters are marked with a "*" prefix in paramNames (e.g. "*parts").
// Remaining positional Starlark args are collected into the variadic slot.
// The keyword form (parts=["a","b"]) is also accepted as a fallback.
// Supplying both positional and keyword args for a variadic param is an error.
func buildMethodBridge(
	receiverName string,
	providerVal reflect.Value,
	method reflect.Method,
	snakeName string,
	paramNames []string,
	receiver *ReflectedReceiver,
) builtinFunc {
	methodType := method.Type

	// Separate named params from the variadic param (at most one, always last).
	var variadicName string // snake_case name without "*"
	var variadicIdx int     // index in paramNames
	namedParams := make([]string, 0, len(paramNames))
	for i, p := range paramNames {
		if strings.HasPrefix(p, "*") {
			variadicName = strings.TrimPrefix(p, "*")
			variadicIdx = i
		} else {
			namedParams = append(namedParams, p)
		}
	}
	numNamed := len(namedParams)
	numParams := len(paramNames)

	return func(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		if variadicName == "" {
			// ── Non-variadic path (unchanged) ──────────────────────
			return callNonVariadic(receiverName, providerVal, method, methodType, snakeName, paramNames, numParams, receiver, args, kwargs)
		}

		// ── Variadic path ──────────────────────────────────────────
		// 1. Unpack named (non-variadic) params.
		namedVals := make([]starlark.Value, numNamed)
		pairs := make([]any, 0, numNamed*2)
		for i, name := range namedParams {
			pairs = append(pairs, name, &namedVals[i])
		}

		// Extract the variadic keyword if present, so UnpackArgs doesn't
		// reject it as unexpected.
		var kwVariadic starlark.Value
		var filteredKwargs []starlark.Tuple
		for _, kv := range kwargs {
			key, _ := starlark.AsString(kv[0])
			if key == variadicName {
				kwVariadic = kv[1]
			} else {
				filteredKwargs = append(filteredKwargs, kv)
			}
		}

		// Positional args beyond the named params are variadic candidates.
		namedArgs := args
		var positionalVariadic starlark.Tuple
		if len(args) > numNamed {
			namedArgs = args[:numNamed]
			positionalVariadic = args[numNamed:]
		}

		if err := starlark.UnpackArgs(snakeName, namedArgs, filteredKwargs, pairs...); err != nil {
			return nil, err
		}

		// 2. Resolve the variadic value.
		if len(positionalVariadic) > 0 && kwVariadic != nil {
			return nil, fmt.Errorf("%s: %s() got both positional and keyword args for variadic param %q",
				receiverName, snakeName, variadicName)
		}

		var variadicList *starlark.List
		if len(positionalVariadic) > 0 {
			elems := make([]starlark.Value, len(positionalVariadic))
			copy(elems, positionalVariadic)
			variadicList = starlark.NewList(elems)
		} else if kwVariadic != nil {
			list, ok := kwVariadic.(*starlark.List)
			if !ok {
				return nil, fmt.Errorf("%s.%s: keyword %s must be a list, got %s",
					receiverName, snakeName, variadicName, kwVariadic.Type())
			}
			variadicList = list
		}

		// 3. Build Go args: named params + variadic slice.
		goArgs := make([]reflect.Value, numParams+1)
		goArgs[0] = providerVal

		for i, sv := range namedVals {
			paramType := methodType.In(i + 1)
			if sv == nil {
				goArgs[i+1] = reflect.Zero(paramType)
				continue
			}
			goVal := reflect.New(paramType).Elem()
			if err := unmarshalValue(sv, goVal); err != nil {
				name := strings.TrimSuffix(namedParams[i], "?")
				return nil, fmt.Errorf("%s.%s: param %s: %w", receiverName, snakeName, name, err)
			}
			goArgs[i+1] = goVal
		}

		// Variadic param: unmarshal list → Go slice type.
		// For CallSlice, the variadic arg must be the slice itself.
		variadicGoType := methodType.In(variadicIdx + 1) // e.g. []string
		if variadicList == nil || variadicList.Len() == 0 {
			goArgs[variadicIdx+1] = reflect.Zero(variadicGoType)
		} else {
			goVal := reflect.New(variadicGoType).Elem()
			if err := unmarshalValue(variadicList, goVal); err != nil {
				return nil, fmt.Errorf("%s.%s: param %s: %w", receiverName, snakeName, variadicName, err)
			}
			goArgs[variadicIdx+1] = goVal
		}

		// 4. Call via CallSlice (variadic methods).
		results := method.Func.CallSlice(goArgs)

		// 5. Shadow Resource results in catalog.
		if receiver.catalog != nil && len(results) > 0 {
			lastIdx := len(results) - 1
			isErr := results[lastIdx].Type().Implements(errorType) && !results[lastIdx].IsNil()
			if !isErr && results[0].Type() != noResultType {
				originID := receiverName + "." + snakeName
				shadowResult(results[0].Interface(), receiver.catalog, originID)
			}
		}

		return classifyReturn(results)
	}
}

// callNonVariadic handles the common case: no variadic params.
func callNonVariadic(
	receiverName string,
	providerVal reflect.Value,
	method reflect.Method,
	methodType reflect.Type,
	snakeName string,
	paramNames []string,
	numParams int,
	receiver *ReflectedReceiver,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
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
