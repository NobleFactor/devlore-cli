//go:build darwin

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
	"github.com/NobleFactor/devlore-cli/internal/registry"
	loreStar "github.com/NobleFactor/devlore-cli/internal/starlark"
)

// darwinNodeCounter provides unique node IDs for Darwin plan bindings.
var darwinNodeCounter uint64

func darwinGenerateNodeID(prefix string, components ...string) string {
	id := atomic.AddUint64(&darwinNodeCounter, 1)
	if len(components) > 0 {
		return fmt.Sprintf("%s-%s-%d", prefix, strings.Join(components, "-"), id)
	}
	return fmt.Sprintf("%s-%d", prefix, id)
}

// DarwinPlanBindings provides macOS-specific plan bindings.
// Uses Homebrew for package management and launchd for services.
type DarwinPlanBindings struct {
	*basePlanBindings
}

// NewPlanBindings creates a new Darwin-specific PlanBindings.
func NewPlanBindings(graph *engine.Graph, h host.Host, project string) PlatformPlanBindings {
	return &DarwinPlanBindings{
		basePlanBindings: newBasePlanBindings(graph, h, project),
	}
}

// PlatformName returns "darwin".
func (d *DarwinPlanBindings) PlatformName() string {
	return "darwin"
}

// PackageManagerName returns the Homebrew manager name.
func (d *DarwinPlanBindings) PackageManagerName() string {
	return d.host.PackageManager().Name()
}

// PackageInstall adds a package installation node using the platform's package manager.
// Supports brew:pkg and port:pkg prefixes to override auto-detection.
func (d *DarwinPlanBindings) PackageInstall(packages ...string) *engine.Node {
	cleanPkgs, manager := parsePackagesWithPrefix(packages)
	node := &engine.Node{
		ID:         darwinGenerateNodeID("package-install", cleanPkgs...),
		Operations: []string{"package-install"},
		Project:    d.project,
		Metadata: map[string]string{
			"packages": strings.Join(cleanPkgs, ","),
		},
	}
	if manager != "" {
		node.Metadata["manager"] = manager
	}
	d.graph.Nodes = append(d.graph.Nodes, node)
	return node
}

// PackageInstallCask adds a Homebrew Cask installation node for GUI applications.
// Note: Cask is Homebrew-specific, so this always uses brew.
func (d *DarwinPlanBindings) PackageInstallCask(packages ...string) *engine.Node {
	node := &engine.Node{
		ID:         darwinGenerateNodeID("package-install-cask", packages...),
		Operations: []string{"package-install"},
		Project:    d.project,
		Metadata: map[string]string{
			"packages": strings.Join(packages, ","),
			"manager":  "brew",
			"cask":     "true",
		},
	}
	d.graph.Nodes = append(d.graph.Nodes, node)
	return node
}

// PackageUpgrade adds a package upgrade node using the platform's package manager.
// Supports brew:pkg and port:pkg prefixes to override auto-detection.
func (d *DarwinPlanBindings) PackageUpgrade(packages ...string) *engine.Node {
	cleanPkgs, manager := parsePackagesWithPrefix(packages)
	node := &engine.Node{
		ID:         darwinGenerateNodeID("package-upgrade", cleanPkgs...),
		Operations: []string{"package-upgrade"},
		Project:    d.project,
		Metadata: map[string]string{
			"packages": strings.Join(cleanPkgs, ","),
		},
	}
	if manager != "" {
		node.Metadata["manager"] = manager
	}
	d.graph.Nodes = append(d.graph.Nodes, node)
	return node
}

// PackageRemove adds a package removal node using the platform's package manager.
// Supports brew:pkg and port:pkg prefixes to override auto-detection.
func (d *DarwinPlanBindings) PackageRemove(packages ...string) *engine.Node {
	cleanPkgs, manager := parsePackagesWithPrefix(packages)
	node := &engine.Node{
		ID:         darwinGenerateNodeID("package-remove", cleanPkgs...),
		Operations: []string{"package-remove"},
		Project:    d.project,
		Metadata: map[string]string{
			"packages": strings.Join(cleanPkgs, ","),
		},
	}
	if manager != "" {
		node.Metadata["manager"] = manager
	}
	d.graph.Nodes = append(d.graph.Nodes, node)
	return node
}

