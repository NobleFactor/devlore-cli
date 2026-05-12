// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"fmt"
	"iter"
	"reflect"
	"strconv"
	"strings"
	"sync"
)

// ReceiverType is the base interface for all receiver descriptors.
//
// Callers that need provider-specific or resource-specific behavior assert to [ProviderReceiverType] or
// [ResourceReceiverType].
type ReceiverType interface {
	Name() string
	ProviderType() reflect.Type
	Methods() iter.Seq[*Method]
	MethodByName(name string) (*Method, bool)
	Do(method string, receiver any, args []any) (reflect.Value, reflect.Value, error)
}

// ProviderReceiverType extends [ReceiverType] with provider-specific capabilities.
type ProviderReceiverType interface {
	ReceiverType
	Roles() ProviderRole
	Construct() ProviderConstructor
}

// ProviderRole declares what roles a provider supports.
//
// ProviderRole is a bitflag partitioned into two zones:
//
//   - Dispatch zone (bits 0–7) declares how the provider's methods are invoked. Providers must set at least one bit in
//     this zone; [AnnounceProvider] panics otherwise.
//   - Placement zone (bits 8–15) modifies where the provider's methods surface in starlark. Orthogonal to the dispatch
//     zone; optional.
//
// [ProviderRole.Dispatch] and [ProviderRole.Placement] project a role value onto its respective zone.
type ProviderRole uint

// Dispatch zone — bits 0–7. Declares how the provider's methods are invoked.
const (
	// RoleModule declares a provider as an immediate-mode starlark global.
	RoleModule ProviderRole = 1 << iota

	// RoleAction declares a provider as a plan-mode graph node creator.
	RoleAction

	// Bits 2–7 reserved for future dispatch modes.
)

// Placement zone — bits 8–15. Modifies where the provider's methods surface in starlark.
const (
	// RoleRoot declares that the provider's methods surface flat at their access-defined namespace root, rather than
	// nested under the provider's own name. For a RoleAction provider, this means the methods appear directly under
	// plan.* (e.g., plan.choose) rather than plan.<provider>.* (e.g., plan.flow.choose). For a RoleModule provider,
	// this means the methods appear as top-level starlark globals (e.g., note()) rather than under the provider name
	// (e.g., ui.note()).
	RoleRoot ProviderRole = 1 << (iota + 8)

	// Bits 9–15 reserved for future placement modifiers.
)

// Zone masks. Callers use these to extract the dispatch or placement bits from a role value.
const (
	roleDispatchMask  ProviderRole = 0x00FF
	rolePlacementMask ProviderRole = 0xFF00
)

// Dispatch returns the dispatch-zone bits of r — which execution modes the provider supports.
//
// Returns:
//   - ProviderRole: the role value masked to the dispatch zone.
func (r ProviderRole) Dispatch() ProviderRole { return r & roleDispatchMask }

// Placement returns the placement-zone bits of r — how the provider's methods are placed in the namespace.
//
// Returns:
//   - ProviderRole: the role value masked to the placement zone.
func (r ProviderRole) Placement() ProviderRole { return r & rolePlacementMask }

// ResourceReceiverType extends [ReceiverType] with resource-specific capabilities.
//
// Resources are data types that flow through starlark code or an execution graph. They are constructed by coercing a
// raw value (e.g., a string path becomes a file.Resource).
type ResourceReceiverType interface {
	ReceiverType
	Construct() ResourceConstructor
}

// ProviderConstructor creates a provider instance bound to the given execution context.
type ProviderConstructor func(ctx *RuntimeEnvironment) (any, error)

// ResourceConstructor coerces a value into a typed resource.
//
// Parameters:
//   - ctx: the execution context.
//   - value: type-specific input (e.g., a string path for file, or []byte / [io.Reader] / URI string for mem).
//
// Returns:
//   - Resource: the constructed resource.
//   - error:    non-nil if construction fails.
type ResourceConstructor func(ctx *RuntimeEnvironment, value any) (Resource, error)

// receiverType holds the fields common to all receiver descriptors.
//
// It can be used to represent any Go type by reflection.
type receiverType struct {
	providerType  reflect.Type
	name          string
	methods       []*Method
	methodMap     map[string]*Method
	dispatchTable *sync.Map
}

