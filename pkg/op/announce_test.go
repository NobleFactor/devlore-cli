// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"fmt"
	"reflect"
	"sync"
	"testing"

	"go.starlark.net/starlark"
)

// announceTestProviderAlpha is a minimal ReceiverFactory for testing.
type announceTestProviderAlpha struct {
	name       string
	registered bool
}

func (p *announceTestProviderAlpha) ReceiverName() string                          { return p.name }
func (p *announceTestProviderAlpha) GetOrCreateProvider(_ Context) ContextProvider { return nil }
func (p *announceTestProviderAlpha) ProviderType() reflect.Type {
	return reflect.TypeOf((*announceTestProviderAlpha)(nil)).Elem()
}
func (p *announceTestProviderAlpha) Register(reg *ActionRegistry, _ Context) {
	p.registered = true
	reg.Register(&registryTestAction{name: p.name + ".action"})
}

// announceTestProviderBeta is a second distinct type for deduplication testing.
type announceTestProviderBeta struct {
	name       string
	registered bool
}

func (p *announceTestProviderBeta) ReceiverName() string                          { return p.name }
func (p *announceTestProviderBeta) GetOrCreateProvider(_ Context) ContextProvider { return nil }
func (p *announceTestProviderBeta) ProviderType() reflect.Type {
	return reflect.TypeOf((*announceTestProviderBeta)(nil)).Elem()
}
func (p *announceTestProviderBeta) Register(reg *ActionRegistry, _ Context) {
	p.registered = true
	reg.Register(&registryTestAction{name: p.name + ".action"})
}

// announceTestPlannedProvider implements both ReceiverFactory and PlanningReceiverFactory.
type announceTestPlannedProvider struct {
	name          string
	registered    bool
	plannedCalled bool
}

func (p *announceTestPlannedProvider) ReceiverName() string                          { return p.name }
func (p *announceTestPlannedProvider) GetOrCreateProvider(_ Context) ContextProvider { return nil }
func (p *announceTestPlannedProvider) ProviderType() reflect.Type {
	return reflect.TypeOf((*announceTestPlannedProvider)(nil)).Elem()
}
func (p *announceTestPlannedProvider) Register(reg *ActionRegistry, _ Context) {
	p.registered = true
}
func (p *announceTestPlannedProvider) NewPlanning(_ *Graph, _ string, _ *ActionRegistry) starlark.Value {
	p.plannedCalled = true
	return starlark.None
}

// announceTestImmediateProvider implements both ReceiverFactory and ExecutingReceiverFactory.
type announceTestImmediateProvider struct {
	name            string
	registered      bool
	immediateCalled bool
}

func (p *announceTestImmediateProvider) ReceiverName() string                          { return p.name }
func (p *announceTestImmediateProvider) GetOrCreateProvider(_ Context) ContextProvider { return nil }
func (p *announceTestImmediateProvider) ProviderType() reflect.Type {
	return reflect.TypeOf((*announceTestImmediateProvider)(nil)).Elem()
}
func (p *announceTestImmediateProvider) Register(reg *ActionRegistry, _ Context) {
	p.registered = true
}
func (p *announceTestImmediateProvider) NewExecuting(_ Context) starlark.Value {
	p.immediateCalled = true
	return starlark.None
}

func TestAnnounce_and_Providers(t *testing.T) {
	resetAnnounced()

	a := &announceTestProviderAlpha{name: "alpha"}
	b := &announceTestProviderBeta{name: "beta"}
	Announce(a)
	Announce(b)

	providers := Providers()
	if len(providers) != 2 {
		t.Fatalf("Providers() returned %d, want 2", len(providers))
	}

	names := map[string]bool{}
	for _, p := range providers {
		names[p.ReceiverName()] = true
	}
	if !names["alpha"] {
		t.Error("expected alpha in Providers()")
	}
	if !names["beta"] {
		t.Error("expected beta in Providers()")
	}
}

func TestProviders_returns_copy(t *testing.T) {
	resetAnnounced()

	Announce(&announceTestProviderAlpha{name: "x"})

	p1 := Providers()
	p2 := Providers()
	if &p1[0] == &p2[0] {
		t.Error("Providers() returned same backing array, want independent copies")
	}
}

func TestInitAll_calls_Register(t *testing.T) {
	resetAnnounced()

	a := &announceTestProviderAlpha{name: "alpha"}
	b := &announceTestProviderBeta{name: "beta"}
	Announce(a)
	Announce(b)

	reg := NewActionRegistry()
	InitAll(reg, Context{})

	if !a.registered {
		t.Error("alpha.Register was not called")
	}
	if !b.registered {
		t.Error("beta.Register was not called")
	}
	if _, ok := reg.Get("alpha.action"); !ok {
		t.Error("alpha.action not in registry after InitAll")
	}
	if _, ok := reg.Get("beta.action"); !ok {
		t.Error("beta.action not in registry after InitAll")
	}
}

