//go:build windows

// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package platform

import (
	"fmt"
	"strings"
	"sync/atomic"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/NobleFactor/devlore-cli/internal/execution"
	"github.com/NobleFactor/devlore-cli/internal/host"
	loreStar "github.com/NobleFactor/devlore-cli/internal/starlark"
)

// windowsNodeCounter provides unique node IDs for Windows plan bindings.
var windowsNodeCounter uint64

func windowsGenerateNodeID(prefix string, components ...string) string {
	id := atomic.AddUint64(&windowsNodeCounter, 1)
	if len(components) > 0 {
		return fmt.Sprintf("%s-%s-%d", prefix, strings.Join(components, "-"), id)
	}
	return fmt.Sprintf("%s-%d", prefix, id)
}

// WindowsPlanBindings provides Windows-specific plan bindings.
// Uses winget for package management and Windows Services for services.
type WindowsPlanBindings struct {
	*basePlanBindings
}

// NewPlanBindings creates a new Windows-specific PlanBindings.
func NewPlanBindings(graph *execution.Graph, h host.Host, project string) PlatformPlanBindings {
	return &WindowsPlanBindings{
		basePlanBindings: newBasePlanBindings(graph, h, project),
	}
}

// PlatformName returns "windows".
func (w *WindowsPlanBindings) PlatformName() string {
	return "windows"
}

// PackageManagerName returns the winget manager name.
func (w *WindowsPlanBindings) PackageManagerName() string {
	return w.host.PackageManager().Name()
}

// PackageInstall adds a package installation node using the platform's package manager.
func (w *WindowsPlanBindings) PackageInstall(packages ...string) *execution.Node {
	node := &execution.Node{
		ID:         windowsGenerateNodeID("package-install", packages...),
		Operations: []string{"package-install"},
		Project:    w.project,
	}
	node.SetSlotImmediate("packages", strings.Join(packages, ","))
	w.graph.Nodes = append(w.graph.Nodes, node)
	return node
}

// PackageUpgrade adds a package upgrade node using the platform's package manager.
func (w *WindowsPlanBindings) PackageUpgrade(packages ...string) *execution.Node {
	node := &execution.Node{
		ID:         windowsGenerateNodeID("package-upgrade", packages...),
		Operations: []string{"package-upgrade"},
		Project:    w.project,
	}
	node.SetSlotImmediate("packages", strings.Join(packages, ","))
	w.graph.Nodes = append(w.graph.Nodes, node)
	return node
}

// PackageRemove adds a package removal node using the platform's package manager.
func (w *WindowsPlanBindings) PackageRemove(packages ...string) *execution.Node {
	node := &execution.Node{
		ID:         windowsGenerateNodeID("package-remove", packages...),
		Operations: []string{"package-remove"},
		Project:    w.project,
	}
	node.SetSlotImmediate("packages", strings.Join(packages, ","))
	w.graph.Nodes = append(w.graph.Nodes, node)
	return node
}

// PackageUpdate adds a package index update node using the platform's package manager.
func (w *WindowsPlanBindings) PackageUpdate() *execution.Node {
	node := &execution.Node{
		ID:         windowsGenerateNodeID("package-update"),
		Operations: []string{"package-update"},
		Project:    w.project,
	}
	w.graph.Nodes = append(w.graph.Nodes, node)
	return node
}

// Configure adds a configuration file node.
func (w *WindowsPlanBindings) Configure(source, target string) *execution.Node {
	node := &execution.Node{
		ID:         windowsGenerateNodeID("configure"),
		Operations: []string{"render", "copy"},
		Project:    w.project,
	}
	node.SetSlotImmediate("source", source)
	node.SetSlotImmediate("path", w.host.ExpandPath(target))
	w.graph.Nodes = append(w.graph.Nodes, node)
	return node
}

// Link adds a symlink creation node (requires admin on Windows).
func (w *WindowsPlanBindings) Link(source, target string) *execution.Node {
	node := &execution.Node{
		ID:         windowsGenerateNodeID("link"),
		Operations: []string{"link"},
		Project:    w.project,
	}
	node.SetSlotImmediate("source", source)
	node.SetSlotImmediate("path", w.host.ExpandPath(target))
	node.SetSlotImmediate("requires_admin", "true")
	w.graph.Nodes = append(w.graph.Nodes, node)
	return node
}

