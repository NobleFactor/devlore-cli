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
	_ starlark.Value      = (*wrapper)(nil) // Interface Guard: ensures *wrapper implements starlark.Value.
	_ starlark.HasAttrs   = (*wrapper)(nil) // Interface Guard: ensures *wrapper implements starlark.HasAttrs.
	_ starlark.Comparable = (*wrapper)(nil) // Interface Guard: ensures *wrapper implements starlark.Comparable.
	_ Projector           = (*wrapper)(nil) // Interface Guard: ensures *wrapper implements Projector.
)

// wrapper wraps a registered Go instance for starlark use.
//
// It implements [starlark.Value], [starlark.HasAttrs], and [starlark.Comparable]. Fields are resolved first by
// marshaling exported struct fields; methods are resolved second via [op.Method.Do] dispatch.
type wrapper struct {
	receiverType op.ReceiverType
	instance     any                   // An instance of ReceiverType
	methods      map[string]*op.Method // snake_name → *Method
	fields       map[string]int        // snake_name → struct field index
	attrNames    []string              // sorted (fields + methods)
}

// NewWrapper wraps a Go value as a starlark surface bound to its receiver type.
//
// The returned value implements [starlark.Value], [starlark.HasAttrs], [starlark.Comparable], and [Unwrapper].
// Callers needing to extract the wrapped Go value type-assert against [Unwrapper]; starlark machinery uses the
// HasAttrs surface directly. Generated module-level smoke tests (module.gen_test.go) call this to exercise
// AttrNames / Attr / Type without standing up a full [Runtime]; production code reaches the same wrapper via
// [Runtime.NewModule].
//
// The receiver type is derived from the value's reflected type via [op.NewReceiverType] (constructing a fresh
// descriptor when no provider has registered the type). Internal callers that already have a receiver type
// (Runtime construction, nested struct projection) bypass this factory and call newWrapper directly.
//
// Parameters:
//   - value: the Go value to wrap.
//
// Returns:
//   - starlark.HasAttrs: the bound starlark surface, ready for AttrNames / Attr / Type.
//   - error: non-nil if the receiver type cannot be derived from value's type.
func NewWrapper(value any) (starlark.HasAttrs, error) {

	receiverType, err := op.NewReceiverType(reflect.TypeOf(value), nil)
	if err != nil {
		return nil, fmt.Errorf("derive receiver type: %w", err)
	}

	return newWrapper(receiverType, value), nil
}

