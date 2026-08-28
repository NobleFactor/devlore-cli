// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package op

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"

	"github.com/NobleFactor/devlore-cli/pkg/assert"
)

// announcements is the package-level registry of all init-time announcements: receiver types (providers,
// resources, types) and deferred-default functions.
//
// One singleton instance, [announced], is the canonical declaration registry. All Announce* and Register* entry points
// wrap announcements method calls; all snapshot consumers ([newReceiverRegistry], [RuntimeEnvironmentSpec.Build],
// parseDeferred) read through methods. Direct access to the maps from outside the type is forbidden — methods are the
// only path so the mutex contract is local and auditable.
//
// The shared mutex serializes both write sets (receiver type + DefaultFunc) and all snapshot reads against each other.
// Init-time registration is single-threaded in practice, but the mutex is cheap insurance against test fixtures and any
// future runtime-registration paths.
type announcements struct {
	mu            sync.Mutex
	receiverTypes map[string]ReceiverType
	defaultFuncs  map[string]DefaultFunc
}

// announced is the package singleton. Construction at package init only — no other instance is ever produced and the
// type is unexported precisely to enforce that.
var announced = &announcements{
	receiverTypes: make(map[string]ReceiverType),
	defaultFuncs:  make(map[string]DefaultFunc),
}

// parseFuncStub is the no-op closure shared across all [announcements.validatorStub] entries.
// [text/template/parse.Parse] only inspects map values for `reflect.Kind == Func`, never invokes them, so identity
// sharing is safe.
var parseFuncStub = func() {}

// region UNEXPORTED METHODS — announcements

// region State management

// registerReceiverType inserts rt into the receiver-type map under rt.Name().
//
// Parameters:
//   - `rt`: the receiver type to register; rt.Name() is the registry key.
//
// Returns:
//   - `error`: non-nil iff a receiver type is already registered under the same name.
func (a *announcements) registerReceiverType(rt ReceiverType) error {

	a.mu.Lock()
	defer a.mu.Unlock()

	name := rt.Name()

	if _, exists := a.receiverTypes[name]; exists {
		return fmt.Errorf("%q already announced", name)
	}

	a.receiverTypes[name] = rt
	return nil
}

