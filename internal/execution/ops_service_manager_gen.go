// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Code generated from gen-receiver templates; DO NOT EDIT.

package execution

import "fmt"

// ServiceManagerStartOp starts a service.
type ServiceManagerStartOp struct{ impl *ServiceManagerService }

func (o *ServiceManagerStartOp) Name() string { return "service-start" }

func (o *ServiceManagerStartOp) Do(ctx *Context, node *Node) (Result, UndoState, error) {
	name, _ := node.GetSlot("name").(string)
	if name == "" {
		return nil, nil, fmt.Errorf("service-start: no service specified")
	}

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] service-start %v\n", name)
		return nil, nil, nil
	}
	return nil, nil, o.impl.Start(name, ctx.Logger)
}

func (o *ServiceManagerStartOp) Undo(_ *Context, _ *Node, _ UndoState) error { return nil }

// ServiceManagerStopOp stops a service.
type ServiceManagerStopOp struct{ impl *ServiceManagerService }

func (o *ServiceManagerStopOp) Name() string { return "service-stop" }

func (o *ServiceManagerStopOp) Do(ctx *Context, node *Node) (Result, UndoState, error) {
	name, _ := node.GetSlot("name").(string)
	if name == "" {
		return nil, nil, fmt.Errorf("service-stop: no service specified")
	}

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] service-stop %v\n", name)
		return nil, nil, nil
	}
	return nil, nil, o.impl.Stop(name, ctx.Logger)
}

func (o *ServiceManagerStopOp) Undo(_ *Context, _ *Node, _ UndoState) error { return nil }

// ServiceManagerRestartOp restarts a service.
type ServiceManagerRestartOp struct{ impl *ServiceManagerService }

func (o *ServiceManagerRestartOp) Name() string { return "service-restart" }

func (o *ServiceManagerRestartOp) Do(ctx *Context, node *Node) (Result, UndoState, error) {
	name, _ := node.GetSlot("name").(string)
	if name == "" {
		return nil, nil, fmt.Errorf("service-restart: no service specified")
	}

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] service-restart %v\n", name)
		return nil, nil, nil
	}
	return nil, nil, o.impl.Restart(name, ctx.Logger)
}

func (o *ServiceManagerRestartOp) Undo(_ *Context, _ *Node, _ UndoState) error { return nil }

// ServiceManagerEnableOp enables a service to start at boot.
type ServiceManagerEnableOp struct{ impl *ServiceManagerService }

func (o *ServiceManagerEnableOp) Name() string { return "service-enable" }

func (o *ServiceManagerEnableOp) Do(ctx *Context, node *Node) (Result, UndoState, error) {
	name, _ := node.GetSlot("name").(string)
	if name == "" {
		return nil, nil, fmt.Errorf("service-enable: no service specified")
	}

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] service-enable %v\n", name)
		return nil, nil, nil
	}
	return nil, nil, o.impl.Enable(name, ctx.Logger)
}

func (o *ServiceManagerEnableOp) Undo(_ *Context, _ *Node, _ UndoState) error { return nil }

// ServiceManagerDisableOp disables a service from starting at boot.
type ServiceManagerDisableOp struct{ impl *ServiceManagerService }

func (o *ServiceManagerDisableOp) Name() string { return "service-disable" }

func (o *ServiceManagerDisableOp) Do(ctx *Context, node *Node) (Result, UndoState, error) {
	name, _ := node.GetSlot("name").(string)
	if name == "" {
		return nil, nil, fmt.Errorf("service-disable: no service specified")
	}

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] service-disable %v\n", name)
		return nil, nil, nil
	}
	return nil, nil, o.impl.Disable(name, ctx.Logger)
}

func (o *ServiceManagerDisableOp) Undo(_ *Context, _ *Node, _ UndoState) error { return nil }

// ServiceManagerOps returns all service manager actions backed by the given ServiceManagerService.
func ServiceManagerOps(impl *ServiceManagerService) []Action {
	return []Action{
		&ServiceManagerStartOp{impl: impl},
		&ServiceManagerStopOp{impl: impl},
		&ServiceManagerRestartOp{impl: impl},
		&ServiceManagerEnableOp{impl: impl},
		&ServiceManagerDisableOp{impl: impl},
	}
}