// newWrapper constructs the unexported [wrapper] from a [op.ReceiverType] and Go instance.
//
// Internal callers ([NewWrapper], [Runtime.buildOne], and [wrapper.marshalReflect] for nested struct projection)
// route through here. Method and field discovery happens once at construction; the returned [wrapper] holds the
// pre-sorted attr name list used by [wrapper.AttrNames].
func newWrapper(receiverType op.ReceiverType, instance any) *wrapper {

	// Discover methods.

	methods := make(map[string]*op.Method)
	seen := make(map[string]bool)

	for method := range receiverType.Methods() {
		snake := camelToSnake(method.Name())
		methods[snake] = method
		seen[snake] = true
	}

	// Discover exported struct fields.

	fields := make(map[string]int)
	elem := reflect.ValueOf(instance)

	for elem.Kind() == reflect.Pointer {
		elem = elem.Elem()
	}

	if elem.Kind() == reflect.Struct {
		t := elem.Type()
		for i := range t.NumField() {
			field := t.Field(i)
			if !field.IsExported() {
				continue
			}
			tag := field.Tag.Get("starlark")
			if tag == "-" {
				continue
			}
			name := tag
			if name == "" {
				name = camelToSnake(field.Name)
			}
			fields[name] = i
			seen[name] = true
		}
	}

	// Build sorted attr list from fields + methods.

	attrNames := make([]string, 0, len(seen))

	for name := range seen {
		attrNames = append(attrNames, name)
	}

	sort.Strings(attrNames)

	return &wrapper{
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
func (w *wrapper) String() string {

	if stringer, ok := w.instance.(fmt.Stringer); ok {
		return stringer.String()
	}
	return w.receiverType.Name()
}

// Type implements starlark.Value.
func (w *wrapper) Type() string { return w.receiverType.Name() }

// Freeze implements starlark.Value.
func (w *wrapper) Freeze() {}

// Truth implements starlark.Value.
func (w *wrapper) Truth() starlark.Bool { return true }

// Hash implements starlark.Value.
func (w *wrapper) Hash() (uint32, error) {

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
//
// Fields are resolved first by marshaling the Go struct field. Methods are resolved second. AttributeResolver is
// checked last.
//
// Parameters:
//   - name: the snake_case attribute name to look up.
//
// Returns:
//   - starlark.Value: the marshaled field value, a method builtin, or a resolved attribute.
//   - error: non-nil if the attribute does not exist.
func (w *wrapper) Attr(name string) (starlark.Value, error) {

	if idx, ok := w.fields[name]; ok {

		elem := reflect.ValueOf(w.instance)

		for elem.Kind() == reflect.Pointer {
			elem = elem.Elem()
		}

		return w.marshalReflect(elem.Field(idx))
	}

	if _, ok := w.methods[name]; ok {
		actionName := w.receiverType.Name() + "." + name
		return starlark.NewBuiltin(actionName, w.dispatch), nil
	}

	if resolver, ok := w.instance.(op.AttributeResolver); ok {
		if resolved := resolver.ResolveAttr(name); resolved != nil {
			return w.marshalReflect(reflect.ValueOf(resolved))
		}
	}

	return nil, NoSuchAttrError(w.receiverType.Name(), name)
}

// AttrNames implements starlark.HasAttrs.
//
// Returns:
//   - []string: sorted list of available method names.
func (w *wrapper) AttrNames() []string { return w.attrNames }

// Project extracts a Go value of the target type from this wrapper.
//
// Project is the bridge's default starlark→Go extraction point — used when a provider method parameter needs the
// wrapped Go instance and the parameter's type does not opt out via [Unmarshaler]. Delegates to [op.Convert] for the
// source/target opt-in cascade so the conversion semantics stay consistent with method dispatch in [op.Method.Invoke].
//
// Parameters:
//   - target: the [reflect.Type] of the Go value to extract.
//
// Returns:
//   - any: the extracted Go value, ready to assign to a target of type target.
//   - error: non-nil if no path through the cascade succeeds.
func (w *wrapper) Project(target reflect.Type) (any, error) {
	return op.Convert(w.executionContext(), w.instance, target)
}

// endregion

// endregion

// region UNEXPORTED METHODS

// region State management

// executionContext returns the [op.ExecutionContext] from the wrapped instance.
//
// Returns:
//   - *op.ExecutionContext: the context, or nil if the instance is not an [op.Provider].
func (w *wrapper) executionContext() *op.ExecutionContext {

	if p, ok := w.instance.(op.Provider); ok {
		return p.ExecutionContext()
	}

	return nil
}

// endregion

// region Behaviors

// Fallible actions

// marshal converts a Go wrapper to a [starlark.Value].
//
// Parameters:
//   - v: the Go wrapper to convert. Nil returns [starlark.None].
//
// Returns:
//   - starlark.Value: the converted Starlark wrapper.
//   - error: non-nil if v contains an unsupported type.
func (w *wrapper) marshal(v any) (starlark.Value, error) {

	if v == nil {
		return starlark.None, nil
	}

	if sv, ok := v.(starlark.Value); ok {
		return sv, nil
	}

	return w.marshalReflect(reflect.ValueOf(v))
}

// marshalMap converts a [reflect.Value] map to a [starlark.Dict].
//
// Parameters:
//   - rv: the [reflect.Value] of kind Map to convert.
//
// Returns:
//   - starlark.Value: the converted Starlark dict.
//   - error: non-nil if any key or wrapper cannot be marshaled.
func (w *wrapper) marshalMap(rv reflect.Value) (starlark.Value, error) {

	if rv.IsNil() {
		return starlark.NewDict(0), nil
	}

	dict := starlark.NewDict(rv.Len())
	iter := rv.MapRange()

	for iter.Next() {

		key, err := w.marshalReflect(iter.Key())

		if err != nil {
			return nil, fmt.Errorf("map key: %w", err)
		}

		val, err := w.marshalReflect(iter.Value())

		if err != nil {
			return nil, fmt.Errorf("map value for %v: %w", iter.Key().Interface(), err)
		}

		if err := dict.SetKey(key, val); err != nil {
			return nil, fmt.Errorf("dict set: %w", err)
		}
	}

	return dict, nil
}

// marshalReflect converts a [reflect.Value] to a [starlark.Value].
//
// Parameters:
//   - rv: the [reflect.Value] to convert.
//
// Returns:
//   - starlark.Value: the converted Starlark wrapper.
//   - error: non-nil if rv contains an unsupported type.
func (w *wrapper) marshalReflect(rv reflect.Value) (starlark.Value, error) {

	for rv.Kind() == reflect.Pointer || rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return starlark.None, nil
		}
		rv = rv.Elem()
	}

	// Check Marshaler for non-struct types.

	if rv.Kind() != reflect.Struct && rv.CanInterface() {
		if m, ok := rv.Interface().(Marshaler); ok {
			return m.MarshalStarlark()
		}
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

		return w.marshalSlice(rv)

	case reflect.Map:
		return w.marshalMap(rv)

	case reflect.Struct:

		// Already a starlark.Value — pass through.

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

		// Custom marshaler — let the type handle it.

		if rv.CanInterface() {
			if m, ok := rv.Interface().(Marshaler); ok {
				return m.MarshalStarlark()
			}
		}

		if rv.CanAddr() {
			if m, ok := rv.Addr().Interface().(Marshaler); ok {
				return m.MarshalStarlark()
			}
		}

		// Wrap as value. Ensure we have a pointer for method dispatch.

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
			return newWrapper(receiverType, ptr.Interface()), nil
		}

		receiverType, err := op.NewReceiverType(ptr.Type(), nil)

		if err != nil {
			return nil, err
		}

		return newWrapper(receiverType, ptr.Interface()), nil

	default:
		return nil, fmt.Errorf("cannot represent %s as a starlark value", rv.Type())
	}
}

