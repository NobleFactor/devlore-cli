// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlark

import (
	"fmt"

	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/internal/execution"
	"github.com/NobleFactor/devlore-cli/internal/host"
)

// TemplatePlan implements plan.template.* bindings using the slot-based model.
type TemplatePlan struct {
	graph   *execution.Graph
	host    host.Host
	project string
}

// NewTemplatePlan creates a new TemplatePlan for the given graph and host.
func NewTemplatePlan(graph *execution.Graph, h host.Host, project string) *TemplatePlan {
	return &TemplatePlan{
		graph:   graph,
		host:    h,
		project: project,
	}
}

// Starlark Value interface
func (t *TemplatePlan) String() string        { return "plan.template" }
func (t *TemplatePlan) Type() string          { return "plan.template" }
func (t *TemplatePlan) Freeze()               {}
func (t *TemplatePlan) Truth() starlark.Bool  { return true }
func (t *TemplatePlan) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable: plan.template") }

// Starlark HasAttrs interface
func (t *TemplatePlan) Attr(name string) (starlark.Value, error) {
	switch name {
	case "render":
		return starlark.NewBuiltin("plan.template.render", t.render), nil
	default:
		return nil, starlark.NoSuchAttrError(fmt.Sprintf("plan.template has no attribute %q", name))
	}
}

func (t *TemplatePlan) AttrNames() []string {
	return []string{"render"}
}

// render adds a template rendering node.
// Usage: plan.template.render(source)
//
// Slots:
//   - source: Input file/content (promise or immediate)
//
// Returns: Promise of the rendered content
func (t *TemplatePlan) render(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var source starlark.Value
	if err := starlark.UnpackArgs("render", args, kwargs, "source", &source); err != nil {
		return nil, err
	}

	node := &execution.Node{
		ID:      generateNodeID("render"),
		Action:  "render",
		Project: t.project,
	}

	if err := FillSlot(node, t.graph, "source", source); err != nil {
		return nil, fmt.Errorf("render: source: %w", err)
	}

	t.graph.Nodes = append(t.graph.Nodes, node)
	return NewOutput(node, t.graph, ""), nil
}
