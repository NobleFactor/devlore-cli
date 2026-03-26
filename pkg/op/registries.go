// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"fmt"
	"reflect"
	"sync"
)

// region Starlark provider receiver registry and the resource registry

var (
	announcedReceiversMutex sync.Mutex
	announcedReceivers      = make(map[reflect.Type]ReceiverFactory)

	resourceRegistry sync.Map
)

// AnnounceReceiver records a provider descriptor.
//
// Called in init(). Does zero initialization — stores the value for later InitAll callback. Duplicate announcements of
// the same type are deduplicated.
func AnnounceReceiver(factory ReceiverFactory) {
	announcedReceiversMutex.Lock()
	defer announcedReceiversMutex.Unlock()
	announcedReceivers[reflect.TypeOf(factory)] = factory
}

// InitAll calls Register on every announcedReceivers receiver.
//
// Called once by the framework when it is ready to build an ActionRegistry.
func InitAll(registry *ActionRegistry, ctx Context) {

	announcedReceiversMutex.Lock()
	factories := make([]ReceiverFactory, 0, len(announcedReceivers))

	for _, p := range announcedReceivers {
		factories = append(factories, p)
	}

	announcedReceiversMutex.Unlock()

	for _, p := range factories {
		p.Register(registry, ctx)
	}
}

// Receivers returns all announcedReceivers receivers.
func Receivers() []ReceiverFactory {

	announcedReceiversMutex.Lock()
	defer announcedReceiversMutex.Unlock()

	factories := make([]ReceiverFactory, 0, len(announcedReceivers))

	for _, p := range announcedReceivers {
		factories = append(factories, p)
	}

	return factories
}

// resetAnnounced clears the announcedReceivers Registry. For testing only.
func resetAnnounced() {
	announcedReceiversMutex.Lock()
	defer announcedReceiversMutex.Unlock()
	announcedReceivers = make(map[reflect.Type]ReceiverFactory)
}

// endregion

// region Resource registry

// ResourceFactory describes a resource type for lazy registration.
//
// Generated code calls AnnounceResource in init() with a lightweight descriptor. The descriptor's Init method is called
// exactly once on first use to complete registration (e.g., RegisterConstructor).
type ResourceFactory interface {
	Name() string
	Type() reflect.Type
	Init() error
}

type resourceEntry struct {
	factory ResourceFactory
	once    sync.Once
	err     error
}

func (e *resourceEntry) init() error {
	e.once.Do(func() {
		e.err = e.factory.Init()
	})
	return e.err
}

// AnnounceResource records a resource descriptor for lazy initialization.
func AnnounceResource(factory ResourceFactory) {
	resourceRegistry.Store(factory.Type(), &resourceEntry{factory: factory})
}

// ResetResourceRegistry clears the resource and constructor registries.
// Test convenience — clears both because sync.Once entries in resourceRegistry
// are coupled to constructorRegistry.
func ResetResourceRegistry() {
	resourceRegistry.Range(func(key, _ any) bool {
		resourceRegistry.Delete(key)
		return true
	})
	constructorRegistry = sync.Map{}
}

// endregion

// region Constructor registry

// constructorRegistry maps reflect.Type → func(any) (any, error).
//
// Types register a constructor so the reflection bridge can construct
// them from simpler representations (e.g., string → Blob).
var constructorRegistry sync.Map

// RegisterConstructor registers a function that constructs a Go value
// from a simpler representation (e.g., string → Blob via NewBlob).
func RegisterConstructor[T any](fn func(any) (T, error)) {
	t := reflect.TypeOf((*T)(nil)).Elem()
	constructorRegistry.Store(t, func(v any) (any, error) {
		return fn(v)
	})
}

// Construct uses the constructor registry to convert value to type T.
func Construct[T any](value any) (T, error) {
	t := reflect.TypeOf((*T)(nil)).Elem()
	ctor, ok := LookupConstructor(t)
	if !ok {
		var zero T
		return zero, fmt.Errorf("no constructor registered for %s", t)
	}
	result, err := ctor(value)
	if err != nil {
		var zero T
		return zero, err
	}
	return result.(T), nil
}

// LookupConstructor returns the constructor for the given type, triggering lazy resource initialization if needed.
func LookupConstructor(targetType reflect.Type) (func(any) (any, error), bool) {

	// Fast path: constructor already registered.

	if ctor, ok := constructorRegistry.Load(targetType); ok {
		return ctor.(func(any) (any, error)), true
	}

	// Slow path: check resource announcement registry for lazy init.

	entry, ok := resourceRegistry.Load(targetType)
	if !ok {
		return nil, false
	}

	if err := entry.(*resourceEntry).init(); err != nil {
		return nil, false
	}

	if ctor, ok := constructorRegistry.Load(targetType); ok {
		return ctor.(func(any) (any, error)), true
	}

	return nil, false
}

// endregion
