// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package execution

import "github.com/NobleFactor/devlore-cli/pkg/op"

// LifecycleHook receives events at phase and node boundaries during execution.
// Hooks are fire-and-forget — a hook panic is recovered and logged but does not
// fail the node or phase. Hooks run synchronously and must not block.
type LifecycleHook interface {
	OnNodeStart(ctx *op.Context, nodeID string, slots map[string]any)
	OnNodeComplete(ctx *op.Context, nodeID string, result op.Result, err error)
	OnPhaseStart(ctx *op.Context, phaseID string)
	OnPhaseComplete(ctx *op.Context, phaseID string, err error)
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
func (r *HookRegistry) FireNodeStart(ctx *op.Context, nodeID string, slots map[string]any) {
	if r == nil {
		return
	}
	for _, h := range r.hooks {
		func() {
			defer func() { _ = recover() }() //nolint:errcheck // intentional panic recovery
			h.OnNodeStart(ctx, nodeID, slots)
		}()
	}
}

// FireNodeComplete notifies all hooks that a node has finished.
func (r *HookRegistry) FireNodeComplete(ctx *op.Context, nodeID string, result op.Result, err error) {
	if r == nil {
		return
	}
	for _, h := range r.hooks {
		func() {
			defer func() { recover() }() //nolint:errcheck // intentional panic recovery
			h.OnNodeComplete(ctx, nodeID, result, err)
		}()
	}
}

// FirePhaseStart notifies all hooks that a phase is about to execute.
func (r *HookRegistry) FirePhaseStart(ctx *op.Context, phaseID string) {
	if r == nil {
		return
	}
	for _, h := range r.hooks {
		func() {
			defer func() { recover() }() //nolint:errcheck // intentional panic recovery
			h.OnPhaseStart(ctx, phaseID)
		}()
	}
}

// FirePhaseComplete notifies all hooks that a phase has finished.
func (r *HookRegistry) FirePhaseComplete(ctx *op.Context, phaseID string, err error) {
	if r == nil {
		return
	}
	for _, h := range r.hooks {
		func() {
			defer func() { recover() }() //nolint:errcheck // intentional panic recovery
			h.OnPhaseComplete(ctx, phaseID, err)
		}()
	}
}
