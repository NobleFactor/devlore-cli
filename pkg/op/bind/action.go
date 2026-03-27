// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package bind

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// RegisterActions registers all action methods on a provider.
//
// Method return signatures determine the action types:
//
//   - [op.Action] produces a result and is guaranteed not to fail. Return signature: () or (T).
//   - [op.FallibleAction] produces a result or an error. Return signature: (error) or (T, error).
//   - [op.CompensableAction] produces a result and its complement or an error. Return signature: (T, U, error).
//
// Every CompensableAction (3 returns) must have a Compensate<GoName> companion method. Missing pairs panic at
// registration time.
func RegisterActions(registry *op.ReceiverRegistry, factory op.ReceiverFactory) {

	allParams := factory.MethodParams()
	registerReceiverParamsReflect(factory, allParams)
	pt := reflect.PointerTo(factory.ProviderType())

	consumedCompensators := make(map[string]bool)

	for goName, paramNames := range allParams {

		method, ok := pt.MethodByName(goName)
		if !ok {
			continue
		}

		mt := method.Type
		snakeName := camelToSnake(goName)
		actionName := factory.ReceiverName() + "." + snakeName

		// Strip Starlark markers from param names.
		cleanedNames := cleanParamNames(paramNames)

		// Classify by return signature.
		hasError := mt.NumOut() > 0 && mt.Out(mt.NumOut()-1).Implements(errorType)

		kind := op.MethodPure
		if hasError {
			kind = op.MethodFallible
		}

		m := &op.Method{
			Factory:    factory,
			Reflect:    method,
			ActionName: actionName,
			ParamNames: cleanedNames,
			Kind:       kind,
		}

		// Check for compensator.
		compensateName := "Compensate" + goName
		if hasError {
			numNonError := mt.NumOut() - 1
			if numNonError >= 2 {
				cm, ok := pt.MethodByName(compensateName)
				if !ok {
					panic(fmt.Sprintf(
						"%s: method %s returns (result, undo, error) but %s.%s is missing",
						actionName, goName, pt.Elem().Name(), compensateName,
					))
				}
				validateCompensateSignature(actionName, cm)
				consumedCompensators[compensateName] = true
				m.Kind = op.MethodCompensable
				m.Compensate = cm
			} else if cm, ok := pt.MethodByName(compensateName); ok {
				validateCompensateSignature(actionName, cm)
				consumedCompensators[compensateName] = true
				m.Kind = op.MethodCompensable
				m.Compensate = cm
			}
		}

		// Wire up dispatch functions.
		m.DoFunc = buildDoFunc(m)
		if m.Kind == op.MethodCompensable {
			m.UndoFunc = buildUndoFunc(m)
		}

		registry.RegisterMethod(m)
	}

	// Validate no orphaned Compensate methods exist on the provider.
	//
	// A Compensate* method whose forward method is in params but was not consumed indicates a pairing bug (e.g.,
	// forward has no error return). Compensate methods whose forward is not in params are ignored — the caller may be
	// registering a subset of methods.

	for i := 0; i < pt.NumMethod(); i++ {
		m := pt.Method(i)
		if !strings.HasPrefix(m.Name, "Compensate") {
			continue
		}
		if consumedCompensators[m.Name] {
			continue
		}
		forwardName := strings.TrimPrefix(m.Name, "Compensate")
		if _, inParams := allParams[forwardName]; !inParams {
			continue
		}
		panic(fmt.Sprintf("%s: %s.%s exists but forward method %s was not registered as an action",
			factory.ReceiverName(),
			pt.Elem().Name(),
			m.Name,
			forwardName,
		))
	}
}

// errorType and noResultType are cached for return-type classification.
var (
	errorType    = reflect.TypeOf((*error)(nil)).Elem()
	noResultType = reflect.TypeOf(op.NoResult{})
)

// resourceType is cached for result-type classification.
var resourceType = reflect.TypeOf((*op.Resource)(nil)).Elem()

// cleanParamNames strips Starlark markers from param names.
func cleanParamNames(paramNames []string) []string {
	cleaned := make([]string, len(paramNames))
	for i, pn := range paramNames {
		cleaned[i] = strings.TrimPrefix(strings.TrimPrefix(strings.TrimSuffix(pn, "?"), "*"), "*")
	}
	return cleaned
}

