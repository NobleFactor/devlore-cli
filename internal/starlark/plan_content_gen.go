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

// ContentPlan implements plan.content.* bindings using the slot-based model.
type ContentPlan struct {
	Receiver
	graph   *execution.Graph
	host    host.Host
	project string
	reg     *execution.ActionRegistry
}

// NewContentPlan creates a new ContentPlan for the given graph and host.
func NewContentPlan(graph *execution.Graph, h host.Host, project string, reg *execution.ActionRegistry) *ContentPlan {
	return &ContentPlan{
		Receiver: NewReceiver("plan.content"),
		graph:    graph,
		host:     h,
		project:  project,
		reg:      reg,
	}
}

// Attr implements starlark.HasAttrs.
func (c *ContentPlan) Attr(name string) (starlark.Value, error) {
	switch name {
	case "literal":
		return MakeAttr("plan.content.literal", c.literal), nil
	default:
		return nil, NoSuchAttrError("plan.content", name)
	}
}

// AttrNames implements starlark.HasAttrs.
func (c *ContentPlan) AttrNames() []string {
	return []string{"literal"}
}

func (c *ContentPlan) literal(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var content starlark.Value
	if err := starlark.UnpackArgs("literal", args, kwargs, "content", &content); err != nil {
		return nil, err
	}

	node := &execution.Node{
		ID:      generateNodeID("literal"),
		Action:  c.reg.MustGet("content.literal"),
		Project: c.project,
	}

	if err := FillSlot(node, c.graph, "content", content); err != nil {
		return nil, fmt.Errorf("literal: content: %w", err)
	}

	c.graph.Nodes = append(c.graph.Nodes, node)
	return NewOutput(node, c.graph, ""), nil
}
