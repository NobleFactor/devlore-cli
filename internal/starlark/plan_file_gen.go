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

// FilePlan implements plan.file.* bindings using the slot-based model.
// Each method adds a node to the execution graph.
type FilePlan struct {
	Receiver
	graph   *execution.Graph
	host    host.Host
	project string
	reg     *execution.ActionRegistry
}

// NewFilePlan creates a new FilePlan for the given graph and host.
func NewFilePlan(graph *execution.Graph, h host.Host, project string, reg *execution.ActionRegistry) *FilePlan {
	return &FilePlan{
		Receiver: NewReceiver("plan.file"),
		graph:    graph,
		host:     h,
		project:  project,
		reg:      reg,
	}
}

// Attr implements starlark.HasAttrs.
func (f *FilePlan) Attr(name string) (starlark.Value, error) {
	switch name {
	case "link":
		return MakeAttr("plan.file.link", f.link), nil
	case "copy":
		return MakeAttr("plan.file.copy", f.copy), nil
	case "write":
		return MakeAttr("plan.file.write", f.write), nil
	case "remove":
		return MakeAttr("plan.file.remove", f.remove), nil
	default:
		return nil, NoSuchAttrError("plan.file", name)
	}
}

// AttrNames implements starlark.HasAttrs.
func (f *FilePlan) AttrNames() []string {
	return []string{"copy", "link", "remove", "write"}
}

func (f *FilePlan) link(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var source, path starlark.Value
	if err := starlark.UnpackArgs("link", args, kwargs, "source", &source, "path", &path); err != nil {
		return nil, err
	}

	node := &execution.Node{
		ID:      generateNodeID("link"),
		Action:  f.reg.MustGet("file.link"),
		Project: f.project,
	}

	if err := FillSlot(node, f.graph, "source", source); err != nil {
		return nil, fmt.Errorf("link: source: %w", err)
	}
	if err := FillSlot(node, f.graph, "path", path); err != nil {
		return nil, fmt.Errorf("link: path: %w", err)
	}

	f.graph.Nodes = append(f.graph.Nodes, node)
	return NewOutput(node, f.graph, ""), nil
}

func (f *FilePlan) copy(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var source, path starlark.Value
	if err := starlark.UnpackArgs("copy", args, kwargs, "source", &source, "path", &path); err != nil {
		return nil, err
	}

	node := &execution.Node{
		ID:      generateNodeID("copy"),
		Action:  f.reg.MustGet("file.copy"),
		Project: f.project,
	}

	if err := FillSlot(node, f.graph, "source", source); err != nil {
		return nil, fmt.Errorf("copy: source: %w", err)
	}
	if err := FillSlot(node, f.graph, "path", path); err != nil {
		return nil, fmt.Errorf("copy: path: %w", err)
	}

	f.graph.Nodes = append(f.graph.Nodes, node)
	return NewOutput(node, f.graph, ""), nil
}

func (f *FilePlan) write(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var content, path starlark.Value
	if err := starlark.UnpackArgs("write", args, kwargs, "content", &content, "path", &path); err != nil {
		return nil, err
	}

	node := &execution.Node{
		ID:      generateNodeID("write"),
		Action:  f.reg.MustGet("file.write"),
		Project: f.project,
	}

	if err := FillSlot(node, f.graph, "content", content); err != nil {
		return nil, fmt.Errorf("write: content: %w", err)
	}
	if err := FillSlot(node, f.graph, "path", path); err != nil {
		return nil, fmt.Errorf("write: path: %w", err)
	}

	f.graph.Nodes = append(f.graph.Nodes, node)
	return NewOutput(node, f.graph, ""), nil
}

func (f *FilePlan) remove(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var path starlark.Value
	if err := starlark.UnpackArgs("remove", args, kwargs, "path", &path); err != nil {
		return nil, err
	}

	node := &execution.Node{
		ID:      generateNodeID("remove"),
		Action:  f.reg.MustGet("file.remove"),
		Project: f.project,
	}

	if err := FillSlot(node, f.graph, "path", path); err != nil {
		return nil, fmt.Errorf("remove: path: %w", err)
	}

	f.graph.Nodes = append(f.graph.Nodes, node)
	return starlark.None, nil
}
