// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlark

import (
	"fmt"

	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/internal/execution"
	"github.com/NobleFactor/devlore-cli/internal/host"
)

// PlanRoot implements the top-level plan namespace using the slot-based model.
// It provides access to sub-namespaces (package, file, archive, git) and
// top-level bindings (source, literal, download, service, shell, depends_on).
type PlanRoot struct {
	graph   *execution.Graph
	host    host.Host
	project string

	// Sub-namespaces (cached)
	packagePlan *PackagePlan
	filePlan    *FilePlan
	archivePlan *ArchivePlan
	gitPlan     *GitPlan
}

// NewPlanRoot creates a new PlanRoot for the given graph and host.
func NewPlanRoot(graph *execution.Graph, h host.Host, project string) *PlanRoot {
	return &PlanRoot{
		graph:       graph,
		host:        h,
		project:     project,
		packagePlan: NewPackagePlan(graph, h, project),
		filePlan:    NewFilePlan(graph, h, project),
		archivePlan: NewArchivePlan(graph, h, project),
		gitPlan:     NewGitPlan(graph, h, project),
	}
}

// Starlark Value interface
func (p *PlanRoot) String() string        { return "plan" }
func (p *PlanRoot) Type() string          { return "plan" }
func (p *PlanRoot) Freeze()               {}
func (p *PlanRoot) Truth() starlark.Bool  { return true }
func (p *PlanRoot) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable: plan") }

// Starlark HasAttrs interface
func (p *PlanRoot) Attr(name string) (starlark.Value, error) {
	switch name {
	// Sub-namespaces
	case "package":
		return p.packagePlan, nil
	case "file":
		return p.filePlan, nil
	case "archive":
		return p.archivePlan, nil
	case "git":
		return p.gitPlan, nil
	// Top-level bindings
	case "source":
		return starlark.NewBuiltin("plan.source", p.source), nil
	case "literal":
		return starlark.NewBuiltin("plan.literal", p.literal), nil
	case "download":
		return starlark.NewBuiltin("plan.download", p.download), nil
	case "service":
		return starlark.NewBuiltin("plan.service", p.service), nil
	case "shell":
		return starlark.NewBuiltin("plan.shell", p.shell), nil
	case "depends_on":
		return starlark.NewBuiltin("plan.depends_on", p.dependsOn), nil
	default:
		return nil, starlark.NoSuchAttrError(fmt.Sprintf("plan has no attribute %q", name))
	}
}

func (p *PlanRoot) AttrNames() []string {
	return []string{
		"archive", "depends_on", "download", "file", "git",
		"literal", "package", "service", "shell", "source",
	}
}

// source creates a source file artifact.
// Usage: plan.source(path)
//
// Slots:
//   - path: Path to existing source file (immediate only)
//
// Returns: Promise of the source file
func (p *PlanRoot) source(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var path starlark.Value
	if err := starlark.UnpackArgs("source", args, kwargs, "path", &path); err != nil {
		return nil, err
	}

	node := &execution.Node{
		ID:         generateNodeID("source"),
		Operations: []string{"source"},
		Project:    p.project,
	}

	if err := FillSlot(node, p.graph, "path", path); err != nil {
		return nil, fmt.Errorf("source: path: %w", err)
	}

	p.graph.Nodes = append(p.graph.Nodes, node)
	return NewOutput(node, p.graph, ""), nil
}

// literal creates a literal content artifact.
// Usage: plan.literal(content)
//
// Slots:
//   - content: Inline content (promise or immediate)
//
// Returns: Promise of the content
func (p *PlanRoot) literal(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var content starlark.Value
	if err := starlark.UnpackArgs("literal", args, kwargs, "content", &content); err != nil {
		return nil, err
	}

	node := &execution.Node{
		ID:         generateNodeID("literal"),
		Operations: []string{"literal"},
		Project:    p.project,
	}

	if err := FillSlot(node, p.graph, "content", content); err != nil {
		return nil, fmt.Errorf("literal: content: %w", err)
	}

	p.graph.Nodes = append(p.graph.Nodes, node)
	return NewOutput(node, p.graph, ""), nil
}