// registerDefaultFunc inserts fn into the default-function map under the given name.
//
// Parameters:
//   - `name`: the identifier as it appears in directive expressions (`{{ name args }}`).
//   - `fn`:   the function to invoke at slot-fill time. Must be non-nil.
//
// Returns:
//   - `error`: non-nil if name is empty, fn is nil, or name is already registered.
func (a *announcements) registerDefaultFunc(name string, fn DefaultFunc) error {

	if name == "" {
		return fmt.Errorf("empty name")
	}

	if fn == nil {
		return fmt.Errorf("%q: nil function", name)
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	if _, exists := a.defaultFuncs[name]; exists {
		return fmt.Errorf("%q already registered", name)
	}

	a.defaultFuncs[name] = fn
	return nil
}

// endregion

// region Behaviors

// snapshotReceiverTypes returns a freshly-allocated slice of all announced receiver types.
//
// It is suitable for receiverRegistry.init to iterate and classify.
//
// Returns:
//   - `[]ReceiverType`: snapshot in arbitrary order; caller sorts or classifies as needed.
func (a *announcements) snapshotReceiverTypes() []ReceiverType {

	a.mu.Lock()
	defer a.mu.Unlock()

	out := make([]ReceiverType, 0, len(a.receiverTypes))

	for _, rt := range a.receiverTypes {
		out = append(out, rt)
	}

	return out
}

// SnapshotReceiverTypes returns a freshly-allocated slice of every announced receiver type.
//
// Intended for boot-discipline tests, code-generation tools, and introspection callers that need to enumerate the
// package-level registry from outside pkg/op. Iteration order is unspecified — callers that need a stable order must
// sort the result themselves.
//
// Returns:
//   - `[]ReceiverType`: snapshot of every receiver type currently in the registry.
func SnapshotReceiverTypes() []ReceiverType {
	return announced.snapshotReceiverTypes()
}

// ReceiverRegistry is the process-wide registry used for environment-free type resolution during starlark
// projection. It is built once from the announced set on first use; every Announce* runs at package init, so the
// snapshot is complete before any projection occurs.
var ReceiverRegistry = sync.OnceValue(newReceiverRegistry)

// defaultFunc returns the DefaultFunc registered under name.
//
// Slot-fill reads through this accessor on every `{{ funcname args }}` command in a deferred default. The funcmap is a
// process-singleton — defaults belong to the provider/resource definition (declared at the directive site by the
// package author), not to any per-runtime state, so the lookup is against the package-level [announced] registry
// directly. Validator stubs built at parse time already gate unknown names from reaching slot-fill; the bool return is
// defensive against runtime-registration races, not the primary check.
//
// Parameters:
//   - `name`: the identifier as it appears in directive expressions (`{{ name args }}`).
//
// Returns:
//   - `DefaultFunc`: the registered function, or nil if name is not registered.
//   - `bool`:        true iff name was found.
func (a *announcements) defaultFunc(name string) (DefaultFunc, bool) {

	a.mu.Lock()
	defer a.mu.Unlock()
	fn, ok := a.defaultFuncs[name]

	return fn, ok
}

// validatorStub returns a fresh map[string]any whose keys mirror the default-function registry and whose values are
// func-kind no-ops accepted by [text/template/parse.Parse] for identifier-resolution checks.
//
// Returns:
//   - `map[string]any`: parser-friendly stub map keyed by registered identifier.
func (a *announcements) validatorStub() map[string]any {

	a.mu.Lock()
	defer a.mu.Unlock()

	out := make(map[string]any, len(a.defaultFuncs))

	for name := range a.defaultFuncs {
		out[name] = parseFuncStub
	}

	return out
}

// endregion

// endregion

// AnnounceProvider registers a provider with its roles and per-method metadata.
//
// Called in init(). Surfaces are declared via [ProviderFlags]: [SurfaceScript] for methods reachable from a script,
// [SurfaceWorkflow] for methods reachable from a workflow.
//
// Companion methods on the provider type — [Method.Plan] via <Name>Planned, [Method.Undo] via Compensate<Name> —
// are discovered automatically by reflection in [NewProviderReceiverType]. No registration is required.
//
// Parameters:
//   - `providerType`: the provider's reflect.Type.
//   - `roles`: the provider's declared roles.
//   - `construct`: creates a provider instance from RuntimeEnvironment.
//   - `methods`: codegen-emitted [MethodMetadata] per Go method, keyed by the method's Go name.
func AnnounceProvider(providerType reflect.Type, flags ProviderFlags, construct ProviderConstructor, methods map[string]MethodMetadata) {

	label := fmt.Sprintf("AnnounceProvider(%s)", providerType)

	assert.Truef(flags.Surfaces() != 0,
		"%s: must declare at least one surface (SurfaceScript or SurfaceWorkflow); got %#x", label, uint(flags))

	methodParameters := make(map[string][]string, len(methods))
	planners := make(map[string]Planner, len(methods))

	for name, metadata := range methods {
		methodParameters[name] = metadata.ParameterNames
		planners[name] = plannerForType(metadata.Planner)
	}

	parsed, err := parseParameters(providerType, methodParameters)
	assert.NoError(label, err)

	rt, err := NewProviderReceiverType(providerType, construct, flags, parsed, planners)
	assert.NoError(label, err)

	// Stamp per-method surface modifiers from the codegen-emitted metadata. Unset entries default to ModifierNone.
	for name, metadata := range methods {
		if method, ok := rt.MethodByName(name); ok {
			method.setClaims(metadata.Claims)
			method.setModifiers(metadata.Modifiers)
		}
	}

	err = announced.registerReceiverType(rt)
	assert.NoError(label, err)
}

// AnnounceResource registers a resource type.
//
// Called in init(). Resources are always RoleResource — they reach no provider surface. They are data types
// constructed by coercing a raw value (e.g., a string path becomes a file.Resource).
//
// Parameters:
//   - `resourceType`: the resource's reflect.Type.
//   - `construct`: coerces a raw value into the typed resource.
//   - `methodParameters`: starlark parameter names per Go method (for attribute access).
//   - `sourceTypes`: Go source types the resource is constructed from (e.g. `*starlark.Function`); each is registered
//     as a `byType` key so [receiverRegistry.ConstructorForSource] resolves the constructor from a source value.
func AnnounceResource(
	resourceType reflect.Type,
	construct ResourceConstructor,
	methodParameters map[string][]string,
	sourceTypes ...reflect.Type,
) {

	label := fmt.Sprintf("AnnounceResource(%s)", resourceType)

	// A sealed resource announces its interface; reflection needs the struct behind it, registered by the
	// provider's own init via [RegisterResourceImplementation]. A struct announcement is its own
	// implementation, so this is a no-op for every provider that has not been sealed yet.
	implementation := resourceImplementationFor(resourceType)
	assert.Truef(implementation != nil,
		"%s: %s is an interface with no registered implementation — the provider's init must call "+
			"op.RegisterResourceImplementation before the generated announcement runs", label, resourceType)

	parsed, err := parseParameters(implementation, methodParameters)
	assert.NoError(label, err)

	rt, err := NewResourceReceiverType(resourceType, implementation, construct, parsed, sourceTypes...)
	assert.NoError(label, err)

	err = announced.registerReceiverType(rt)
	assert.NoError(label, err)
}

// AnnounceType registers a bare receiver type for an arbitrary Go struct.
//
// Called in init(). This is for Go types that need method dispatch in starlark but are neither providers nor resources
// (e.g., Go AST types returned by the goast provider). The receiver type has no constructor and no roles — it exists
// solely so marshalReflect can wrap instances with method dispatch.
//
// Parameters:
//   - `goType`: the Go struct's reflect.Type.
//   - `methods`: codegen-emitted [MethodMetadata] per Go method, keyed by the method's Go name.
func AnnounceType(goType reflect.Type, methods map[string]MethodMetadata) {

	label := fmt.Sprintf("AnnounceType(%s)", goType)

	methodParameters := make(map[string][]string, len(methods))
	for name, metadata := range methods {
		methodParameters[name] = metadata.ParameterNames
	}

	parsed, err := parseParameters(goType, methodParameters)
	assert.NoError(label, err)

	base, err := newReceiverType(goType, parsed, nil, false)
	assert.NoError(label, err)

	// Stamp per-method surface modifiers from the codegen-emitted metadata. Unset entries default to ModifierNone.
	for name, metadata := range methods {
		if method, ok := base.MethodByName(name); ok {
			method.setModifiers(metadata.Modifiers)
		}
	}

	err = announced.registerReceiverType(&base)
	assert.NoError(label, err)
}

// receiverRegistry stores receiver types in four sorted lists plus cross-cutting lookup maps.
//
// Workflows are providers reaching SurfaceWorkflow. Scripts are providers reaching SurfaceScript. Planners mirror
// workflows for the plan.* namespace. Resources are data types that flow through starlark code or an execution
// graph. A provider may appear in both workflows and scripts.
//
// The byType map enables lookup by reflect.Type for marshalReflect (wrapping Go return values) and the executor
// (dispatching graph nodes).
type receiverRegistry struct {
	workflows []ProviderReceiverType        // sorted by name; providers reaching SurfaceWorkflow
	scripts   []ProviderReceiverType        // sorted by name; providers reaching SurfaceScript
	planners  []ProviderReceiverType        // sorted by name; mirrors workflows for plan.* routing
	promoted  []ProviderReceiverType        // sorted by name; providers whose placement is PlacementPromoted
	resources []ResourceReceiverType        // sorted by name; data types
	byName    map[string]ReceiverType       // all receiver types by name
	byType    map[reflect.Type]ReceiverType // all receiver types by reflect.Type

	// mu guards byName and byType. The sorted lists need no guard: they are appended only at construction (from
	// announced providers/resources), while runtime derive-and-register touches only the maps — a derived type is a
	// plain receiverType, never a Provider/Resource, so it never reaches the list-appending switch cases.
	mu sync.RWMutex

	// productTypeOnce guards the lazy build of productTypeIndex.
	productTypeOnce sync.Once

	// productTypeIndex maps a result's canonical type id to its concrete reflect.Type, built once over every action
	// method's product return type. Resume reads it to retype a reloaded result to its produced Go type.
	productTypeIndex map[string]reflect.Type

	// compensatingActionOnce guards the lazy build of compensatingActionIndex.
	compensatingActionOnce sync.Once

	// compensatingActionIndex maps a compensating action's dotted name (e.g. "file.compensate_file_mutation") to the
	// means to invoke it, built once over every action provider's Compensate* methods. A receipt's compensatingAction
	// resolves through it so the receipt names its own undo directly; see [receiverRegistry.CompensatingActionByName].
	compensatingActionIndex map[string]compensatingAction
}

// compensatingAction is a registered Compensate* method plus the means to invoke it, keyed in the registry by its
// dotted name (provider name + snake method name).
//
// A receipt's compensatingAction resolves through [receiverRegistry.CompensatingActionByName] so the receipt names its
// own undo directly, independent of the forward-action Compensate<Name> convention.
type compensatingAction struct {
	providerReceiverType ProviderReceiverType
	compensatorType      reflect.Type // the undo-state type the action accepts; the registration-time type check

	// invoke is the typed adapter baked once at index build (step 43: reflect once at registration). It closes
	// over the resolved reflect.Method and the mandated activation-first shape (step 27's floor), so the unwind
	// path makes a plain call — no per-call reflection decisions, and a shape mismatch is caught at the one
	// guarded registration site instead of a latent reflect panic at rollback.
	invoke func(receiver any, activation *ActivationRecord, undoState any) error
}

// newReceiverRegistry creates a populated registry from all announced receivers.
//
// Returns:
//   - `*receiverRegistry`: the populated registry.
func newReceiverRegistry() *receiverRegistry {

	registry := &receiverRegistry{
		byName: make(map[string]ReceiverType),
		byType: make(map[reflect.Type]ReceiverType),
	}

	registry.init()
	return registry
}

// region EXPORTED METHODS

// region State management

// Actions returns all providers that can be deferred to graph nodes.
//
// Returns:
//   - `[]ProviderReceiverType`: sorted by receiver name.
func (r *receiverRegistry) Workflows() []ProviderReceiverType { return r.workflows }

// Modules returns all providers that can be called directly from a starlark runtime.
//
// Returns:
//   - `[]ProviderReceiverType`: sorted by receiver name.
func (r *receiverRegistry) Scripts() []ProviderReceiverType { return r.scripts }

// Planners returns all providers available in the plan.* namespace.
//
// Returns:
//   - `[]ProviderReceiverType`: sorted by receiver name.
func (r *receiverRegistry) Planners() []ProviderReceiverType { return r.planners }

// PromotedProviders returns every provider whose [Placement] is [PlacementPromoted].
//
// A promoted provider surfaces its methods at the namespace root of every surface it reaches, rather than
// qualified by its own name.
//
// The list is NOT filtered by surface, and callers are not expected to filter it. Placement applies to every
// surface a provider reaches: one reaching both surfaces is promoted on both — top-level globals for a script,
// and directly under plan.* for a workflow. ui is the case: note() in a script and plan.note() in a workflow,
// from one directive.
//
// This comment once said callers filter by surface and named plan.Provider as the example. plan.Provider does
// not filter, and the behavior described was never implemented; corrected 2026-08-27, after ui became the first
// provider both reaching every surface and promoted, which made the difference observable.
//
// Returns:
//   - `[]ProviderReceiverType`: sorted by receiver name.
func (r *receiverRegistry) PromotedProviders() []ProviderReceiverType { return r.promoted }

// Resources returns all resource data types.
//
// Returns:
//   - `[]ResourceReceiverType`: sorted by receiver name.
func (r *receiverRegistry) Resources() []ResourceReceiverType { return r.resources }

// Type returns the receiver type registered under the given name.
//
// Parameters:
//   - `name`: the receiver name (e.g., "file").
//
// Returns:
//   - `ReceiverType`: the receiver type.
//   - `bool`: true if found.
func (r *receiverRegistry) Type(name string) (ReceiverType, bool) {

	r.mu.RLock()
	defer r.mu.RUnlock()

	rt, ok := r.byName[name]
	return rt, ok
}

// TypeByReflection returns the receiver type registered for the given Go type.
//
// Parameters:
//   - t: the reflect.Type to look up (pointer or struct).
//
// Returns:
//   - `ReceiverType`: the receiver type.
//   - `bool`: true if found.
func (r *receiverRegistry) TypeByReflection(t reflect.Type) (ReceiverType, bool) {

	r.mu.RLock()
	defer r.mu.RUnlock()

	rt, ok := r.byType[t]
	return rt, ok
}

// ConstructorForSource returns the resource constructor registered for a Go source type.
//
// A resource declares its source types via [AnnounceResource]; each is keyed in byType to the resource's receiver type.
// The planner calls this to construct a resource from a bare source value (e.g. a `*starlark.Function` becomes a
// `function.Resource`) without naming the provider.
//
// Parameters:
//   - `sourceType`: the Go source value's reflect.Type.
//
// Returns:
//   - `ResourceConstructor`: the constructor; nil when no resource declares this source type.
//   - `bool`: true when a constructor is found.
func (r *receiverRegistry) ConstructorForSource(sourceType reflect.Type) (ResourceConstructor, bool) {

	r.mu.RLock()
	defer r.mu.RUnlock()

	rt, ok := r.byType[sourceType]
	if !ok {
		return nil, false
	}

	rrt, ok := rt.(ResourceReceiverType)
	if !ok {
		return nil, false
	}

	return rrt.Construct(), true
}

// ResourceConstructorByTypeID returns the resource constructor for the canonical Go type id carried in a resource
// URI's fragment (see [typeIDOf] / [ExtractTagSpecific]).
//
// Resume rebuilds each saved ledger generation's Resource object from its URI without importing the provider package:
// the URI fragment names the concrete type, which this resolves to the type's registered [ResourceConstructor]. The
// caller passes the URI's `specific` part (not the full tag URI) as the constructor value.
//
// Parameters:
//   - `typeID`: the canonical Go type id (`<pkg-path>.<Name>`) from a resource URI fragment.
//
// Returns:
//   - `ResourceConstructor`: the registered constructor for the resource type.
//   - `bool`: true when a resource type with that type id is registered.
func (r *receiverRegistry) ResourceConstructorByTypeID(typeID string) (ResourceConstructor, bool) {

	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, resourceType := range r.resources {
		if resourceType.TypeID() == typeID {
			return resourceType.Construct(), true
		}
	}

	return nil, false
}

// UnpackerByTypeID resolves the canonical Go type id carried in a content resource URI's fragment to the type's
// [Unpacker].
//
// Graph load reconstructs each document content-section entry through this: the URI fragment names the concrete
// resource type, and the [Unpacker] method set is the registration — [Unpacker.Unpack] is dispatched on a zero
// value of the announced type, so transport rides the existing announced inventory with no registry of its own.
//
// Parameters:
//   - `typeID`: the canonical Go type id (`<pkg-path>.<Name>`) from a resource URI fragment.
//
// Returns:
//   - `Unpacker`: a zero value of the resource type, ready to dispatch [Unpacker.Unpack].
//   - `bool`: true when a resource type with that type id is registered and implements [Unpacker].
func (r *receiverRegistry) UnpackerByTypeID(typeID string) (Unpacker, bool) {

	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, resourceType := range r.resources {
		if resourceType.TypeID() == typeID {
			unpacker, ok := reflect.New(resourceType.ProviderType()).Interface().(Unpacker)
			return unpacker, ok
		}
	}

	return nil, false
}

// ProductTypeByID resolves a result's recorded canonical type id to the concrete [reflect.Type] it was produced as.
//
// The index is built once over every registered action method's product return type ([Method.ResultType]); since a
// result is always a method's product, it covers every possible result type — including the leaf type behind a
// combinator's `any` return, which the static signature could never name. Resume reads it to retype a reloaded
// (untyped) result to its produced Go type.
//
// Parameters:
//   - `id`: the canonical type id recorded at [ReceiptBase.Commit] (see [canonicalID]).
//
// Returns:
//   - `reflect.Type`: the product type; nil when the id is unknown.
//   - `bool`: true when the id resolved.
func (r *receiverRegistry) ProductTypeByID(id string) (reflect.Type, bool) {

	r.productTypeOnce.Do(func() {
		index := make(map[string]reflect.Type)
		for _, providerType := range r.workflows {
			for method := range providerType.Methods() {

				productType := method.ResultType()
				if productType == nil {
					continue
				}

				index[canonicalID(productType)] = productType

				// A resource result is also keyed by its ANNOUNCED type id, which is what
				// [ReceiptBase.Commit] records — [canonicalIDOf] asks the resource rather than reflecting on
				// it. The two spellings differ by the pointer star for an unsealed provider and by the whole
				// name for a sealed one, so indexing both is what lets a receipt resolve its own product type
				// either way.
				if productType.Implements(resourceInterfaceType) {
					index[typeIDOf(productType)] = productType
				}
			}
		}
		r.productTypeIndex = index
	})

	productType, ok := r.productTypeIndex[id]
	return productType, ok
}

// CompensatingActionByName resolves a compensating action's dotted name to its registered Compensate* method.
//
// The index is built lazily over every action provider's exported Compensate* methods (keyed by provider name + "." +
// snake(Compensate<X>)). A receipt's compensatingAction is resolved through it first; callers fall back to the
// forward-action Compensate<Name> path when it misses (a not-yet-migrated provider whose compensatingAction is still a
// dispatch action).
//
// Parameters:
//   - `name`: the dotted compensating-action name — provider name + "." + snake(Compensate<X>).
//
// Returns:
//   - `compensatingAction`: the resolved compensating action.
//   - `bool`: false when no compensating action is registered under that name.
func (r *receiverRegistry) CompensatingActionByName(name string) (compensatingAction, bool) {

	r.compensatingActionOnce.Do(func() {
		index := make(map[string]compensatingAction)
		for _, providerType := range r.workflows {
			reflectType := providerType.ProviderType()
			if reflectType.Kind() != reflect.Pointer {
				reflectType = reflect.PointerTo(reflectType)
			}
			for i := 0; i < reflectType.NumMethod(); i++ {
				method := reflectType.Method(i)
				if !strings.HasPrefix(method.Name, "Compensate") {
					continue
				}
				funcType := method.Func.Type()

				// The required floor (step 27): a compensating action is dispatched with an activation in hand;
				// its shape is (receiver, *ActivationRecord, compensator). A nonconforming Compensate* method is
				// a programming error — codegen rejects it at generation time; this backstop catches
				// hand-announced types.
				assert.Truef(funcType.NumIn() >= 3 && funcType.In(1) == activationRecordType,
					"compensating action %s.%s must declare *ActivationRecord as its first parameter (step 27)",
					providerType.Name(), method.Name)

				fn := method.Func
				index[providerType.Name()+"."+CamelToSnake(method.Name)] = compensatingAction{
					providerReceiverType: providerType,
					compensatorType:      funcType.In(funcType.NumIn() - 1),
					invoke: func(receiver any, activation *ActivationRecord, undoState any) error {
						results := fn.Call([]reflect.Value{
							reflect.ValueOf(receiver),
							reflect.ValueOf(activation),
							reflect.ValueOf(undoState),
						})
						return errorFromValue(results[0])
					},
				}
			}
		}
		r.compensatingActionIndex = index
	})

	comp, ok := r.compensatingActionIndex[name]
	return comp, ok
}

// TypeByReflectionOrDerive returns the receiver type for the given Go type, deriving one via reflection if necessary.
//
// Announced types (via [AnnounceProvider], [AnnounceResource], [AnnounceType]) are returned as-is. Unannounced types
// get a derived [ReceiverType] with positional parameter names (arg0, arg1, ...) and are registered for future lookups.
//
// Parameters:
//   - t: the reflect.Type to look up or derive (pointer or struct).
//
// Returns:
//   - `ReceiverType`: the receiver type descriptor.
func (r *receiverRegistry) TypeByReflectionOrDerive(reflectType reflect.Type) ReceiverType {

	// Fast path: an announced (or previously-derived) type resolves under a read lock — a pure map read that
	// concurrent dispatch can run in parallel.

	r.mu.RLock()
	rt := r.lookupLocked(reflectType)
	r.mu.RUnlock()

	if rt != nil {
		return rt
	}

	// Miss: derive via reflection and register under the write lock. Re-check first — a concurrent caller may have
	// derived the same type between the read-lock release and the write-lock acquisition.

	r.mu.Lock()
	defer r.mu.Unlock()

	if rt := r.lookupLocked(reflectType); rt != nil {
		return rt
	}

	derived, err := NewReceiverType(reflectType, deriveMethodParams(reflectType))

	if err != nil {
		//nolint:errcheck // diagnose-ignored-error: params fallback; see docs/architecture/2.8-eventing-infrastructure.md
		derived, _ = NewReceiverType(reflectType, nil)
	}

	if derived != nil {
		r.registerLocked(derived)
	}

	return derived
}

// lookupLocked resolves reflectType against byType, checking the alternate pointer↔struct form (announced types may
// be stored under the struct type while callers pass the pointer type, or vice versa). The caller must hold r.mu for
// reading or writing.
//
// Parameters:
//   - `reflectType`: the reflect.Type to look up (pointer or struct).
//
// Returns:
//   - `ReceiverType`: the registered receiver type, or nil if neither form is registered.
func (r *receiverRegistry) lookupLocked(reflectType reflect.Type) ReceiverType {

	if rt, ok := r.byType[reflectType]; ok {
		return rt
	}

	if reflectType.Kind() == reflect.Pointer {
		if rt, ok := r.byType[reflectType.Elem()]; ok {
			return rt
		}
	} else {
		if rt, ok := r.byType[reflect.PointerTo(reflectType)]; ok {
			return rt
		}
	}

	return nil
}

// endregion

// region Behaviors

// ActionByPath finds the action method whose canonical name matches.
//
// Performs a linear scan over every registered action provider's methods and matches against [Method.ActionName]. This
// is the fully-qualified <pkg-path>.<receiverName>.<methodName> form stored on receipts after [ReceiptBase.Commit]). It
// is used by [RecoveryStack.Unwind] to locate the [Compensate] companion for each receipt-bearing entry and by
// recovery-ledger reload to bind closures to receipts deserialized from disk.
//
// Parameters:
//   - `name`: the canonical action name returned by [Method.ActionName].
//
// Returns:
//   - `ProviderReceiverType`: the provider that owns the matched method.
//   - `*Method`: the matched method.
//   - `bool`: true if a match was found.
func (r *receiverRegistry) ActionByPath(name string) (ProviderReceiverType, *Method, bool) {

	for _, rt := range r.byName {

		prt, ok := rt.(ProviderReceiverType)
		if !ok || prt.Flags().Surfaces()&SurfaceWorkflow == 0 {
			continue
		}

		for m := range prt.Methods() {
			if m.ActionName() == name {
				return prt, m, true
			}
		}
	}

	return nil, nil, false
}

// ActionByName returns the action provider registered under the given name.
//
// Parameters:
//   - `name`: the receiver name (e.g., "file").
//
// Returns:
//   - `ProviderReceiverType`: the provider.
//   - `bool`: true if found.
func (r *receiverRegistry) ActionByName(name string) (ProviderReceiverType, bool) {

	rt, ok := r.byName[name]
	if !ok {
		return nil, false
	}

	prt, ok := rt.(ProviderReceiverType)
	if !ok {
		return nil, false
	}

	if prt.Flags().Surfaces()&SurfaceWorkflow == 0 {
		return nil, false
	}

	return prt, true
}

// BuildAction looks up an [Action] by its name (e.g., "file.write_text") and constructs it via [NewAction].
//
// Registry-only: no [RuntimeEnvironment] required. Plan-time writers (planners, graph builders, migration plan
// builders)/ that hold a [*receiverRegistry] use this to bind an [Action] onto a fresh Node at construction time.
//
// The returned [Action]'s `Do` method consumes a [RuntimeEnvironment] at dispatch time (via the activation record).
// This constructor only needs the registry to resolve the provider type and method descriptor.
//
// Parameters:
//   - `name`: the short dotted action label (e.g., "file.copy").
//
// Returns:
//   - `Action`: the constructed action.
//   - `error`: non-nil if name has no dot, the receiver isn't a registered action provider, or the method isn't found
//     on that provider.
func (r *receiverRegistry) BuildAction(name ActionName) (Action, error) {

	dot := strings.LastIndex(string(name), ".")

	if dot < 0 {
		return nil, fmt.Errorf("invalid action name %q: no dot", name)
	}

	receiverName := string(name[:dot])
	methodSnake := string(name[dot+1:])

	prt, ok := r.ActionByName(receiverName)
	if !ok {
		return nil, fmt.Errorf("unknown action provider: %s", receiverName)
	}

	var method *Method

	for m := range prt.Methods() {
		if CamelToSnake(m.Name()) == methodSnake {
			method = m
			break
		}
	}

	if method == nil {
		return nil, fmt.Errorf("action %q: method %q not found on %q", name, methodSnake, receiverName)
	}

	return NewAction(prt, method, name), nil
}

// ModuleByName returns the module provider registered under the given name.
//
// Parameters:
//   - `name`: the receiver name (e.g., "file").
//
// Returns:
//   - `ProviderReceiverType`: the provider.
//   - `bool`: true if found.
func (r *receiverRegistry) ModuleByName(name string) (ProviderReceiverType, bool) {

	rt, ok := r.byName[name]
	if !ok {
		return nil, false
	}

	prt, ok := rt.(ProviderReceiverType)

	if !ok {
		return nil, false
	}

	if prt.Flags().Surfaces()&SurfaceScript == 0 {
		return nil, false
	}

	return prt, true
}

// PlannerByName returns the planner provider registered under the given name.
//
// Parameters:
//   - `name`: the receiver name (e.g., "file").
//
// Returns:
//   - `ProviderReceiverType`: the provider.
//   - `bool`: true if found.
func (r *receiverRegistry) PlannerByName(name string) (ProviderReceiverType, bool) {

	rt, ok := r.byName[name]
	if !ok {
		return nil, false
	}

	prt, ok := rt.(ProviderReceiverType)
	if !ok {
		return nil, false
	}

	if prt.Flags().Surfaces()&SurfaceWorkflow == 0 {
		return nil, false
	}

	return prt, true
}

// ResourceByName returns the resource type registered under the given name.
//
// Parameters:
//   - `name`: the receiver name (e.g., "file.Resource").
//
// Returns:
//   - `ResourceReceiverType`: the resource type.
//   - `bool`: true if found.
func (r *receiverRegistry) ResourceByName(name string) (ResourceReceiverType, bool) {

	rt, ok := r.byName[name]
	if !ok {
		return nil, false
	}

	rrt, ok := rt.(ResourceReceiverType)
	if !ok {
		return nil, false
	}

	return rrt, true
}

// endregion

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// init populates the registry from all announced receivers.
func (r *receiverRegistry) init() {

	for _, rt := range announced.snapshotReceiverTypes() {
		r.register(rt)
	}
}

// endregion

// endregion

// insertSortedProvider inserts a provider receiver type into a sorted slice, maintaining sort order by name.
//
// Parameters:
//   - `slice`: the existing sorted slice.
//   - `rt`: the provider receiver type to insert.
//
// Returns:
//   - `[]ProviderReceiverType`: the updated sorted slice.
func insertSortedProvider(slice []ProviderReceiverType, rt ProviderReceiverType) []ProviderReceiverType {

	name := rt.Name()
	index := sort.Search(len(slice), func(i int) bool { return slice[i].Name() >= name })

	slice = append(slice, nil)
	copy(slice[index+1:], slice[index:])
	slice[index] = rt

	return slice
}

// insertSortedResource inserts a resource receiver type into a sorted slice, maintaining sort order by name.
//
// Parameters:
//   - `slice`: the existing sorted slice.
//   - `rt`: the resource receiver type to insert.
//
// Returns:
//   - `[]ResourceReceiverType`: the updated sorted slice.
func insertSortedResource(slice []ResourceReceiverType, rt ResourceReceiverType) []ResourceReceiverType {

	name := rt.Name()
	idx := sort.Search(len(slice), func(i int) bool { return slice[i].Name() >= name })

	slice = append(slice, nil)
	copy(slice[idx+1:], slice[idx:])
	slice[idx] = rt

	return slice
}

// register adds a receiver type to the appropriate lists based on its concrete type and capabilities.
//
// Parameters:
//   - `rt`: the receiver type to register.
func (r *receiverRegistry) register(rt ReceiverType) {

	r.mu.Lock()
	defer r.mu.Unlock()

	r.registerLocked(rt)
}

// registerLocked indexes rt by name and reflect type and files it into the role-sorted lists. The caller must hold
// r.mu for writing.
//
// Parameters:
//   - `rt`: the receiver type to register.
func (r *receiverRegistry) registerLocked(rt ReceiverType) {

	if rt == nil {
		return
	}

	r.byName[rt.Name()] = rt
	r.byType[rt.ProviderType()] = rt

	switch v := rt.(type) {
	case ProviderReceiverType:
		flags := v.Flags()
		if flags.Surfaces()&SurfaceScript != 0 {
			r.scripts = insertSortedProvider(r.scripts, v)
		}
		if flags.Surfaces()&SurfaceWorkflow != 0 {
			r.workflows = insertSortedProvider(r.workflows, v)
			r.planners = insertSortedProvider(r.planners, v)
		}
		if flags.Placement() == PlacementPromoted {
			r.promoted = insertSortedProvider(r.promoted, v)
		}
	case ResourceReceiverType:
		r.resources = insertSortedResource(r.resources, v)
		for _, sourceType := range v.SourceTypes() {
			r.byType[sourceType] = rt
		}
	}
}