// dispatcher is the cached dispatch closure signature.
type dispatcher func(receiver any, args []any) (reflect.Value, reflect.Value, error)

// NewReceiverType creates a ReceiverType for an arbitrary Go type via reflection.
//
// Parameters:
//   - providerType: the Go type (pointer or struct).
//   - methodParameters: parsed Parameter values per Go method, or nil for positional names. The wire-form
//     parameter tokens are cracked into Parameter values upstream by parseParameters at the announce boundary.
//
// Returns:
//   - ReceiverType: the receiver type descriptor.
//   - error: non-nil if the type cannot be introspected.
func NewReceiverType(providerType reflect.Type, methodParameters map[string][]Parameter) (ReceiverType, error) {

	base, err := newReceiverType(providerType, methodParameters, false)
	if err != nil {
		return nil, err
	}
	return &base, nil
}

// MethodByName returns the method registered under the given Go name.
//
// Parameters:
//   - name: the Go method name (CamelCase, e.g., "WriteText").
//
// Returns:
//   - *Method: the method descriptor.
//   - bool: true if found.
func (t *receiverType) MethodByName(name string) (*Method, bool) {
	m, ok := t.methodMap[name]
	return m, ok
}

// Methods returns an iterator over the receiver's methods, sorted by name.
//
// Returns:
//   - iter.Seq[*Method]: the method iterator.
func (t *receiverType) Methods() iter.Seq[*Method] {

	return func(yield func(*Method) bool) {
		for _, m := range t.methods {
			if !yield(m) {
				return
			}
		}
	}
}

// ProviderType returns the reflect.Type of the provider or resource struct.
//
// Returns:
//   - reflect.Type: the type.
func (t *receiverType) ProviderType() reflect.Type { return t.providerType }

// ReceiverName returns the name used to identify this receiver in starlark.
//
// Returns:
//   - string: the receiver name (e.g., "file", "file.Resource").
func (t *receiverType) Name() string { return t.name }

// Do invokes the named method on receiver with the given arguments.
//
// On the first call for a given method, Do builds and caches an optimized dispatch closure that captures the method's
// reflect.Func, pre-computed zero values for nil arguments, and the return extraction path. Subsequent calls bypass all
// per-method setup and go straight through the cached closure.
//
// Parameters:
//   - method: the Go method name (CamelCase, e.g., "WriteText").
//   - receiver: the Go object to invoke the method on.
//   - args: positional arguments (nil entries become zero values of the parameter type).
//
// Returns:
//   - reflect.Value: the result (invalid if the method returns nothing).
//   - reflect.Value: the compensation state (invalid unless compensable).
//   - error: the method's error return, or a lookup error if the method doesn't exist.
func (t *receiverType) Do(method string, receiver any, args []any) (reflect.Value, reflect.Value, error) {

	fn, ok := t.dispatchTable.Load(method)

	if !ok {
		m, found := t.methodMap[method]
		if !found {
			return reflect.Value{}, reflect.Value{}, fmt.Errorf("%s has no method %s", t.name, method)
		}
		fn, _ = t.dispatchTable.LoadOrStore(method, compileDispatcher(m))
	}

	return fn.(dispatcher)(receiver, args)
}

// providerReceiverType is the concrete descriptor for providers.
type providerReceiverType struct {
	receiverType
	roles     ProviderRole
	construct ProviderConstructor
}

// NewProviderReceiverType creates a [ProviderReceiverType] from a provider's reflect.Type and its capabilities.
//
// Parameters:
//   - providerType: the provider's reflect.Type.
//   - construct: creates a provider instance from RuntimeEnvironment.
//   - roles: the provider's declared roles (RoleModule, RoleAction, or both).
//   - methodParameters: parsed Parameter values per Go method. The wire-form parameter tokens are cracked into
//     Parameter values upstream by parseParameters at the announce boundary.
//
// Returns:
//   - ProviderReceiverType: the descriptor.
//   - error: non-nil if method classification fails.
func NewProviderReceiverType(
	providerType reflect.Type,
	construct ProviderConstructor,
	roles ProviderRole,
	methodParameters map[string][]Parameter,
) (ProviderReceiverType, error) {

	base, err := newReceiverType(providerType, methodParameters, true)
	if err != nil {
		return nil, err
	}

	return &providerReceiverType{
		receiverType: base,
		construct:    construct,
		roles:        roles,
	}, nil
}