// PackageUpdate adds a package index update node using the platform's package manager.
func (d *DarwinPlanBindings) PackageUpdate() *engine.Node {
	node := &engine.Node{
		ID:         darwinGenerateNodeID("package-update"),
		Operations: []string{"package-update"},
		Project:    d.project,
		Metadata:   map[string]string{},
	}
	d.graph.Nodes = append(d.graph.Nodes, node)
	return node
}

// parsePackagesWithPrefix extracts manager override from brew:pkg or port:pkg prefixes.
// Returns clean package names and the manager override (if any).
func parsePackagesWithPrefix(packages []string) ([]string, string) {
	clean := make([]string, len(packages))
	var manager string
	for i, p := range packages {
		pkg, prefix := registry.ParsePackagePrefix(p)
		clean[i] = pkg
		if prefix != "" {
			manager = prefix // Last prefix wins if mixed (shouldn't happen)
		}
	}
	return clean, manager
}

// Configure adds a configuration file node.
func (d *DarwinPlanBindings) Configure(source, target string) *engine.Node {
	node := &engine.Node{
		ID:         darwinGenerateNodeID("configure"),
		Operations: []string{"expand", "copy"},
		Source:     source,
		Target:     d.host.ExpandPath(target),
		Project:    d.project,
	}
	d.graph.Nodes = append(d.graph.Nodes, node)
	return node
}

// Link adds a symlink creation node.
func (d *DarwinPlanBindings) Link(source, target string) *engine.Node {
	node := &engine.Node{
		ID:         darwinGenerateNodeID("link"),
		Operations: []string{"link"},
		Source:     source,
		Target:     d.host.ExpandPath(target),
		Project:    d.project,
	}
	d.graph.Nodes = append(d.graph.Nodes, node)
	return node
}

// Copy adds a file copy node.
func (d *DarwinPlanBindings) Copy(source, target string) *engine.Node {
	node := &engine.Node{
		ID:         darwinGenerateNodeID("copy"),
		Operations: []string{"copy"},
		Source:     source,
		Target:     d.host.ExpandPath(target),
		Project:    d.project,
	}
	d.graph.Nodes = append(d.graph.Nodes, node)
	return node
}

// Mkdir adds a directory creation node.
func (d *DarwinPlanBindings) Mkdir(target string) *engine.Node {
	node := &engine.Node{
		ID:         darwinGenerateNodeID("mkdir"),
		Operations: []string{"mkdir"},
		Target:     d.host.ExpandPath(target),
		Project:    d.project,
	}
	d.graph.Nodes = append(d.graph.Nodes, node)
	return node
}

// Service adds a launchd service management node.
func (d *DarwinPlanBindings) Service(name string, action loreStar.ServiceAction) *engine.Node {
	node := &engine.Node{
		ID:         darwinGenerateNodeID("launchd", name, action.String()),
		Operations: []string{"launchd-" + action.String()},
		Project:    d.project,
		Metadata: map[string]string{
			"service": name,
			"action":  action.String(),
		},
	}
	d.graph.Nodes = append(d.graph.Nodes, node)
	return node
}

// Shell adds a shell command execution node.
func (d *DarwinPlanBindings) Shell(command string) *engine.Node {
	node := &engine.Node{
		ID:         darwinGenerateNodeID("shell"),
		Operations: []string{"shell"},
		Project:    d.project,
		Metadata: map[string]string{
			"command": command,
		},
	}
	d.graph.Nodes = append(d.graph.Nodes, node)
	return node
}

// DependsOn creates a dependency edge between nodes.
func (d *DarwinPlanBindings) DependsOn(from, to *engine.Node) {
	d.graph.Edges = append(d.graph.Edges, engine.Edge{
		From:     to.ID,
		To:       from.ID,
		Relation: "depends_on",
	})
}

