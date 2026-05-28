// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlarkbridge

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/NobleFactor/devlore-cli/pkg/assert"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

var (
	_ starlark.Value      = (*goReceiver)(nil) // Interface Guard: ensures *goReceiver implements starlark.Value.
	_ starlark.HasAttrs   = (*goReceiver)(nil) // Interface Guard: ensures *goReceiver implements starlark.HasAttrs.
	_ starlark.Comparable = (*goReceiver)(nil) // Interface Guard: ensures *goReceiver implements starlark.Comparable.
	_ Projector           = (*goReceiver)(nil) // Interface Guard: ensures *goReceiver implements Projector.
)

// goReceiver wraps a registered Go instance for starlark use.
//
// It implements [starlark.Value], [starlark.HasAttrs], [starlark.Comparable], and [Projector]. Attribute resolution
// checks exported struct fields first (projected to starlark via [toStarlarkReflect]); on miss, it checks the wrapped
// instance's methods (dispatched through [op.Method.Invoke]); on miss, it delegates to
// [op.AttributeResolver.ResolveAttr] when the wrapped instance implements it. A final miss surfaces as a starlark
// NoSuchAttr error via [NoSuchAttrError].
type goReceiver struct {
	receiverType op.ReceiverType
	instance     any                   // An instance of receiverType.
	methods      map[string]*op.Method // snake_name → *Method
	fields       map[string]int        // snake_name → struct field index
	attrNames    []string              // sorted (fields + methods)
}

// NewGoReceiver wraps a Go value as a starlark surface bound to its derived receiver type.
//
// The bridge derives the receiver type via [op.NewReceiverType] from `value`'s reflect type and returns a goReceiver
// carrying that type plus the wrapped instance. Use [NewProvider] when the receiver type is already known (provider
// construction); use NewGoReceiver for ad-hoc wrapping where the type must be inferred.
//
// Parameters:
//   - `value`: the Go value to wrap.
//
// Returns:
//   - [`starlark.HasAttrs`]: the bound starlark surface, ready for [goReceiver.AttrNames] / [goReceiver.Attr] /
//     [goReceiver.Type].
//   - `error`: non-nil if the receiver type cannot be derived from `value`'s reflect type.
func NewGoReceiver(value any) (starlark.HasAttrs, error) {

	receiverType, err := op.NewReceiverType(reflect.TypeOf(value), nil)
	if err != nil {
		return nil, fmt.Errorf("derive receiver type: %w", err)
	}

	return newGoReceiver(receiverType, value), nil
}

// NewProvider wraps a Go provider instance as a starlark surface bound to the supplied receiver type.
//
// The provider variant of [NewGoReceiver]: the caller has already produced (or looked up) the matching
// [op.ReceiverType] and passes it explicitly, skipping the type derivation step.
//
// Parameters:
//   - `receiverType`: the provider receiver type descriptor.
//   - `instance`: the Go provider instance.
//
// Returns:
//   - [starlark.HasAttrs]: the bound starlark surface.
func NewProvider(receiverType op.ReceiverType, instance any) starlark.HasAttrs {
	return newGoReceiver(receiverType, instance)
}

