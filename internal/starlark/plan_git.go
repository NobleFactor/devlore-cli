// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlark

import (
	"fmt"

	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/internal/execution"
	"github.com/NobleFactor/devlore-cli/internal/host"
)

// GitPlan implements plan.git.* bindings using the slot-based model.
type GitPlan struct {
	graph   *execution.Graph
	host    host.Host
	project string
}

// NewGitPlan creates a new GitPlan for the given graph and host.
func NewGitPlan(graph *execution.Graph, h host.Host, project string) *GitPlan {
	return &GitPlan{
		graph:   graph,
		host:    h,
		project: project,
	}
}

// Starlark Value interface
func (g *GitPlan) String() string        { return "plan.git" }
func (g *GitPlan) Type() string          { return "plan.git" }
func (g *GitPlan) Freeze()               {}
func (g *GitPlan) Truth() starlark.Bool  { return true }
func (g *GitPlan) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable: plan.git") }

// Starlark HasAttrs interface
func (g *GitPlan) Attr(name string) (starlark.Value, error) {
	switch name {
	case "clone":
		return starlark.NewBuiltin("plan.git.clone", g.clone), nil
	case "checkout":
		return starlark.NewBuiltin("plan.git.checkout", g.checkout), nil
	case "pull":
		return starlark.NewBuiltin("plan.git.pull", g.pull), nil
	default:
		return nil, starlark.NoSuchAttrError(fmt.Sprintf("plan.git has no attribute %q", name))
	}
}

func (g *GitPlan) AttrNames() []string {
	return []string{"checkout", "clone", "pull"}
}

// clone clones a git repository.
// Usage: plan.git.clone(url, path)
//
// Slots:
//   - url: Repository URL to clone (promise or immediate)
//   - path: Local path to clone into (promise or immediate)
//
// Returns: Promise of the cloned repository
func (g *GitPlan) clone(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var url, path starlark.Value
	if err := starlark.UnpackArgs("clone", args, kwargs, "url", &url, "path", &path); err != nil {
		return nil, err
	}

	node := &execution.Node{
		ID:         generateNodeID("git-clone"),
		Operations: []string{"git-clone"},
		Project:    g.project,
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

// checkout checks out a git ref in a repository.
// Usage: plan.git.checkout(repo, ref)
//
// Slots:
//   - repo: Repository to checkout in (promise from clone)
//   - ref: Branch, tag, or commit to checkout (promise or immediate)
//
// Returns: Promise of the checked out repository
func (g *GitPlan) checkout(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var repo, ref starlark.Value
	if err := starlark.UnpackArgs("checkout", args, kwargs, "repo", &repo, "ref", &ref); err != nil {
		return nil, err
	}

	node := &execution.Node{
		ID:         generateNodeID("git-checkout"),
		Operations: []string{"git-checkout"},
		Project:    g.project,
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

// pull pulls latest changes in a repository.
// Usage: plan.git.pull(repo)
//
// Slots:
//   - repo: Repository to pull in (promise from clone)
//
// Returns: Promise of the updated repository
func (g *GitPlan) pull(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var repo starlark.Value
	if err := starlark.UnpackArgs("pull", args, kwargs, "repo", &repo); err != nil {
		return nil, err
	}

	node := &execution.Node{
		ID:         generateNodeID("git-pull"),
		Operations: []string{"git-pull"},
		Project:    g.project,
	}

	if err := FillSlot(node, g.graph, "repo", repo); err != nil {
		return nil, fmt.Errorf("pull: repo: %w", err)
	}

	g.graph.Nodes = append(g.graph.Nodes, node)
	return NewOutput(node, g.graph, ""), nil
}