// marshalSlice converts a [reflect.Value] slice to a [starlark.List].
//
// Parameters:
//   - rv: the [reflect.Value] of kind Slice to convert.
//
// Returns:
//   - starlark.Value: the converted Starlark list.
//   - error: non-nil if any element cannot be marshaled.
func (w *wrapper) marshalSlice(rv reflect.Value) (starlark.Value, error) {

	if rv.IsNil() {
		return starlark.NewList(nil), nil
	}

	elems := make([]starlark.Value, rv.Len())

	for i := range rv.Len() {

		val, err := w.marshalReflect(rv.Index(i))

		if err != nil {
			return nil, fmt.Errorf("slice index %d: %w", i, err)
		}

		elems[i] = val
	}

	return starlark.NewList(elems), nil
}

// unmarshalValue recursively converts a Starlark wrapper into a [reflect.Value] target.
//
// Parameters:
//   - sv: the Starlark wrapper to convert.
//   - rv: the [reflect.Value] to populate (must be settable).
//
// Returns:
//   - error: non-nil if the conversion fails.
func (w *wrapper) unmarshalValue(sv starlark.Value, rv reflect.Value) error {

	// Nil starlark value: set to zero.

	if sv == nil {
		rv.Set(reflect.Zero(rv.Type()))
		return nil
	}

	// Handle any/interface target: convert the starlark value to its natural Go equivalent.

	if rv.Kind() == reflect.Interface {

		var val any

		switch v := sv.(type) {

		case starlark.NoneType:
			rv.Set(reflect.Zero(rv.Type()))
			return nil

		case starlark.String:
			val = string(v)

		case starlark.Int:

			i, ok := v.Int64()

			if !ok {
				return fmt.Errorf("int value out of range")
			}

			val = int(i)

		case starlark.Bool:
			val = bool(v)

		case starlark.Float:
			val = float64(v)

		case starlark.Bytes:
			val = []byte(v)

		case *starlark.List:

			result := make([]any, v.Len())

			for i := range v.Len() {
				if err := w.unmarshalValue(v.Index(i), reflect.ValueOf(&result[i]).Elem()); err != nil {
					return fmt.Errorf("list index %d: %w", i, err)
				}
			}

			val = result

		case *starlark.Dict:

			result := make(map[any]any, v.Len())

			for _, item := range v.Items() {

				var key, value any

				if err := w.unmarshalValue(item[0], reflect.ValueOf(&key).Elem()); err != nil {
					return fmt.Errorf("dict key: %w", err)
				}

				if err := w.unmarshalValue(item[1], reflect.ValueOf(&value).Elem()); err != nil {
					return fmt.Errorf("dict key %v: %w", key, err)
				}

				result[key] = value
			}

			val = result

		case starlark.HasAttrs:

			names := v.AttrNames()
			result := make(map[string]any, len(names))

			for _, name := range names {

				attr, err := v.Attr(name)

				if err != nil {
					return fmt.Errorf("attr %q: %w", name, err)
				}

				var value any

				if err := w.unmarshalValue(attr, reflect.ValueOf(&value).Elem()); err != nil {
					return fmt.Errorf("attr %q: %w", name, err)
				}

				result[name] = value
			}

			val = result

		default:

			if reflect.TypeOf(sv).Kind() == reflect.Pointer {
				val = sv
			} else {
				return fmt.Errorf("unsupported starlark type: %s", sv.Type())
			}
		}

		rv.Set(reflect.ValueOf(val))
		return nil
	}

	// Handle None → zero value.

	if _, ok := sv.(starlark.NoneType); ok {
		rv.Set(reflect.Zero(rv.Type()))
		return nil
	}

	// Custom unmarshaler: let the destination Go type absorb the starlark value via [Unmarshaler]. Match the pattern
	// established in [NodeBuilder.assignTarget] — addressable destinations get their pointer; pointer-typed
	// destinations get allocated fresh when nil so the [Unmarshaler] value has somewhere to write.

	if rv.CanAddr() {
		if u, ok := rv.Addr().Interface().(Unmarshaler); ok {
			return u.UnmarshalStarlark(sv)
		}
	}

	if rv.Kind() == reflect.Pointer {

		if rv.IsNil() {
			rv.Set(reflect.New(rv.Type().Elem()))
		}

		if u, ok := rv.Interface().(Unmarshaler); ok {
			return u.UnmarshalStarlark(sv)
		}
	}

	// Pass through Go pointer handles: if the starlark.Value is directly assignable to the target type, use it as-is.

	if reflect.TypeOf(sv).AssignableTo(rv.Type()) {
		rv.Set(reflect.ValueOf(sv))
		return nil
	}

	// Dereference/allocate pointers.

	for rv.Kind() == reflect.Pointer {

		if rv.IsNil() {
			rv.Set(reflect.New(rv.Type().Elem()))
		}

		rv = rv.Elem()
	}

	switch rv.Kind() {

	case reflect.String:

		s, ok := starlark.AsString(sv)

		if !ok {
			return fmt.Errorf("expected string, got %s", sv.Type())
		}

		rv.SetString(s)
		return nil

	case reflect.Bool:

		b, ok := sv.(starlark.Bool)

		if !ok {
			return fmt.Errorf("expected bool, got %s", sv.Type())
		}

		rv.SetBool(bool(b))
		return nil

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:

		si, ok := sv.(starlark.Int)

		if !ok {
			return fmt.Errorf("expected int, got %s", sv.Type())
		}

		i, ok := si.Int64()

		if !ok {
			return fmt.Errorf("int value out of range")
		}

		rv.SetInt(i)
		return nil

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:

		si, ok := sv.(starlark.Int)

		if !ok {
			return fmt.Errorf("expected int, got %s", sv.Type())
		}

		u, ok := si.Uint64()

		if !ok {
			return fmt.Errorf("uint value out of range")
		}

		rv.SetUint(u)
		return nil

	case reflect.Float32, reflect.Float64:

		switch v := sv.(type) {
		case starlark.Float:
			rv.SetFloat(float64(v))
		case starlark.Int:

			i, ok := v.Int64()

			if !ok {
				return fmt.Errorf("int too large to convert to float")
			}

			rv.SetFloat(float64(i))
		default:
			return fmt.Errorf("expected float or int, got %s", sv.Type())
		}

		return nil

	case reflect.Slice:

		if rv.Type().Elem().Kind() == reflect.Uint8 {

			b, ok := sv.(starlark.Bytes)

			if !ok {
				return fmt.Errorf("expected bytes, got %s", sv.Type())
			}

			rv.SetBytes([]byte(b))
			return nil
		}

		list, ok := sv.(*starlark.List)

		if !ok {
			return fmt.Errorf("expected list, got %s", sv.Type())
		}

		return w.unmarshalSlice(list, rv)

	case reflect.Map:

		dict, ok := sv.(*starlark.Dict)

		if !ok {
			return fmt.Errorf("expected dict, got %s", sv.Type())
		}

		return w.unmarshalMap(dict, rv)

	case reflect.Struct:
		return w.unmarshalStruct(sv, rv)

	default:
		return fmt.Errorf("cannot unmarshal into %s", rv.Type())
	}
}

