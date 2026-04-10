// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"fmt"
	"reflect"
	"sort"
	"sync"
)

var (
	announcedReceiverTypes = make(map[string]ReceiverType)
	mutex                  = sync.Mutex{}
)

// AnnounceProvider registers a provider with its roles.
//
// Called in init(). Roles are declared via [ProviderRole] flags: [RoleModule] for immediate-mode starlark globals,
// [RoleAction] for plan-mode graph node creation.
//
// Companion methods on the provider type — [Method.Plan] via <Name>Planned, [Method.Undo] via Compensate<Name> —
// are discovered automatically by reflection in [NewProviderReceiverType]. No registration is required.
//
// Parameters:
//   - providerType: the provider's reflect.Type.
//   - roles: the provider's declared roles.
//   - construct: creates a provider instance from ExecutionContext.
//   - methodParameters: starlark parameter names per Go method.
func AnnounceProvider(providerType reflect.Type, roles ProviderRole, construct ProviderConstructor, methodParameters map[string][]string) {

	rt, err := NewProviderReceiverType(providerType, construct, roles, methodParameters)
	if err != nil {
		panic(fmt.Sprintf("AnnounceProvider(%s): %v", providerType, err))
	}

	mutex.Lock()
	defer mutex.Unlock()
	announcedReceiverTypes[rt.ReceiverName()] = rt
}

// AnnounceResource registers a resource type.
//
// Called in init(). Resources are always RoleResource — they cannot be actions or modules. They are data types
// constructed by coercing a raw value (e.g., a string path becomes a file.Resource).
//
// Parameters:
//   - resourceType: the resource's reflect.Type.
//   - construct: coerces a raw value into the typed resource.
//   - methodParameters: starlark parameter names per Go method (for attribute access).
func AnnounceResource(
	resourceType reflect.Type,
	construct ResourceConstructor,
	methodParameters map[string][]string,
) {

	rt, err := NewResourceReceiverType(resourceType, construct, methodParameters)
	if err != nil {
		panic(fmt.Sprintf("AnnounceResource(%s): %v", resourceType, err))
	}

	mutex.Lock()
	defer mutex.Unlock()
	announcedReceiverTypes[rt.ReceiverName()] = rt
}

// AnnounceType registers a bare receiver type for an arbitrary Go struct.
//
// Called in init(). This is for Go types that need method dispatch in starlark but are neither providers nor resources
// (e.g., Go AST types returned by the goast provider). The receiver type has no constructor and no roles — it exists
// solely so marshalReflect can wrap instances with method dispatch.
//
// Parameters:
//   - goType: the Go struct's reflect.Type.
//   - methodParameters: starlark parameter names per Go method.
func AnnounceType(goType reflect.Type, methodParameters map[string][]string) {

	base, err := newReceiverType(goType, methodParameters)
	if err != nil {
		panic(fmt.Sprintf("AnnounceType(%s): %v", goType, err))
	}

	mutex.Lock()
	defer mutex.Unlock()
	announcedReceiverTypes[base.ReceiverName()] = &base
}

// ReceiverRegistry stores receiver types in four sorted lists plus cross-cutting lookup maps.
//
// Actions are providers with RoleAction (graph scope). Modules are providers with RoleModule (script scope). Planners
// mirror actions for the plan.* namespace. Resources are data types that flow through starlark code or an execution
// graph. A provider may appear in both actions and modules.
//
// The byType map enables lookup by reflect.Type for marshalReflect (wrapping Go return values) and the executor
// (dispatching graph nodes).
type ReceiverRegistry struct {
	actions   []ProviderReceiverType        // sorted by name; providers with RoleAction
	modules   []ProviderReceiverType        // sorted by name; providers with RoleModule
	planners  []ProviderReceiverType        // sorted by name; mirrors actions for plan.* routing
	resources []ResourceReceiverType        // sorted by name; data types
	byName    map[string]ReceiverType       // all receiver types by name
	byType    map[reflect.Type]ReceiverType // all receiver types by reflect.Type
}

