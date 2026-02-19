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

// NetPlan implements plan.net.* bindings using the slot-based model.
type NetPlan struct {
	Receiver
	graph   *execution.Graph
	host    host.Host
	project string
	reg     *execution.ActionRegistry
}

// NewNetPlan creates a new NetPlan for the given graph and host.
func NewNetPlan(graph *execution.Graph, h host.Host, project string, reg *execution.ActionRegistry) *NetPlan {
	return &NetPlan{
		Receiver: NewReceiver("plan.net"),
		graph:    graph,
		host:     h,
		project:  project,
		reg:      reg,
	}
}

// Attr implements starlark.HasAttrs.
func (n *NetPlan) Attr(name string) (starlark.Value, error) {
	switch name {
	case "download":
		return MakeAttr("plan.net.download", n.download), nil
	default:
		return nil, NoSuchAttrError("plan.net", name)
	}
}

// AttrNames implements starlark.HasAttrs.
func (n *NetPlan) AttrNames() []string {
	return []string{"download"}
}

func (n *NetPlan) download(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var url starlark.Value
	if err := starlark.UnpackArgs("download", args, kwargs, "url", &url); err != nil {
		return nil, err
	}

	node := &execution.Node{
		ID:      generateNodeID("download"),
		Action:  n.reg.MustGet("net.download"),
		Project: n.project,
	}

	if err := FillSlot(node, n.graph, "url", url); err != nil {
		return nil, fmt.Errorf("download: url: %w", err)
	}

	n.graph.Nodes = append(n.graph.Nodes, node)
	return NewOutput(node, n.graph, ""), nil
}
