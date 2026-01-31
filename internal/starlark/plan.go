// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlark

import (
	"fmt"
	"strings"
	"sync/atomic"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/NobleFactor/devlore-cli/internal/execution"
	"github.com/NobleFactor/devlore-cli/internal/host"
)

// nodeCounter provides unique node IDs across all plan bindings.
var nodeCounter uint64

// generateNodeID creates a unique node ID with the given prefix and components.
func generateNodeID(prefix string, components ...string) string {
	id := atomic.AddUint64(&nodeCounter, 1)
	if len(components) > 0 {
		return fmt.Sprintf("%s-%s-%d", prefix, strings.Join(components, "-"), id)
	}
	return fmt.Sprintf("%s-%d", prefix, id)
}

// =============================================================================
// Plan Bindings Implementation
// =============================================================================

// planBindings implements PlanBindings by building graph nodes.
type planBindings struct {
	graph   *execution.Graph
	host    host.Host
	project string // Package name for grouping
}

// NewPlanBindings creates a new PlanBindings for the given graph and host.
func NewPlanBindings(graph *execution.Graph, h host.Host, project string) PlanBindings {
	return &planBindings{
		graph:   graph,
		host:    h,
		project: project,
	}
}

// Graph returns the underlying execution graph.
func (p *planBindings) Graph() *execution.Graph {
	return p.graph
}

// PackageInstall adds a package installation node.
func (p *planBindings) PackageInstall(packages ...string) *execution.Node {
	node := &execution.Node{
		ID:         generateNodeID("package-install", packages...),
		Operations: []string{"package-install"},
		Project:    p.project,
		Metadata: map[string]string{
			"packages": strings.Join(packages, ","),
		},
	}
	p.graph.Nodes = append(p.graph.Nodes, node)
	return node
}

// PackageUpgrade adds a package upgrade node.
func (p *planBindings) PackageUpgrade(packages ...string) *execution.Node {
	node := &execution.Node{
		ID:         generateNodeID("package-upgrade", packages...),
		Operations: []string{"package-upgrade"},
		Project:    p.project,
		Metadata: map[string]string{
			"packages": strings.Join(packages, ","),
		},
	}
	p.graph.Nodes = append(p.graph.Nodes, node)
	return node
}

// PackageRemove adds a package removal node.
func (p *planBindings) PackageRemove(packages ...string) *execution.Node {
	node := &execution.Node{
		ID:         generateNodeID("package-remove", packages...),
		Operations: []string{"package-remove"},
		Project:    p.project,
		Metadata: map[string]string{
			"packages": strings.Join(packages, ","),
		},
	}
	p.graph.Nodes = append(p.graph.Nodes, node)
	return node
}

// PackageUpdate adds a package index update node.
func (p *planBindings) PackageUpdate() *execution.Node {
	node := &execution.Node{
		ID:         generateNodeID("package-update"),
		Operations: []string{"package-update"},
		Project:    p.project,
		Metadata:   map[string]string{},
	}
	p.graph.Nodes = append(p.graph.Nodes, node)
	return node
}

// Configure adds a configuration file node (template expansion + copy).
func (p *planBindings) Configure(source, target string) *execution.Node {
	node := &execution.Node{
		ID:         generateNodeID("configure"),
		Operations: []string{"expand", "copy"},
		Source:     source,
		Target:     p.host.ExpandPath(target),
		Project:    p.project,
	}
	p.graph.Nodes = append(p.graph.Nodes, node)
	return node
}

// Link adds a symlink creation node.
func (p *planBindings) Link(source, target string) *execution.Node {
	node := &execution.Node{
		ID:         generateNodeID("link"),
		Operations: []string{"link"},
		Source:     source,
		Target:     p.host.ExpandPath(target),
		Project:    p.project,
	}
	p.graph.Nodes = append(p.graph.Nodes, node)
	return node
}

// Copy adds a file copy node.
func (p *planBindings) Copy(source, target string) *execution.Node {
	node := &execution.Node{
		ID:         generateNodeID("copy"),
		Operations: []string{"copy"},
		Source:     source,
		Target:     p.host.ExpandPath(target),
		Project:    p.project,
	}
	p.graph.Nodes = append(p.graph.Nodes, node)
	return node
}

// Mkdir adds a directory creation node.
func (p *planBindings) Mkdir(target string) *execution.Node {
	node := &execution.Node{
		ID:         generateNodeID("mkdir"),
		Operations: []string{"mkdir"},
		Target:     p.host.ExpandPath(target),
		Project:    p.project,
	}
	p.graph.Nodes = append(p.graph.Nodes, node)
	return node
}

// Write adds a file write node (write content directly to target).
func (p *planBindings) Write(target, content string) *execution.Node {
	node := &execution.Node{
		ID:         generateNodeID("write"),
		Operations: []string{"file-write"},
		Target:     p.host.ExpandPath(target),
		Project:    p.project,
		Metadata: map[string]string{
			"content": content,
		},
	}
	p.graph.Nodes = append(p.graph.Nodes, node)
	return node
}

