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

	"github.com/NobleFactor/devlore-cli/internal/engine"
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
func NewPlanBindings(graph *engine.Graph, h host.Host, project string) PlatformPlanBindings {
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
func (w *WindowsPlanBindings) PackageInstall(packages ...string) *engine.Node {
	node := &engine.Node{
		ID:         windowsGenerateNodeID("package-install", packages...),
		Operations: []string{"package-install"},
		Project:    w.project,
		Metadata: map[string]string{
			"packages": strings.Join(packages, ","),
		},
	}
	w.graph.Nodes = append(w.graph.Nodes, node)
	return node
}

// PackageUpgrade adds a package upgrade node using the platform's package manager.
func (w *WindowsPlanBindings) PackageUpgrade(packages ...string) *engine.Node {
	node := &engine.Node{
		ID:         windowsGenerateNodeID("package-upgrade", packages...),
		Operations: []string{"package-upgrade"},
		Project:    w.project,
		Metadata: map[string]string{
			"packages": strings.Join(packages, ","),
		},
	}
	w.graph.Nodes = append(w.graph.Nodes, node)
	return node
}

// PackageRemove adds a package removal node using the platform's package manager.
func (w *WindowsPlanBindings) PackageRemove(packages ...string) *engine.Node {
	node := &engine.Node{
		ID:         windowsGenerateNodeID("package-remove", packages...),
		Operations: []string{"package-remove"},
		Project:    w.project,
		Metadata: map[string]string{
			"packages": strings.Join(packages, ","),
		},
	}
	w.graph.Nodes = append(w.graph.Nodes, node)
	return node
}

// PackageUpdate adds a package index update node using the platform's package manager.
func (w *WindowsPlanBindings) PackageUpdate() *engine.Node {
	node := &engine.Node{
		ID:         windowsGenerateNodeID("package-update"),
		Operations: []string{"package-update"},
		Project:    w.project,
		Metadata:   map[string]string{},
	}
	w.graph.Nodes = append(w.graph.Nodes, node)
	return node
}

// Configure adds a configuration file node.
func (w *WindowsPlanBindings) Configure(source, target string) *engine.Node {
	node := &engine.Node{
		ID:         windowsGenerateNodeID("configure"),
		Operations: []string{"expand", "copy"},
		Source:     source,
		Target:     w.host.ExpandPath(target),
		Project:    w.project,
	}
	w.graph.Nodes = append(w.graph.Nodes, node)
	return node
}

// Link adds a symlink creation node (requires admin on Windows).
func (w *WindowsPlanBindings) Link(source, target string) *engine.Node {
	node := &engine.Node{
		ID:         windowsGenerateNodeID("link"),
		Operations: []string{"link"},
		Source:     source,
		Target:     w.host.ExpandPath(target),
		Project:    w.project,
		Metadata: map[string]string{
			"requires_admin": "true",
		},
	}
	w.graph.Nodes = append(w.graph.Nodes, node)
	return node
}

// Copy adds a file copy node.
func (w *WindowsPlanBindings) Copy(source, target string) *engine.Node {
	node := &engine.Node{
		ID:         windowsGenerateNodeID("copy"),
		Operations: []string{"copy"},
		Source:     source,
		Target:     w.host.ExpandPath(target),
		Project:    w.project,
	}
	w.graph.Nodes = append(w.graph.Nodes, node)
	return node
}

// Mkdir adds a directory creation node.
func (w *WindowsPlanBindings) Mkdir(target string) *engine.Node {
	node := &engine.Node{
		ID:         windowsGenerateNodeID("mkdir"),
		Operations: []string{"mkdir"},
		Target:     w.host.ExpandPath(target),
		Project:    w.project,
	}
	w.graph.Nodes = append(w.graph.Nodes, node)
	return node
}

// Service adds a Windows Service management node.
func (w *WindowsPlanBindings) Service(name string, action loreStar.ServiceAction) *engine.Node {
	node := &engine.Node{
		ID:         windowsGenerateNodeID("winservice", name, action.String()),
		Operations: []string{"winservice-" + action.String()},
		Project:    w.project,
		Metadata: map[string]string{
			"service": name,
			"action":  action.String(),
		},
	}
	w.graph.Nodes = append(w.graph.Nodes, node)
	return node
}

// Shell adds a shell command execution node (PowerShell on Windows).
func (w *WindowsPlanBindings) Shell(command string) *engine.Node {
	node := &engine.Node{
		ID:         windowsGenerateNodeID("shell"),
		Operations: []string{"powershell"},
		Project:    w.project,
		Metadata: map[string]string{
			"command": command,
		},
	}
	w.graph.Nodes = append(w.graph.Nodes, node)
	return node
}

// DependsOn creates a dependency edge between nodes.
func (w *WindowsPlanBindings) DependsOn(from, to *engine.Node) {
	w.graph.Edges = append(w.graph.Edges, engine.Edge{
		From:     to.ID,
		To:       from.ID,
		Relation: "depends_on",
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
		"link":      starlark.NewBuiltin("link", w.linkBuiltin),
		"copy":      starlark.NewBuiltin("copy", w.copyBuiltin),
		"mkdir":     starlark.NewBuiltin("mkdir", w.mkdirBuiltin),
	})

	return starlarkstruct.FromStringDict(starlark.String("plan"), starlark.StringDict{
		"package":    packageOps,
		"file":       fileOps,
		"service":    starlark.NewBuiltin("service", w.serviceBuiltin),
		"shell":      starlark.NewBuiltin("shell", w.shellBuiltin),
		"depends_on": starlark.NewBuiltin("depends_on", w.dependsOnBuiltin),
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
	return windowsNodeToStarlark(node), nil
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
	return windowsNodeToStarlark(node), nil
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
	node := w.PackageRemove(packages...)
	return windowsNodeToStarlark(node), nil
}

func (w *WindowsPlanBindings) packageUpdateBuiltin(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	node := w.PackageUpdate()
	return windowsNodeToStarlark(node), nil
}

func (w *WindowsPlanBindings) configureBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var source, target string
	if err := starlark.UnpackArgs("configure", args, kwargs, "source", &source, "target", &target); err != nil {
		return nil, err
	}
	node := w.Configure(source, target)
	return windowsNodeToStarlark(node), nil
}

func (w *WindowsPlanBindings) linkBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var source, target string
	if err := starlark.UnpackArgs("link", args, kwargs, "source", &source, "target", &target); err != nil {
		return nil, err
	}
	node := w.Link(source, target)
	return windowsNodeToStarlark(node), nil
}

func (w *WindowsPlanBindings) copyBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var source, target string
	if err := starlark.UnpackArgs("copy", args, kwargs, "source", &source, "target", &target); err != nil {
		return nil, err
	}
	node := w.Copy(source, target)
	return windowsNodeToStarlark(node), nil
}

func (w *WindowsPlanBindings) mkdirBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var target string
	if err := starlark.UnpackArgs("mkdir", args, kwargs, "target", &target); err != nil {
		return nil, err
	}
	node := w.Mkdir(target)
	return windowsNodeToStarlark(node), nil
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
	return windowsNodeToStarlark(node), nil
}

func (w *WindowsPlanBindings) shellBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var command string
	if err := starlark.UnpackArgs("shell", args, kwargs, "command", &command); err != nil {
		return nil, err
	}
	node := w.Shell(command)
	return windowsNodeToStarlark(node), nil
}

func (w *WindowsPlanBindings) dependsOnBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
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

	w.graph.Edges = append(w.graph.Edges, engine.Edge{
		From:     toIDStr,
		To:       fromIDStr,
		Relation: "depends_on",
	})

	return starlark.None, nil
}

// windowsNodeToStarlark converts an engine.Node to a Starlark struct.
func windowsNodeToStarlark(node *engine.Node) starlark.Value {
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