// Copy adds a file copy node.
func (w *WindowsPlanBindings) Copy(source, target string) *execution.Node {
	node := &execution.Node{
		ID:         windowsGenerateNodeID("copy"),
		Operations: []string{"copy"},
		Project:    w.project,
	}
	node.SetSlotImmediate("source", source)
	node.SetSlotImmediate("path", w.host.ExpandPath(target))
	w.graph.Nodes = append(w.graph.Nodes, node)
	return node
}

// Write adds a file write node (write content directly to target).
func (w *WindowsPlanBindings) Write(target, content string) *execution.Node {
	node := &execution.Node{
		ID:         windowsGenerateNodeID("write"),
		Operations: []string{"write"},
		Project:    w.project,
	}
	node.SetSlotImmediate("content", content)
	node.SetSlotImmediate("path", w.host.ExpandPath(target))
	w.graph.Nodes = append(w.graph.Nodes, node)
	return node
}

// Service adds a Windows Service management node.
func (w *WindowsPlanBindings) Service(name string, action loreStar.ServiceAction) *execution.Node {
	node := &execution.Node{
		ID:         windowsGenerateNodeID("winservice", name, action.String()),
		Operations: []string{"winservice-" + action.String()},
		Project:    w.project,
	}
	node.SetSlotImmediate("name", name)
	node.SetSlotImmediate("action", action.String())
	w.graph.Nodes = append(w.graph.Nodes, node)
	return node
}

// Shell adds a shell command execution node (PowerShell on Windows).
func (w *WindowsPlanBindings) Shell(command string) *execution.Node {
	node := &execution.Node{
		ID:         windowsGenerateNodeID("shell"),
		Operations: []string{"powershell"},
		Project:    w.project,
	}
	node.SetSlotImmediate("command", command)
	w.graph.Nodes = append(w.graph.Nodes, node)
	return node
}

// DependsOn creates a dependency edge between nodes.
func (w *WindowsPlanBindings) DependsOn(from, to *execution.Node) {
	w.graph.Edges = append(w.graph.Edges, execution.Edge{
		From: to.ID,
		To:   from.ID,
	})
}

// ToStarlark converts the plan bindings to a Starlark struct.
// Uses nested structs: plan.package.install(), plan.file.copy(), etc.
func (w *WindowsPlanBindings) ToStarlark() starlark.Value {
	// Package operations namespace: plan.package.*
	packageOps := starlarkstruct.FromStringDict(starlark.String("package"), starlark.StringDict{
		"install": starlark.NewBuiltin("install", w.packageInstallBuiltin),
		"upgrade": starlark.NewBuiltin("upgrade", w.packageUpgradeBuiltin),
		"remove":  starlark.NewBuiltin("remove", w.packageRemoveBuiltin),
		"update":  starlark.NewBuiltin("update", w.packageUpdateBuiltin),
	})

	// File operations namespace: plan.file.*
	fileOps := starlarkstruct.FromStringDict(starlark.String("file"), starlark.StringDict{
		"configure": starlark.NewBuiltin("configure", w.configureBuiltin),
		"copy":      starlark.NewBuiltin("copy", w.copyBuiltin),
		"link":      starlark.NewBuiltin("link", w.linkBuiltin),
		"write":     starlark.NewBuiltin("write", w.writeBuiltin),
	})

	return starlarkstruct.FromStringDict(starlark.String("plan"), starlark.StringDict{
		// Namespaces
		"file":    fileOps,
		"package": packageOps,
		// Global functions (at root of plan)
		"gather":  starlark.NewBuiltin("gather", w.gatherBuiltin),
		"service": starlark.NewBuiltin("service", w.serviceBuiltin),
		"shell":   starlark.NewBuiltin("shell", w.shellBuiltin),
	})
}

func (w *WindowsPlanBindings) packageInstallBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
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
	node := w.PackageInstall(packages...)
	packagesStr := strings.Join(packages, ",")
	return loreStar.NewOutput(node, w.graph, packagesStr, loreStar.OutputPackage), nil
}

func (w *WindowsPlanBindings) packageUpgradeBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
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
	node := w.PackageUpgrade(packages...)
	packagesStr := strings.Join(packages, ",")
	return loreStar.NewOutput(node, w.graph, packagesStr, loreStar.OutputPackage), nil
}

func (w *WindowsPlanBindings) packageRemoveBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
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
	_ = w.PackageRemove(packages...)
	// Remove produces no output
	return starlark.None, nil
}

