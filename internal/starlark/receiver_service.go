// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlark

import (
	"io"

	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/internal/host"
)

// ServiceReceiver provides the service.* Starlark namespace.
//
// Backing implementation: host.ServiceManager.
type ServiceReceiver struct {
	Receiver
	sm     host.ServiceManager
	output io.Writer
}

// NewServiceReceiver creates a new service receiver.
func NewServiceReceiver(sm host.ServiceManager, output io.Writer) *ServiceReceiver {
	return &ServiceReceiver{
		Receiver: NewReceiver("service"),
		sm:       sm,
		output:   output,
	}
}

func (r *ServiceReceiver) Attr(name string) (starlark.Value, error) {
	switch name {
	case "exists":
		return MakeAttr("service.exists", r.exists), nil
	case "status":
		return MakeAttr("service.status", r.status), nil
	case "start":
		return MakeAttr("service.start", r.start), nil
	case "stop":
		return MakeAttr("service.stop", r.stop), nil
	case "enable":
		return MakeAttr("service.enable", r.enable), nil
	case "disable":
		return MakeAttr("service.disable", r.disable), nil
	default:
		return nil, NoSuchAttrError("service", name)
	}
}

func (r *ServiceReceiver) AttrNames() []string {
	return []string{"disable", "enable", "exists", "start", "status", "stop"}
}

func (r *ServiceReceiver) exists(_ *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "name", &name); err != nil {
		return nil, err
	}
	return starlark.Bool(r.sm.Exists(name)), nil
}

func (r *ServiceReceiver) status(_ *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "name", &name); err != nil {
		return nil, err
	}
	return starlark.String(r.sm.Status(name)), nil
}

func (r *ServiceReceiver) start(_ *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "name", &name); err != nil {
		return nil, err
	}
	return resultToStarlark(r.sm.Start(name)), nil
}

func (r *ServiceReceiver) stop(_ *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "name", &name); err != nil {
		return nil, err
	}
	return resultToStarlark(r.sm.Stop(name)), nil
}

func (r *ServiceReceiver) enable(_ *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "name", &name); err != nil {
		return nil, err
	}
	return resultToStarlark(r.sm.Enable(name)), nil
}

func (r *ServiceReceiver) disable(_ *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "name", &name); err != nil {
		return nil, err
	}
	return resultToStarlark(r.sm.Disable(name)), nil
}
