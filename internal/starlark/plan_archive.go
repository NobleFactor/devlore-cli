// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlark

import (
	"fmt"

	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/internal/execution"
	"github.com/NobleFactor/devlore-cli/internal/host"
)

// ArchivePlan implements plan.archive.* bindings using the slot-based model.
type ArchivePlan struct {
	graph   *execution.Graph
	host    host.Host
	project string
}

// NewArchivePlan creates a new ArchivePlan for the given graph and host.
func NewArchivePlan(graph *execution.Graph, h host.Host, project string) *ArchivePlan {
	return &ArchivePlan{
		graph:   graph,
		host:    h,
		project: project,
	}
}

// Starlark Value interface
func (a *ArchivePlan) String() string        { return "plan.archive" }
func (a *ArchivePlan) Type() string          { return "plan.archive" }
func (a *ArchivePlan) Freeze()               {}
func (a *ArchivePlan) Truth() starlark.Bool  { return true }
func (a *ArchivePlan) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable: plan.archive") }

// Starlark HasAttrs interface
func (a *ArchivePlan) Attr(name string) (starlark.Value, error) {
	switch name {
	case "extract":
		return starlark.NewBuiltin("plan.archive.extract", a.extract), nil
	default:
		return nil, starlark.NoSuchAttrError(fmt.Sprintf("plan.archive has no attribute %q", name))
	}
}

func (a *ArchivePlan) AttrNames() []string {
	return []string{"extract"}
}

// extract extracts an archive to a target directory.
// Usage: plan.archive.extract(archive, prefix)
//
// Slots:
//   - archive: Archive file to extract (promise or immediate)
//   - prefix: Directory to extract into (promise or immediate)
//
// Returns: Promise of the extracted directory
func (a *ArchivePlan) extract(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var archive, prefix starlark.Value
	if err := starlark.UnpackArgs("extract", args, kwargs, "archive", &archive, "prefix", &prefix); err != nil {
		return nil, err
	}

	node := &execution.Node{
		ID:         generateNodeID("archive-extract"),
		Operations: []string{"archive-extract"},
		Project:    a.project,
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