// newGoReceiver is the shared constructor for [NewGoReceiver] and [NewProvider].
//
// It builds the snake-cased method index, projects exported struct fields via [getTypeInfo], and sorts the combined
// name set for [goReceiver.AttrNames].
//
// Parameters:
//   - `receiverType`: the type descriptor whose methods populate the method index.
//   - `instance`: the wrapped Go value; its exported struct fields are projected to starlark.
//
// Returns:
//   - *goReceiver: the constructed receiver.
func newGoReceiver(receiverType op.ReceiverType, instance any) *goReceiver {

	methods := make(map[string]*op.Method)
	seen := make(map[string]bool)

	for method := range receiverType.Methods() {
		snake := op.CamelToSnake(method.Name())
		methods[snake] = method
		seen[snake] = true
	}

	fields := make(map[string]int)

	if info := getTypeInfo(reflect.TypeOf(instance)); info != nil {
		for _, fi := range info.fields {
			fields[fi.starName] = fi.index
			seen[fi.starName] = true
		}
	}

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

// String implements [starlark.Value].
//
// Delegates to the wrapped instance's [fmt.Stringer.String] when the instance satisfies it; otherwise returns the
// receiver type's name.
//
// Returns:
//   - `string`: the wrapped value's string representation, or the receiver type's name.
func (g *goReceiver) String() string {

	if stringer, ok := g.instance.(fmt.Stringer); ok {
		return stringer.String()
	}
	return g.receiverType.Name()
}

// Type implements [starlark.Value].
//
// Returns:
//   - `string`: the receiver type's name (the starlark-visible type label).
func (g *goReceiver) Type() string { return g.receiverType.Name() }

// Freeze implements [starlark.Value].
//
// goReceiver values are effectively immutable from the starlark side (mutation happens only through Go method calls
// that observe their own thread-safety contracts), so Freeze is a no-op.
func (g *goReceiver) Freeze() {}

// Truth implements [starlark.Value].
//
// All goReceiver values are truthy; the bridge never represents a "false" Go instance distinct from a
// starlark None.
//
// Returns:
//   - [starlark.Bool]: always true.
func (g *goReceiver) Truth() starlark.Bool { return true }

// Hash implements [starlark.Value].
//
// Hashable only when the wrapped instance is a Resource with a non-empty URI — the URI provides a stable identity for
// Starlark's set/dict keys. Non-Resource values are unhashable; starlark surfaces this as a runtime error.
//
// Returns:
//   - `uint32`: a stable hash derived from the Resource's URI.
//   - `error`: non-nil when the wrapped instance is not a URI-bearing [op.Resource].
func (g *goReceiver) Hash() (uint32, error) {

	if res, ok := g.instance.(op.Resource); ok {
		if uri := res.URI(); uri != "" {
			return hashString(uri), nil
		}
	}

	return 0, fmt.Errorf("unhashable type: %s", g.receiverType.Name())
}

// endregion

// region Behaviors

// Attr implements [starlark.HasAttrs].
//
// Resolution order: exported struct field → declared method → [op.AttributeResolver.ResolveAttr] delegation (when the
// wrapped instance implements it). A field hit projects through [toStarlarkReflect]; a method hit returns a
// [starlark.Builtin] bound to [goReceiver.dispatch]; a dynamic-resolver hit projects through [toStarlarkReflect]; a
// final miss returns a [NoSuchAttrError].
//
// Parameters:
//   - `name`: the snake-cased attribute name to resolve.
//
// Returns:
//   - [starlark.Value]: the resolved attribute, never nil on success.
//   - `error`: non-nil when the attribute does not exist or projection fails.
func (g *goReceiver) Attr(name string) (starlark.Value, error) {

	if idx, ok := g.fields[name]; ok {
		return g.toStarlarkReflect(elem(reflect.ValueOf(g.instance)).Field(idx))
	}

	if _, ok := g.methods[name]; ok {
		actionName := g.receiverType.Name() + "." + name
		return starlark.NewBuiltin(actionName, g.dispatch), nil
	}

	if resolver, ok := g.instance.(op.AttributeResolver); ok {
		if resolved := resolver.ResolveAttr(name); resolved != nil {
			return g.toStarlarkReflect(reflect.ValueOf(resolved))
		}
	}

	return nil, NoSuchAttrError(g.receiverType.Name(), name)
}

// AttrNames implements [starlark.HasAttrs].
//
// The returned slice aliases the precomputed sorted name set built by [newGoReceiver]; callers must not mutate it.
//
// Returns:
//   - []string: the sorted union of exported field names and declared method names.
func (g *goReceiver) AttrNames() []string { return g.attrNames }

// Project implements [Projector] by extracting a Go value of the requested target type from the wrapped instance.
//
// Delegates to [op.Convert], which routes through the registered converter cascade (Resource constructors,
// [op.SourceConverter] implementations, registered type-to-type converters, primitive assignability). The runtime
// environment is the wrapped instance's own (when it satisfies [op.Provider]) or nil otherwise.
//
// Parameters:
//   - `target`: the declared Go target type.
//
// Returns:
//   - `any`: the projected Go value.
//   - `error`: non-nil when no converter route reaches the target type.
func (g *goReceiver) Project(target reflect.Type) (any, error) {
	return op.Convert(g.runtimeEnvironment(), g.instance, target)
}

// endregion

// endregion

// region UNEXPORTED METHODS

// region State management

// runtimeEnvironment returns the [op.RuntimeEnvironment] associated with the wrapped instance.
//
// Used by [goReceiver.dispatch] to build the [op.ActivationRecord] and by [goReceiver.Project] to route conversions
// through the right registry.
//
// Returns:
//   - *op.RuntimeEnvironment: the provider's runtime environment, or nil, if the instance is not a provider.
func (g *goReceiver) runtimeEnvironment() *op.RuntimeEnvironment {

	if p, ok := g.instance.(op.Provider); ok {
		return p.RuntimeEnvironment()
	}

	return nil
}

// endregion

// region Behaviors

// toStarlark converts a Go value into a [starlark.Value] for return to starlark callers.
//
// Nil maps to [starlark.None]; values already implementing [starlark.Value] pass through; everything else routes
// through reflection-based projection via [toStarlarkReflect].
//
// Parameters:
//   - `v`: the Go value to project.
//
// Returns:
//   - [starlark.Value]: the projected starlark value.
//   - `error`: non-nil when the reflection-based projection fails.
func (g *goReceiver) toStarlark(v any) (starlark.Value, error) {

	if v == nil {
		return starlark.None, nil
	}

	if sv, ok := v.(starlark.Value); ok {
		return sv, nil
	}

	return g.toStarlarkReflect(reflect.ValueOf(v))
}

// toStarlarkMap converts a Go map (held in `rv`) into a [starlark.Dict].
//
// A nil map projects to an empty Dict. Each entry's key and value are recursively projected via [toStarlarkReflect];
// failures bubble up with the failing key in the error message for value-side failures.
//
// Parameters:
//   - `rv`: the map's [`reflect.Value`].
//
// Returns:
//   - [starlark.Value]: the projected dict.
//   - `error`: non-nil when any key or value fails projection or [starlark.Dict.SetKey] fails.
func (g *goReceiver) toStarlarkMap(rv reflect.Value) (starlark.Value, error) {

	if rv.IsNil() {
		return starlark.NewDict(0), nil
	}

	dict := starlark.NewDict(rv.Len())
	iter := rv.MapRange()

	for iter.Next() {

		key, err := g.toStarlarkReflect(iter.Key())

		if err != nil {
			return nil, fmt.Errorf("map key: %w", err)
		}

		val, err := g.toStarlarkReflect(iter.Value())

		if err != nil {
			return nil, fmt.Errorf("map value for %v: %w", iter.Key().Interface(), err)
		}

		if err := dict.SetKey(key, val); err != nil {
			return nil, fmt.Errorf("dict set: %w", err)
		}
	}

	return dict, nil
}

// toStarlarkReflect converts a `[reflect.Value]` of a Go type into a `[starlark.Value]`.
//
// Pointers and interfaces are dereferenced via [elem]; a nil pointer or interface projects to [starlark.None].
// Primitives map directly to their starlark counterparts. Slices of bytes become [starlark.Bytes]; other slices recurse
// through [goReceiver.toStarlarkSlice]; maps recurse through [goReceiver.toStarlarkMap]; structs are wrapped in a new
// goReceiver bound to the appropriate [op.ReceiverType] (looked up via the runtime environment's registry when
// available, otherwise derived fresh).
//
// Parameters:
//   - `rv`: the `[reflect.Value]` to project.
//
// Returns:
//   - `starlark.Value`: the projected starlark value.
//   - `error`: non-nil when the projection fails or the value's kind has no starlark representation.
func (g *goReceiver) toStarlarkReflect(rv reflect.Value) (starlark.Value, error) {

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

		return g.toStarlarkSlice(rv)

	case reflect.Map:
		return g.toStarlarkMap(rv)

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

		runtimeEnvironment := g.runtimeEnvironment()

		if runtimeEnvironment != nil && runtimeEnvironment.ReceiverRegistry != nil {
			receiverType := runtimeEnvironment.ReceiverRegistry.TypeByReflectionOrDerive(ptr.Type())
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

// toStarlarkSlice converts a Go slice (held in `rv`) into a [starlark.List].
//
// A nil slice projects to an empty List; non-nil slices recurse element-by-element through [toStarlarkReflect]. Errors
// include the failing index for diagnostics.
//
// Parameters:
//   - `rv`: the slice's reflect value.
//
// Returns:
//   - `starlark.Value`: the projected list.
//   - `error`: non-nil when any element fails projection.
func (g *goReceiver) toStarlarkSlice(rv reflect.Value) (starlark.Value, error) {

	if rv.IsNil() {
		return starlark.NewList(nil), nil
	}

	elems := make([]starlark.Value, rv.Len())

	for i := range rv.Len() {

		val, err := g.toStarlarkReflect(rv.Index(i))

		if err != nil {
			return nil, fmt.Errorf("slice index %d: %w", i, err)
		}

		elems[i] = val
	}

	return starlark.NewList(elems), nil
}

// dispatch is the [starlark.Builtin] body that backs every method call on a goReceiver-wrapped instance.
//
// The flow:
//
//  1. Resolve the [*op.Method] from the builtin name's snake-cased tail.
//  2. Classify the method's parameters into named / variadic / kwargs.
//  3. Partition the incoming kwargs into known names (passed to [starlark.UnpackArgs]) and extras (collected for the
//     **kwargs sink or rejected when the method declares none).
//  4. Build the [op.Method.Invoke] slot map from unpacked values, with [op.DeferredDefault] resolved against the live
//     runtime environment for absent kwargs that declare a default.
//  5. Fold the variadic positional + keyword forms into a single [*starlark.List] slot when the method declares a
//     variadic parameter.
//  6. Collect remaining extras into the **kwargs slot when the method declares one.
//  7. Build a non-graph [*op.ActivationRecord] (immediate-mode dispatch has no graph node, so `Graph` and `Unit`
//     are both nil) via [op.NewActivationRecord] and call [op.Method.Invoke]. Resources interned by the dispatch
//     carry an empty producer stamp.
//  8. Project the result back to starlark via [goReceiver.toStarlark]; nil result → [starlark.None].
//
// Parameters:
//   - `_`: the [starlark.Thread] (unused — dispatch is synchronous from the bridge's perspective).
//   - `builtin`: the [starlark.Builtin] whose name carries the qualified action name (`<provider>.<method>`).
//   - `args`: positional arguments from the starlark call.
//   - `kwargs`: keyword arguments from the starlark call.
//
// Returns:
//   - [starlark.Value]: the projected return value from [op.Method.Invoke].
//   - `error`: non-nil when unpacking or default value resolution fails, args or kwargs are misused, or conversion,
//     method invocation, or final projection fails.
func (g *goReceiver) dispatch(
	_ *starlark.Thread,
	builtin *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {

	actionName := builtin.Name()
	name := actionName[strings.LastIndex(actionName, ".")+1:]
	method := g.methods[name]
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

		// starlark.UnpackArgs uses a trailing "?" on the pair name to mark a kwarg optional. namedParams carries clean
		// names. Here we reconstruct the "?" suffix so UnpackArgs sees the optional convention.

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

			// Truly absent kwarg — fill from the parameter's declared default if one exists. Literal-form defaults
			// arrive already typed (parseDefaultExpression widens via reflect.Value.Convert at announcement time);
			// deferred-default forms (op.DeferredDefault) resolve here against the live runtime environment and the
			// already-filled sibling slots in the slot map.

			if namedDefaults[i] != nil {
				value := namedDefaults[i]
				if d, ok := value.(op.DeferredDefault); ok {
					resolved, err := d.Resolve(g.runtimeEnvironment(), slots, namedTypes[i])
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

	// Immediate-mode starlark dispatch (codegen, REPL, ad-hoc calls) has no graph in scope: there's
	// no Graph to walk and no Unit to stamp. [op.ResourceCatalog.GetOrCreate] interns Resources
	// produced by this dispatch with an empty producer stamp (no lineage edge); graph dispatch goes
	// through the executor, which constructs activations with both Graph and Unit set.

	runtimeEnvironment := assert.NonZero("goReceiver.runtimeEnvironment", g.runtimeEnvironment())
	activationRecord := op.NewActivationRecord(nil, nil, runtimeEnvironment)

	result, _, err := method.Invoke(activationRecord, g.instance, slots)
	if err != nil {
		return nil, err
	}

	if result == nil {
		return starlark.None, nil
	}

	return g.toStarlark(result)
}

// CompareSameType implements [starlark.Comparable].
//
// Supports only [syntax.EQL] and [syntax.NEQ]. Equality delegates to [op.Comparer.Equal] when the wrapped instance
// implements it; otherwise compares the underlying any-typed `instance` fields by Go `==`. Ordering operators (`<`,
// `<=`, `>`, `>=`) are rejected with a clear error.
//
// Parameters:
//   - `cmp`: the comparison operator.
//   - `x`: the right-hand side; must be a goReceiver of the same type (enforced by Starlark's same-type contract).
//   - `_`: the comparison depth limit (unused; goReceiver equality is shallow).
//
// Returns:
//   - `bool`: the comparison result.
//   - `error`: non-nil when `cmp` is not EQL or NEQ.
func (g *goReceiver) CompareSameType(cmp syntax.Token, x starlark.Value, _ int) (bool, error) {

	other := x.(*goReceiver)
	var equal bool

	if c, ok := g.instance.(op.Comparer); ok {
		equal = c.Equal(other.instance)
	} else {
		equal = g.instance == other.instance
	}

	switch cmp {
	case syntax.EQL:
		return equal, nil
	case syntax.NEQ:
		return !equal, nil
	default:
		return false, fmt.Errorf("%s not supported between %q values", cmp, g.Type())
	}
}

// endregion

// endregion

// region UNEXPORTED FUNCTIONS

// region Types

// typeInfo holds reflect-derived metadata about a Go struct for starlark field projection.
//
// Built lazily by [getTypeInfo] from a struct's [`reflect.Type`].
type typeInfo struct {
	fields []fieldInfo
}

// fieldInfo maps a single exported Go struct field to its starlark-visible name.
//
// `index` is the field's index for [reflect.Value.Field]; `starName` is the snake-cased Go field name (or the value of
// the `starlark` struct tag when present and not `"-"`).
type fieldInfo struct {
	index    int
	starName string
}

// endregion

// region Helpers

// elem returns the concrete value behind any number of pointer / interface indirections.
//
// Walks the chain until either a non-pointer / non-interface kind or a nil pointer / interface is reached. A nil
// terminator short-circuits the walk so callers can test [reflect.Value.IsNil] on the returned value.
//
// Parameters:
//   - `v`: the [`reflect.Value`] to unwrap.
//
// Returns:
//   - [`reflect.Value`]: the deepest concrete value reachable through pointer / interface indirection.
func elem(v reflect.Value) reflect.Value {

	for v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return v
		}
		v = v.Elem()
	}

	return v
}

// getTypeInfo derives the [typeInfo] for `t` by walking its exported fields.
//
// Pointer types are dereferenced before introspection; non-struct types yield nil so callers can short-circuit.
// Per-field starlark naming uses the `starlark` struct tag when present (skipping fields tagged `"-"`); absent tags
// fall back to [op.CamelToSnake] on the Go field name.
//
// Parameters:
//   - `t`: the [`reflect.Type`] to introspect.
//
// Returns:
//   - `*typeInfo`: the field metadata, or nil for non-struct types.
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

// hashString returns a stable DJB2-style hash of `s` for use as a [starlark.Value.Hash] return value.
//
// Parameters:
//   - `s`: the input string.
//
// Returns:
//   - `uint32`: the hash value.
func hashString(s string) uint32 {

	var hash uint32

	for _, c := range s {
		hash = hash*31 + uint32(c)
	}

	return hash
}

// NoSuchAttrError returns a starlark-style "no such attribute" error for a given type and attribute name.
//
// Centralized so the wording and quoting stay consistent across the bridge.
//
// Parameters:
//   - `typeName`: the receiver type's name.
//   - `attr`: the missing attribute name.
//
// Returns:
//   - `error`: a formatted error suitable for return from [starlark.HasAttrs.Attr].
func NoSuchAttrError(typeName, attr string) error {
	return fmt.Errorf("%q object has no attribute %q", typeName, attr)
}

// endregion

// endregion