// buildDoFunc creates the Do dispatch closure for a Method.
func buildDoFunc(m *op.Method) func(ctx *op.Context, slots map[string]any) (op.Result, op.Complement, error) {
	switch m.Kind {
	case op.MethodPure:
		return func(ctx *op.Context, slots map[string]any) (op.Result, op.Complement, error) {
			if err := initCallableSlots(ctx, slots, m.Reflect.Type, m.ParamNames); err != nil {
				panic(fmt.Sprintf("%s: %v", m.ActionName, err))
			}
			goArgs, err := coerceArgs(m, *ctx, slots)
			if err != nil {
				panic(fmt.Sprintf("%s: %v", m.ActionName, err))
			}
			if ctx.DryRun {
				dryRunLog(m, ctx, slots)
				return nil, nil, nil
			}
			results := invokeMethod(m, goArgs)
			var result op.Result
			if len(results) > 0 {
				v := results[0].Interface()
				if _, isNoResult := v.(op.NoResult); !isNoResult {
					result = v
				}
			}
			result = shadowResult(result, ctx.Catalog, ctx.NodeID)
			return result, nil, nil
		}

	case op.MethodFallible:
		return func(ctx *op.Context, slots map[string]any) (op.Result, op.Complement, error) {
			if err := initCallableSlots(ctx, slots, m.Reflect.Type, m.ParamNames); err != nil {
				return nil, nil, fmt.Errorf("%s: %w", m.ActionName, err)
			}
			goArgs, err := coerceArgs(m, *ctx, slots)
			if err != nil {
				return nil, nil, err
			}
			if ctx.DryRun {
				dryRunLog(m, ctx, slots)
				return nil, nil, nil
			}
			results := invokeMethod(m, goArgs)
			result, doErr := classifyFallibleReturn(results)
			if doErr == nil {
				result = shadowResult(result, ctx.Catalog, ctx.NodeID)
			}
			return result, nil, doErr
		}

	case op.MethodCompensable:
		return func(ctx *op.Context, slots map[string]any) (op.Result, op.Complement, error) {
			if err := initCallableSlots(ctx, slots, m.Reflect.Type, m.ParamNames); err != nil {
				return nil, nil, fmt.Errorf("%s: %w", m.ActionName, err)
			}
			goArgs, err := coerceArgs(m, *ctx, slots)
			if err != nil {
				return nil, nil, err
			}
			if ctx.DryRun {
				dryRunLog(m, ctx, slots)
				return nil, nil, nil
			}
			results := invokeMethod(m, goArgs)
			result, undoState, doErr := classifyCompensableReturn(results)
			if doErr == nil {
				result = shadowResult(result, ctx.Catalog, ctx.NodeID)
			}
			return result, undoState, doErr
		}

	default:
		panic(fmt.Sprintf("unknown method kind: %d", m.Kind))
	}
}

// buildUndoFunc creates the Undo dispatch closure for a compensable Method.
func buildUndoFunc(m *op.Method) func(ctx *op.Context, complement op.Complement) error {
	return func(ctx *op.Context, complement op.Complement) error {
		if complement == nil {
			return nil
		}
		goArgs := []reflect.Value{
			reflect.ValueOf(m.Factory.GetOrCreateProvider(*ctx)),
			reflect.ValueOf(complement),
		}
		results := m.Compensate.Func.Call(goArgs)
		if len(results) > 0 && !results[len(results)-1].IsNil() {
			return results[len(results)-1].Interface().(error)
		}
		return nil
	}
}

// invokeMethod calls the Go method, handling variadic dispatch.
func invokeMethod(m *op.Method, goArgs []reflect.Value) []reflect.Value {
	if m.Reflect.Type.IsVariadic() {
		return m.Reflect.Func.CallSlice(goArgs)
	}
	return m.Reflect.Func.Call(goArgs)
}

// coerceArgs converts slot values to Go method parameter types.
func coerceArgs(m *op.Method, ctx op.Context, slots map[string]any) ([]reflect.Value, error) {
	methodType := m.Reflect.Type
	goArgs := make([]reflect.Value, len(m.ParamNames)+1)
	goArgs[0] = reflect.ValueOf(m.Factory.GetOrCreateProvider(ctx))
	for i, name := range m.ParamNames {
		sv := slots[name]
		paramType := methodType.In(i + 1)
		val, err := coerceSlotValue(sv, paramType)
		if err != nil {
			return nil, fmt.Errorf("%s: param %s: %w", m.ActionName, name, err)
		}
		goArgs[i+1] = val
	}
	return goArgs, nil
}