func (w *WindowsPlanBindings) packageUpdateBuiltin(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	node := w.PackageUpdate()
	return loreStar.NewOutput(node, w.graph, "<index>", loreStar.OutputPackage), nil
}

func (w *WindowsPlanBindings) configureBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var inputArg starlark.Value
	var out string
	if err := starlark.UnpackArgs("configure", args, kwargs, "input", &inputArg, "out", &out); err != nil {
		return nil, err
	}

	input, err := loreStar.ResolveInput(inputArg)
	if err != nil {
		return nil, fmt.Errorf("configure: input: %w", err)
	}

	node := w.Configure(input.Path(), out)
	input.DependOn(node)
	return loreStar.NewOutput(node, w.graph, node.GetSlot("path"), loreStar.OutputFile), nil
}

func (w *WindowsPlanBindings) linkBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var inputArg starlark.Value
	var out string
	if err := starlark.UnpackArgs("link", args, kwargs, "input", &inputArg, "out", &out); err != nil {
		return nil, err
	}

	input, err := loreStar.ResolveInput(inputArg)
	if err != nil {
		return nil, fmt.Errorf("link: input: %w", err)
	}

	node := w.Link(input.Path(), out)
	input.DependOn(node)
	return loreStar.NewOutput(node, w.graph, node.GetSlot("path"), loreStar.OutputSymlink), nil
}

func (w *WindowsPlanBindings) copyBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var inputArg starlark.Value
	var out string
	if err := starlark.UnpackArgs("copy", args, kwargs, "input", &inputArg, "out", &out); err != nil {
		return nil, err
	}

	input, err := loreStar.ResolveInput(inputArg)
	if err != nil {
		return nil, fmt.Errorf("copy: input: %w", err)
	}

	node := w.Copy(input.Path(), out)
	input.DependOn(node)
	return loreStar.NewOutput(node, w.graph, node.GetSlot("path"), loreStar.OutputFile), nil
}

func (w *WindowsPlanBindings) writeBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var inputArg starlark.Value
	var out string
	if err := starlark.UnpackArgs("write", args, kwargs, "input", &inputArg, "out", &out); err != nil {
		return nil, err
	}

	input, err := loreStar.ResolveInput(inputArg)
	if err != nil {
		return nil, fmt.Errorf("write: input: %w", err)
	}

	expandedOut := w.host.ExpandPath(out)
	node := &execution.Node{
		ID:         windowsGenerateNodeID("write"),
		Operations: []string{"write"},
		Project:    w.project,
	}
	node.SetSlotImmediate("source", input.Path())
	node.SetSlotImmediate("path", expandedOut)
	w.graph.Nodes = append(w.graph.Nodes, node)
	input.DependOn(node)
	return loreStar.NewOutput(node, w.graph, expandedOut, loreStar.OutputFile), nil
}

func (w *WindowsPlanBindings) serviceBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name, action string
	if err := starlark.UnpackArgs("service", args, kwargs, "name", &name, "action", &action); err != nil {
		return nil, err
	}

	var serviceAction loreStar.ServiceAction
	switch action {
	case "start":
		serviceAction = loreStar.ServiceStart
	case "stop":
		serviceAction = loreStar.ServiceStop
	case "restart":
		serviceAction = loreStar.ServiceRestart
	case "enable":
		serviceAction = loreStar.ServiceEnable
	case "disable":
		serviceAction = loreStar.ServiceDisable
	default:
		return nil, fmt.Errorf("service: unknown action %q", action)
	}

	node := w.Service(name, serviceAction)
	return loreStar.NewOutput(node, w.graph, name, loreStar.OutputService), nil
}

func (w *WindowsPlanBindings) shellBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var command string
	if err := starlark.UnpackArgs("shell", args, kwargs, "command", &command); err != nil {
		return nil, err
	}
	node := w.Shell(command)
	return loreStar.NewOutput(node, w.graph, command, loreStar.OutputCommand), nil
}

func (w *WindowsPlanBindings) gatherBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("gather: expected at least 2 arguments, got %d", len(args))
	}

	outputs := make([]*loreStar.Output, 0, len(args))
	for i, arg := range args {
		output, ok := arg.(*loreStar.Output)
		if !ok {
			return nil, fmt.Errorf("gather: argument %d must be an Output, got %s", i+1, arg.Type())
		}
		outputs = append(outputs, output)
	}

	return loreStar.NewGather(w.graph, outputs...), nil
}