func TestInitAll_PlannedProvider_type_assertion(t *testing.T) {
	resetAnnounced()

	pp := &announceTestPlannedProvider{name: "planned"}
	Announce(pp)

	providers := Providers()
	if len(providers) != 1 {
		t.Fatalf("Providers() returned %d, want 1", len(providers))
	}

	p, ok := providers[0].(PlanningReceiverFactory)
	if !ok {
		t.Fatal("expected PlanningReceiverFactory type assertion to succeed")
	}
	p.NewPlanning(nil, "", nil)
	if !pp.plannedCalled {
		t.Error("NewPlanning was not called")
	}
}

func TestInitAll_ImmediateProvider_type_assertion(t *testing.T) {
	resetAnnounced()

	ip := &announceTestImmediateProvider{name: "immediate"}
	Announce(ip)

	providers := Providers()
	if len(providers) != 1 {
		t.Fatalf("Providers() returned %d, want 1", len(providers))
	}

	p, ok := providers[0].(ExecutingReceiverFactory)
	if !ok {
		t.Fatal("expected ExecutingReceiverFactory type assertion to succeed")
	}
	p.NewExecuting(Context{})
	if !ip.immediateCalled {
		t.Error("NewExecuting was not called")
	}
}

func TestInitAll_plain_provider_not_PlannedProvider(t *testing.T) {
	resetAnnounced()

	plain := &announceTestProviderAlpha{name: "plain"}
	Announce(plain)

	providers := Providers()
	if _, ok := providers[0].(PlanningReceiverFactory); ok {
		t.Error("plain provider should not satisfy PlanningReceiverFactory")
	}
	if _, ok := providers[0].(ExecutingReceiverFactory); ok {
		t.Error("plain provider should not satisfy ExecutingReceiverFactory")
	}
}

func TestAnnounce_concurrent(t *testing.T) {
	resetAnnounced()

	// Concurrent announce of the same type deduplicates to 1 entry.
	var wg sync.WaitGroup
	for i := range 100 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			Announce(&announceTestProviderAlpha{name: "concurrent"})
			_ = n
		}(i)
	}
	wg.Wait()

	providers := Providers()
	if len(providers) != 1 {
		t.Errorf("Providers() returned %d, want 1 (same type deduplicates)", len(providers))
	}
}

func TestAnnounce_deduplicates_same_type(t *testing.T) {
	resetAnnounced()

	// Announcing the same type twice keeps only the last value.
	first := &announceTestProviderAlpha{name: "first"}
	second := &announceTestProviderAlpha{name: "second"}
	Announce(first)
	Announce(second)

	providers := Providers()
	if len(providers) != 1 {
		t.Fatalf("Providers() returned %d, want 1", len(providers))
	}
	if providers[0].ReceiverName() != "second" {
		t.Errorf("expected last-announced value, got %q", providers[0].ReceiverName())
	}
}

// --- Resource announcement tests ---

// testResource is a minimal resource type for testing lazy registration.
type testResource struct {
	ResourceBase
	Value string
}

// testResourceDescriptor implements ResourceDescriptor for testResource.
type testResourceDescriptor struct {
	initCalled int
	initErr    error
}

func (d *testResourceDescriptor) Name() string       { return "test.Resource" }
func (d *testResourceDescriptor) Type() reflect.Type { return reflect.TypeOf(testResource{}) }
func (d *testResourceDescriptor) Init() error {
	d.initCalled++
	if d.initErr != nil {
		return d.initErr
	}
	RegisterConstructor(func(v any) (testResource, error) {
		s, ok := v.(string)
		if !ok {
			return testResource{}, fmt.Errorf("testResource: expected string, got %T", v)
		}
		return testResource{Value: s}, nil
	})
	return nil
}

