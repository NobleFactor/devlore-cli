// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import "go.starlark.net/starlark"

// StarlarkRuntime manages an immediate-mode Starlark scripting runtime using announced
// providers. Receivers listed in BindingConfig.Receivers are constructed as Starlark globals.
// External consumers (e.g., noblefactor-ops) use this to obtain framework-managed receivers
// without hand-coding receiver construction.
type StarlarkRuntime struct {
	cfg      BindingConfig
	included map[string]bool
	ctx      Context // stored by Initialize for receiver context injection
}

// NewStarlarkRuntime creates a runtime with the given configuration.
// Receivers listed in cfg.Receivers are included when BuildReceivers is called.
func NewStarlarkRuntime(cfg BindingConfig) *StarlarkRuntime {
	included := make(map[string]bool, len(cfg.Receivers))
	for _, name := range cfg.Receivers {
		included[name] = true
	}
	return &StarlarkRuntime{
		cfg:      cfg,
		included: included,
	}
}

// Initialize registers all providers' actions with the registry and stores the context
// for later receiver initialization.
func (rt *StarlarkRuntime) Initialize(reg *ActionRegistry, ctx ContextBase) {
	rt.ctx = Context{ContextBase: ctx}
	InitAll(reg, rt.ctx)
}

// BuildReceivers constructs immediate receivers for providers listed in cfg.Receivers.
// Returns a StringDict suitable for merging into Starlark globals.
func (rt *StarlarkRuntime) BuildReceivers() starlark.StringDict {
	globals := starlark.StringDict{}
	for _, p := range Providers() {
		if !rt.included[p.Name()] {
			continue
		}
		if recv := rt.buildOne(p); recv != nil {
			globals[p.Name()] = recv
		}
	}
	return globals
}

// BuildReceiver constructs a single immediate receiver by provider name.
// Returns the receiver and true, or nil and false if the provider is not
// found or does not implement ImmediateProvider.
func (rt *StarlarkRuntime) BuildReceiver(name string) (starlark.Value, bool) {
	for _, p := range Providers() {
		if p.Name() != name {
			continue
		}
		if recv := rt.buildOne(p); recv != nil {
			return recv, true
		}
		return nil, false
	}
	return nil, false
}

// Included reports whether the named provider is in the receiver set.
func (rt *StarlarkRuntime) Included(name string) bool {
	return rt.included[name]
}

// buildOne constructs an immediate receiver from a provider, injecting context if available.
func (rt *StarlarkRuntime) buildOne(p Provider) starlark.Value {
	ip, ok := p.(ImmediateProvider)
	if !ok {
		return nil
	}
	recv := ip.NewImmediate(rt.cfg)
	if rr, ok := recv.(*ReflectedReceiver); ok && rt.ctx.Root != nil {
		rr.SetContext(rt.ctx)
	}
	return recv
}
