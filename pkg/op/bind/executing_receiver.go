// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package bind

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"go.starlark.net/starlark"
)

// MethodParams maps Go method names (CamelCase) to Starlark parameter
// name lists. Optional params use the "name?" suffix.
type MethodParams map[string][]string

// ExecutingReceiver wraps a Go struct for immediate-mode Starlark use.
//
// Exported methods become Starlark builtins; method dispatch, argument unpacking, and return value marshaling are
// handled by reflection. Optional catalog and stack fields enable resource management integration. When set, Resource
// results are shadowed in the catalog after successful dispatch.
type ExecutingReceiver struct {
	receiver
	providerValue reflect.Value
	methods       map[string]*methodBridge
	attrList      []string
	catalog       *op.ResourceCatalog // optional; set via SetCatalog
	stack         *op.RecoveryStack   // optional; set via SetStack
}

// methodBridge pairs a snake_case method name with its Starlark bridge function.
type methodBridge struct {
	name     string
	bridge   builtinFunc
	property bool // true for 0-param methods with primitive returns — called eagerly in Attr
}

// WrapProviderInExecutingReceiver wraps a Go struct for immediate-mode Starlark use.
// Params are looked up from the receiver params registry (populated by
// RegisterActions during InitAll). Only methods listed in the
// registered params are exposed. Compensate* methods are excluded.
//
// Parameters:
//   - name: the receiver name exposed to Starlark.
//   - provider: the Go struct to wrap (must be a pointer).
//
// Returns:
//   - *ExecutingReceiver: the wrapped receiver with method bridges.
func WrapProviderInExecutingReceiver(factory op.ReceiverFactory, provider any) *ExecutingReceiver {

	name := factory.ReceiverName()

	entry, ok := lookupReceiverParams(reflect.TypeOf(provider))
	if !ok {
		// Auto-register params from the factory interface. This handles
		// providers whose Register method does not call RegisterActions
		// (e.g., immediate-only generated receivers).
		registerReceiverParamsReflect(factory, factory.MethodParams())
		entry, ok = lookupReceiverParams(reflect.TypeOf(provider))
		if !ok {
			panic(fmt.Sprintf("WrapProviderInExecutingReceiver(%s): no params registered — was RegisterActions called?", name))
		}
	}

	rv := reflect.ValueOf(provider)
	r := &ExecutingReceiver{
		receiver:      newReceiver(name),
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

		// Property detection: 0 user params and the first return type is a
		// primitive (not a struct/pointer-to-struct). Properties are called
		// eagerly in Attr instead of returning a callable.
		isProperty := len(paramNames) == 0 && isPrimitiveReturn(m.Type)

		r.methods[snakeName] = &methodBridge{
			name:     snakeName,
			bridge:   bridge,
			property: isProperty,
		}
	}

	r.attrList = make([]string, 0, len(r.methods))
	for name := range r.methods {
		r.attrList = append(r.attrList, name)
	}
	sort.Strings(r.attrList)

	return r
}

// region EXPORTED METHODS

// region State management

// SetCatalog sets the resource catalog for immediate-mode dispatch.
// When set, Resource results are shadowed in the catalog after each
// successful method call.
//
// Parameters:
//   - c: the resource catalog to use.
func (r *ExecutingReceiver) SetCatalog(c *op.ResourceCatalog) { r.catalog = c }

// SetContext injects the execution Context into the receiver's underlying provider.
// This allows immediate receivers to access Context-scoped services like RecoverySite.
//
// Parameters:
//   - ctx: the execution context to inject.
func (r *ExecutingReceiver) SetContext(ctx op.Context) {

	if r.providerValue.Kind() == reflect.Ptr && !r.providerValue.IsNil() {
		type contextSetter interface{ SetContext(op.Context) }
		if cs, ok := r.providerValue.Interface().(contextSetter); ok {
			cs.SetContext(ctx)
		}
	}
}

// SetStack sets the recovery stack for immediate-mode dispatch.
//
// Parameters:
//   - s: the recovery stack to use.
func (r *ExecutingReceiver) SetStack(s *op.RecoveryStack) { r.stack = s }

// endregion

// region Behaviors

// Actions

// Attr implements starlark.HasAttrs.
//
// Parameters:
//   - name: the attribute name to look up.
//
// Returns:
//   - starlark.Value: the method builtin.
//   - error: non-nil if the attribute does not exist.
func (r *ExecutingReceiver) Attr(name string) (starlark.Value, error) {

	m, ok := r.methods[name]
	if ok {
		if m.property {
			// Properties are called eagerly — invoke the bridge with no args
			// and return the result directly.
			return m.bridge(nil, nil, nil, nil)
		}
		return starlark.NewBuiltin(r.receiver.name+"."+name, m.bridge), nil
	}

	// Dynamic attributes: if the provider implements AttributeResolver,
	// delegate unknown attribute lookups to it. The returned value is
	// sent through the marshaler.
	if ar, ok := r.providerValue.Interface().(op.AttributeResolver); ok {
		if val := ar.ResolveAttr(name); val != nil {
			return Marshal(val)
		}
	}

	return nil, NoSuchAttrError(r.receiver.name, name)
}