// download downloads a file from a URL.
// Usage: plan.download(url)
//
// Slots:
//   - url: URL to download from (promise or immediate)
//
// Returns: Promise of the downloaded file
func (p *PlanRoot) download(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var url starlark.Value
	if err := starlark.UnpackArgs("download", args, kwargs, "url", &url); err != nil {
		return nil, err
	}

	node := &execution.Node{
		ID:         generateNodeID("download"),
		Operations: []string{"download"},
		Project:    p.project,
	}

	if err := FillSlot(node, p.graph, "url", url); err != nil {
		return nil, fmt.Errorf("download: url: %w", err)
	}

	p.graph.Nodes = append(p.graph.Nodes, node)
	return NewOutput(node, p.graph, ""), nil
}

// service manages a system service.
// Usage: plan.service(name, action)
//
// Slots:
//   - name: Service name (promise or immediate)
//   - action: Action to perform: start, stop, restart, enable, disable (immediate)
//
// Returns: Promise of the service operation
func (p *PlanRoot) service(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name, action starlark.Value
	if err := starlark.UnpackArgs("service", args, kwargs, "name", &name, "action", &action); err != nil {
		return nil, err
	}

	// Validate action is a known value
	actionStr, ok := starlark.AsString(action)
	if !ok {
		return nil, fmt.Errorf("service: action must be a string, got %s", action.Type())
	}
	switch actionStr {
	case "start", "stop", "restart", "enable", "disable":
		// Valid
	default:
		return nil, fmt.Errorf("service: unknown action %q", actionStr)
	}

	node := &execution.Node{
		ID:         generateNodeID("service"),
		Operations: []string{"service-" + actionStr},
		Project:    p.project,
	}

	if err := FillSlot(node, p.graph, "name", name); err != nil {
		return nil, fmt.Errorf("service: name: %w", err)
	}
	if err := FillSlot(node, p.graph, "action", action); err != nil {
		return nil, fmt.Errorf("service: action: %w", err)
	}

	p.graph.Nodes = append(p.graph.Nodes, node)
	return NewOutput(node, p.graph, ""), nil
}

// shell runs a shell command.
// Usage: plan.shell(command)
//
// Slots:
//   - command: Command to execute (promise or immediate)
//
// Returns: Promise of the command result
func (p *PlanRoot) shell(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var command starlark.Value
	if err := starlark.UnpackArgs("shell", args, kwargs, "command", &command); err != nil {
		return nil, err
	}

	node := &execution.Node{
		ID:         generateNodeID("shell"),
		Operations: []string{"shell"},
		Project:    p.project,
	}

	if err := FillSlot(node, p.graph, "command", command); err != nil {
		return nil, fmt.Errorf("shell: command: %w", err)
	}

	p.graph.Nodes = append(p.graph.Nodes, node)
	return NewOutput(node, p.graph, ""), nil
}

// dependsOn creates an explicit dependency between two nodes.
// Usage: plan.depends_on(consumer, producer)
//
// This is a graph-wiring primitive for explicit ordering when there's no
// data flow between nodes. The consumer will not execute until the producer
// completes.
//
// Arguments:
//   - consumer: Node that depends on the producer (Output)
//   - producer: Node that must complete first (Output)
//
// Returns: None
func (p *PlanRoot) dependsOn(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("depends_on: expected 2 arguments, got %d", len(args))
	}

	// Extract node ID from first argument (consumer - depends ON the second)
	consumerID, err := extractNodeID(args[0], "first")
	if err != nil {
		return nil, fmt.Errorf("depends_on: %w", err)
	}

	// Extract node ID from second argument (producer - depended upon)
	producerID, err := extractNodeID(args[1], "second")
	if err != nil {
		return nil, fmt.Errorf("depends_on: %w", err)
	}

	// Create edge in the graph: consumer depends_on producer
	p.graph.Edges = append(p.graph.Edges, execution.Edge{
		From:     producerID,
		To:       consumerID,
		Relation: "depends_on",
	})

	return starlark.None, nil
}
