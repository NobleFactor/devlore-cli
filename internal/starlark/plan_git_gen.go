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

// GitPlan implements plan.git.* bindings using the slot-based model.
type GitPlan struct {
	Receiver
	graph   *execution.Graph
	host    host.Host
	project string
	reg     *execution.ActionRegistry
}

// NewGitPlan creates a new GitPlan for the given graph and host.
func NewGitPlan(graph *execution.Graph, h host.Host, project string, reg *execution.ActionRegistry) *GitPlan {
	return &GitPlan{
		Receiver: NewReceiver("plan.git"),
		graph:    graph,
		host:     h,
		project:  project,
		reg:      reg,
	}
}

// Attr implements starlark.HasAttrs.
func (g *GitPlan) Attr(name string) (starlark.Value, error) {
	switch name {
	case "checkout":
		return MakeAttr("plan.git.checkout", g.checkout), nil
	case "clone":
		return MakeAttr("plan.git.clone", g.clone), nil
	case "pull":
		return MakeAttr("plan.git.pull", g.pull), nil
	// Predicate methods — return RuntimePredicate for plan.choose()
	case "installed":
		return starlark.NewBuiltin("plan.git.installed", g.predicateInstalled), nil
	default:
		return nil, NoSuchAttrError("plan.git", name)
	}
}

func (g *GitPlan) predicateInstalled(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	return gitInstalled(), nil
}

// AttrNames implements starlark.HasAttrs.
func (g *GitPlan) AttrNames() []string {
	return []string{"checkout", "clone", "installed", "pull"}
}

func (g *GitPlan) checkout(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var repo, ref starlark.Value
	if err := starlark.UnpackArgs("checkout", args, kwargs, "repo", &repo, "ref", &ref); err != nil {
		return nil, err
	}

	node := &execution.Node{
		ID:        generateNodeID("git-checkout"),
		Action: g.reg.MustGet("git.checkout"),
		Project:   g.project,
	}

	if err := FillSlot(node, g.graph, "repo", repo); err != nil {
		return nil, fmt.Errorf("checkout: repo: %w", err)
	}
	if err := FillSlot(node, g.graph, "ref", ref); err != nil {
		return nil, fmt.Errorf("checkout: ref: %w", err)
	}

	g.graph.Nodes = append(g.graph.Nodes, node)
	return NewOutput(node, g.graph, ""), nil
}

func (g *GitPlan) clone(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var url, path starlark.Value
	if err := starlark.UnpackArgs("clone", args, kwargs, "url", &url, "path", &path); err != nil {
		return nil, err
	}

	node := &execution.Node{
		ID:        generateNodeID("git-clone"),
		Action: g.reg.MustGet("git.clone"),
		Project:   g.project,
	}

	if err := FillSlot(node, g.graph, "url", url); err != nil {
		return nil, fmt.Errorf("clone: url: %w", err)
	}
	if err := FillSlot(node, g.graph, "path", path); err != nil {
		return nil, fmt.Errorf("clone: path: %w", err)
	}

	g.graph.Nodes = append(g.graph.Nodes, node)
	return NewOutput(node, g.graph, ""), nil
}

func (g *GitPlan) pull(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var repo starlark.Value
	if err := starlark.UnpackArgs("pull", args, kwargs, "repo", &repo); err != nil {
		return nil, err
	}

	node := &execution.Node{
		ID:        generateNodeID("git-pull"),
		Action: g.reg.MustGet("git.pull"),
		Project:   g.project,
	}

	if err := FillSlot(node, g.graph, "repo", repo); err != nil {
		return nil, fmt.Errorf("pull: repo: %w", err)
	}

	g.graph.Nodes = append(g.graph.Nodes, node)
	return NewOutput(node, g.graph, ""), nil
}