// region EXPORTED METHODS

// region State management

// Construct returns the provider constructor function.
//
// Returns:
//   - ProviderConstructor: the constructor.
func (rt *providerReceiverType) Construct() ProviderConstructor { return rt.construct }

// Roles returns the provider's declared roles.
//
// Returns:
//   - ProviderRole: the role flags.
func (rt *providerReceiverType) Roles() ProviderRole { return rt.roles }

// endregion

// endregion

// resourceReceiverType is the concrete descriptor for resources.
type resourceReceiverType struct {
	receiverType
	construct ResourceConstructor
}

// NewResourceReceiverType creates a [ResourceReceiverType] from a resource's reflect.Type.
//
// Parameters:
//   - resourceType: the resource's reflect.Type.
//   - construct: coerces a raw value into the typed resource.
//   - methodParameters: starlark parameter names per Go method.
//
// Returns:
//   - ResourceReceiverType: the descriptor.
//   - error: non-nil if method classification fails.
func NewResourceReceiverType(
	resourceType reflect.Type,
	construct ResourceConstructor,
	methodParameters map[string][]Parameter,
) (ResourceReceiverType, error) {

	base, err := newReceiverType(resourceType, methodParameters, false)
	if err != nil {
		return nil, err
	}

	return &resourceReceiverType{
		receiverType: base,
		construct:    construct,
	}, nil
}

// region EXPORTED METHODS

// region State management

// Construct returns the resource constructor (coercion function).
//
// Returns:
//   - ResourceConstructor: the constructor.
func (rt *resourceReceiverType) Construct() ResourceConstructor { return rt.construct }

// endregion

// endregion

// newReceiverType builds the shared descriptor fields from a reflect.Type and method parameter map.
//
// Parameters:
//   - providerType: the reflect.Type of the provider or resource.
//   - methodParameters: parsed Parameter values per Go method, or nil for positional auto-naming. The wire
//     grammar is cracked upstream by parseParameters; newReceiverType consumes typed Parameter values only.
//   - isProvider: true if this receiver is an op.Provider (enables companion lookup).
//
// Returns:
//   - receiverType: the populated base.
//   - error: non-nil if method classification fails.
func newReceiverType(providerType reflect.Type, methodParameters map[string][]Parameter, isProvider bool) (receiverType, error) {

	name := receiverName(providerType)

	// Reject reserved parameter names before building the method set. The planner uses "options", "args", and
	// "kwargs" for cross-cutting concerns and for the variadic markers; provider methods cannot claim any of
	// them as plain parameter names.
	for methodName, params := range methodParameters {
		for _, p := range params {
			if err := reservedParameterError(p); err != nil {
				return receiverType{}, fmt.Errorf("provider %s method %s: %w", name, methodName, err)
			}
		}
	}

	methods := make([]*Method, 0, len(methodParameters))
	methodMap := make(map[string]*Method, len(methodParameters))

	// Use the pointer type so pointer-receiver methods are visible to reflect.
	methodType := providerType
	if methodType.Kind() != reflect.Ptr {
		methodType = reflect.PointerTo(methodType)
	}

	if methodParameters != nil {

		for reflectedMethod := range methodType.Methods() {

			if parameters, ok := methodParameters[reflectedMethod.Name]; ok {

				method, err := methodFromReflectedMethod(methodType, reflectedMethod, parameters, isProvider)

				if err != nil {
					return receiverType{}, err
				}

				methodMap[method.Name()] = method
				methods = append(methods, method)
			}
		}

	} else {

		for reflectedMethod := range methodType.Methods() {

			// Skip the receiver (index 0) and any framework-injected first parameter (*ActivationRecord at index 1)
			// when synthesizing auto-positional names. Mirrors the detection rule in [Method.NewMethod]
			// (firstParamIsActivation).
			startIdx := 1
			if reflectedMethod.Type.NumIn() >= 2 && reflectedMethod.Type.In(1) == activationRecordType {
				startIdx = 2
			}
			numParams := reflectedMethod.Type.NumIn() - startIdx
			parameters := make([]Parameter, numParams)

			for i := range numParams {
				parameters[i] = Parameter{Name: strconv.Itoa(i), Type: reflectedMethod.Type.In(startIdx + i)}
			}

			method, err := methodFromReflectedMethod(providerType, reflectedMethod, parameters, isProvider)

			if err != nil {
				return receiverType{}, err
			}

			methodMap[method.Name()] = method
			methods = append(methods, method)
		}
	}

	return receiverType{
		providerType:  providerType,
		name:          name,
		methods:       methods,
		methodMap:     methodMap,
		dispatchTable: &sync.Map{},
	}, nil
}