// Service adds a service management node.
func (p *planBindings) Service(name string, action ServiceAction) *execution.Node {
	node := &execution.Node{
		ID:         generateNodeID("service", name, action.String()),
		Operations: []string{"service-" + action.String()},
		Project:    p.project,
		Metadata: map[string]string{
			"service": name,
			"action":  action.String(),
		},
	}
	p.graph.Nodes = append(p.graph.Nodes, node)
	return node
}

// Shell adds a shell command execution node.
func (p *planBindings) Shell(command string) *execution.Node {
	node := &execution.Node{
		ID:         generateNodeID("shell"),
		Operations: []string{"shell"},
		Project:    p.project,
		Metadata: map[string]string{
			"command": command,
		},
	}
	p.graph.Nodes = append(p.graph.Nodes, node)
	return node
}

// DependsOn creates a dependency edge between nodes.
func (p *planBindings) DependsOn(from, to *execution.Node) {
	p.graph.Edges = append(p.graph.Edges, execution.Edge{
		From:     to.ID,
		To:       from.ID,
		Relation: "depends_on",
	})
}

// =============================================================================
// Starlark Conversion
// =============================================================================

// StarlarkPlanBindings wraps PlanBindings for Starlark conversion.
type StarlarkPlanBindings struct {
	PlanBindings
}

// ToStarlark converts the plan bindings to a Starlark struct.
// Exposed to phase scripts as the third argument.
//
// Starlark API uses nested structs:
//
//	plan.package.install("pkg1", "pkg2", ...)  # Install packages
//	plan.package.upgrade("pkg1", ...)          # Upgrade packages
//	plan.package.remove("pkg1", ...)           # Remove packages
//	plan.package.update()                      # Update package index
//	plan.file.configure(source, target)        # Configure file (template + copy)
//	plan.file.link(source, target)             # Create symlink
//	plan.file.copy(source, target)             # Copy file
//	plan.file.mkdir(target)                    # Create directory
//	plan.file.write(target, content)           # Write content to file
//	plan.service(name, action)                 # Manage service
//	plan.shell(command)                        # Run shell command
//	plan.depends_on(from, to)                  # Create dependency
func (s *StarlarkPlanBindings) ToStarlark() starlark.Value {
	// Package operations namespace: plan.package.*
	packageOps := starlarkstruct.FromStringDict(starlark.String("package"), starlark.StringDict{
		"install": starlark.NewBuiltin("install", s.packageInstallBuiltin),
		"upgrade": starlark.NewBuiltin("upgrade", s.packageUpgradeBuiltin),
		"remove":  starlark.NewBuiltin("remove", s.packageRemoveBuiltin),
		"update":  starlark.NewBuiltin("update", s.packageUpdateBuiltin),
	})

	// File operations namespace: plan.file.*
	fileOps := starlarkstruct.FromStringDict(starlark.String("file"), starlark.StringDict{
		"configure": starlark.NewBuiltin("configure", s.configureBuiltin),
		"link":      starlark.NewBuiltin("link", s.linkBuiltin),
		"copy":      starlark.NewBuiltin("copy", s.copyBuiltin),
		"mkdir":     starlark.NewBuiltin("mkdir", s.mkdirBuiltin),
		"write":     starlark.NewBuiltin("write", s.writeBuiltin),
	})

	return starlarkstruct.FromStringDict(starlark.String("plan"), starlark.StringDict{
		"package":    packageOps,
		"file":       fileOps,
		"service":    starlark.NewBuiltin("service", s.serviceBuiltin),
		"shell":      starlark.NewBuiltin("shell", s.shellBuiltin),
		"depends_on": starlark.NewBuiltin("depends_on", s.dependsOnBuiltin),
	})
}

func (s *StarlarkPlanBindings) packageInstallBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	packages := make([]string, len(args))
	for i, arg := range args {
		str, ok := starlark.AsString(arg)
		if !ok {
			return nil, fmt.Errorf("install: argument %d is not a string", i)
		}
		packages[i] = str
	}
	if len(packages) == 0 {
		return nil, fmt.Errorf("install: at least one package required")
	}
	node := s.PackageInstall(packages...)
	return nodeToStarlark(node), nil
}

func (s *StarlarkPlanBindings) packageUpgradeBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	packages := make([]string, len(args))
	for i, arg := range args {
		str, ok := starlark.AsString(arg)
		if !ok {
			return nil, fmt.Errorf("upgrade: argument %d is not a string", i)
		}
		packages[i] = str
	}
	if len(packages) == 0 {
		return nil, fmt.Errorf("upgrade: at least one package required")
	}
	node := s.PackageUpgrade(packages...)
	return nodeToStarlark(node), nil
}

