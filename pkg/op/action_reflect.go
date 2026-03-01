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
	return classifyActionReturn(results)
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

// classifyActionReturn interprets Go method return values for the action layer.
// Unlike classifyReturn (which marshals to Starlark), this keeps Go-native values.
//
// Patterns (error always consumed from last position):
//
//	(error)          → (nil, nil, error)
//	(T, error)       → (T, nil, error)
//	(T, U, error)    → (T, U, error)
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
		result = results[0].Interface()
	}
	if n > 1 {
		undoState = results[1].Interface()
	}

	return result, undoState, err
}

// RegisterReflectedActions registers all action methods on a provider.
// Methods must return error as their last value to qualify as actions.
// Methods without error returns (e.g., Exists, IsDir) are skipped —
// they are immediate-only, not graph actions.
// A Compensate<GoName> companion method upgrades the action to CompensableAction.
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

		snakeName := CamelToSnake(goName)
		actionName := name + "." + snakeName

		base := reflectedAction{
			name:       actionName,
			provider:   pv,
			method:     method,
			paramNames: paramNames,
		}

		compensateName := "Compensate" + goName
		if cm, ok := pt.MethodByName(compensateName); ok {
			reg.Register(&reflectedCompensableAction{
				reflectedAction: base,
				compensate:      cm,
			})
		} else {
			reg.Register(&base)
		}
	}
}
