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
// It provides access to sub-namespaces (package, file, template, encryption,
// archive, git, service, shell, net, content) and top-level bindings
// (source, gather).
type PlanRoot struct {
	graph   *execution.Graph
	host    host.Host
	project string
	reg     *execution.ActionRegistry

	// Sub-namespaces (cached)
	packagePlan    *PackagePlan
	filePlan       *FilePlan
	templatePlan   *TemplatePlan
	encryptionPlan *EncryptionPlan
	archivePlan    *ArchivePlan
	gitPlan        *GitPlan
	servicePlan    *ServicePlan
	shellPlan      *ShellPlan
	netPlan        *NetPlan
	contentPlan    *ContentPlan
}

// NewPlanRoot creates a new PlanRoot for the given graph and host.
func NewPlanRoot(graph *execution.Graph, h host.Host, project string, reg *execution.ActionRegistry) *PlanRoot {
	return &PlanRoot{
		graph:          graph,
		host:           h,
		project:        project,
		reg:            reg,
		packagePlan:    NewPackagePlan(graph, h, project, reg),
		filePlan:       NewFilePlan(graph, h, project, reg),
		templatePlan:   NewTemplatePlan(graph, h, project, reg),
		encryptionPlan: NewEncryptionPlan(graph, h, project, reg),
		archivePlan:    NewArchivePlan(graph, h, project, reg),
		gitPlan:        NewGitPlan(graph, h, project, reg),
		servicePlan:    NewServicePlan(graph, h, project, reg),
		shellPlan:      NewShellPlan(graph, h, project, reg),
		netPlan:        NewNetPlan(graph, h, project, reg),
		contentPlan:    NewContentPlan(graph, h, project, reg),
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
	case "archive":
		return p.archivePlan, nil
	case "content":
		return p.contentPlan, nil
	case "encryption":
		return p.encryptionPlan, nil
	case "file":
		return p.filePlan, nil
	case "git":
		return p.gitPlan, nil
	case "net":
		return p.netPlan, nil
	case "package":
		return p.packagePlan, nil
	case "service":
		return p.servicePlan, nil
	case "shell":
		return p.shellPlan, nil
	case "template":
		return p.templatePlan, nil
	// Top-level bindings (graph construction primitives)
	case "choose":
		return starlark.NewBuiltin("plan.choose", p.choose), nil
	case "source":
		return starlark.NewBuiltin("plan.source", p.source), nil
	case "gather":
		return starlark.NewBuiltin("plan.gather", p.gather), nil
	default:
		return nil, starlark.NoSuchAttrError(fmt.Sprintf("plan has no attribute %q", name))
	}
}

func (p *PlanRoot) AttrNames() []string {
	return []string{
		"archive", "choose", "content", "encryption", "file", "gather", "git",
		"net", "package", "service", "shell", "source", "template",
	}
}

// choose creates a conditional branch in the execution graph.
// Usage: plan.choose(when=predicate, then=callback)
//
// The predicate is evaluated at execution time on the target machine.
// If it matches, the nodes built by the callback are executed.
//
// Arguments:
//   - when: A predicate (e.g., plan.package.not_installed("docker-ce"))
//   - then: A callable that builds graph nodes when the predicate matches
//
// Returns: Output of the choose node
func (p *PlanRoot) choose(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var when starlark.Value
	var then starlark.Callable

	if err := starlark.UnpackArgs("choose", args, kwargs,
		"when", &when,
		"then", &then,
	); err != nil {
		return nil, err
	}

	// Extract predicate from the when argument.
	pred, ok := when.(*RuntimePredicate)
	if !ok {
		return nil, fmt.Errorf("choose: when must be a predicate (e.g., plan.package.not_installed(...)), got %s", when.Type())
	}

	// Snapshot current graph state to track nodes added by the callback.
	nodesBefore := len(p.graph.Nodes)

	// Execute the callback to build sub-graph nodes.
	_, err := starlark.Call(thread, then, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("choose: then callback: %w", err)
	}

	// Collect nodes added by the callback into a branch phase.
	branchPhaseID := generateNodeID("choose-branch")
	branchPhase := &execution.Phase{
		ID:     branchPhaseID,
		Name:   "choose-branch",
		Status: execution.PhasePending,
	}
	for i := nodesBefore; i < len(p.graph.Nodes); i++ {
		branchPhase.NodeIDs = append(branchPhase.NodeIDs, p.graph.Nodes[i].ID)
	}
	p.graph.Phases = append(p.graph.Phases, branchPhase)

	// Create the choose node with the predicate and branch phase.
	chooseNode := &execution.Node{
		ID:      generateNodeID("choose"),
		Action:  p.reg.MustGet("flow.choose"),
		Project: p.project,
	}
	chooseNode.SetSlotImmediate("cases", []execution.ChooseCase{
		{Predicate: pred, PhaseID: branchPhaseID},
	})

	p.graph.Nodes = append(p.graph.Nodes, chooseNode)
	return NewOutput(chooseNode, p.graph, ""), nil
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
		ID:      generateNodeID("source"),
		Action:  p.reg.MustGet("file.source"),
		Project: p.project,
	}

	if err := FillSlot(node, p.graph, "path", path); err != nil {
		return nil, fmt.Errorf("source: path: %w", err)
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
