// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"fmt"
	"reflect"
	"strings"
)

// actionBase holds shared fields for all reflected action types.
type actionBase struct {
	name       string
	provider   reflect.Value
	method     reflect.Method
	paramNames []string
}

func (a *actionBase) Name() string               { return a.name }
func (a *actionBase) getProvider() reflect.Value { return a.provider }

func (a *actionBase) Params() []ParamInfo {
	params := make([]ParamInfo, len(a.paramNames))
	for i, name := range a.paramNames {
		params[i] = ParamInfo{Name: name, Type: a.method.Type.In(i + 1)}
	}
	return params
}

// coerceArgs converts slot values to Go method parameter types.
func (a *actionBase) coerceArgs(slots map[string]any) ([]reflect.Value, error) {
	methodType := a.method.Type
	goArgs := make([]reflect.Value, len(a.paramNames)+1)
	goArgs[0] = a.provider

	for i, name := range a.paramNames {
		sv := slots[name]
		paramType := methodType.In(i + 1) // skip receiver
		val, err := coerceSlotValue(sv, paramType)
		if err != nil {
			return nil, fmt.Errorf("%s: param %s: %w", a.name, name, err)
		}
		goArgs[i+1] = val
	}
	return goArgs, nil
}

// dryRunLog writes dry-run output to the context writer.
func (a *actionBase) dryRunLog(ctx *Context, slots map[string]any) {
	_, _ = fmt.Fprintf(ctx.Writer, "[dry-run] %s", a.name)
	for _, name := range a.paramNames {
		_, _ = fmt.Fprintf(ctx.Writer, " %v", slots[name])
	}
	_, _ = fmt.Fprintln(ctx.Writer)
}

// reflectedPureAction implements Action via reflection.
type reflectedPureAction struct {
	actionBase
}

func (a *reflectedPureAction) Do(ctx *Context, slots map[string]any) (Result, Complement, error) {
	if err := initCallableSlots(ctx, slots, a.method.Type, a.paramNames); err != nil {
		panic(fmt.Sprintf("%s: %v", a.name, err))
	}

	goArgs, err := a.coerceArgs(slots)
	if err != nil {
		// Pure actions cannot fail. Coercion errors indicate a framework bug.
		panic(fmt.Sprintf("%s: %v", a.name, err))
	}

	if ctx.DryRun {
		a.dryRunLog(ctx, slots)
		return nil, nil, nil
	}

	results := a.method.Func.Call(goArgs)

	var result Result
	if len(results) > 0 {
		v := results[0].Interface()
		if _, isNoResult := v.(NoResult); !isNoResult {
			result = v
		}
	}

	result = shadowResult(result, ctx.Catalog, ctx.NodeID)
	return result, nil, nil
}

// reflectedFallibleAction implements FallibleAction via reflection.
type reflectedFallibleAction struct {
	actionBase
}

func (a *reflectedFallibleAction) Do(ctx *Context, slots map[string]any) (Result, Complement, error) {
	if err := initCallableSlots(ctx, slots, a.method.Type, a.paramNames); err != nil {
		return nil, nil, fmt.Errorf("%s: %w", a.name, err)
	}

	goArgs, err := a.coerceArgs(slots)
	if err != nil {
		return nil, nil, err
	}

	if ctx.DryRun {
		a.dryRunLog(ctx, slots)
		return nil, nil, nil
	}

	results := a.method.Func.Call(goArgs)
	result, doErr := classifyFallibleReturn(results)

	if doErr == nil {
		result = shadowResult(result, ctx.Catalog, ctx.NodeID)
	}

	return result, nil, doErr
}

// reflectedCompensableAction implements CompensableAction via reflection.
type reflectedCompensableAction struct {
	actionBase
	compensate reflect.Method
}

func (a *reflectedCompensableAction) Do(ctx *Context, slots map[string]any) (Result, Complement, error) {
	if err := initCallableSlots(ctx, slots, a.method.Type, a.paramNames); err != nil {
		return nil, nil, fmt.Errorf("%s: %w", a.name, err)
	}

	goArgs, err := a.coerceArgs(slots)
	if err != nil {
		return nil, nil, err
	}

	if ctx.DryRun {
		a.dryRunLog(ctx, slots)
		return nil, nil, nil
	}

	results := a.method.Func.Call(goArgs)
	result, undoState, doErr := classifyCompensableReturn(results)

	if doErr == nil {
		result = shadowResult(result, ctx.Catalog, ctx.NodeID)
	}

	return result, undoState, doErr
}