// ToStarlark converts the plan bindings to a Starlark struct.
// Uses nested structs: plan.package.install(), plan.file.copy(), etc.
func (d *DarwinPlanBindings) ToStarlark() starlark.Value {
	// Package operations namespace: plan.package.*
	packageOps := starlarkstruct.FromStringDict(starlark.String("package"), starlark.StringDict{
		"install":      starlark.NewBuiltin("install", d.packageInstallBuiltin),
		"install_cask": starlark.NewBuiltin("install_cask", d.packageInstallCaskBuiltin),
		"upgrade":      starlark.NewBuiltin("upgrade", d.packageUpgradeBuiltin),
		"remove":       starlark.NewBuiltin("remove", d.packageRemoveBuiltin),
		"update":       starlark.NewBuiltin("update", d.packageUpdateBuiltin),
	})

	// File operations namespace: plan.file.*
	fileOps := starlarkstruct.FromStringDict(starlark.String("file"), starlark.StringDict{
		"configure": starlark.NewBuiltin("configure", d.configureBuiltin),
		"link":      starlark.NewBuiltin("link", d.linkBuiltin),
		"copy":      starlark.NewBuiltin("copy", d.copyBuiltin),
		"mkdir":     starlark.NewBuiltin("mkdir", d.mkdirBuiltin),
	})

	return starlarkstruct.FromStringDict(starlark.String("plan"), starlark.StringDict{
		"package":    packageOps,
		"file":       fileOps,
		"service":    starlark.NewBuiltin("service", d.serviceBuiltin),
		"shell":      starlark.NewBuiltin("shell", d.shellBuiltin),
		"depends_on": starlark.NewBuiltin("depends_on", d.dependsOnBuiltin),
	})
}

func (d *DarwinPlanBindings) packageInstallBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
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
	node := d.PackageInstall(packages...)
	return nodeToStarlark(node), nil
}

func (d *DarwinPlanBindings) packageInstallCaskBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	packages := make([]string, len(args))
	for i, arg := range args {
		str, ok := starlark.AsString(arg)
		if !ok {
			return nil, fmt.Errorf("install_cask: argument %d is not a string", i)
		}
		packages[i] = str
	}
	if len(packages) == 0 {
		return nil, fmt.Errorf("install_cask: at least one package required")
	}
	node := d.PackageInstallCask(packages...)
	return nodeToStarlark(node), nil
}

func (d *DarwinPlanBindings) packageUpgradeBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
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
	node := d.PackageUpgrade(packages...)
	return nodeToStarlark(node), nil
}

func (d *DarwinPlanBindings) packageRemoveBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
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
	node := d.PackageRemove(packages...)
	return nodeToStarlark(node), nil
}

func (d *DarwinPlanBindings) packageUpdateBuiltin(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	node := d.PackageUpdate()
	return nodeToStarlark(node), nil
}

func (d *DarwinPlanBindings) configureBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var source, target string
	if err := starlark.UnpackArgs("configure", args, kwargs, "source", &source, "target", &target); err != nil {
		return nil, err
	}
	node := d.Configure(source, target)
	return nodeToStarlark(node), nil
}

func (d *DarwinPlanBindings) linkBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var source, target string
	if err := starlark.UnpackArgs("link", args, kwargs, "source", &source, "target", &target); err != nil {
		return nil, err
	}
	node := d.Link(source, target)
	return nodeToStarlark(node), nil
}

func (d *DarwinPlanBindings) copyBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var source, target string
	if err := starlark.UnpackArgs("copy", args, kwargs, "source", &source, "target", &target); err != nil {
		return nil, err
	}
	node := d.Copy(source, target)
	return nodeToStarlark(node), nil
}

func (d *DarwinPlanBindings) mkdirBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var target string
	if err := starlark.UnpackArgs("mkdir", args, kwargs, "target", &target); err != nil {
		return nil, err
	}
	node := d.Mkdir(target)
	return nodeToStarlark(node), nil
}

func (d *DarwinPlanBindings) serviceBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
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

	node := d.Service(name, serviceAction)
	return nodeToStarlark(node), nil
}

func (d *DarwinPlanBindings) shellBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var command string
	if err := starlark.UnpackArgs("shell", args, kwargs, "command", &command); err != nil {
		return nil, err
	}
	node := d.Shell(command)
	return nodeToStarlark(node), nil
}

func (d *DarwinPlanBindings) dependsOnBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
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

	d.graph.Edges = append(d.graph.Edges, engine.Edge{
		From:     toIDStr,
		To:       fromIDStr,
		Relation: "depends_on",
	})

	return starlark.None, nil
}

// nodeToStarlark converts an engine.Node to a Starlark struct.
func nodeToStarlark(node *engine.Node) starlark.Value {
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
