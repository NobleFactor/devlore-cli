// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package service

import (
	"fmt"

	"github.com/NobleFactor/devlore-cli/internal/execution"
)

// Start starts a service.
type Start struct{ Impl *Provider }

func (o *Start) Name() string { return "service.start" }

func (o *Start) Do(ctx *execution.Context, slots map[string]any) (execution.Result, execution.UndoState, error) {
	name, _ := slots["name"].(string)
	if name == "" {
		return nil, nil, fmt.Errorf("service-start: no service specified")
	}

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] service-start %v\n", name)
		return nil, nil, nil
	}
	state, err := o.Impl.Start(name, ctx.Logger)
	return nil, state, err
}

func (o *Start) Undo(_ *execution.Context, _ map[string]any, state execution.UndoState) error {
	s, _ := state.(map[string]any)
	if s == nil {
		return nil
	}
	return o.Impl.CompensateStart(s)
}

// Stop stops a service.
type Stop struct{ Impl *Provider }

func (o *Stop) Name() string { return "service.stop" }

func (o *Stop) Do(ctx *execution.Context, slots map[string]any) (execution.Result, execution.UndoState, error) {
	name, _ := slots["name"].(string)
	if name == "" {
		return nil, nil, fmt.Errorf("service-stop: no service specified")
	}

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] service-stop %v\n", name)
		return nil, nil, nil
	}
	state, err := o.Impl.Stop(name, ctx.Logger)
	return nil, state, err
}

func (o *Stop) Undo(_ *execution.Context, _ map[string]any, state execution.UndoState) error {
	s, _ := state.(map[string]any)
	if s == nil {
		return nil
	}
	return o.Impl.CompensateStop(s)
}

// Restart restarts a service.
type Restart struct{ Impl *Provider }

func (o *Restart) Name() string { return "service.restart" }

func (o *Restart) Do(ctx *execution.Context, slots map[string]any) (execution.Result, execution.UndoState, error) {
	name, _ := slots["name"].(string)
	if name == "" {
		return nil, nil, fmt.Errorf("service-restart: no service specified")
	}

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] service-restart %v\n", name)
		return nil, nil, nil
	}
	state, err := o.Impl.Restart(name, ctx.Logger)
	return nil, state, err
}

func (o *Restart) Undo(_ *execution.Context, _ map[string]any, state execution.UndoState) error {
	s, _ := state.(map[string]any)
	if s == nil {
		return nil
	}
	return o.Impl.CompensateRestart(s)
}

// Enable enables a service to start at boot.
type Enable struct{ Impl *Provider }

func (o *Enable) Name() string { return "service.enable" }

func (o *Enable) Do(ctx *execution.Context, slots map[string]any) (execution.Result, execution.UndoState, error) {
	name, _ := slots["name"].(string)
	if name == "" {
		return nil, nil, fmt.Errorf("service-enable: no service specified")
	}

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] service-enable %v\n", name)
		return nil, nil, nil
	}
	state, err := o.Impl.Enable(name, ctx.Logger)
	return nil, state, err
}

func (o *Enable) Undo(_ *execution.Context, _ map[string]any, state execution.UndoState) error {
	s, _ := state.(map[string]any)
	if s == nil {
		return nil
	}
	return o.Impl.CompensateEnable(s)
}

// Disable disables a service from starting at boot.
type Disable struct{ Impl *Provider }

func (o *Disable) Name() string { return "service.disable" }

func (o *Disable) Do(ctx *execution.Context, slots map[string]any) (execution.Result, execution.UndoState, error) {
	name, _ := slots["name"].(string)
	if name == "" {
		return nil, nil, fmt.Errorf("service-disable: no service specified")
	}

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] service-disable %v\n", name)
		return nil, nil, nil
	}
	state, err := o.Impl.Disable(name, ctx.Logger)
	return nil, state, err
}

func (o *Disable) Undo(_ *execution.Context, _ map[string]any, state execution.UndoState) error {
	s, _ := state.(map[string]any)
	if s == nil {
		return nil
	}
	return o.Impl.CompensateDisable(s)
}

// Register registers all service actions with the given registry.
func Register(reg *execution.ActionRegistry) {
	p := &Provider{}
	reg.Register(&Start{Impl: p})
	reg.Register(&Stop{Impl: p})
	reg.Register(&Restart{Impl: p})
	reg.Register(&Enable{Impl: p})
	reg.Register(&Disable{Impl: p})
}
