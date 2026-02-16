// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlark

import (
	"fmt"

	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/internal/execution"
	"github.com/NobleFactor/devlore-cli/internal/host"
)

// EncryptionPlan implements plan.encryption.* bindings using the slot-based model.
type EncryptionPlan struct {
	graph   *execution.Graph
	host    host.Host
	project string
	reg     *execution.ActionRegistry
}

// NewEncryptionPlan creates a new EncryptionPlan for the given graph and host.
func NewEncryptionPlan(graph *execution.Graph, h host.Host, project string, reg *execution.ActionRegistry) *EncryptionPlan {
	return &EncryptionPlan{
		graph:   graph,
		host:    h,
		project: project,
		reg:     reg,
	}
}

// Starlark Value interface
func (e *EncryptionPlan) String() string        { return "plan.encryption" }
func (e *EncryptionPlan) Type() string          { return "plan.encryption" }
func (e *EncryptionPlan) Freeze()               {}
func (e *EncryptionPlan) Truth() starlark.Bool  { return true }
func (e *EncryptionPlan) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable: plan.encryption") }

// Starlark HasAttrs interface
func (e *EncryptionPlan) Attr(name string) (starlark.Value, error) {
	switch name {
	case "decrypt":
		return starlark.NewBuiltin("plan.encryption.decrypt", e.decrypt), nil
	default:
		return nil, starlark.NoSuchAttrError(fmt.Sprintf("plan.encryption has no attribute %q", name))
	}
}

func (e *EncryptionPlan) AttrNames() []string {
	return []string{"decrypt"}
}

// decrypt adds a decryption node.
// Usage: plan.encryption.decrypt(source)
//
// Slots:
//   - source: Input file/content (promise or immediate)
//
// Returns: Promise of the decrypted content
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
