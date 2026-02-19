// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Code generated from gen-receiver templates; DO NOT EDIT.

package starlark

import (
	"fmt"
	"strings"

	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/internal/execution"
	"github.com/NobleFactor/devlore-cli/internal/host"
	"github.com/NobleFactor/devlore-cli/internal/lorepackage"
)

// PackagePlan implements plan.package.* bindings using the slot-based model.
// Each method adds a node to the execution graph.
//
// Package names may include manager prefixes (brew:pkg, cask:pkg, port:pkg)
// for platform-specific package manager overrides.
type PackagePlan struct {
	Receiver
	graph   *execution.Graph
	host    host.Host
	project string
	reg     *execution.ActionRegistry
}

// NewPackagePlan creates a new PackagePlan for the given graph and host.
func NewPackagePlan(graph *execution.Graph, h host.Host, project string, reg *execution.ActionRegistry) *PackagePlan {
	return &PackagePlan{
		Receiver: NewReceiver("plan.package"),
		graph:    graph,
		host:     h,
		project:  project,
		reg:      reg,
	}
}

// Attr implements starlark.HasAttrs.
func (p *PackagePlan) Attr(name string) (starlark.Value, error) {
	switch name {
	case "install":
		return MakeAttr("plan.package.install", p.install), nil
	case "upgrade":
		return MakeAttr("plan.package.upgrade", p.upgrade), nil
	case "remove":
		return MakeAttr("plan.package.remove", p.remove), nil
	case "update":
		return MakeAttr("plan.package.update", p.update), nil
	// Predicate methods — return RuntimePredicate for plan.choose()
	case "installed":
		return starlark.NewBuiltin("plan.package.installed", p.predicateInstalled), nil
	case "not_installed":
		return starlark.NewBuiltin("plan.package.not_installed", p.predicateNotInstalled), nil
	case "version_gte":
		return starlark.NewBuiltin("plan.package.version_gte", p.predicateVersionGTE), nil
	default:
		return nil, NoSuchAttrError("plan.package", name)
	}
}

func (p *PackagePlan) predicateInstalled(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	if err := starlark.UnpackArgs("installed", args, kwargs, "name", &name); err != nil {
		return nil, err
	}
	return packageInstalled(p.host.PackageManager(), name), nil
}

func (p *PackagePlan) predicateNotInstalled(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	if err := starlark.UnpackArgs("not_installed", args, kwargs, "name", &name); err != nil {
		return nil, err
	}
	return packageNotInstalled(p.host.PackageManager(), name), nil
}

func (p *PackagePlan) predicateVersionGTE(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name, version string
	if err := starlark.UnpackArgs("version_gte", args, kwargs, "name", &name, "version", &version); err != nil {
		return nil, err
	}
	return packageVersionGTE(p.host.PackageManager(), name, version), nil
}

// AttrNames implements starlark.HasAttrs.
func (p *PackagePlan) AttrNames() []string {
	return []string{"install", "installed", "not_installed", "remove", "update", "upgrade", "version_gte"}
}

func (p *PackagePlan) install(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	packages, err := argsToStrings("install", args)
	if err != nil {
		return nil, err
	}
	if len(packages) == 0 {
		return nil, fmt.Errorf("install: at least one package required")
	}

	cleanPkgs, manager, isCask := parsePackagesWithPrefix(packages)
	node := &execution.Node{
		ID:      generateNodeID("package-install", cleanPkgs...),
		Action:  p.reg.MustGet("pkg.install"),
		Project: p.project,
	}
	node.SetSlotImmediate("packages", strings.Join(cleanPkgs, ","))
	if manager != "" {
		node.SetSlotImmediate("manager", manager)
	}
	if isCask {
		node.SetSlotImmediate("cask", "true")
	}

	p.graph.Nodes = append(p.graph.Nodes, node)
	return NewOutput(node, p.graph, ""), nil
}

func (p *PackagePlan) upgrade(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	packages, err := argsToStrings("upgrade", args)
	if err != nil {
		return nil, err
	}
	if len(packages) == 0 {
		return nil, fmt.Errorf("upgrade: at least one package required")
	}

	cleanPkgs, manager, isCask := parsePackagesWithPrefix(packages)
	node := &execution.Node{
		ID:      generateNodeID("package-upgrade", cleanPkgs...),
		Action:  p.reg.MustGet("pkg.upgrade"),
		Project: p.project,
	}
	node.SetSlotImmediate("packages", strings.Join(cleanPkgs, ","))
	if manager != "" {
		node.SetSlotImmediate("manager", manager)
	}
	if isCask {
		node.SetSlotImmediate("cask", "true")
	}

	p.graph.Nodes = append(p.graph.Nodes, node)
	return NewOutput(node, p.graph, ""), nil
}

func (p *PackagePlan) remove(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	packages, err := argsToStrings("remove", args)
	if err != nil {
		return nil, err
	}
	if len(packages) == 0 {
		return nil, fmt.Errorf("remove: at least one package required")
	}

	cleanPkgs, manager, isCask := parsePackagesWithPrefix(packages)
	node := &execution.Node{
		ID:      generateNodeID("package-remove", cleanPkgs...),
		Action:  p.reg.MustGet("pkg.remove"),
		Project: p.project,
	}
	node.SetSlotImmediate("packages", strings.Join(cleanPkgs, ","))
	if manager != "" {
		node.SetSlotImmediate("manager", manager)
	}
	if isCask {
		node.SetSlotImmediate("cask", "true")
	}

	p.graph.Nodes = append(p.graph.Nodes, node)
	return starlark.None, nil
}

func (p *PackagePlan) update(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	if len(args) > 0 {
		return nil, fmt.Errorf("update: takes no arguments")
	}

	node := &execution.Node{
		ID:      generateNodeID("package-update"),
		Action:  p.reg.MustGet("pkg.update"),
		Project: p.project,
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

// parsePackagesWithPrefix extracts manager override from brew:, cask:, or port: prefixes.
func parsePackagesWithPrefix(packages []string) (cleanPkgs []string, manager string, isCask bool) {
	cleanPkgs = make([]string, len(packages))
	for i, p := range packages {
		pkg, prefix := lorepackage.ParsePackagePrefix(p)
		cleanPkgs[i] = pkg
		if prefix != "" {
			if prefix == "cask" {
				manager = "brew"
				isCask = true
			} else {
				manager = prefix
			}
		}
	}
	return cleanPkgs, manager, isCask
}
