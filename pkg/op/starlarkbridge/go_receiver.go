// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlarkbridge

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
)

var (
	_ starlark.Value      = (*goReceiver)(nil) // Interface Guard: ensures *goReceiver implements starlark.Value.
	_ starlark.HasAttrs   = (*goReceiver)(nil) // Interface Guard: ensures *goReceiver implements starlark.HasAttrs.
	_ starlark.Comparable = (*goReceiver)(nil) // Interface Guard: ensures *goReceiver implements starlark.Comparable.
	_ Projector           = (*goReceiver)(nil) // Interface Guard: ensures *goReceiver implements Projector.
)

// goReceiver wraps a registered Go instance for starlark use.
//
// It implements [starlark.Value], [starlark.HasAttrs], [starlark.Comparable], and [Projector]. Fields are resolved
// first by projecting exported struct fields to starlark; methods are resolved second via [op.Method.Do]
// dispatch.
type goReceiver struct {
	receiverType op.ReceiverType
	instance     any                   // An instance of ReceiverType
	methods      map[string]*op.Method // snake_name → *Method
	fields       map[string]int        // snake_name → struct field index
	attrNames    []string              // sorted (fields + methods)
}

// NewGoReceiver wraps a Go value as a starlark surface bound to its receiver type.
//
// The returned value implements [starlark.Value], [starlark.HasAttrs], [starlark.Comparable], and [Unwrapper].
//
// Parameters:
//   - value: the Go value to wrap.
//
// Returns:
//   - starlark.HasAttrs: the bound starlark surface, ready for AttrNames / Attr / Type.
//   - error: non-nil if the receiver type cannot be derived from value's type.
func NewGoReceiver(value any) (starlark.HasAttrs, error) {

	receiverType, err := op.NewReceiverType(reflect.TypeOf(value), nil)
	if err != nil {
		return nil, fmt.Errorf("derive receiver type: %w", err)
	}

	return newGoReceiver(receiverType, value), nil
}

// NewProvider wraps a Go provider instance as a starlark surface bound to the given receiver type.
//
// Parameters:
//   - rt: the provider receiver type descriptor.
//   - instance: the Go provider instance.
//
// Returns:
//   - starlark.HasAttrs: the bound starlark surface.
func NewProvider(rt op.ReceiverType, instance any) starlark.HasAttrs {
	return newGoReceiver(rt, instance)
}

// newGoReceiver constructs the unexported [goReceiver] from a [op.ReceiverType] and Go instance.
func newGoReceiver(receiverType op.ReceiverType, instance any) *goReceiver {

	// Discover methods.

	methods := make(map[string]*op.Method)
	seen := make(map[string]bool)

	for method := range receiverType.Methods() {
		snake := op.CamelToSnake(method.Name())
		methods[snake] = method
		seen[snake] = true
	}

	// Discover exported struct fields using centralized introspection logic.

	fields := make(map[string]int)

	if info := getTypeInfo(reflect.TypeOf(instance)); info != nil {
		for _, fi := range info.fields {
			fields[fi.starName] = fi.index
			seen[fi.starName] = true
		}
	}

	// Build sorted attr list from fields + methods.

	attrNames := make([]string, 0, len(seen))

	for name := range seen {
		attrNames = append(attrNames, name)
	}

	sort.Strings(attrNames)

	return &goReceiver{
		receiverType: receiverType,
		instance:     instance,
		methods:      methods,
		fields:       fields,
		attrNames:    attrNames,
	}
}

// region EXPORTED METHODS

// region State management

// String implements starlark.Value.
func (w *goReceiver) String() string {

	if stringer, ok := w.instance.(fmt.Stringer); ok {
		return stringer.String()
	}
	return w.receiverType.Name()
}

// Type implements starlark.Value.
func (w *goReceiver) Type() string { return w.receiverType.Name() }

// Freeze implements starlark.Value.
func (w *goReceiver) Freeze() {}

// Truth implements starlark.Value.
func (w *goReceiver) Truth() starlark.Bool { return true }

// Hash implements starlark.Value.
func (w *goReceiver) Hash() (uint32, error) {

	if res, ok := w.instance.(op.Resource); ok {
		if uri := res.URI(); uri != "" {
			return hashString(uri), nil
		}
	}

	return 0, fmt.Errorf("unhashable type: %s", w.receiverType.Name())
}

// endregion

// region Behaviors