// region HELPER FUNCTIONS

// reservedParameterError returns an error if p declares a parameter name reserved by the planner.
//
// The name "options" is always reserved — providers cannot declare it in any form. The names "args" and
// "kwargs" are reserved as plain parameter names; the variadic marker (`*args`) and the kwargs marker
// (`**kwargs`) remain valid as catch-all positional and catch-all keyword parameters respectively. Variadic and
// kwargs flags exempt the corresponding name; everything else (Optional, plain, Optional+Default) is rejected
// when the name matches a reserved identifier.
//
// Parameters:
//   - p: the parsed Parameter to check.
//
// Returns:
//   - error: non-nil with a descriptive message when p names a reserved identifier; nil otherwise.
func reservedParameterError(p Parameter) error {

	if p.Variadic && p.Name == "args" {
		return nil
	}
	if p.Kwargs && p.Name == "kwargs" {
		return nil
	}

	switch p.Name {
	case "options":
		return fmt.Errorf("declares reserved parameter %q (name reserved for the planner's options kwarg)", p.Name)
	case "args":
		return fmt.Errorf("declares reserved parameter %q (name reserved; variadic positionals must be spelled %q)", p.Name, "*args")
	case "kwargs":
		return fmt.Errorf("declares reserved parameter %q (name reserved; keyword catch-alls must be spelled %q)", p.Name, "**kwargs")
	}

	return nil
}

// compileDispatcher creates an optimized dispatch closure for a method.
//
// The closure captures the method's reflect.Func, pre-computed zero values, the variadic flag, and the return
// extraction path so that none of this work repeats on subsequent calls.
func compileDispatcher(m *Method) dispatcher {

	fn := m.do.Func
	numParams := m.do.Type.NumIn() - 1
	isVariadic := m.do.Type.IsVariadic()

	// Pre-compute zero values for each parameter type.

	zeros := make([]reflect.Value, numParams)

	for i := range numParams {
		zeros[i] = reflect.Zero(m.do.Type.In(i + 1))
	}

	// Select the return extraction path once.

	callMethod := func(args []reflect.Value) []reflect.Value {
		if isVariadic {
			return fn.CallSlice(args)
		}
		return fn.Call(args)
	}

	packArgs := func(receiver any, args []any) []reflect.Value {

		values := make([]reflect.Value, len(args)+1)
		values[0] = reflect.ValueOf(receiver)

		for i, arg := range args {
			if arg == nil {
				values[i+1] = zeros[i]
			} else {
				values[i+1] = reflect.ValueOf(arg)
			}
		}

		return values
	}

	toErrorOrNil := func(rv reflect.Value) error {
		if rv.IsNil() {
			return nil
		}
		return rv.Interface().(error)
	}

	switch m.kind {
	case MethodAction:
		return func(receiver any, args []any) (reflect.Value, reflect.Value, error) {
			callMethod(packArgs(receiver, args))
			return reflect.Value{}, reflect.Value{}, nil
		}

	case MethodFallibleAction:
		return func(receiver any, args []any) (reflect.Value, reflect.Value, error) {
			results := callMethod(packArgs(receiver, args))
			return reflect.Value{}, reflect.Value{}, toErrorOrNil(results[0])
		}

	case MethodFunction:
		return func(receiver any, args []any) (reflect.Value, reflect.Value, error) {
			results := callMethod(packArgs(receiver, args))
			return results[0], reflect.Value{}, nil
		}

	case MethodFallibleFunction:
		return func(receiver any, args []any) (reflect.Value, reflect.Value, error) {
			results := callMethod(packArgs(receiver, args))
			return results[0], reflect.Value{}, toErrorOrNil(results[1])
		}

	case MethodCompensableFunction:
		return func(receiver any, args []any) (reflect.Value, reflect.Value, error) {
			results := callMethod(packArgs(receiver, args))
			return results[0], results[1], toErrorOrNil(results[2])
		}
	}

	panic("unreachable")
}

