// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Code generated from gen-receiver templates; DO NOT EDIT.

package starlark

import (
	"fmt"

	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/internal/execution"
	"github.com/NobleFactor/devlore-cli/internal/host"
)

// ServicePlan implements plan.service.* bindings using the slot-based model.
// Each method adds a service management node to the execution graph.
type ServicePlan struct {
	Receiver
	graph   *execution.Graph
	host    host.Host
	project string
	reg     *execution.ActionRegistry
}

// NewServicePlan creates a new ServicePlan for the given graph and host.
func NewServicePlan(graph *execution.Graph, h host.Host, project string, reg *execution.ActionRegistry) *ServicePlan {
	return &ServicePlan{
		Receiver: NewReceiver("plan.service"),
		graph:    graph,
		host:     h,
		project:  project,
		reg:      reg,
	}
}

// Attr implements starlark.HasAttrs.
func (s *ServicePlan) Attr(name string) (starlark.Value, error) {
	switch name {
	case "start":
		return MakeAttr("plan.service.start", s.start), nil
	case "stop":
		return MakeAttr("plan.service.stop", s.stop), nil
	case "restart":
		return MakeAttr("plan.service.restart", s.restart), nil
	case "enable":
		return MakeAttr("plan.service.enable", s.enable), nil
	case "disable":
		return MakeAttr("plan.service.disable", s.disable), nil
	// Predicate methods — return RuntimePredicate for plan.choose()
	case "exists":
		return starlark.NewBuiltin("plan.service.exists", s.predicateExists), nil
	case "running":
		return starlark.NewBuiltin("plan.service.running", s.predicateRunning), nil
	case "enabled":
		return starlark.NewBuiltin("plan.service.enabled", s.predicateEnabled), nil
	default:
		return nil, NoSuchAttrError("plan.service", name)
	}
}

func (s *ServicePlan) predicateExists(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	if err := starlark.UnpackArgs("exists", args, kwargs, "name", &name); err != nil {
		return nil, err
	}
	return serviceExists(s.host.ServiceManager(), name), nil
}

func (s *ServicePlan) predicateRunning(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	if err := starlark.UnpackArgs("running", args, kwargs, "name", &name); err != nil {
		return nil, err
	}
	return serviceRunning(s.host.ServiceManager(), name), nil
}

func (s *ServicePlan) predicateEnabled(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	if err := starlark.UnpackArgs("enabled", args, kwargs, "name", &name); err != nil {
		return nil, err
	}
	return serviceEnabled(s.host.ServiceManager(), name), nil
}

// AttrNames implements starlark.HasAttrs.
func (s *ServicePlan) AttrNames() []string {
	return []string{"disable", "enable", "enabled", "exists", "restart", "running", "start", "stop"}
}

func (s *ServicePlan) start(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return s.serviceAction("start", args, kwargs)
}

func (s *ServicePlan) stop(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return s.serviceAction("stop", args, kwargs)
}

func (s *ServicePlan) restart(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return s.serviceAction("restart", args, kwargs)
}

func (s *ServicePlan) enable(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return s.serviceAction("enable", args, kwargs)
}

func (s *ServicePlan) disable(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return s.serviceAction("disable", args, kwargs)
}

func (s *ServicePlan) serviceAction(action string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name starlark.Value
	if err := starlark.UnpackArgs(action, args, kwargs, "name", &name); err != nil {
		return nil, err
	}

	node := &execution.Node{
		ID:      generateNodeID("service-" + action),
		Action:  s.reg.MustGet("service." + action),
		Project: s.project,
	}

	if err := FillSlot(node, s.graph, "name", name); err != nil {
		return nil, fmt.Errorf("%s: name: %w", action, err)
	}

	s.graph.Nodes = append(s.graph.Nodes, node)
	return NewOutput(node, s.graph, ""), nil
}