// Attr implements starlark.HasAttrs.
func (w *goReceiver) Attr(name string) (starlark.Value, error) {

	if idx, ok := w.fields[name]; ok {
		return w.toStarlarkReflect(elem(reflect.ValueOf(w.instance)).Field(idx))
	}

	if _, ok := w.methods[name]; ok {
		actionName := w.receiverType.Name() + "." + name
		return starlark.NewBuiltin(actionName, w.dispatch), nil
	}

	if resolver, ok := w.instance.(op.AttributeResolver); ok {
		if resolved := resolver.ResolveAttr(name); resolved != nil {
			return w.toStarlarkReflect(reflect.ValueOf(resolved))
		}
	}

	return nil, NoSuchAttrError(w.receiverType.Name(), name)
}

// AttrNames implements starlark.HasAttrs.
func (w *goReceiver) AttrNames() []string { return w.attrNames }

// Project extracts a Go value of the target type from this receiver.
func (w *goReceiver) Project(target reflect.Type) (any, error) {
	return op.Convert(w.executionContext(), w.instance, target)
}

// endregion

// endregion

// region UNEXPORTED METHODS

// region State management

// executionContext returns the [op.RuntimeEnvironment] from the wrapped instance.
func (w *goReceiver) executionContext() *op.RuntimeEnvironment {

	if p, ok := w.instance.(op.Provider); ok {
		return p.RuntimeEnvironment()
	}

	return nil
}

// endregion

// region Behaviors

// toStarlark converts a Go receiver to a [starlark.Value].
func (w *goReceiver) toStarlark(v any) (starlark.Value, error) {

	if v == nil {
		return starlark.None, nil
	}

	if sv, ok := v.(starlark.Value); ok {
		return sv, nil
	}

	return w.toStarlarkReflect(reflect.ValueOf(v))
}

// toStarlarkMap converts a [reflect.Value] map to a [starlark.Dict].
func (w *goReceiver) toStarlarkMap(rv reflect.Value) (starlark.Value, error) {

	if rv.IsNil() {
		return starlark.NewDict(0), nil
	}

	dict := starlark.NewDict(rv.Len())
	iter := rv.MapRange()

	for iter.Next() {

		key, err := w.toStarlarkReflect(iter.Key())

		if err != nil {
			return nil, fmt.Errorf("map key: %w", err)
		}

		val, err := w.toStarlarkReflect(iter.Value())

		if err != nil {
			return nil, fmt.Errorf("map value for %v: %w", iter.Key().Interface(), err)
		}

		if err := dict.SetKey(key, val); err != nil {
			return nil, fmt.Errorf("dict set: %w", err)
		}
	}

	return dict, nil
}

// toStarlarkReflect converts a [reflect.Value] to a [starlark.Value].
func (w *goReceiver) toStarlarkReflect(rv reflect.Value) (starlark.Value, error) {

	rv = elem(rv)

	if (rv.Kind() == reflect.Pointer || rv.Kind() == reflect.Interface) && rv.IsNil() {
		return starlark.None, nil
	}

	switch rv.Kind() {

	case reflect.String:
		return starlark.String(rv.String()), nil

	case reflect.Bool:
		return starlark.Bool(rv.Bool()), nil

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return starlark.MakeInt64(rv.Int()), nil

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return starlark.MakeUint64(rv.Uint()), nil

	case reflect.Float32, reflect.Float64:
		return starlark.Float(rv.Float()), nil

	case reflect.Slice:

		if rv.Type().Elem().Kind() == reflect.Uint8 {
			return starlark.Bytes(rv.Bytes()), nil
		}

		return w.toStarlarkSlice(rv)

	case reflect.Map:
		return w.toStarlarkMap(rv)

	case reflect.Struct:

		if rv.CanInterface() {
			if sv, ok := rv.Interface().(starlark.Value); ok {
				return sv, nil
			}
		}

		if rv.CanAddr() {
			if sv, ok := rv.Addr().Interface().(starlark.Value); ok {
				return sv, nil
			}
		}

		var ptr reflect.Value

		if rv.CanAddr() {
			ptr = rv.Addr()
		} else {
			ptr = reflect.New(rv.Type())
			ptr.Elem().Set(rv)
		}

		ctx := w.executionContext()

		if ctx != nil && ctx.Registry != nil {
			receiverType := ctx.Registry.TypeByReflectionOrDerive(ptr.Type())
			if receiverType != nil {
				return newGoReceiver(receiverType, ptr.Interface()), nil
			}
		}

		receiverType, err := op.NewReceiverType(ptr.Type(), nil)

		if err != nil {
			return nil, err
		}

		return newGoReceiver(receiverType, ptr.Interface()), nil

	default:
		return nil, fmt.Errorf("cannot represent %s as a starlark value", rv.Type())
	}
}

