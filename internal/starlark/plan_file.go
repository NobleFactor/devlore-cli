// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlark

import (
	"fmt"

	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/internal/execution"
	"github.com/NobleFactor/devlore-cli/internal/host"
)

// FilePlan implements plan.file.* bindings using the slot-based model.
// Each method adds a node to the execution graph.
//
// Slots can be filled with either:
// - Immediate values (strings known at analysis time)
// - Promises (Output handles that create edges)
type FilePlan struct {
	graph   *execution.Graph
	host    host.Host
	project string
}

// NewFilePlan creates a new FilePlan for the given graph and host.
func NewFilePlan(graph *execution.Graph, h host.Host, project string) *FilePlan {
	return &FilePlan{
		graph:   graph,
		host:    h,
		project: project,
	}
}

// Starlark Value interface
func (f *FilePlan) String() string        { return "plan.file" }
func (f *FilePlan) Type() string          { return "plan.file" }
func (f *FilePlan) Freeze()               {}
func (f *FilePlan) Truth() starlark.Bool  { return true }
func (f *FilePlan) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable: plan.file") }

// Starlark HasAttrs interface
func (f *FilePlan) Attr(name string) (starlark.Value, error) {
	switch name {
	case "configure":
		return starlark.NewBuiltin("plan.file.configure", f.configure), nil
	case "link":
		return starlark.NewBuiltin("plan.file.link", f.link), nil
	case "copy":
		return starlark.NewBuiltin("plan.file.copy", f.copy), nil
	case "write":
		return starlark.NewBuiltin("plan.file.write", f.write), nil
	case "remove":
		return starlark.NewBuiltin("plan.file.remove", f.remove), nil
	default:
		return nil, starlark.NoSuchAttrError(fmt.Sprintf("plan.file has no attribute %q", name))
	}
}

func (f *FilePlan) AttrNames() []string {
	return []string{"configure", "copy", "link", "remove", "write"}
}

// configure adds a configuration file node (template expansion + copy).
// Usage: plan.file.configure(source, path)
//
// Slots:
//   - source: Input file/content (promise or immediate)
//   - path: Destination path (promise or immediate)
//
// Returns: Promise of the configured file
func (f *FilePlan) configure(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var source, path starlark.Value
	if err := starlark.UnpackArgs("configure", args, kwargs, "source", &source, "path", &path); err != nil {
		return nil, err
	}

	renderNode := &execution.Node{
		ID:        generateNodeID("render"),
		Action: "render",
		Project:   f.project,
	}
	if err := FillSlot(renderNode, f.graph, "source", source); err != nil {
		return nil, fmt.Errorf("configure: source: %w", err)
	}
	f.graph.Nodes = append(f.graph.Nodes, renderNode)

	copyNode := &execution.Node{
		ID:        generateNodeID("configure"),
		Action: "copy",
		Project:   f.project,
	}
	if err := FillSlot(copyNode, f.graph, "path", path); err != nil {
		return nil, fmt.Errorf("configure: path: %w", err)
	}
	f.graph.Nodes = append(f.graph.Nodes, copyNode)

	f.graph.Edges = append(f.graph.Edges, execution.Edge{
		From: renderNode.ID,
		To:   copyNode.ID,
	})

	return NewOutput(copyNode, f.graph, ""), nil
}

// link adds a symlink creation node.
// Usage: plan.file.link(source, path)
//
// Slots:
//   - source: File to link to (promise or immediate)
//   - path: Where to create the symlink (promise or immediate)
//
// Returns: Promise of the symlink
func (f *FilePlan) link(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var source, path starlark.Value
	if err := starlark.UnpackArgs("link", args, kwargs, "source", &source, "path", &path); err != nil {
		return nil, err
	}

	node := &execution.Node{
		ID:         generateNodeID("link"),
		Action: "link",
		Project:    f.project,
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

// copy adds a file copy node.
// Usage: plan.file.copy(source, path)
//
// Slots:
//   - source: File to copy (promise or immediate)
//   - path: Destination path (promise or immediate)
//
// Returns: Promise of the copied file
func (f *FilePlan) copy(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var source, path starlark.Value
	if err := starlark.UnpackArgs("copy", args, kwargs, "source", &source, "path", &path); err != nil {
		return nil, err
	}

	node := &execution.Node{
		ID:         generateNodeID("copy"),
		Action: "copy",
		Project:    f.project,
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

// write adds a file write node.
// Usage: plan.file.write(content, path)
//
// Slots:
//   - content: Content to write (promise or immediate)
//   - path: Destination path (promise or immediate)
//
// Returns: Promise of the written file
func (f *FilePlan) write(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var content, path starlark.Value
	if err := starlark.UnpackArgs("write", args, kwargs, "content", &content, "path", &path); err != nil {
		return nil, err
	}

	node := &execution.Node{
		ID:         generateNodeID("write"),
		Action: "write",
		Project:    f.project,
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

// remove adds a file/directory removal node.
// Usage: plan.file.remove(path)
//
// Slots:
//   - path: Path to remove (promise or immediate)
//
// Returns: None (removal produces no output)
func (f *FilePlan) remove(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var path starlark.Value
	if err := starlark.UnpackArgs("remove", args, kwargs, "path", &path); err != nil {
		return nil, err
	}

	node := &execution.Node{
		ID:         generateNodeID("remove"),
		Action: "remove",
		Project:    f.project,
	}

	if err := FillSlot(node, f.graph, "path", path); err != nil {
		return nil, fmt.Errorf("remove: path: %w", err)
	}

	f.graph.Nodes = append(f.graph.Nodes, node)
	return starlark.None, nil
}
