// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"fmt"
	"reflect"
)

// reflectedAction implements Action via reflection.
// Slot values (Go-native from FillSlot/unmarshalToAny) are coerced to
// method parameter types, the method is called, and return values are
// classified into (Result, UndoState, error).
type reflectedAction struct {
	name       string
	provider   reflect.Value
	method     reflect.Method
	paramNames []string
}

func (a *reflectedAction) Name() string { return a.name }

func (a *reflectedAction) Do(ctx *Context, slots map[string]any) (Result, UndoState, error) {
	methodType := a.method.Type
	goArgs := make([]reflect.Value, len(a.paramNames)+1)
	goArgs[0] = a.provider

	for i, name := range a.paramNames {
		sv := slots[name]
		paramType := methodType.In(i + 1) // skip receiver
		val, err := coerceSlotValue(sv, paramType)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: param %s: %w", a.name, name, err)
		}
		goArgs[i+1] = val
	}

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Writer, "[dry-run] %s", a.name)
		for _, name := range a.paramNames {
			_, _ = fmt.Fprintf(ctx.Writer, " %v", slots[name])
		}
		_, _ = fmt.Fprintln(ctx.Writer)
		return nil, nil, nil
	}

	results := a.method.Func.Call(goArgs)
	result, undoState, err := classifyActionReturn(results)

	if err == nil {
		result = shadowResult(result, ctx.Catalog, ctx.NodeID)
	}

	return result, undoState, err
}

// reflectedCompensableAction extends reflectedAction with Undo.
type reflectedCompensableAction struct {
	reflectedAction
	compensate reflect.Method
}

func (a *reflectedCompensableAction) Undo(_ *Context, state UndoState) error {
	if state == nil {
		return nil
	}
	goArgs := []reflect.Value{
		a.provider,
		reflect.ValueOf(state),
	}
	results := a.compensate.Func.Call(goArgs)
	if len(results) > 0 && !results[len(results)-1].IsNil() {
		return results[len(results)-1].Interface().(error)
	}
	return nil
}

// coerceSlotValue converts a slot Go value to the target parameter type.
//
// Coercion order:
//  1. nil → zero value
//  2. Directly assignable → assign
//  3. Convertible → convert (handles int→os.FileMode, int→int64, etc.)
//  4. Map → struct (handles map[string]any → AnalysisConfig, etc.)
//  5. Constructor registry → construct (handles string→Resource, etc.)
//  6. Error
func coerceSlotValue(slotValue any, targetType reflect.Type) (reflect.Value, error) {
	if slotValue == nil {
		return reflect.Zero(targetType), nil
	}

	sv := reflect.ValueOf(slotValue)

	if sv.Type().AssignableTo(targetType) {
		return sv, nil
	}

	if sv.Type().ConvertibleTo(targetType) {
		return sv.Convert(targetType), nil
	}

	// map[string]any → struct: iterate struct fields, coerce each value from map.
	if sv.Type().Kind() == reflect.Map && sv.Type().Key().Kind() == reflect.String &&
		targetType.Kind() == reflect.Struct {
		return coerceMapToStruct(sv, targetType)
	}

	// []T → []U: coerce each element individually (e.g. []string → []Resource).
	if sv.Type().Kind() == reflect.Slice && targetType.Kind() == reflect.Slice {
		return coerceSlice(sv, targetType)
	}

	if ctor, ok := constructorRegistry.Load(targetType); ok {
		result, err := ctor.(func(any) (any, error))(slotValue)
		if err != nil {
			return reflect.Value{}, err
		}
		return reflect.ValueOf(result), nil
	}

	return reflect.Value{}, fmt.Errorf("cannot coerce %T to %s", slotValue, targetType)
}

// coerceMapToStruct converts a map[string]any to a target struct type.
// Keys are matched to exported struct fields by snake_case name (or starlark tag).
// Each value is recursively coerced to the field's type.
func coerceMapToStruct(mapVal reflect.Value, targetType reflect.Type) (reflect.Value, error) {
	result := reflect.New(targetType).Elem()
	info := getTypeInfo(targetType)

	iter := mapVal.MapRange()
	for iter.Next() {
		key := iter.Key().String()
		fi, ok := info.byName[key]
		if !ok {
			continue // skip unknown keys
		}
		fieldVal, err := coerceSlotValue(iter.Value().Interface(), fi.goType)
		if err != nil {
			return reflect.Value{}, fmt.Errorf("field %s: %w", key, err)
		}
		result.Field(fi.index).Set(fieldVal)
	}
	return result, nil
}

// coerceSlice converts a source slice to a target slice type by coercing each
// element individually (e.g. []string → []pkg.Resource via constructors).
func coerceSlice(srcSlice reflect.Value, targetType reflect.Type) (reflect.Value, error) {
	elemType := targetType.Elem()
	n := srcSlice.Len()
	result := reflect.MakeSlice(targetType, n, n)
	for i := range n {
		coerced, err := coerceSlotValue(srcSlice.Index(i).Interface(), elemType)
		if err != nil {
			return reflect.Value{}, fmt.Errorf("element %d: %w", i, err)
		}
		result.Index(i).Set(coerced)
	}
	return result, nil
}

