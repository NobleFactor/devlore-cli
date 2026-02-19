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

// TemplatePlan implements plan.template.* bindings using the slot-based model.
type TemplatePlan struct {
	Receiver
	graph   *execution.Graph
	host    host.Host
	project string
	reg     *execution.ActionRegistry
}

// NewTemplatePlan creates a new TemplatePlan for the given graph and host.
func NewTemplatePlan(graph *execution.Graph, h host.Host, project string, reg *execution.ActionRegistry) *TemplatePlan {
	return &TemplatePlan{
		Receiver: NewReceiver("plan.template"),
		graph:    graph,
		host:     h,
		project:  project,
		reg:      reg,
	}
}

// Attr implements starlark.HasAttrs.
func (t *TemplatePlan) Attr(name string) (starlark.Value, error) {
	switch name {
	case "render":
		return MakeAttr("plan.template.render", t.render), nil
	default:
		return nil, NoSuchAttrError("plan.template", name)
	}
}

// AttrNames implements starlark.HasAttrs.
func (t *TemplatePlan) AttrNames() []string {
	return []string{"render"}
}

func (t *TemplatePlan) render(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var source starlark.Value
	if err := starlark.UnpackArgs("render", args, kwargs, "source", &source); err != nil {
		return nil, err
	}

	node := &execution.Node{
		ID:      generateNodeID("render"),
		Action:  t.reg.MustGet("template.render"),
		Project: t.project,
	}

	if err := FillSlot(node, t.graph, "source", source); err != nil {
		return nil, fmt.Errorf("render: source: %w", err)
	}

	t.graph.Nodes = append(t.graph.Nodes, node)
	return NewOutput(node, t.graph, ""), nil
}
