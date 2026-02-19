// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlark

import (
	"io"

	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/internal/execution/provider/service"
	"github.com/NobleFactor/devlore-cli/internal/host"
)

// ServiceReceiver provides the service.* Starlark namespace.
// Forward operations (start, stop, restart, enable, disable) delegate to
// service.Provider. Query operations (exists, status) use the platform's
// ServiceManager directly.
type ServiceReceiver struct {
	Receiver
	provider *service.Provider
	sm       host.ServiceManager
	output   io.Writer
}

// NewServiceReceiver creates a new service receiver.
func NewServiceReceiver(sm host.ServiceManager, output io.Writer) *ServiceReceiver {
	return &ServiceReceiver{
		Receiver: NewReceiver("service"),
		provider: &service.Provider{},
		sm:       sm,
		output:   output,
	}
}

func (r *ServiceReceiver) queryAttr(name string) (starlark.Value, error) {
	switch name {
	case "exists":
		return MakeAttr("service.exists", r.exists), nil
	case "status":
		return MakeAttr("service.status", r.status), nil
	default:
		return nil, NoSuchAttrError("service", name)
	}
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