// unmarshalMap converts a [starlark.Dict] into a typed Go map via reflection.
//
// Parameters:
//   - dict: the Starlark dict to convert.
//   - rv: the [reflect.Value] of kind Map to populate.
//
// Returns:
//   - error: non-nil if any key or wrapper cannot be unmarshaled.
func (w *wrapper) unmarshalMap(dict *starlark.Dict, rv reflect.Value) error {

	m := reflect.MakeMapWithSize(rv.Type(), dict.Len())
	keyType := rv.Type().Key()
	valType := rv.Type().Elem()

	for _, item := range dict.Items() {

		key := reflect.New(keyType).Elem()

		if err := w.unmarshalValue(item[0], key); err != nil {
			return fmt.Errorf("dict key: %w", err)
		}

		val := reflect.New(valType).Elem()

		if err := w.unmarshalValue(item[1], val); err != nil {
			return fmt.Errorf("dict value: %w", err)
		}

		m.SetMapIndex(key, val)
	}

	rv.Set(m)
	return nil
}

// unmarshalSlice converts a [starlark.List] into a typed Go slice via reflection.
//
// Parameters:
//   - list: the Starlark list to convert.
//   - rv: the [reflect.Value] of kind Slice to populate.
//
// Returns:
//   - error: non-nil if any element cannot be unmarshaled.
func (w *wrapper) unmarshalSlice(list *starlark.List, rv reflect.Value) error {

	n := list.Len()
	slice := reflect.MakeSlice(rv.Type(), n, n)

	for i := range n {

		if err := w.unmarshalValue(list.Index(i), slice.Index(i)); err != nil {
			return fmt.Errorf("list index %d: %w", i, err)
		}
	}

	rv.Set(slice)
	return nil
}