func (s *StarlarkPlanBindings) packageRemoveBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	packages := make([]string, len(args))
	for i, arg := range args {
		str, ok := starlark.AsString(arg)
		if !ok {
			return nil, fmt.Errorf("remove: argument %d is not a string", i)
		}
		packages[i] = str
	}
	if len(packages) == 0 {
		return nil, fmt.Errorf("remove: at least one package required")
	}
	node := s.PackageRemove(packages...)
	return nodeToStarlark(node), nil
}

func (s *StarlarkPlanBindings) packageUpdateBuiltin(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	node := s.PackageUpdate()
	return nodeToStarlark(node), nil
}

func (s *StarlarkPlanBindings) configureBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var source, target string
	if err := starlark.UnpackArgs("configure", args, kwargs, "source", &source, "target", &target); err != nil {
		return nil, err
	}
	node := s.Configure(source, target)
	return nodeToStarlark(node), nil
}

func (s *StarlarkPlanBindings) linkBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var source, target string
	if err := starlark.UnpackArgs("link", args, kwargs, "source", &source, "target", &target); err != nil {
		return nil, err
	}
	node := s.Link(source, target)
	return nodeToStarlark(node), nil
}

func (s *StarlarkPlanBindings) copyBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var source, target string
	if err := starlark.UnpackArgs("copy", args, kwargs, "source", &source, "target", &target); err != nil {
		return nil, err
	}
	node := s.Copy(source, target)
	return nodeToStarlark(node), nil
}

func (s *StarlarkPlanBindings) mkdirBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var target string
	if err := starlark.UnpackArgs("mkdir", args, kwargs, "target", &target); err != nil {
		return nil, err
	}
	node := s.Mkdir(target)
	return nodeToStarlark(node), nil
}

func (s *StarlarkPlanBindings) writeBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var target, content string
	if err := starlark.UnpackArgs("write", args, kwargs, "target", &target, "content", &content); err != nil {
		return nil, err
	}
	node := s.Write(target, content)
	return nodeToStarlark(node), nil
}

func (s *StarlarkPlanBindings) serviceBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name, action string
	if err := starlark.UnpackArgs("service", args, kwargs, "name", &name, "action", &action); err != nil {
		return nil, err
	}

	var serviceAction ServiceAction
	switch action {
	case "start":
		serviceAction = ServiceStart
	case "stop":
		serviceAction = ServiceStop
	case "restart":
		serviceAction = ServiceRestart
	case "enable":
		serviceAction = ServiceEnable
	case "disable":
		serviceAction = ServiceDisable
	default:
		return nil, fmt.Errorf("service: unknown action %q", action)
	}

	node := s.Service(name, serviceAction)
	return nodeToStarlark(node), nil
}

func (s *StarlarkPlanBindings) shellBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var command string
	if err := starlark.UnpackArgs("shell", args, kwargs, "command", &command); err != nil {
		return nil, err
	}
	node := s.Shell(command)
	return nodeToStarlark(node), nil
}

func (s *StarlarkPlanBindings) dependsOnBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("depends_on: expected 2 arguments, got %d", len(args))
	}

	fromStruct, ok := args[0].(*starlarkstruct.Struct)
	if !ok {
		return nil, fmt.Errorf("depends_on: first argument must be a node")
	}
	toStruct, ok := args[1].(*starlarkstruct.Struct)
	if !ok {
		return nil, fmt.Errorf("depends_on: second argument must be a node")
	}

	fromID, err := fromStruct.Attr("id")
	if err != nil {
		return nil, fmt.Errorf("depends_on: first argument has no id")
	}
	toID, err := toStruct.Attr("id")
	if err != nil {
		return nil, fmt.Errorf("depends_on: second argument has no id")
	}

	fromIDStr, _ := starlark.AsString(fromID)
	toIDStr, _ := starlark.AsString(toID)

	// Create edge in the graph
	graph := s.Graph()
	graph.Edges = append(graph.Edges, execution.Edge{
		From:     toIDStr,
		To:       fromIDStr,
		Relation: "depends_on",
	})

	return starlark.None, nil
}

// nodeToStarlark converts a execution.Node to a Starlark struct.
func nodeToStarlark(node *execution.Node) starlark.Value {
	ops := make([]starlark.Value, len(node.Operations))
	for i, op := range node.Operations {
		ops[i] = starlark.String(op)
	}

	metadata := starlark.NewDict(len(node.Metadata))
	for k, v := range node.Metadata {
		_ = metadata.SetKey(starlark.String(k), starlark.String(v))
	}

	return starlarkstruct.FromStringDict(starlark.String("node"), starlark.StringDict{
		"id":         starlark.String(node.ID),
		"operations": starlark.NewList(ops),
		"source":     starlark.String(node.Source),
		"target":     starlark.String(node.Target),
		"project":    starlark.String(node.Project),
		"metadata":   metadata,
	})
}