// toStarlarkSlice converts a [reflect.Value] slice to a [starlark.List].
func (w *goReceiver) toStarlarkSlice(rv reflect.Value) (starlark.Value, error) {

	if rv.IsNil() {
		return starlark.NewList(nil), nil
	}

	elems := make([]starlark.Value, rv.Len())

	for i := range rv.Len() {

		val, err := w.toStarlarkReflect(rv.Index(i))

		if err != nil {
			return nil, fmt.Errorf("slice index %d: %w", i, err)
		}

		elems[i] = val
	}

	return starlark.NewList(elems), nil
}

// dispatch dispatches a starlark builtin invocation to the underlying Go method.
func (w *goReceiver) dispatch(_ *starlark.Thread, builtin *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {

	actionName := builtin.Name()
	name := actionName[strings.LastIndex(actionName, ".")+1:]
	method := w.methods[name]
	params := method.Parameters()

	var namedParams []string
	var namedOptional []bool
	var namedDefaults []any
	var namedTypes []reflect.Type
	var variadicName string
	var variadicIdx int
	var kwargsName string
	var kwargsIdx int

	for i, p := range params {
		switch {
		case p.Kwargs:
			kwargsName = p.Name
			kwargsIdx = i
		case p.Variadic:
			variadicName = p.Name
			variadicIdx = i
		default:
			namedParams = append(namedParams, p.Name)
			namedOptional = append(namedOptional, p.Optional)
			namedDefaults = append(namedDefaults, p.Default)
			namedTypes = append(namedTypes, p.Type)
		}
	}

	numNamed := len(namedParams)
	numParams := len(params)

	unpackArgs := args
	unpackKwargs := kwargs

	var positionalVariadic starlark.Tuple
	var kwVariadic starlark.Value
	var extraKwargs []starlark.Tuple

	if variadicName != "" || kwargsName != "" {

		knownKwargs := make(map[string]bool, numNamed+1)

		for _, n := range namedParams {
			knownKwargs[n] = true
		}

		if variadicName != "" {
			knownKwargs[variadicName] = true
		}

		unpackKwargs = nil

		for _, kv := range kwargs {

			key, _ := starlark.AsString(kv[0])

			switch {
			case key == variadicName:
				kwVariadic = kv[1]
			case knownKwargs[key]:
				unpackKwargs = append(unpackKwargs, kv)
			default:
				extraKwargs = append(extraKwargs, kv)
			}
		}

		if kwargsName == "" && len(extraKwargs) > 0 {
			key, _ := starlark.AsString(extraKwargs[0][0])
			return nil, fmt.Errorf("%s() got an unexpected keyword argument %q", actionName, key)
		}

		if len(args) > numNamed {
			unpackArgs = args[:numNamed]
			positionalVariadic = args[numNamed:]
		}
	}

	vals := make([]starlark.Value, numNamed)
	pairs := make([]any, 0, numNamed*2)

	for i, n := range namedParams {
		// starlark.UnpackArgs uses a trailing "?" on the pair name to mark a kwarg optional. namedParams holds
		// clean names; reconstruct the "?" suffix so UnpackArgs sees the optional convention.
		unpackName := n
		if namedOptional[i] {
			unpackName += "?"
		}
		pairs = append(pairs, unpackName, &vals[i])
	}

	if err := starlark.UnpackArgs(actionName, unpackArgs, unpackKwargs, pairs...); err != nil {
		return nil, err
	}

	slots := make(map[string]any, numParams)

	for i, sv := range vals {

		if sv == nil {
			// Truly absent kwarg — fill from the parameter's directive default if one exists. Literal-form
			// defaults arrive already typed (parseDefaultExpression widens via reflect.Value.Convert at
			// announce time); deferred-default forms (op.DeferredDefault) resolve here against the live
			// runtime environment and the already-filled sibling slots in the slots map.
			if namedDefaults[i] != nil {
				value := namedDefaults[i]
				if d, ok := value.(op.DeferredDefault); ok {
					resolved, err := d.Resolve(w.executionContext(), slots, namedTypes[i])
					if err != nil {
						return nil, fmt.Errorf("%s(): %s: default: %w", actionName, namedParams[i], err)
					}
					value = resolved
				}
				slots[namedParams[i]] = value
			}
			continue
		}

		var val any

		if err := toGoInto(sv, reflect.ValueOf(&val).Elem()); err != nil {
			return nil, fmt.Errorf("%s(): %s: %w", actionName, namedParams[i], err)
		}

		slots[namedParams[i]] = val
	}

	if variadicName != "" {

		if len(positionalVariadic) > 0 && kwVariadic != nil {
			return nil, fmt.Errorf("%s() got multiple values for argument %q", actionName, variadicName)
		}

		var variadicList *starlark.List

		if len(positionalVariadic) > 0 {
			elems := make([]starlark.Value, len(positionalVariadic))
			copy(elems, positionalVariadic)
			variadicList = starlark.NewList(elems)
		} else if kwVariadic != nil {
			list, ok := kwVariadic.(*starlark.List)
			if !ok {
				return nil, fmt.Errorf("%s(): keyword %s must be a list, got %s", actionName, variadicName, kwVariadic.Type())
			}
			variadicList = list
		}

		if variadicList != nil && variadicList.Len() > 0 {
			var val any
			if err := toGoInto(variadicList, reflect.ValueOf(&val).Elem()); err != nil {
				return nil, fmt.Errorf("%s(): %s: %w", actionName, variadicName, err)
			}
			slots[params[variadicIdx].Name] = val
		}
	}

	if kwargsName != "" {

		kwargsMap := make(map[string]any, len(extraKwargs))

		for _, kv := range extraKwargs {
			key, _ := starlark.AsString(kv[0])
			var val any
			if err := toGoInto(kv[1], reflect.ValueOf(&val).Elem()); err != nil {
				return nil, fmt.Errorf("%s(): keyword %s: %w", actionName, key, err)
			}
			kwargsMap[key] = val
		}

		slots[params[kwargsIdx].Name] = kwargsMap
	}

	runtimeEnvironment := w.executionContext()
	// Immediate-mode starlark dispatch (codegen, REPL, ad-hoc calls) has no graph node to derive a SiteID from.
	// Synthesize a stable, non-empty label per actionName so producer methods that strictly require
	// activation.SiteID (e.g., [op.ResourceCatalog.GetOrCreate]) accept the call. Real graph dispatch goes
	// through the executor, which builds the activation with the actual node's ID as SiteID.
	activationRecord := &op.ActivationRecord{Runtime: runtimeEnvironment, SiteID: "starlark:" + actionName, Context: runtimeEnvironment.Context}
	result, _, err := method.Invoke(activationRecord, w.instance, slots)
	if err != nil {
		return nil, err
	}

	if result == nil {
		return starlark.None, nil
	}

	return w.toStarlark(result)
}

// CompareSameType implements starlark.Comparable.
func (w *goReceiver) CompareSameType(cmp syntax.Token, x starlark.Value, _ int) (bool, error) {

	other := x.(*goReceiver)
	var equal bool

	if c, ok := w.instance.(op.Comparer); ok {
		equal = c.Equal(other.instance)
	} else {
		equal = w.instance == other.instance
	}

	switch cmp {
	case syntax.EQL:
		return equal, nil
	case syntax.NEQ:
		return !equal, nil
	default:
		return false, fmt.Errorf("%s not supported between %q values", cmp, w.Type())
	}
}

// endregion

// endregion

// typeInfo holds struct introspection results for Starlark field mapping.
type typeInfo struct {
	fields []fieldInfo
}

// fieldInfo maps a single exported Go struct field to its Starlark name.
type fieldInfo struct {
	index    int
	starName string
}

// elem returns the concrete value behind pointers and interfaces.
func elem(v reflect.Value) reflect.Value {

	for v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return v
		}
		v = v.Elem()
	}

	return v
}

// getTypeInfo returns struct metadata for the given type.
func getTypeInfo(t reflect.Type) *typeInfo {

	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	if t.Kind() != reflect.Struct {
		return nil
	}

	info := &typeInfo{}

	for i := range t.NumField() {

		f := t.Field(i)

		if !f.IsExported() {
			continue
		}

		tag := f.Tag.Get("starlark")

		if tag == "-" {
			continue
		}

		name := tag

		if name == "" {
			name = op.CamelToSnake(f.Name)
		}

		info.fields = append(info.fields, fieldInfo{index: i, starName: name})
	}

	return info
}

// hashString returns a simple hash of the given string.
func hashString(s string) uint32 {

	var hash uint32

	for _, c := range s {
		hash = hash*31 + uint32(c)
	}

	return hash
}

// NoSuchAttrError returns an error for an unknown attribute.
func NoSuchAttrError(typeName, attr string) error {
	return fmt.Errorf("%q object has no attribute %q", typeName, attr)
}