// unmarshalStruct converts a starlarkstruct.Struct, starlark.Dict, or starlark.HasAttrs into a typed Go struct via
// reflection. Fields are matched by Starlark name.
//
// Parameters:
//   - sv: the Starlark wrapper.
//   - rv: the [reflect.Value] of kind Struct to populate.
//
// Returns:
//   - error: non-nil if sv is an unsupported type, or if any field fails.
func (w *wrapper) unmarshalStruct(sv starlark.Value, rv reflect.Value) error {

	info := getTypeInfo(rv.Type())

	switch v := sv.(type) {

	case *starlark.Dict:
		return w.unmarshalDict(v, rv, info)

	case starlark.String, starlark.Int, starlark.Float, starlark.Bool, *starlark.List, starlark.Bytes:
		return fmt.Errorf("expected struct or dict for %s, got %s", rv.Type().Name(), sv.Type())

	case starlark.HasAttrs:
		return w.unmarshalHasAttrs(v, rv, info)

	default:
		return fmt.Errorf("expected struct or dict, got %s", sv.Type())
	}
}

// unmarshalHasAttrs populates a Go struct from a [starlark.HasAttrs] wrapper.
//
// Parameters:
//   - v: the Starlark wrapper with named attributes.
//   - rv: the [reflect.Value] of kind Struct to populate.
//   - info: the type metadata for the target struct.
//
// Returns:
//   - error: non-nil if any field fails to unmarshal.
func (w *wrapper) unmarshalHasAttrs(v starlark.HasAttrs, rv reflect.Value, info *typeInfo) error {

	for i := range info.fields {

		fi := &info.fields[i]
		attr, err := v.Attr(fi.starName)

		if err != nil {
			continue
		}

		if err := w.unmarshalValue(attr, rv.Field(fi.index)); err != nil {
			return fmt.Errorf("field %s: %w", fi.starName, err)
		}
	}

	return nil
}

