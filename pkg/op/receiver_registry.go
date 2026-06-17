// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

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
// Called in init(). Roles are declared via [ProviderRole] flags: [RoleModule] for immediate-mode starlark globals,
// [RoleAction] for plan-mode graph node creation.
//
// Companion methods on the provider type — [Method.Plan] via <Name>Planned, [Method.Undo] via Compensate<Name> —
// are discovered automatically by reflection in [NewProviderReceiverType]. No registration is required.
//
// Parameters:
//   - `providerType`: the provider's reflect.Type.
//   - `roles`: the provider's declared roles.
//   - `construct`: creates a provider instance from RuntimeEnvironment.
//   - `methods`: codegen-emitted [MethodMetadata] per Go method, keyed by the method's Go name.
func AnnounceProvider(providerType reflect.Type, roles ProviderRole, construct ProviderConstructor, methods map[string]MethodMetadata) {

	label := fmt.Sprintf("AnnounceProvider(%s)", providerType)

	assert.Truef(roles.Dispatch() != 0, "%s: roles must set at least one dispatch-zone bit (RoleModule or RoleAction); got %#x", label, uint(roles))

	methodParameters := make(map[string][]string, len(methods))
	planners := make(map[string]Planner, len(methods))

	for name, metadata := range methods {
		methodParameters[name] = metadata.ParameterNames
		planners[name] = plannerForType(metadata.Planner)
	}

	parsed, err := parseParameters(providerType, methodParameters)
	assert.NoError(label, err)

	rt, err := NewProviderReceiverType(providerType, construct, roles, parsed, planners)
	assert.NoError(label, err)

	// Stamp per-method surface modifiers from the codegen-emitted metadata. Unset entries default to ModifierNone.
	for name, metadata := range methods {
		if method, ok := rt.MethodByName(name); ok {
			method.setModifiers(metadata.Modifiers)
		}
	}

	err = announced.registerReceiverType(rt)
	assert.NoError(label, err)
}

// AnnounceResource registers a resource type.
//
// Called in init(). Resources are always RoleResource — they cannot be actions or modules. They are data types
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

	parsed, err := parseParameters(resourceType, methodParameters)
	assert.NoError(label, err)

	rt, err := NewResourceReceiverType(resourceType, construct, parsed, sourceTypes...)
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
// Actions are providers with RoleAction (graph scope). Modules are providers with RoleModule (script scope). Planners
// mirror actions for the plan.* namespace. Resources are data types that flow through starlark code or an execution
// graph. A provider may appear in both actions and modules.
//
// The byType map enables lookup by reflect.Type for marshalReflect (wrapping Go return values) and the executor
// (dispatching graph nodes).
type receiverRegistry struct {
	actions   []ProviderReceiverType        // sorted by name; providers with RoleAction
	modules   []ProviderReceiverType        // sorted by name; providers with RoleModule
	planners  []ProviderReceiverType        // sorted by name; mirrors actions for plan.* routing
	roots     []ProviderReceiverType        // sorted by name; providers with the RoleRoot placement-zone bit
	resources []ResourceReceiverType        // sorted by name; data types
	byName    map[string]ReceiverType       // all receiver types by name
	byType    map[reflect.Type]ReceiverType // all receiver types by reflect.Type

	// mu guards byName and byType. The sorted lists need no guard: they are appended only at construction (from
	// announced providers/resources), while runtime derive-and-register touches only the maps — a derived type is a
	// plain receiverType, never a Provider/Resource, so it never reaches the list-appending switch cases.
	mu sync.RWMutex
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
func (r *receiverRegistry) Actions() []ProviderReceiverType { return r.actions }

// Modules returns all providers that can be called directly from a starlark runtime.
//
// Returns:
//   - `[]ProviderReceiverType`: sorted by receiver name.
func (r *receiverRegistry) Modules() []ProviderReceiverType { return r.modules }

// Planners returns all providers available in the plan.* namespace.
//
// Returns:
//   - `[]ProviderReceiverType`: sorted by receiver name.
func (r *receiverRegistry) Planners() []ProviderReceiverType { return r.planners }

// RootProviders returns every provider with the [RoleRoot] placement-zone bit set.
//
// Root providers surface their methods flat at their access-defined namespace root rather than nested under the
// provider's own name. Callers that need a specific dispatch mode filter the returned slice further via
// [ProviderRole.Dispatch] — e.g., plan.Provider filters to RoleAction to discover its planner-primitive peers.
//
// Returns:
//   - `[]ProviderReceiverType`: sorted by receiver name.
func (r *receiverRegistry) RootProviders() []ProviderReceiverType { return r.roots }

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
		if !ok || prt.Roles()&RoleAction == 0 {
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

	if prt.Roles()&RoleAction == 0 {
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
func (r *receiverRegistry) BuildAction(name string) (Action, error) {

	dot := strings.LastIndex(name, ".")

	if dot < 0 {
		return nil, fmt.Errorf("invalid action name %q: no dot", name)
	}

	receiverName := name[:dot]
	methodSnake := name[dot+1:]

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

	if prt.Roles()&RoleModule == 0 {
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

	if prt.Roles()&RoleAction == 0 {
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
		roles := v.Roles()
		if roles&RoleModule != 0 {
			r.modules = insertSortedProvider(r.modules, v)
		}
		if roles&RoleAction != 0 {
			r.actions = insertSortedProvider(r.actions, v)
			r.planners = insertSortedProvider(r.planners, v)
		}
		if roles.Placement()&RoleRoot != 0 {
			r.roots = insertSortedProvider(r.roots, v)
		}
	case ResourceReceiverType:
		r.resources = insertSortedResource(r.resources, v)
		for _, sourceType := range v.SourceTypes() {
			r.byType[sourceType] = rt
		}
	}
}