// classifyActionReturn interprets Go method return values for the action layer.
// Unlike classifyReturn (which marshals to Starlark), this keeps Go-native values.
//
// Arity (after stripping trailing error):
//
//	0 → (nil, nil, error)            — error-only method
//	1 → (result, nil, error)         — Action
//	2 → (result, undoState, error)   — CompensableAction
//
// NoResult at position 0 yields nil Result.
func classifyActionReturn(results []reflect.Value) (Result, UndoState, error) {
	n := len(results)
	if n == 0 {
		return nil, nil, nil
	}

	var err error
	if results[n-1].Type().Implements(errorType) {
		if !results[n-1].IsNil() {
			err = results[n-1].Interface().(error)
		}
		n--
	}

	var result Result
	var undoState UndoState

	if n > 0 {
		v := results[0].Interface()
		if _, isNoResult := v.(NoResult); !isNoResult {
			result = v
		}
	}
	if n > 1 {
		undoState = results[1].Interface()
	}

	return result, undoState, err
}

// resourceType is cached for result-type classification.
var resourceType = reflect.TypeOf((*Resource)(nil)).Elem()

// shadowResult shadows Resource results in the catalog without changing the
// result type. Provider methods return resources by value (e.g., file.Resource).
// The Resource interface requires pointer receivers (resourceBase), so a
// temporary pointer is created for the Shadow call. The catalog stamps
// id/originID on the underlying ResourceBase; the stamped value is returned.
func shadowResult(result Result, catalog *ResourceCatalog, originID string) Result {
	if result == nil || catalog == nil {
		return result
	}

	rv := reflect.ValueOf(result)
	t := rv.Type()

	// Already satisfies Resource (pointer type or interface).
	if t.Implements(resourceType) {
		catalog.Shadow(result.(Resource), originID)
		return result
	}

	// Struct whose pointer satisfies Resource (common case: file.Resource value).
	// Create a temporary pointer for the Shadow call, then return the
	// stamped value (same type as original).
	if t.Kind() == reflect.Struct && reflect.PointerTo(t).Implements(resourceType) {
		ptr := reflect.New(t)
		ptr.Elem().Set(rv)
		catalog.Shadow(ptr.Interface().(Resource), originID)
		return ptr.Elem().Interface()
	}

	// Slice where elements satisfy Resource.
	if t.Kind() == reflect.Slice {
		et := t.Elem()
		directImpl := et.Implements(resourceType)
		ptrImpl := et.Kind() == reflect.Struct && reflect.PointerTo(et).Implements(resourceType)
		if directImpl || ptrImpl {
			for i := 0; i < rv.Len(); i++ {
				if directImpl {
					catalog.Shadow(rv.Index(i).Interface().(Resource), originID)
				} else {
					ptr := reflect.New(et)
					ptr.Elem().Set(rv.Index(i))
					catalog.Shadow(ptr.Interface().(Resource), originID)
				}
			}
		}
	}

	return result
}

// RegisterReflectedActions registers all action methods on a provider.
// Methods must return error as their last value to qualify as actions.
// Methods without error returns (e.g., Exists, IsDir) are skipped —
// they are immediate-only, not graph actions.
//
// Arity determines action type:
//
//   - 1 return  (error)             → Action (error-only, no result)
//   - 2 returns (result, error)     → Action
//   - 3 returns (result, undo, error) → CompensableAction
//
// Every CompensableAction (3 returns) must have a Compensate<GoName>
// companion method. Missing pairs panic at registration time.
func RegisterReflectedActions(reg *ActionRegistry, name string, provider any, params MethodParams) {
	pv := reflect.ValueOf(provider)
	pt := pv.Type()

	for goName, paramNames := range params {
		method, ok := pt.MethodByName(goName)
		if !ok {
			continue
		}

		mt := method.Type
		if mt.NumOut() == 0 || !mt.Out(mt.NumOut()-1).Implements(errorType) {
			continue
		}

		snakeName := camelToSnake(goName)
		actionName := name + "." + snakeName

		base := reflectedAction{
			name:       actionName,
			provider:   pv,
			method:     method,
			paramNames: paramNames,
		}

		// 3 returns (result, undo, error) → CompensableAction.
		// The companion Compensate<GoName> method is mandatory.
		compensateName := "Compensate" + goName
		numNonError := mt.NumOut() - 1 // subtract trailing error
		if numNonError >= 2 {
			cm, ok := pt.MethodByName(compensateName)
			if !ok {
				panic(fmt.Sprintf(
					"%s: method %s returns (result, undo, error) but %s.%s is missing",
					actionName, goName, pt.Elem().Name(), compensateName,
				))
			}
			reg.Register(&reflectedCompensableAction{
				reflectedAction: base,
				compensate:      cm,
			})
		} else if cm, ok := pt.MethodByName(compensateName); ok {
			// 1-2 returns but has a Compensate companion — allow it.
			reg.Register(&reflectedCompensableAction{
				reflectedAction: base,
				compensate:      cm,
			})
		} else {
			reg.Register(&base)
		}
	}
}

// InitActionProvider injects the execution Context into the provider backing a reflected action.
// For actions that are not reflection-based or whose provider does not embed ProviderBase, this is a no-op.
func InitActionProvider(action Action, ctx Context) {
	var pv reflect.Value

	switch a := action.(type) {
	case *reflectedAction:
		pv = a.provider
	case *reflectedCompensableAction:
		pv = a.provider
	default:
		return
	}

	if pv.Kind() == reflect.Ptr && !pv.IsNil() {
		InitProvider(pv.Interface(), ctx)
	}
}