// methodFromReflectedMethod creates a [Method] from a reflected Go method on a receiver type.
//
// Companion discovery is automatic:
//   - If a method with name "<Name>Planned" exists on the receiver type, it is attached as the plan-time output
//     spec companion. Its signature must match the forward method (same parameters) and return (T, error) where T
//     matches the forward method's first return type. [NewMethod] validates the match and returns an error on
//     mismatch.
//   - If the forward method is compensable (returns T, U, error), a method with name "Compensate<Name>" is
//     required on the receiver type. Its absence is a fatal error.
//
// Parameters:
//   - receiverType: the reflect.Type of the receiver (pointer type) for companion lookup.
//   - method: the reflected Go method.
//   - parameters: parsed Parameter values matching the method's non-receiver parameters.
//   - isProvider: true if this receiver is an op.Provider (enables companion lookup).
//
// Returns:
//   - *Method: the classified method.
//   - error: non-nil if the return signature is invalid or a required companion is missing.
func methodFromReflectedMethod(receiverType reflect.Type, method reflect.Method, parameters []Parameter, isProvider bool) (*Method, error) {

	mt := method.Type
	numOut := mt.NumOut()
	lastIsError := numOut > 0 && mt.Out(numOut-1).Implements(errorType)

	var plannedMethod *reflect.Method
	var undoMethod *reflect.Method

	if isProvider {

		// Ensure we are searching the pointer type so pointer-receiver methods are visible.
		searchType := receiverType
		if searchType.Kind() != reflect.Ptr {
			searchType = reflect.PointerTo(searchType)
		}

		if planned, ok := searchType.MethodByName(method.Name + "Planned"); ok {
			plannedMethod = &planned
		}

		if numOut == 3 && lastIsError {
			compensateName := "Compensate" + method.Name
			compensateMethod, ok := searchType.MethodByName(compensateName)
			if !ok {
				return nil, fmt.Errorf("provider %s: compensable method %s requires a '%s' companion to support recovery",
					receiverName(receiverType),
					method.Name,
					compensateName)
			}
			undoMethod = &compensateMethod
		}
	}

	return NewMethod(&method, parameters, plannedMethod, undoMethod, isProvider)
}

// deriveMethodParams discovers exported methods on a Go type and generates positional parameter names.
//
// Used by [ReceiverRegistry.TypeByReflectionOrDerive] to build a [ReceiverType] for types not pre-announced via init().
//
// Parameters:
//   - goType: the Go type to introspect (pointer or struct).
//
// Returns:
//   - map[string][]Parameter: method name → Parameter values keyed by positional name (arg0, arg1, ...).
//     Each Parameter carries the Go method's reflect.Type for the corresponding positional slot.
func deriveMethodParams(goType reflect.Type) map[string][]Parameter {

	ptrType := goType
	if ptrType.Kind() != reflect.Pointer {
		ptrType = reflect.PointerTo(ptrType)
	}

	params := make(map[string][]Parameter)

	for i := range ptrType.NumMethod() {
		m := ptrType.Method(i)
		if !m.IsExported() {
			continue
		}

		mt := m.Type
		numIn := mt.NumIn() - 1 // exclude receiver

		ps := make([]Parameter, numIn)
		for j := range numIn {
			ps[j] = Parameter{Name: fmt.Sprintf("arg%d", j), Type: mt.In(j + 1)}
		}

		params[m.Name] = ps
	}

	return params
}

func receiverName(providerType reflect.Type) string {

	if providerType.Kind() == reflect.Ptr {
		providerType = providerType.Elem()
	}

	pkgPath := providerType.PkgPath()
	pkgName := pkgPath[strings.LastIndex(pkgPath, "/")+1:]

	typeName := providerType.Name()

	switch typeName {
	case "Provider":
		return pkgName
	case "Resource":
		return pkgName + ".Resource"
	default:
		return typeName
	}
}

// endregion
