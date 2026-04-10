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

// executingReceiver is the shared base for bind types that dispatch starlark calls to [op.Method.Do].
//
// It implements [starlark.Value] and [starlark.HasAttrs]. Concrete types embed it to inherit dispatch, attribute
// resolution, and the starlark value protocol. [Provider] embeds it directly. [Resource] embeds it and adds field
// access and comparability. [Value] embeds it for arbitrary Go types.
type executingReceiver struct {
	receiverType op.ReceiverType
	receiver     any                   // method.Do target — the Go object
	methods      map[string]*op.Method // snake_name → *Method
	attrNames    []string              // sorted
}

// newExecutingReceiver constructs an [executingReceiver] from a receiver type and instance.
//
// Parameters:
//   - rt: the receiver type descriptor.
//   - instance: the Go provider or resource.
//
// Returns:
//   - executingReceiver: the initialized base.
func newExecutingReceiver(rt op.ReceiverType, receiver any) executingReceiver {

	methods := make(map[string]*op.Method)
	names := make([]string, 0)
	for method := range rt.Methods() {
		snake := camelToSnake(method.Name())
		methods[snake] = method
		names = append(names, snake)
	}
	sort.Strings(names)

	return executingReceiver{
		receiverType: rt,
		receiver:     receiver,
		methods:      methods,
		attrNames:    names,
	}
}

// region EXPORTED METHODS

// region State management

// String implements starlark.Value.
func (r *executingReceiver) String() string { return r.receiverType.ReceiverName() }

// Type implements starlark.Value.
func (r *executingReceiver) Type() string { return r.receiverType.ReceiverName() }

// Freeze implements starlark.Value.
func (r *executingReceiver) Freeze() {}

// Truth implements starlark.Value.
func (r *executingReceiver) Truth() starlark.Bool { return true }

// Hash implements starlark.Value.
func (r *executingReceiver) Hash() (uint32, error) {
	return 0, fmt.Errorf("unhashable type: %s", r.receiverType.ReceiverName())
}

// endregion

// region Behaviors

// Attr implements starlark.HasAttrs.
//
// Parameters:
//   - name: the snake_case attribute name to look up.
//
// Returns:
//   - starlark.Value: a builtin bound to this receiver's dispatch method.
//   - error: non-nil if the attribute does not exist.
func (r *executingReceiver) Attr(name string) (starlark.Value, error) {

	if _, ok := r.methods[name]; ok {
		actionName := r.receiverType.ReceiverName() + "." + name
		return starlark.NewBuiltin(actionName, r.dispatch), nil
	}

	if resolver, ok := r.receiver.(op.AttributeResolver); ok {
		if resolved := resolver.ResolveAttr(name); resolved != nil {
			return marshalReflect(reflect.ValueOf(resolved))
		}
	}

	return nil, NoSuchAttrError(r.receiverType.ReceiverName(), name)
}

// AttrNames implements starlark.HasAttrs.
//
// Returns:
//   - []string: sorted list of available method names.
func (r *executingReceiver) AttrNames() []string { return r.attrNames }

