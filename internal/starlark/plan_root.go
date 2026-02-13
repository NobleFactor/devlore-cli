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
	case "gather":
		return starlark.NewBuiltin("plan.gather", p.gather), nil
	default:
		return nil, starlark.NoSuchAttrError(fmt.Sprintf("plan has no attribute %q", name))
	}
}

func (p *PlanRoot) AttrNames() []string {
	return []string{
		"archive", "download", "file", "gather", "git",
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
		Operation: "source",
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
		Operation: "literal",
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
		Operation: "download",
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
		Operation: "service-" + actionStr,
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
		Operation: "shell",
		Project:    p.project,
	}

	if err := FillSlot(node, p.graph, "command", command); err != nil {
		return nil, fmt.Errorf("shell: command: %w", err)
	}

	p.graph.Nodes = append(p.graph.Nodes, node)
	return NewOutput(node, p.graph, ""), nil
}

// gather creates a handle for parallel execution of multiple nodes.
// Usage: plan.gather(promise1, promise2, ...)
//
// This collects multiple promises into a single handle. When the handle is
// passed to another operation, it creates edges from ALL gathered nodes to
// the consumer, enabling parallel execution.
//
// Arguments:
//   - promises: Two or more Output values to gather
//
// Returns: Gather handle that can be passed to other operations
//
// Example:
//
//	a = plan.file.copy(src1, dst1)
//	b = plan.file.copy(src2, dst2)
//	c = plan.file.copy(src3, dst3)
//	group = plan.gather(a, b, c)
//	d = plan.whatever(group)  # d waits for a, b, c (which run in parallel)
func (p *PlanRoot) gather(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("gather: expected at least 2 arguments, got %d", len(args))
	}

	outputs := make([]*Output, 0, len(args))
	for i, arg := range args {
		output, ok := arg.(*Output)
		if !ok {
			return nil, fmt.Errorf("gather: argument %d must be an Output, got %s", i+1, arg.Type())
		}
		outputs = append(outputs, output)
	}

	return NewGather(p.graph, outputs...), nil
}