func (a *reflectedCompensableAction) Undo(_ *Context, state Complement) error {
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

// errorType and noResultType are cached for return-type classification.
var (
	errorType    = reflect.TypeOf((*error)(nil)).Elem()
	noResultType = reflect.TypeOf(NoResult{})
)

// validateSlotType checks whether a Go value can be coerced to a target type
// at plan time. This mirrors the coercion paths in coerceSlotValue without
// performing the actual coercion. Returns an error for obvious mismatches;
// returns nil when coercion is plausible.
func validateSlotType(goVal any, targetType reflect.Type) error {
	if goVal == nil {
		return nil
	}

	sv := reflect.ValueOf(goVal)

	// Direct assignment.
	if sv.Type().AssignableTo(targetType) {
		return nil
	}

	// Convert (int → os.FileMode, int → int64, etc.).
	if sv.Type().ConvertibleTo(targetType) {
		return nil
	}

	// map[string]any → struct coercion.
	if sv.Type().Kind() == reflect.Map && sv.Type().Key().Kind() == reflect.String &&
		targetType.Kind() == reflect.Struct {
		return nil
	}

	// []T → []U slice coercion.
	if sv.Type().Kind() == reflect.Slice && targetType.Kind() == reflect.Slice {
		return nil
	}

	// Constructor registry.
	if _, ok := constructorRegistry.Load(targetType); ok {
		return nil
	}

	// CallableResource → func type coercion.
	if isCallableResource(goVal) && isFuncType(targetType) {
		return nil
	}

	return fmt.Errorf("cannot coerce %T to %s", goVal, targetType)
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

// classifyFallibleReturn interprets Go method return values for FallibleAction.
//
// Arity (after stripping trailing error):
//
//	0 → (nil, error)
//	1 → (result, error)
func classifyFallibleReturn(results []reflect.Value) (Result, error) {
	n := len(results)
	if n == 0 {
		return nil, nil
	}

	var err error
	if results[n-1].Type().Implements(errorType) {
		if !results[n-1].IsNil() {
			err = results[n-1].Interface().(error)
		}
		n--
	}

	var result Result
	if n > 0 {
		v := results[0].Interface()
		if _, isNoResult := v.(NoResult); !isNoResult {
			result = v
		}
	}

	return result, err
}

// classifyCompensableReturn interprets Go method return values for CompensableAction.
//
// Arity (after stripping trailing error):
//
//	0 → (nil, nil, error)
//	1 → (result, nil, error)
//	2 → (result, undoState, error)
func classifyCompensableReturn(results []reflect.Value) (Result, Complement, error) {
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
	var undoState Complement

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
		_, _ = catalog.Shadow(result.(Resource), originID)
		return result
	}

	// Struct whose pointer satisfies Resource (common case: file.Resource value).
	// Create a temporary pointer for the Shadow call, then return the
	// stamped value (same type as original).
	if t.Kind() == reflect.Struct && reflect.PointerTo(t).Implements(resourceType) {
		ptr := reflect.New(t)
		ptr.Elem().Set(rv)
		_, _ = catalog.Shadow(ptr.Interface().(Resource), originID)
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
					_, _ = catalog.Shadow(rv.Index(i).Interface().(Resource), originID)
				} else {
					ptr := reflect.New(et)
					ptr.Elem().Set(rv.Index(i))
					_, _ = catalog.Shadow(ptr.Interface().(Resource), originID)
				}
			}
		}
	}

	return result
}