// unmarshalDict populates a Go struct from a [starlark.Dict].
//
// Parameters:
//   - dict: the Starlark dict to read from.
//   - rv: the [reflect.Value] of kind Struct to populate.
//   - info: the type metadata for the target struct.
//
// Returns:
//   - error: non-nil if any field fails to unmarshal.
func (w *wrapper) unmarshalDict(dict *starlark.Dict, rv reflect.Value, info *typeInfo) error {

	for i := range info.fields {

		fi := &info.fields[i]
		val, found, err := dict.Get(starlark.String(fi.starName))

		if err != nil {
			return fmt.Errorf("field %s: %w", fi.starName, err)
		}

		if !found {
			continue
		}

		if err := w.unmarshalValue(val, rv.Field(fi.index)); err != nil {
			return fmt.Errorf("field %s: %w", fi.starName, err)
		}
	}

	return nil
}

// dispatch dispatches a starlark builtin invocation to the underlying Go method.
//
// Parameters:
//   - thread: the starlark thread.
//   - builtin: the starlark builtin that triggered the dispatch.
//   - args: positional starlark arguments.
//   - kwargs: keyword starlark arguments.
//
// Returns:
//   - starlark.Value: the marshaled return wrapper.
//   - error: non-nil if the dispatch fails.
func (w *wrapper) dispatch(_ *starlark.Thread, builtin *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {

	actionName := builtin.Name()

	name := actionName[strings.LastIndex(actionName, ".")+1:]
	method := w.methods[name]
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

	numNamed := len(namedParams)
	numParams := len(params)

	// Filter args and kwargs when variadic or kwargs parameters are present.

	unpackArgs := args
	unpackKwargs := kwargs

	var positionalVariadic starlark.Tuple
	var kwVariadic starlark.Value
	var extraKwargs []starlark.Tuple

	if variadicName != "" || kwargsName != "" {

		knownKwargs := make(map[string]bool, numNamed+1)

		for _, n := range namedParams {
			knownKwargs[strings.TrimSuffix(n, "?")] = true
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

	// Unpack named params.

	vals := make([]starlark.Value, numNamed)
	pairs := make([]any, 0, numNamed*2)

	for i, n := range namedParams {
		pairs = append(pairs, n, &vals[i])
	}

	if err := starlark.UnpackArgs(actionName, unpackArgs, unpackKwargs, pairs...); err != nil {
		return nil, err
	}

	// Project starlark values to their natural Go equivalents and collect
	// them in a slots map keyed by each parameter's raw Name. Target-type
	// matching is deferred to [op.Method.Invoke], which runs each slot
	// through [op.Convert] against the method's declared parameter type.
	// No starlark values survive past this boundary.

	slots := make(map[string]any, numParams)

	for i, sv := range vals {

		if sv == nil {
			continue
		}

		var val any
		if err := w.unmarshalValue(sv, reflect.ValueOf(&val).Elem()); err != nil {
			name := strings.TrimSuffix(namedParams[i], "?")
			return nil, fmt.Errorf("%s(): %s: %w", actionName, name, err)
		}
		slots[namedParams[i]] = val
	}

	// Resolve variadic parameter.

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
			if err := w.unmarshalValue(variadicList, reflect.ValueOf(&val).Elem()); err != nil {
				return nil, fmt.Errorf("%s(): %s: %w", actionName, variadicName, err)
			}
			slots[params[variadicIdx].Name] = val
		}
	}

	// Build **kwargs map.

	if kwargsName != "" {

		kwargsMap := make(map[string]any, len(extraKwargs))

		for _, kv := range extraKwargs {
			key, _ := starlark.AsString(kv[0])
			var val any
			if err := w.unmarshalValue(kv[1], reflect.ValueOf(&val).Elem()); err != nil {
				return nil, fmt.Errorf("%s(): keyword %s: %w", actionName, key, err)
			}
			kwargsMap[key] = val
		}

		slots[params[kwargsIdx].Name] = kwargsMap
	}

	// Dispatch through Method.Invoke: Go values in, Go values out. Invoke
	// runs each slot through op.Convert against the parameter's declared
	// type — string → *Resource via registry construction, []any → []string
	// via slice-lift, etc. No special cases live here.

	result, _, err := method.Invoke(w.executionContext(), w.instance, slots)
	if err != nil {
		return nil, err
	}

	// Marshal the result back to starlark. TypeByReflectionOrDerive inside
	// r.marshal handles unregistered struct returns by deriving a
	// ReceiverType on demand so any Go type round-trips symmetrically.

	if result == nil {
		return starlark.None, nil
	}

	return w.marshal(result)
}

// CompareSameType implements starlark.Comparable.
//
// Delegates to [op.Comparer] on the wrapped instance if available, otherwise falls back to Go's pointer identity
// (==). Starlark guarantees both values have the same Type() before calling this method.
//
// Parameters:
//   - cmp: the comparison operator (EQL, NEQ, LT, LE, GT, GE).
//   - y: the other value (must be *value).
//   - depth: recursion depth (unused).
//
// Returns:
//   - bool: true if the comparison holds.
//   - error: non-nil if ordering is requested (only equality is supported).
func (w *wrapper) CompareSameType(cmp syntax.Token, x starlark.Value, _ int) (bool, error) {

	other := x.(*wrapper)
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

// camelToSnake converts a CamelCase identifier to snake_case.
func camelToSnake(s string) string { return op.CamelToSnake(s) }

// getTypeInfo returns struct metadata for the given type.
//
// Parameters:
//   - t: the [reflect.Type] to introspect (pointer or struct).
//
// Returns:
//   - *typeInfo: the field metadata.
func getTypeInfo(t reflect.Type) *typeInfo {

	for t.Kind() == reflect.Pointer {
		t = t.Elem()
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
			name = camelToSnake(f.Name)
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
//
// Parameters:
//   - typeName: the type name of the value being accessed.
//   - attr: the attribute name.
//
// Returns:
//   - error: the formatted error.
func NoSuchAttrError(typeName, attr string) error {
	return fmt.Errorf("%q object has no attribute %q", typeName, attr)
}
