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

// ArchivePlan implements plan.archive.* bindings using the slot-based model.
type ArchivePlan struct {
	Receiver
	graph   *execution.Graph
	host    host.Host
	project string
	reg     *execution.ActionRegistry
}

// NewArchivePlan creates a new ArchivePlan for the given graph and host.
func NewArchivePlan(graph *execution.Graph, h host.Host, project string, reg *execution.ActionRegistry) *ArchivePlan {
	return &ArchivePlan{
		Receiver: NewReceiver("plan.archive"),
		graph:    graph,
		host:     h,
		project:  project,
		reg:      reg,
	}
}

// Attr implements starlark.HasAttrs.
func (a *ArchivePlan) Attr(name string) (starlark.Value, error) {
	switch name {
	case "extract":
		return MakeAttr("plan.archive.extract", a.extract), nil
	default:
		return nil, NoSuchAttrError("plan.archive", name)
	}
}

// AttrNames implements starlark.HasAttrs.
func (a *ArchivePlan) AttrNames() []string {
	return []string{"extract"}
}

func (a *ArchivePlan) extract(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var archive, prefix starlark.Value
	if err := starlark.UnpackArgs("extract", args, kwargs, "archive", &archive, "prefix", &prefix); err != nil {
		return nil, err
	}

	node := &execution.Node{
		ID:        generateNodeID("archive-extract"),
		Action: a.reg.MustGet("archive.extract"),
		Project:   a.project,
	}

	if err := FillSlot(node, a.graph, "archive", archive); err != nil {
		return nil, fmt.Errorf("extract: archive: %w", err)
	}
	if err := FillSlot(node, a.graph, "prefix", prefix); err != nil {
		return nil, fmt.Errorf("extract: prefix: %w", err)
	}

	a.graph.Nodes = append(a.graph.Nodes, node)
	return NewOutput(node, a.graph, ""), nil
}
