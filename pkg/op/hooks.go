// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

// LifecycleHook receives events at subgraph and node boundaries during execution.
// Hooks are fire-and-forget — a hook panic is recovered and logged but does not
// fail the node or subgraph. Hooks run synchronously and must not block.
type LifecycleHook interface {
	OnNodeStart(ctx *RuntimeEnvironment, nodeID string, slots map[string]any)
	OnNodeComplete(ctx *RuntimeEnvironment, nodeID string, result Result, err error)
	OnSubgraphStart(ctx *RuntimeEnvironment, subgraphID string)
	OnSubgraphComplete(ctx *RuntimeEnvironment, subgraphID string, err error)
}

// HookRegistry holds registered lifecycle hooks and provides fire methods.
// A nil *HookRegistry is safe to use — all fire methods are no-ops.
type HookRegistry struct {
	hooks []LifecycleHook
}

// NewHookRegistry creates an empty hook registry.
func NewHookRegistry() *HookRegistry {
	return &HookRegistry{}
}

// Register adds a lifecycle hook to the registry.
func (r *HookRegistry) Register(hook LifecycleHook) {
	r.hooks = append(r.hooks, hook)
}

// FireNodeStart notifies all hooks that a node is about to execute.
func (r *HookRegistry) FireNodeStart(runtimeEnvironment *RuntimeEnvironment, nodeID string, slots map[string]any) {
	if r == nil {
		return
	}
	for _, h := range r.hooks {
		func() {
			defer func() { _ = recover() }() //nolint:errcheck // intentional panic recovery
			h.OnNodeStart(runtimeEnvironment, nodeID, slots)
		}()
	}
}

// FireNodeComplete notifies all hooks that a node has finished.
func (r *HookRegistry) FireNodeComplete(runtimeEnvironment *RuntimeEnvironment, nodeID string, result Result, err error) {
	if r == nil {
		return
	}
	for _, h := range r.hooks {
		func() {
			defer func() { recover() }() //nolint:errcheck // intentional panic recovery
			h.OnNodeComplete(runtimeEnvironment, nodeID, result, err)
		}()
	}
}

// FireSubgraphStart notifies all hooks that a subgraph is about to execute.
func (r *HookRegistry) FireSubgraphStart(runtimeEnvironment *RuntimeEnvironment, subgraphID string) {
	if r == nil {
		return
	}
	for _, h := range r.hooks {
		func() {
			defer func() { recover() }() //nolint:errcheck // intentional panic recovery
			h.OnSubgraphStart(runtimeEnvironment, subgraphID)
		}()
	}
}

// FireSubgraphComplete notifies all hooks that a subgraph has finished.
func (r *HookRegistry) FireSubgraphComplete(runtimeEnvironment *RuntimeEnvironment, subgraphID string, err error) {
	if r == nil {
		return
	}
	for _, h := range r.hooks {
		func() {
			defer func() { recover() }() //nolint:errcheck // intentional panic recovery
			h.OnSubgraphComplete(runtimeEnvironment, subgraphID, err)
		}()
	}
}