// AttrNames implements starlark.HasAttrs.
//
// Returns:
//   - []string: sorted list of available method names.
func (r *ExecutingReceiver) AttrNames() []string {

	return r.attrList
}

// endregion

// endregion

// region UNEXPORTED METHODS

// isPrimitiveReturn reports whether a method's first non-error return type is a primitive Go type (not a struct or
// pointer-to-struct). Used for property detection.
//
// Parameters:
//   - mt: the method's reflect.Type.
//
// Returns:
//   - bool: true if the first return type is a primitive.
func isPrimitiveReturn(mt reflect.Type) bool {

	if mt.NumOut() == 0 {
		return false
	}
	rt := mt.Out(0)
	// Skip receiver (index 0 is the first actual return for Type.Out)
	if rt.Implements(errorType) && mt.NumOut() == 1 {
		return false // only returns error — not a property
	}
	for rt.Kind() == reflect.Pointer {
		rt = rt.Elem()
	}
	return rt.Kind() != reflect.Struct
}

// buildMethodBridge creates a builtinFunc that bridges a Go method to Starlark.
// The receiver parameter provides optional catalog/stack integration —
// Resource results are shadowed in the catalog after successful dispatch.
//
// Variadic parameters are marked with a "*" prefix in paramNames (e.g. "*parts").
// Remaining positional Starlark args are collected into the variadic slot.
// The keyword form (parts=["a","b"]) is also accepted as a fallback.
// Supplying both positional and keyword args for a variadic param is an error.
//
// Parameters:
//   - receiverName: the qualified receiver name for error messages.
//   - providerVal: the reflected provider value.
//   - method: the Go method to bridge.
//   - snakeName: the snake_case method name.
//   - paramNames: Starlark parameter names (with "?" for optional, "*" for variadic).
//   - receiver: the ExecutingReceiver for catalog/stack integration.
//
// Returns:
//   - builtinFunc: the Starlark-callable bridge function.
func buildMethodBridge(
	receiverName string,
	providerVal reflect.Value,
	method reflect.Method,
	snakeName string,
	paramNames []string,
	receiver *ExecutingReceiver,
) builtinFunc {

	methodType := method.Type

	// Separate named params from variadic (*) and kwargs (**) params.
	var variadicName string // snake_case name without "*"
	var variadicIdx int     // index in paramNames
	var kwargsName string   // snake_case name without "**"
	var kwargsIdx int       // index in paramNames
	namedParams := make([]string, 0, len(paramNames))
	for i, p := range paramNames {
		switch {
		case strings.HasPrefix(p, "**"):
			kwargsName = strings.TrimPrefix(p, "**")
			kwargsIdx = i
		case strings.HasPrefix(p, "*"):
			variadicName = strings.TrimPrefix(p, "*")
			variadicIdx = i
		default:
			namedParams = append(namedParams, p)
		}
	}
	numNamed := len(namedParams)
	numParams := len(paramNames)

	// Build set of known kwarg names for filtering.
	knownKwargs := make(map[string]bool, numNamed+1)
	for _, name := range namedParams {
		knownKwargs[strings.TrimSuffix(name, "?")] = true
	}
	if variadicName != "" {
		knownKwargs[variadicName] = true
	}

	return func(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		if variadicName == "" && kwargsName == "" {
			// --- Simple path: no variadic, no kwargs ---
			return callNonVariadic(receiverName, providerVal, method, methodType, snakeName, paramNames, numParams, receiver, thread, args, kwargs)
		}

		// --- Extended path: variadic and/or kwargs ---
		// 1. Unpack named params.
		namedVals := make([]starlark.Value, numNamed)
		pairs := make([]any, 0, numNamed*2)
		for i, name := range namedParams {
			pairs = append(pairs, name, &namedVals[i])
		}

		// Split kwargs: known → UnpackArgs, variadic → extracted, rest → **kwargs.
		var kwVariadic starlark.Value
		var filteredKwargs []starlark.Tuple
		var extraKwargs []starlark.Tuple
		for _, kv := range kwargs {
			key, _ := starlark.AsString(kv[0])
			switch {
			case key == variadicName:
				kwVariadic = kv[1]
			case knownKwargs[key]:
				filteredKwargs = append(filteredKwargs, kv)
			default:
				extraKwargs = append(extraKwargs, kv)
			}
		}

		// Reject unknown kwargs if no **kwargs param to collect them.
		if kwargsName == "" && len(extraKwargs) > 0 {
			key, _ := starlark.AsString(extraKwargs[0][0])
			return nil, fmt.Errorf("%s: %s() got unexpected keyword argument %q",
				receiverName, snakeName, key)
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

		// 2. Build Go args.
		goArgs := make([]reflect.Value, numParams+1)
		goArgs[0] = providerVal

		for i, sv := range namedVals {
			paramType := methodType.In(i + 1)
			if sv == nil {
				goArgs[i+1] = reflect.Zero(paramType)
				continue
			}

			if starFn, ok := sv.(starlark.Callable); ok && paramType.Kind() == reflect.Func {
				adapted, err := buildCallableFunc(starFn, thread, paramType)
				if err != nil {
					name := strings.TrimSuffix(namedParams[i], "?")
					return nil, fmt.Errorf("%s.%s: param %s: adapt callable: %w", receiverName, snakeName, name, err)
				}
				goArgs[i+1] = reflect.ValueOf(adapted)
				continue
			}

			goVal := reflect.New(paramType).Elem()
			if err := unmarshalValue(sv, goVal); err != nil {
				name := strings.TrimSuffix(namedParams[i], "?")
				return nil, fmt.Errorf("%s.%s: param %s: %w", receiverName, snakeName, name, err)
			}
			goArgs[i+1] = goVal
		}

		// 3. Resolve the variadic value (if present).
		if variadicName != "" {
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

			variadicGoType := methodType.In(variadicIdx + 1)
			if variadicList == nil || variadicList.Len() == 0 {
				goArgs[variadicIdx+1] = reflect.Zero(variadicGoType)
			} else {
				goVal := reflect.New(variadicGoType).Elem()
				if err := unmarshalValue(variadicList, goVal); err != nil {
					return nil, fmt.Errorf("%s.%s: param %s: %w", receiverName, snakeName, variadicName, err)
				}
				goArgs[variadicIdx+1] = goVal
			}
		}

		// 4. Build **kwargs map from extra keyword args.
		if kwargsName != "" {
			kwargsMap := make(map[string]any, len(extraKwargs))
			for _, kv := range extraKwargs {
				key, _ := starlark.AsString(kv[0])
				val, err := unmarshalToAny(kv[1])
				if err != nil {
					return nil, fmt.Errorf("%s.%s: kwarg %s: %w", receiverName, snakeName, key, err)
				}
				kwargsMap[key] = val
			}
			goArgs[kwargsIdx+1] = reflect.ValueOf(kwargsMap)
		}

		// 5. Call the Go method.
		// CallSlice only when Go-variadic (has *args but no **kwargs).
		// When **kwargs is present, *args maps to an explicit []T param.
		var results []reflect.Value
		if variadicName != "" && kwargsName == "" {
			results = method.Func.CallSlice(goArgs)
		} else {
			results = method.Func.Call(goArgs)
		}

		// 6. Shadow Resource results in catalog.
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
//
// Parameters:
//   - receiverName: the qualified receiver name for error messages.
//   - providerVal: the reflected provider value.
//   - method: the Go method to call.
//   - methodType: the method's reflect.ProviderType.
//   - snakeName: the snake_case method name.
//   - paramNames: Starlark parameter names.
//   - numParams: the number of parameters.
//   - receiver: the ExecutingReceiver for catalog/stack integration.
//   - thread: the Starlark thread.
//   - args: positional Starlark arguments.
//   - kwargs: keyword Starlark arguments.
//
// Returns:
//   - starlark.Value: the marshaled return value.
//   - error: non-nil if the call fails.
func callNonVariadic(
	receiverName string,
	providerVal reflect.Value,
	method reflect.Method,
	methodType reflect.Type,
	snakeName string,
	paramNames []string,
	numParams int,
	receiver *ExecutingReceiver,
	thread *starlark.Thread,
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

		// Callable params: adapt *starlark.Function → Go func type via
		// buildCallableFunc (full-signature marshal/unmarshal).
		if starFn, ok := sv.(starlark.Callable); ok && paramType.Kind() == reflect.Func {
			adapted, err := buildCallableFunc(starFn, thread, paramType)
			if err != nil {
				name := strings.TrimSuffix(paramNames[i], "?")
				return nil, fmt.Errorf("%s.%s: param %s: adapt callable: %w", receiverName, snakeName, name, err)
			}
			goArgs[i+1] = reflect.ValueOf(adapted)
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
//	(T)                        → Marshal(T)
//	(T, error)                 → Marshal(T) or error
//	(T, map[string]any, error) → Marshal(T), discard undo state, or error
//	(T, *RecoveryStack, error) → Marshal(T), discard stack, or error
//	(NoResult, ...)            → None (sentinel for no-output methods)
//
// Parameters:
//   - results: the Go method return values.
//
// Returns:
//   - starlark.Value: the marshaled first return value (or None).
//   - error: non-nil if the method returned an error.
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

// endregion
