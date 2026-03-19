// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"reflect"
	"sync"
)

// --- ReceiverFactory announcement ---

var (
	announceMu sync.Mutex
	announced  = make(map[reflect.Type]ReceiverFactory)
)

// AnnounceReceiver records a provider descriptor. Called in init().
// Does zero initialization — stores the value for later InitAll callback.
// Duplicate announcements of the same type are deduplicated.
//
// Parameters:
//   - p: the provider to announce.
func AnnounceReceiver(factory ReceiverFactory) {

	announceMu.Lock()
	defer announceMu.Unlock()
	announced[reflect.TypeOf(factory)] = factory
}

// InitAll calls Register on every announced provider.
// Called once by the framework when it is ready to build an ActionRegistry.
//
// Parameters:
//   - reg: the action registry to populate.
//   - ctx: the execution context for provider initialization.
func InitAll(reg *ActionRegistry, ctx Context) {

	announceMu.Lock()
	providers := make([]ReceiverFactory, 0, len(announced))
	for _, p := range announced {
		providers = append(providers, p)
	}
	announceMu.Unlock()

	for _, p := range providers {
		p.Register(reg, ctx)
	}
}

// Receivers returns all announced providers (for introspection/debugging).
//
// Returns:
//   - []ReceiverFactory: a copy of the announced provider list.
func Receivers() []ReceiverFactory {

	announceMu.Lock()
	defer announceMu.Unlock()
	out := make([]ReceiverFactory, 0, len(announced))
	for _, p := range announced {
		out = append(out, p)
	}
	return out
}

// resetAnnounced clears the announced registry. For testing only.
func resetAnnounced() {

	announceMu.Lock()
	defer announceMu.Unlock()
	announced = make(map[reflect.Type]ReceiverFactory)
}

// --- Resource announcement ---

// ResourceFactory describes a resource type for lazy registration. Generated code calls
// AnnounceResource in init() with a lightweight descriptor. The descriptor's Init method is called
// exactly once on first use to complete registration (e.g., RegisterConstructor).
type ResourceFactory interface {
	// Name returns a human-readable name for the resource type (e.g., "file.Resource").
	Name() string

	// Type returns the reflect.ProviderType of the resource struct (e.g., reflect.TypeOf(file.Resource{})).
	Type() reflect.Type

	// Init completes registration for this resource type. Called exactly once, lazily, on first use.
	// Implementations typically call RegisterConstructor. Errors are cached — Init is never retried.
	Init() error
}

// resourceEntry wraps a ResourceFactory with sync.Once for exactly-once initialization and error caching.
type resourceEntry struct {
	factory ResourceFactory
	once    sync.Once
	err     error
}

// init calls the descriptor's Init exactly once. Subsequent calls return the cached error (nil on success).
func (e *resourceEntry) init() error {

	e.once.Do(func() {
		e.err = e.factory.Init()
	})
	return e.err
}

// resourceRegistry maps reflect.ProviderType → *resourceEntry. Populated by AnnounceResource in init(),
// consulted by ensureResourceInit on first use.
var resourceRegistry sync.Map

// AnnounceResource records a resource descriptor for lazy initialization. Called in generated init()
// functions. Does zero initialization — stores the descriptor for later lazy Init on first use.
//
// Parameters:
//   - factory: resource descriptor providing ReceiverName, ProviderType, and Init
func AnnounceResource(factory ResourceFactory) {

	resourceRegistry.Store(factory.Type(), &resourceEntry{factory: factory})
}

// ensureResourceInit looks up a constructor for the given type. If no constructor is registered but a
// resource descriptor has been announced, calls Init to complete registration (exactly once, with
// error caching).
//
// Parameters:
//   - targetType: the reflect.ProviderType to look up a constructor for
//
// Returns:
//   - constructor function, true if found (either already registered or lazily initialized)
//   - nil, false if no constructor and no announced descriptor
func ensureResourceInit(targetType reflect.Type) (func(any) (any, error), bool) {

	// Fast path: constructor already registered.
	if ctor, ok := constructorRegistry.Load(targetType); ok {
		return ctor.(func(any) (any, error)), true
	}

	// Slow path: check resource announcement registry for lazy init.
	entry, ok := resourceRegistry.Load(targetType)
	if !ok {
		return nil, false
	}

	// Init the descriptor (exactly once, error cached).
	if err := entry.(*resourceEntry).init(); err != nil {
		return nil, false
	}

	// Re-check: Init should have called RegisterConstructor.
	if ctor, ok := constructorRegistry.Load(targetType); ok {
		return ctor.(func(any) (any, error)), true
	}

	return nil, false
}

// resetAnnouncedResources clears the resource announcement registry. For testing only.
func resetAnnouncedResources() {

	resourceRegistry.Range(func(key, _ any) bool {
		resourceRegistry.Delete(key)
		return true
	})
}
