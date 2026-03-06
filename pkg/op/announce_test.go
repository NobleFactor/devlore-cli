// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"sync"
	"testing"

	"go.starlark.net/starlark"
)

// announceTestProvider is a minimal Provider for testing.
type announceTestProvider struct {
	name       string
	registered bool
}

func (p *announceTestProvider) Name() string { return p.name }
func (p *announceTestProvider) Register(reg *ActionRegistry, _ Context) {
	p.registered = true
	reg.Register(&registryTestAction{name: p.name + ".action"})
}

// announceTestPlannedProvider implements both Provider and PlannedProvider.
type announceTestPlannedProvider struct {
	name          string
	registered    bool
	plannedCalled bool
}

func (p *announceTestPlannedProvider) Name() string { return p.name }
func (p *announceTestPlannedProvider) Register(reg *ActionRegistry, _ Context) {
	p.registered = true
}
func (p *announceTestPlannedProvider) NewPlanned(_ *Graph, _ string, _ *ActionRegistry) starlark.Value {
	p.plannedCalled = true
	return starlark.None
}

// announceTestImmediateProvider implements both Provider and ImmediateProvider.
type announceTestImmediateProvider struct {
	name            string
	registered      bool
	immediateCalled bool
}

func (p *announceTestImmediateProvider) Name() string { return p.name }
func (p *announceTestImmediateProvider) Register(reg *ActionRegistry, _ Context) {
	p.registered = true
}
func (p *announceTestImmediateProvider) NewImmediate(_ BindingConfig) starlark.Value {
	p.immediateCalled = true
	return starlark.None
}

func TestAnnounce_and_Providers(t *testing.T) {
	resetAnnounced()

	a := &announceTestProvider{name: "alpha"}
	b := &announceTestProvider{name: "beta"}
	Announce(a)
	Announce(b)

	providers := Providers()
	if len(providers) != 2 {
		t.Fatalf("Providers() returned %d, want 2", len(providers))
	}
	if providers[0].Name() != "alpha" {
		t.Errorf("Providers()[0].Name() = %q, want %q", providers[0].Name(), "alpha")
	}
	if providers[1].Name() != "beta" {
		t.Errorf("Providers()[1].Name() = %q, want %q", providers[1].Name(), "beta")
	}
}

func TestProviders_returns_copy(t *testing.T) {
	resetAnnounced()

	Announce(&announceTestProvider{name: "x"})

	p1 := Providers()
	p2 := Providers()
	if &p1[0] == &p2[0] {
		t.Error("Providers() returned same backing array, want independent copies")
	}
}

func TestInitAll_calls_Register(t *testing.T) {
	resetAnnounced()

	a := &announceTestProvider{name: "alpha"}
	b := &announceTestProvider{name: "beta"}
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

	p, ok := providers[0].(PlannedProvider)
	if !ok {
		t.Fatal("expected PlannedProvider type assertion to succeed")
	}
	p.NewPlanned(nil, "", nil)
	if !pp.plannedCalled {
		t.Error("NewPlanned was not called")
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

	p, ok := providers[0].(ImmediateProvider)
	if !ok {
		t.Fatal("expected ImmediateProvider type assertion to succeed")
	}
	p.NewImmediate(BindingConfig{})
	if !ip.immediateCalled {
		t.Error("NewImmediate was not called")
	}
}

func TestInitAll_plain_provider_not_PlannedProvider(t *testing.T) {
	resetAnnounced()

	plain := &announceTestProvider{name: "plain"}
	Announce(plain)

	providers := Providers()
	if _, ok := providers[0].(PlannedProvider); ok {
		t.Error("plain announceTestProvider should not satisfy PlannedProvider")
	}
	if _, ok := providers[0].(ImmediateProvider); ok {
		t.Error("plain announceTestProvider should not satisfy ImmediateProvider")
	}
}

func TestAnnounce_concurrent(t *testing.T) {
	resetAnnounced()

	var wg sync.WaitGroup
	for i := range 100 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			Announce(&announceTestProvider{name: "concurrent"})
			_ = n
		}(i)
	}
	wg.Wait()

	providers := Providers()
	if len(providers) != 100 {
		t.Errorf("Providers() returned %d, want 100", len(providers))
	}
}
