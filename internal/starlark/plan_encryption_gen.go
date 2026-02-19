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

// EncryptionPlan implements plan.encryption.* bindings using the slot-based model.
type EncryptionPlan struct {
	Receiver
	graph   *execution.Graph
	host    host.Host
	project string
	reg     *execution.ActionRegistry
}

// NewEncryptionPlan creates a new EncryptionPlan for the given graph and host.
func NewEncryptionPlan(graph *execution.Graph, h host.Host, project string, reg *execution.ActionRegistry) *EncryptionPlan {
	return &EncryptionPlan{
		Receiver: NewReceiver("plan.encryption"),
		graph:    graph,
		host:     h,
		project:  project,
		reg:      reg,
	}
}

// Attr implements starlark.HasAttrs.
func (e *EncryptionPlan) Attr(name string) (starlark.Value, error) {
	switch name {
	case "decrypt":
		return MakeAttr("plan.encryption.decrypt", e.decrypt), nil
	default:
		return nil, NoSuchAttrError("plan.encryption", name)
	}
}

// AttrNames implements starlark.HasAttrs.
func (e *EncryptionPlan) AttrNames() []string {
	return []string{"decrypt"}
}

func (e *EncryptionPlan) decrypt(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var source starlark.Value
	if err := starlark.UnpackArgs("decrypt", args, kwargs, "source", &source); err != nil {
		return nil, err
	}

	node := &execution.Node{
		ID:      generateNodeID("decrypt"),
		Action:  e.reg.MustGet("encryption.decrypt"),
		Project: e.project,
	}

	if err := FillSlot(node, e.graph, "source", source); err != nil {
		return nil, fmt.Errorf("decrypt: source: %w", err)
	}

	e.graph.Nodes = append(e.graph.Nodes, node)
	return NewOutput(node, e.graph, ""), nil
}