// NewReceiverRegistry creates a populated registry from all announced receivers.
//
// Returns:
//   - *ReceiverRegistry: the populated registry.
func NewReceiverRegistry() *ReceiverRegistry {

	registry := &ReceiverRegistry{
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
//   - []ProviderReceiverType: sorted by receiver name.
func (r *ReceiverRegistry) Actions() []ProviderReceiverType { return r.actions }

// Modules returns all providers that can be called directly from a starlark runtime.
//
// Returns:
//   - []ProviderReceiverType: sorted by receiver name.
func (r *ReceiverRegistry) Modules() []ProviderReceiverType { return r.modules }

// Planners returns all providers available in the plan.* namespace.
//
// Returns:
//   - []ProviderReceiverType: sorted by receiver name.
func (r *ReceiverRegistry) Planners() []ProviderReceiverType { return r.planners }

// Resources returns all resource data types.
//
// Returns:
//   - []ResourceReceiverType: sorted by receiver name.
func (r *ReceiverRegistry) Resources() []ResourceReceiverType { return r.resources }

// Type returns the receiver type registered under the given name.
//
// Parameters:
//   - name: the receiver name (e.g., "file").
//
// Returns:
//   - ReceiverType: the receiver type.
//   - bool: true if found.
func (r *ReceiverRegistry) Type(name string) (ReceiverType, bool) {

	rt, ok := r.byName[name]
	return rt, ok
}

// TypeByReflection returns the receiver type registered for the given Go type.
//
// Parameters:
//   - t: the reflect.Type to look up (pointer or struct).
//
// Returns:
//   - ReceiverType: the receiver type.
//   - bool: true if found.
func (r *ReceiverRegistry) TypeByReflection(t reflect.Type) (ReceiverType, bool) {

	rt, ok := r.byType[t]
	return rt, ok
}

// endregion

// region Behaviors

// ActionByName returns the action provider registered under the given name.
//
// Parameters:
//   - name: the receiver name (e.g., "file").
//
// Returns:
//   - ProviderReceiverType: the provider.
//   - bool: true if found.
func (r *ReceiverRegistry) ActionByName(name string) (ProviderReceiverType, bool) {

	for _, rt := range r.actions {
		if rt.ReceiverName() == name {
			return rt, true
		}
	}
	return nil, false
}

// ModuleByName returns the module provider registered under the given name.
//
// Parameters:
//   - name: the receiver name (e.g., "file").
//
// Returns:
//   - ProviderReceiverType: the provider.
//   - bool: true if found.
func (r *ReceiverRegistry) ModuleByName(name string) (ProviderReceiverType, bool) {

	for _, rt := range r.modules {
		if rt.ReceiverName() == name {
			return rt, true
		}
	}
	return nil, false
}

// PlannerByName returns the planner provider registered under the given name.
//
// Parameters:
//   - name: the receiver name (e.g., "file").
//
// Returns:
//   - ProviderReceiverType: the provider.
//   - bool: true if found.
func (r *ReceiverRegistry) PlannerByName(name string) (ProviderReceiverType, bool) {

	for _, rt := range r.planners {
		if rt.ReceiverName() == name {
			return rt, true
		}
	}
	return nil, false
}

// ResourceByName returns the resource type registered under the given name.
//
// Parameters:
//   - name: the receiver name (e.g., "file.Resource").
//
// Returns:
//   - ResourceReceiverType: the resource type.
//   - bool: true if found.
func (r *ReceiverRegistry) ResourceByName(name string) (ResourceReceiverType, bool) {

	for _, rt := range r.resources {
		if rt.ReceiverName() == name {
			return rt, true
		}
	}
	return nil, false
}

// endregion

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// init populates the registry from all announced receivers.
func (r *ReceiverRegistry) init() {

	mutex.Lock()

	types := make([]ReceiverType, 0, len(announcedReceiverTypes))
	for _, rt := range announcedReceiverTypes {
		types = append(types, rt)
	}

	mutex.Unlock()

	for _, rt := range types {
		r.register(rt)
	}
}

// endregion

// endregion

// insertSortedProvider inserts a provider receiver type into a sorted slice, maintaining sort order by name.
//
// Parameters:
//   - slice: the existing sorted slice.
//   - rt: the provider receiver type to insert.
//
// Returns:
//   - []ProviderReceiverType: the updated sorted slice.
func insertSortedProvider(slice []ProviderReceiverType, rt ProviderReceiverType) []ProviderReceiverType {

	name := rt.ReceiverName()
	idx := sort.Search(len(slice), func(i int) bool {
		return slice[i].ReceiverName() >= name
	})

	slice = append(slice, nil)
	copy(slice[idx+1:], slice[idx:])
	slice[idx] = rt

	return slice
}

// insertSortedResource inserts a resource receiver type into a sorted slice, maintaining sort order by name.
//
// Parameters:
//   - slice: the existing sorted slice.
//   - rt: the resource receiver type to insert.
//
// Returns:
//   - []ResourceReceiverType: the updated sorted slice.
func insertSortedResource(slice []ResourceReceiverType, rt ResourceReceiverType) []ResourceReceiverType {

	name := rt.ReceiverName()
	idx := sort.Search(len(slice), func(i int) bool {
		return slice[i].ReceiverName() >= name
	})

	slice = append(slice, nil)
	copy(slice[idx+1:], slice[idx:])
	slice[idx] = rt

	return slice
}

// register adds a receiver type to the appropriate lists based on its concrete type and capabilities.
//
// Parameters:
//   - rt: the receiver type to register.
func (r *ReceiverRegistry) register(rt ReceiverType) {

	r.byName[rt.ReceiverName()] = rt
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
	case ResourceReceiverType:
		r.resources = insertSortedResource(r.resources, v)
	}
}