// endregion

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// dispatch dispatches a starlark builtin invocation to the underlying Go method.
//
// Parameters:
//   - thread: the starlark thread.
//   - builtin: the starlark builtin that triggered the dispatch.
//   - args: positional starlark arguments.
//   - kwargs: keyword starlark arguments.
//
// Returns:
//   - starlark.Value: the marshaled return value.
//   - error: non-nil if the dispatch fails.
func (r *executingReceiver) dispatch(thread *starlark.Thread, builtin *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {

	actionName := builtin.Name()
	name := actionName[strings.LastIndex(actionName, ".")+1:]
	method := r.methods[name]
	params := method.Parameters()

	// Classify parameters.

	var namedParams []string
	var variadicName string
	var variadicIdx int
	var kwargsName string
	var kwargsIdx int

	for i, p := range params {
		switch {
		case strings.HasPrefix(p.Name, "**"):
			kwargsName = strings.TrimPrefix(p.Name, "**")
			kwargsIdx = i
		case strings.HasPrefix(p.Name, "*"):
			variadicName = strings.TrimPrefix(p.Name, "*")
			variadicIdx = i
		default:
			namedParams = append(namedParams, p.Name)
		}
	}

	if variadicName == "" && kwargsName == "" {
		return r.dispatchSimple(method, actionName, namedParams, args, kwargs, thread)
	}

	return r.dispatchVariadic(method, actionName, params, namedParams, variadicName, variadicIdx, kwargsName, kwargsIdx, args, kwargs, thread)
}

// dispatchSimple handles the common case: no variadic or kwargs parameters.
func (r *executingReceiver) dispatchSimple(
	method *op.Method,
	actionName string,
	paramNames []string,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
	thread *starlark.Thread,
) (starlark.Value, error) {

	params := method.Parameters()
	numParams := len(paramNames)

	// Unpack starlark args into value slots.

	vals := make([]starlark.Value, numParams)
	pairs := make([]any, 0, numParams*2)
	for i, name := range paramNames {
		pairs = append(pairs, name, &vals[i])
	}
	if err := starlark.UnpackArgs(actionName, args, kwargs, pairs...); err != nil {
		return nil, err
	}

	// Convert starlark values to Go values.

	goArgs := make([]any, numParams)
	for i, sv := range vals {
		if sv == nil {
			continue
		}
		if s, ok := starlark.AsString(sv); ok {
			if coerced, err := r.coerceResource(params[i].Type, s); err != nil {
				name := strings.TrimSuffix(paramNames[i], "?")
				return nil, fmt.Errorf("%s: param %s: %w", actionName, name, err)
			} else if coerced != nil {
				goArgs[i] = coerced
				continue
			}
		}
		goVal := reflect.New(params[i].Type).Elem()
		if err := unmarshalValue(sv, goVal); err != nil {
			name := strings.TrimSuffix(paramNames[i], "?")
			return nil, fmt.Errorf("%s: param %s: %w", actionName, name, err)
		}
		goArgs[i] = goVal.Interface()
	}

	// Call the method.

	result, _, err := method.Do(r.receiver, goArgs)
	if err != nil {
		return nil, err
	}

	// Marshal the result.

	if !result.IsValid() {
		return starlark.None, nil
	}
	return marshalReflect(result)
}

// dispatchVariadic handles the extended case: variadic (*) and/or kwargs (**) parameters.
func (r *executingReceiver) dispatchVariadic(
	method *op.Method,
	actionName string,
	params []op.Parameter,
	namedParams []string,
	variadicName string,
	variadicIdx int,
	kwargsName string,
	kwargsIdx int,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
	thread *starlark.Thread,
) (starlark.Value, error) {

	numNamed := len(namedParams)
	numParams := len(params)

	// Build set of known kwarg names for filtering.

	knownKwargs := make(map[string]bool, numNamed+1)
	for _, name := range namedParams {
		knownKwargs[strings.TrimSuffix(name, "?")] = true
	}
	if variadicName != "" {
		knownKwargs[variadicName] = true
	}

	// Unpack named params.

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

	if kwargsName == "" && len(extraKwargs) > 0 {
		key, _ := starlark.AsString(extraKwargs[0][0])
		return nil, fmt.Errorf("%s: got unexpected keyword argument %q", actionName, key)
	}

	namedArgs := args
	var positionalVariadic starlark.Tuple
	if len(args) > numNamed {
		namedArgs = args[:numNamed]
		positionalVariadic = args[numNamed:]
	}

	if err := starlark.UnpackArgs(actionName, namedArgs, filteredKwargs, pairs...); err != nil {
		return nil, err
	}

	// Build Go args.

	goArgs := make([]any, numParams)

	for i, sv := range namedVals {
		if sv == nil {
			continue
		}
		goVal := reflect.New(params[i].Type).Elem()
		if err := unmarshalValue(sv, goVal); err != nil {
			name := strings.TrimSuffix(namedParams[i], "?")
			return nil, fmt.Errorf("%s: param %s: %w", actionName, name, err)
		}
		goArgs[i] = goVal.Interface()
	}

	// Resolve the variadic value.

	if variadicName != "" {
		if len(positionalVariadic) > 0 && kwVariadic != nil {
			return nil, fmt.Errorf("%s: got both positional and keyword args for variadic param %q", actionName, variadicName)
		}

		var variadicList *starlark.List
		if len(positionalVariadic) > 0 {
			elems := make([]starlark.Value, len(positionalVariadic))
			copy(elems, positionalVariadic)
			variadicList = starlark.NewList(elems)
		} else if kwVariadic != nil {
			list, ok := kwVariadic.(*starlark.List)
			if !ok {
				return nil, fmt.Errorf("%s: keyword %s must be a list, got %s", actionName, variadicName, kwVariadic.Type())
			}
			variadicList = list
		}

		variadicGoType := params[variadicIdx].Type
		if variadicList == nil || variadicList.Len() == 0 {
			goArgs[variadicIdx] = nil
		} else {
			goVal := reflect.New(variadicGoType).Elem()
			if err := unmarshalValue(variadicList, goVal); err != nil {
				return nil, fmt.Errorf("%s: param %s: %w", actionName, variadicName, err)
			}
			goArgs[variadicIdx] = goVal.Interface()
		}
	}

	// Build **kwargs map from extra keyword args.

	if kwargsName != "" {
		kwargsMap := make(map[string]any, len(extraKwargs))
		for _, kv := range extraKwargs {
			key, _ := starlark.AsString(kv[0])
			val, err := unmarshalToAny(kv[1])
			if err != nil {
				return nil, fmt.Errorf("%s: kwarg %s: %w", actionName, key, err)
			}
			kwargsMap[key] = val
		}
		goArgs[kwargsIdx] = kwargsMap
	}

	// Call the method.

	result, _, err := method.Do(r.receiver, goArgs)
	if err != nil {
		return nil, err
	}

	// Marshal the result.

	if !result.IsValid() {
		return starlark.None, nil
	}
	return marshalReflect(result)
}

// coerceResource converts a starlark string to a Go resource instance when the target type is a registered resource.
//
// Returns (nil, nil) when the target type is not a resource — callers should fall through to normal unmarshaling.
func (r *executingReceiver) coerceResource(paramType reflect.Type, s string) (any, error) {

	elemType := paramType
	isPtr := false
	if elemType.Kind() == reflect.Ptr {
		elemType = elemType.Elem()
		isPtr = true
	}

	// Get the registry from the provider's execution context. The package-level
	// registry global is only set when the starlark runtime starts — it is not
	// available in tests that wire providers directly via bind.NewProvider.
	provider, ok := r.receiver.(op.Provider)
	if !ok {
		return nil, nil
	}
	ctx := provider.ExecutionContext()
	if ctx == nil || ctx.Registry == nil {
		return nil, nil
	}

	rt, ok := ctx.Registry.TypeByReflection(elemType)
	if !ok {
		return nil, nil
	}
	rrt, ok := rt.(op.ResourceReceiverType)
	if !ok {
		return nil, nil
	}

	resource, err := rrt.Construct()(ctx, s)
	if err != nil {
		return nil, err
	}
	if !isPtr {
		return reflect.ValueOf(resource).Elem().Interface(), nil
	}
	return resource, nil
}

// endregion

// endregion

// NoSuchAttrError returns an error for an unknown attribute.
//
// Parameters:
//   - receiver: the receiver name.
//   - attr: the attribute name.
//
// Returns:
//   - error: the formatted error.
func NoSuchAttrError(receiver, attr string) error {
	return fmt.Errorf("%s has no .%s attribute", receiver, attr)
}
