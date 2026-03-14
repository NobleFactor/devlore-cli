// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import "go.starlark.net/starlark"

// StarlarkRuntime manages an immediate-mode Starlark scripting runtime using announced
// receivers. Receivers listed in BindingConfig.Receivers are constructed as Starlark globals.
// External consumers (e.g., noblefactor-ops) use this to obtain framework-managed receivers
// without hand-coding receiver construction.
type StarlarkRuntime struct {
	cfg      *BindingConfig
	included map[string]bool
	ctx      Context // stored by Initialize for receiver context injection
}

// NewStarlarkRuntime creates a runtime with the given configuration.
// Receivers listed in cfg.Receivers are included when BuildReceivers is called.
//
// Parameters:
//   - cfg: configuration specifying which receivers to include.
//
// Returns:
//   - *StarlarkRuntime: the initialized runtime.
func NewStarlarkRuntime(cfg *BindingConfig) *StarlarkRuntime {

	included := make(map[string]bool, len(cfg.Receivers))
	for _, p := range cfg.Receivers {
		included[p.ReceiverName()] = true
	}
	return &StarlarkRuntime{
		cfg:      cfg,
		included: included,
	}
}

// region EXPORTED METHODS

// region State management

// Included reports whether the named provider is in the receiver set.
//
// Parameters:
//   - name: the provider name to check.
//
// Returns:
//   - bool: true if the provider is included in this runtime's receiver set.
func (rt *StarlarkRuntime) Included(name string) bool {

	return rt.included[name]
}

// HasGraphBuilder reports whether the plan.* graph namespace is enabled.
//
// Returns:
//   - bool: true if the graph builder is included.
func (rt *StarlarkRuntime) HasGraphBuilder() bool {

	return rt.cfg.GraphBuilder
}

// endregion

// region Behaviors

// Initialize registers all providers' actions with the registry and stores the context
// for later receiver initialization.
//
// Parameters:
//   - reg: the action registry to populate.
//   - ctx: the base execution context for provider initialization.
func (rt *StarlarkRuntime) Initialize(reg *ActionRegistry, ctx ContextBase) {

	rt.ctx = Context{ContextBase: ctx}
	InitAll(reg, rt.ctx)
}

// BuildReceiver constructs a single immediate receiver by provider name.
//
// Parameters:
//   - name: the provider name to build.
//
// Returns:
//   - starlark.Value: the constructed receiver, or nil if not found.
//   - bool: true if the provider was found and implements ExecutingReceiverFactory.
func (rt *StarlarkRuntime) BuildReceiver(name string) (starlark.Value, bool) {

	for _, p := range Providers() {
		if p.ReceiverName() != name {
			continue
		}
		if recv := rt.buildOne(p); recv != nil {
			return recv, true
		}
		return nil, false
	}
	return nil, false
}

// BuildReceivers constructs immediate receivers for factories listed in cfg.Receivers.
//
// Returns:
//   - starlark.StringDict: receiver map suitable for merging into Starlark globals.
func (rt *StarlarkRuntime) BuildReceivers() starlark.StringDict {

	globals := starlark.StringDict{}
	for _, p := range Providers() {
		if !rt.included[p.ReceiverName()] {
			continue
		}
		if recv := rt.buildOne(p); recv != nil {
			globals[p.ReceiverName()] = recv
		}
	}
	return globals
}

// endregion

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// buildOne constructs an immediate receiver from a provider, injecting context if available.
//
// Parameters:
//   - p: the provider to construct a receiver from.
//
// Returns:
//   - starlark.Value: the constructed receiver, or nil if the provider is not immediate.
func (rt *StarlarkRuntime) buildOne(p ReceiverFactory) starlark.Value {

	ip, ok := p.(ExecutingReceiverFactory)
	if !ok {
		return nil
	}
	recv := ip.NewExecuting(rt.ctx)
	if rr, ok := recv.(*ExecutingReceiver); ok && rt.ctx.Root != nil {
		rr.SetContext(rt.ctx)
	}
	return recv
}

// endregion

// endregion