func TestAnnounceResource_LazyInit(t *testing.T) {
	resetAnnouncedResources()
	t.Cleanup(func() {
		resetAnnouncedResources()
		constructorRegistry.Delete(reflect.TypeOf(testResource{}))
	})

	desc := &testResourceDescriptor{}
	AnnounceResource(desc)

	// Constructor should not be registered yet.
	if _, ok := constructorRegistry.Load(reflect.TypeOf(testResource{})); ok {
		t.Fatal("constructor should not be registered before first use")
	}

	// loadConstructor should trigger lazy init.
	ctor, ok := loadConstructor(reflect.TypeOf(testResource{}))
	if !ok {
		t.Fatal("loadConstructor returned false after announcement")
	}
	if desc.initCalled != 1 {
		t.Fatalf("Init called %d times, want 1", desc.initCalled)
	}

	// Verify the constructor works.
	result, err := ctor("hello")
	if err != nil {
		t.Fatalf("constructor error: %v", err)
	}
	r, ok := result.(testResource)
	if !ok {
		t.Fatalf("constructor returned %T, want testResource", result)
	}
	if r.Value != "hello" {
		t.Errorf("Value = %q, want %q", r.Value, "hello")
	}
}

func TestAnnounceResource_InitCalledOnce(t *testing.T) {
	resetAnnouncedResources()
	t.Cleanup(func() {
		resetAnnouncedResources()
		constructorRegistry.Delete(reflect.TypeOf(testResource{}))
	})

	desc := &testResourceDescriptor{}
	AnnounceResource(desc)

	// Call loadConstructor multiple times.
	for range 5 {
		_, _ = loadConstructor(reflect.TypeOf(testResource{}))
	}

	if desc.initCalled != 1 {
		t.Errorf("Init called %d times, want 1 (sync.Once guarantee)", desc.initCalled)
	}
}

func TestAnnounceResource_InitErrorCached(t *testing.T) {
	resetAnnouncedResources()
	t.Cleanup(func() {
		resetAnnouncedResources()
		constructorRegistry.Delete(reflect.TypeOf(testResource{}))
	})

	desc := &testResourceDescriptor{initErr: fmt.Errorf("init failed")}
	AnnounceResource(desc)

	// First call: Init fails, constructor not registered.
	_, ok := loadConstructor(reflect.TypeOf(testResource{}))
	if ok {
		t.Fatal("loadConstructor should return false when Init fails")
	}
	if desc.initCalled != 1 {
		t.Fatalf("Init called %d times, want 1", desc.initCalled)
	}

	// Second call: error cached, Init not retried.
	_, ok = loadConstructor(reflect.TypeOf(testResource{}))
	if ok {
		t.Fatal("loadConstructor should still return false (cached error)")
	}
	if desc.initCalled != 1 {
		t.Errorf("Init called %d times, want 1 (error should be cached)", desc.initCalled)
	}
}

func TestAnnounceResource_NoDescriptor(t *testing.T) {
	resetAnnouncedResources()

	// No descriptor announced for testResource.
	_, ok := loadConstructor(reflect.TypeOf(testResource{}))
	if ok {
		t.Fatal("loadConstructor should return false when no descriptor announced")
	}
}

func TestAnnounceResource_FastPath(t *testing.T) {
	resetAnnouncedResources()
	t.Cleanup(func() {
		resetAnnouncedResources()
		constructorRegistry.Delete(reflect.TypeOf(testResource{}))
	})

	// Register constructor directly (simulating existing eager registration).
	RegisterConstructor(func(v any) (testResource, error) {
		return testResource{Value: "eager"}, nil
	})

	desc := &testResourceDescriptor{}
	AnnounceResource(desc)

	// loadConstructor should find the existing constructor without calling Init.
	ctor, ok := loadConstructor(reflect.TypeOf(testResource{}))
	if !ok {
		t.Fatal("loadConstructor should find eagerly registered constructor")
	}
	if desc.initCalled != 0 {
		t.Errorf("Init called %d times, want 0 (fast path should skip Init)", desc.initCalled)
	}

	result, err := ctor("anything")
	if err != nil {
		t.Fatalf("constructor error: %v", err)
	}
	r := result.(testResource)
	if r.Value != "eager" {
		t.Errorf("Value = %q, want %q (eager constructor)", r.Value, "eager")
	}
}

func TestAnnounceResource_ConcurrentFirstUse(t *testing.T) {
	resetAnnouncedResources()
	t.Cleanup(func() {
		resetAnnouncedResources()
		constructorRegistry.Delete(reflect.TypeOf(testResource{}))
	})

	desc := &testResourceDescriptor{}
	AnnounceResource(desc)

	var wg sync.WaitGroup
	results := make([]bool, 100)
	for i := range 100 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_, ok := loadConstructor(reflect.TypeOf(testResource{}))
			results[n] = ok
		}(i)
	}
	wg.Wait()

	// All goroutines should succeed.
	for i, ok := range results {
		if !ok {
			t.Errorf("goroutine %d: loadConstructor returned false", i)
		}
	}

	// Init called exactly once despite concurrent access.
	if desc.initCalled != 1 {
		t.Errorf("Init called %d times, want 1", desc.initCalled)
	}
}
