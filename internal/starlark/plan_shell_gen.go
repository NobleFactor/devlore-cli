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

// ShellPlan implements plan.shell.* bindings using the slot-based model.
type ShellPlan struct {
	Receiver
	graph   *execution.Graph
	host    host.Host
	project string
	reg     *execution.ActionRegistry
}

// NewShellPlan creates a new ShellPlan for the given graph and host.
func NewShellPlan(graph *execution.Graph, h host.Host, project string, reg *execution.ActionRegistry) *ShellPlan {
	return &ShellPlan{
		Receiver: NewReceiver("plan.shell"),
		graph:    graph,
		host:     h,
		project:  project,
		reg:      reg,
	}
}

// Attr implements starlark.HasAttrs.
func (s *ShellPlan) Attr(name string) (starlark.Value, error) {
	switch name {
	case "exec":
		return MakeAttr("plan.shell.exec", s.exec), nil
	default:
		return nil, NoSuchAttrError("plan.shell", name)
	}
}

// AttrNames implements starlark.HasAttrs.
func (s *ShellPlan) AttrNames() []string {
	return []string{"exec"}
}

func (s *ShellPlan) exec(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var command starlark.Value
	if err := starlark.UnpackArgs("exec", args, kwargs, "command", &command); err != nil {
		return nil, err
	}

	node := &execution.Node{
		ID:      generateNodeID("shell"),
		Action:  s.reg.MustGet("shell.exec"),
		Project: s.project,
	}

	if err := FillSlot(node, s.graph, "command", command); err != nil {
		return nil, fmt.Errorf("exec: command: %w", err)
	}

	s.graph.Nodes = append(s.graph.Nodes, node)
	return NewOutput(node, s.graph, ""), nil
}
