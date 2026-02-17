// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package execution

// LifecycleHook receives events at phase and node boundaries during execution.
// Hooks are fire-and-forget — a hook panic is recovered and logged but does not
// fail the node or phase. Hooks run synchronously and must not block.
type LifecycleHook interface {
	OnNodeStart(ctx *Context, nodeID string, slots map[string]any)
	OnNodeComplete(ctx *Context, nodeID string, result Result, err error)
	OnPhaseStart(ctx *Context, phaseID string)
	OnPhaseComplete(ctx *Context, phaseID string, err error)
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
func (r *HookRegistry) FireNodeStart(ctx *Context, nodeID string, slots map[string]any) {
	if r == nil {
		return
	}
	for _, h := range r.hooks {
		func() {
			defer func() { recover() }()
			h.OnNodeStart(ctx, nodeID, slots)
		}()
	}
}

// FireNodeComplete notifies all hooks that a node has finished.
func (r *HookRegistry) FireNodeComplete(ctx *Context, nodeID string, result Result, err error) {
	if r == nil {
		return
	}
	for _, h := range r.hooks {
		func() {
			defer func() { recover() }()
			h.OnNodeComplete(ctx, nodeID, result, err)
		}()
	}
}

// FirePhaseStart notifies all hooks that a phase is about to execute.
func (r *HookRegistry) FirePhaseStart(ctx *Context, phaseID string) {
	if r == nil {
		return
	}
	for _, h := range r.hooks {
		func() {
			defer func() { recover() }()
			h.OnPhaseStart(ctx, phaseID)
		}()
	}
}

// FirePhaseComplete notifies all hooks that a phase has finished.
func (r *HookRegistry) FirePhaseComplete(ctx *Context, phaseID string, err error) {
	if r == nil {
		return
	}
	for _, h := range r.hooks {
		func() {
			defer func() { recover() }()
			h.OnPhaseComplete(ctx, phaseID, err)
		}()
	}
}
