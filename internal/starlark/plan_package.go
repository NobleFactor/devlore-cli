// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlark

import (
	"fmt"
	"strings"

	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/internal/execution"
	"github.com/NobleFactor/devlore-cli/internal/host"
)

// PackagePlan implements plan.package.* bindings using the slot-based model.
// Each method adds a node to the execution graph.
type PackagePlan struct {
	graph   *execution.Graph
	host    host.Host
	project string
}

// NewPackagePlan creates a new PackagePlan for the given graph and host.
func NewPackagePlan(graph *execution.Graph, h host.Host, project string) *PackagePlan {
	return &PackagePlan{
		graph:   graph,
		host:    h,
		project: project,
	}
}

// Starlark Value interface
func (p *PackagePlan) String() string        { return "plan.package" }
func (p *PackagePlan) Type() string          { return "plan.package" }
func (p *PackagePlan) Freeze()               {}
func (p *PackagePlan) Truth() starlark.Bool  { return true }
func (p *PackagePlan) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable: plan.package") }

// Starlark HasAttrs interface
func (p *PackagePlan) Attr(name string) (starlark.Value, error) {
	switch name {
	case "install":
		return starlark.NewBuiltin("plan.package.install", p.install), nil
	case "upgrade":
		return starlark.NewBuiltin("plan.package.upgrade", p.upgrade), nil
	case "remove":
		return starlark.NewBuiltin("plan.package.remove", p.remove), nil
	case "update":
		return starlark.NewBuiltin("plan.package.update", p.update), nil
	default:
		return nil, starlark.NoSuchAttrError(fmt.Sprintf("plan.package has no attribute %q", name))
	}
}

func (p *PackagePlan) AttrNames() []string {
	return []string{"install", "remove", "update", "upgrade"}
}

// install adds a package installation node.
// Usage: plan.package.install("pkg1", "pkg2", ...)
//
// Slots: packages (list of package names, immediate strings)
// Returns: Promise of installed packages
func (p *PackagePlan) install(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	packages, err := argsToStrings("install", args)
	if err != nil {
		return nil, err
	}
	if len(packages) == 0 {
		return nil, fmt.Errorf("install: at least one package required")
	}

	node := &execution.Node{
		ID:         generateNodeID("package-install", packages...),
		Operation: "package-install",
		Project:    p.project,
	}
	node.SetSlotImmediate("packages", strings.Join(packages, ","))

	p.graph.Nodes = append(p.graph.Nodes, node)
	return NewOutput(node, p.graph, ""), nil
}

// upgrade adds a package upgrade node.
// Usage: plan.package.upgrade("pkg1", "pkg2", ...)
//
// Slots: packages (list of package names, immediate strings)
// Returns: Promise of upgraded packages
func (p *PackagePlan) upgrade(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	packages, err := argsToStrings("upgrade", args)
	if err != nil {
		return nil, err
	}
	if len(packages) == 0 {
		return nil, fmt.Errorf("upgrade: at least one package required")
	}

	node := &execution.Node{
		ID:         generateNodeID("package-upgrade", packages...),
		Operation: "package-upgrade",
		Project:    p.project,
	}
	node.SetSlotImmediate("packages", strings.Join(packages, ","))

	p.graph.Nodes = append(p.graph.Nodes, node)
	return NewOutput(node, p.graph, ""), nil
}

// remove adds a package removal node.
// Usage: plan.package.remove("pkg1", "pkg2", ...)
//
// Slots: packages (list of package names, immediate strings)
// Returns: None (removal produces no output)
func (p *PackagePlan) remove(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	packages, err := argsToStrings("remove", args)
	if err != nil {
		return nil, err
	}
	if len(packages) == 0 {
		return nil, fmt.Errorf("remove: at least one package required")
	}

	node := &execution.Node{
		ID:         generateNodeID("package-remove", packages...),
		Operation: "package-remove",
		Project:    p.project,
	}
	node.SetSlotImmediate("packages", strings.Join(packages, ","))

	p.graph.Nodes = append(p.graph.Nodes, node)
	// Remove produces no output
	return starlark.None, nil
}

// update adds a package index update node.
// Usage: plan.package.update()
//
// Slots: (none)
// Returns: Promise of updated package index
func (p *PackagePlan) update(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	if len(args) > 0 {
		return nil, fmt.Errorf("update: takes no arguments")
	}

	node := &execution.Node{
		ID:         generateNodeID("package-update"),
		Operation: "package-update",
		Project:    p.project,
	}

	p.graph.Nodes = append(p.graph.Nodes, node)
	return NewOutput(node, p.graph, ""), nil
}

// argsToStrings converts Starlark args to a string slice.
func argsToStrings(funcName string, args starlark.Tuple) ([]string, error) {
	result := make([]string, len(args))
	for i, arg := range args {
		str, ok := starlark.AsString(arg)
		if !ok {
			return nil, fmt.Errorf("%s: argument %d is not a string", funcName, i)
		}
		result[i] = str
	}
	return result, nil
}