// RegisterReflectedActions registers all action methods on a provider.
// Method return signature determines action type:
//
//   - No error return: () or (T) → Action (pure)
//   - 1 return + error: (error) or (T, error) → FallibleAction
//   - 2+ returns + error: (T, undo, error) → CompensableAction
//
// Every CompensableAction (3 returns) must have a Compensate<GoName>
// companion method. Missing pairs panic at registration time.
func RegisterReflectedActions(reg *ActionRegistry, name string, provider any, params MethodParams) {
	registerReceiverParamsReflect(name, provider, params)

	pv := reflect.ValueOf(provider)
	pt := pv.Type()

	consumedCompensators := make(map[string]bool)

	for goName, paramNames := range params {
		method, ok := pt.MethodByName(goName)
		if !ok {
			continue
		}

		mt := method.Type
		snakeName := camelToSnake(goName)
		actionName := name + "." + snakeName

		// Strip '?' suffixes from param names. The '?' is a Starlark
		// UnpackArgs marker for optional parameters; it must not leak
		// into the action layer where slots are stored without the suffix.
		cleanedNames := make([]string, len(paramNames))
		for i, pn := range paramNames {
			cleanedNames[i] = strings.TrimSuffix(pn, "?")
		}

		base := actionBase{
			name:       actionName,
			provider:   pv,
			method:     method,
			paramNames: cleanedNames,
		}

		hasError := mt.NumOut() > 0 && mt.Out(mt.NumOut()-1).Implements(errorType)

		if !hasError {
			// Pure Action — no error return.
			reg.Register(&reflectedPureAction{actionBase: base})
			continue
		}

		// Has error return. Classify by non-error return count.
		compensateName := "Compensate" + goName
		numNonError := mt.NumOut() - 1 // subtract trailing error

		if numNonError >= 2 {
			// 3+ returns (result, undo, error) → CompensableAction.
			cm, ok := pt.MethodByName(compensateName)
			if !ok {
				panic(fmt.Sprintf(
					"%s: method %s returns (result, undo, error) but %s.%s is missing",
					actionName, goName, pt.Elem().Name(), compensateName,
				))
			}
			validateCompensateSignature(actionName, cm)
			consumedCompensators[compensateName] = true
			reg.Register(&reflectedCompensableAction{
				actionBase: base,
				compensate: cm,
			})
		} else if cm, ok := pt.MethodByName(compensateName); ok {
			// 1-2 returns but has a Compensate companion — allow it as CompensableAction.
			validateCompensateSignature(actionName, cm)
			consumedCompensators[compensateName] = true
			reg.Register(&reflectedCompensableAction{
				actionBase: base,
				compensate: cm,
			})
		} else {
			// FallibleAction — error return, no Compensate companion.
			reg.Register(&reflectedFallibleAction{actionBase: base})
		}
	}

	// Validate no orphaned Compensate methods exist on the provider.
	// A Compensate* method whose forward method is in params but was not
	// consumed indicates a pairing bug (e.g., forward has no error return).
	// Compensate methods whose forward is not in params are ignored — the
	// caller may be registering a subset of methods.
	for i := 0; i < pt.NumMethod(); i++ {
		m := pt.Method(i)
		if !strings.HasPrefix(m.Name, "Compensate") {
			continue
		}
		if consumedCompensators[m.Name] {
			continue
		}
		forwardName := strings.TrimPrefix(m.Name, "Compensate")
		if _, inParams := params[forwardName]; !inParams {
			continue
		}
		panic(fmt.Sprintf(
			"%s: %s.%s exists but forward method %s was not registered as an action",
			name, pt.Elem().Name(), m.Name, forwardName,
		))
	}
}

// validateCompensateSignature panics if a Compensate method has a malformed
// signature. The expected shape is func(receiver, undoState) error.
func validateCompensateSignature(actionName string, cm reflect.Method) {
	mt := cm.Type
	// NumIn includes receiver, so 2 = receiver + undoState.
	if mt.NumIn() != 2 {
		panic(fmt.Sprintf(
			"%s: %s must accept exactly 1 parameter (undo state), got %d",
			actionName, cm.Name, mt.NumIn()-1,
		))
	}
	if mt.NumOut() != 1 || !mt.Out(0).Implements(errorType) {
		panic(fmt.Sprintf(
			"%s: %s must return exactly (error), got %d return values",
			actionName, cm.Name, mt.NumOut(),
		))
	}
}

// InitActionProvider injects the execution Context into the provider backing a reflected action.
// For actions that are not reflection-based or whose provider does not embed ProviderBase, this is a no-op.
func InitActionProvider(action Action, ctx Context) {
	type hasProvider interface{ getProvider() reflect.Value }
	hp, ok := action.(hasProvider)
	if !ok {
		return
	}
	pv := hp.getProvider()
	if pv.Kind() == reflect.Ptr && !pv.IsNil() {
		InitProvider(pv.Interface(), ctx)
	}
}