// dryRunLog writes dry-run output to the context writer.
func dryRunLog(m *op.Method, ctx *op.Context, slots map[string]any) {
	_, _ = fmt.Fprintf(ctx.Writer, "[dry-run] %s", m.ActionName)
	for _, name := range m.ParamNames {
		_, _ = fmt.Fprintf(ctx.Writer, " %v", slots[name])
	}
	_, _ = fmt.Fprintln(ctx.Writer)
}

// classifyCompensableReturn interprets Go method return values for CompensableAction.
//
// Arity (after stripping trailing error):
//
//	0 → (nil, nil, error)
//	1 → (result, nil, error)
//	2 → (result, undoState, error)
func classifyCompensableReturn(results []reflect.Value) (op.Result, op.Complement, error) {
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

	var result op.Result
	var undoState op.Complement

	if n > 0 {
		v := results[0].Interface()
		if _, isNoResult := v.(op.NoResult); !isNoResult {
			result = v
		}
	}
	if n > 1 {
		undoState = results[1].Interface()
	}

	return result, undoState, err
}

// classifyFallibleReturn interprets Go method return values for FallibleAction.
//
// Arity (after stripping trailing error):
//
//	0 → (nil, error)
//	1 → (result, error)
func classifyFallibleReturn(results []reflect.Value) (op.Result, error) {
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

	var result op.Result
	if n > 0 {
		v := results[0].Interface()
		if _, isNoResult := v.(op.NoResult); !isNoResult {
			result = v
		}
	}

	return result, err
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

	if ctor, ok := op.LookupConstructor(targetType); ok {
		result, err := ctor(slotValue)
		if err != nil {
			return reflect.Value{}, err
		}
		return reflect.ValueOf(result), nil
	}

	return reflect.Value{}, fmt.Errorf("cannot coerce %T to %s", slotValue, targetType)
}

// shadowResult shadows [op.Resource] results in the catalog without changing the result type.
//
// [op.ReceiverFactory] methods return resources by value (e.g., file.Resource). The [op.Resource] interface requires
// pointer receivers, so a temporary pointer is created for the Shadow call. The catalog stamps id/originID on the
// underlying [op.ResourceBase]; and the stamped value is returned.
func shadowResult(result op.Result, catalog *op.ResourceCatalog, originID string) op.Result {

	if result == nil || catalog == nil {
		return result
	}

	rv := reflect.ValueOf(result)
	t := rv.Type()

	// Already satisfies Resource (pointer type or interface).
	if t.Implements(resourceType) {
		_, _ = catalog.Shadow(result.(op.Resource), originID)
		return result
	}

	// Struct whose pointer satisfies Resource (common case: file.Resource value).
	//
	// Create a temporary pointer for the Shadow call, then return the
	// stamped value (same type as original).

	if t.Kind() == reflect.Struct && reflect.PointerTo(t).Implements(resourceType) {
		ptr := reflect.New(t)
		ptr.Elem().Set(rv)
		_, _ = catalog.Shadow(ptr.Interface().(op.Resource), originID)
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
					_, _ = catalog.Shadow(rv.Index(i).Interface().(op.Resource), originID)
				} else {
					ptr := reflect.New(et)
					ptr.Elem().Set(rv.Index(i))
					_, _ = catalog.Shadow(ptr.Interface().(op.Resource), originID)
				}
			}
		}
	}

	return result
}

// validateCompensateSignature panics if a Compensate method has a malformed signature.
//
// The expected shape is func(receiver, undoState) error.
func validateCompensateSignature(actionName string, cm reflect.Method) {
	mt := cm.Type
	// NumIn includes receiver, so 2 = receiver + undoState.
	if mt.NumIn() != 2 {
		panic(fmt.Sprintf("%s: %s must accept exactly 1 parameter (undo state), got %d",
			actionName,
			cm.Name,
			mt.NumIn()-1,
		))
	}
	if mt.NumOut() != 1 || !mt.Out(0).Implements(errorType) {
		panic(fmt.Sprintf("%s: %s must return exactly (error), got %d return values",
			actionName,
			cm.Name,
			mt.NumOut(),
		))
	}
}

// validateSlotType checks whether a Go value can be coerced to a target type at plan time.
//
// This mirrors the coercion paths in coerceSlotValue without performing the actual coercion. Returns an error for
// obvious mismatches; returns nil when coercion is plausible.
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

	// Constructor registry (includes lazy init from resource announcements).

	if _, ok := op.LookupConstructor(targetType); ok {
		return nil
	}

	// CallableResource → func type coercion.
	
	if isCallableResource(goVal) && isFuncType(targetType) {
		return nil
	}

	return fmt.Errorf("cannot coerce %T to %s", goVal, targetType)
}
