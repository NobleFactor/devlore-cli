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

	"github.com/NobleFactor/devlore-cli/internal/execution"
	"github.com/NobleFactor/devlore-cli/internal/host"
	"github.com/NobleFactor/devlore-cli/internal/lorepackage"
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

// DarwinPlanBindings provides macOS-specific plan bindings using the slot-based model.
// Uses Homebrew for package management and launchd for services.
type DarwinPlanBindings struct {
	*basePlanBindings
}

// NewPlanBindings creates a new Darwin-specific PlanBindings.
func NewPlanBindings(graph *execution.Graph, h host.Host, project string) PlatformPlanBindings {
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

// ToStarlark converts the plan bindings to a Starlark struct.
func (d *DarwinPlanBindings) ToStarlark() starlark.Value {
	packageOps := starlarkstruct.FromStringDict(starlark.String("package"), starlark.StringDict{
		"install": starlark.NewBuiltin("install", d.packageInstallBuiltin),
		"remove":  starlark.NewBuiltin("remove", d.packageRemoveBuiltin),
		"update":  starlark.NewBuiltin("update", d.packageUpdateBuiltin),
		"upgrade": starlark.NewBuiltin("upgrade", d.packageUpgradeBuiltin),
	})

	fileOps := starlarkstruct.FromStringDict(starlark.String("file"), starlark.StringDict{
		"configure": starlark.NewBuiltin("configure", d.configureBuiltin),
		"copy":      starlark.NewBuiltin("copy", d.copyBuiltin),
		"link":      starlark.NewBuiltin("link", d.linkBuiltin),
		"write":     starlark.NewBuiltin("write", d.writeBuiltin),
	})

	return starlarkstruct.FromStringDict(starlark.String("plan"), starlark.StringDict{
		"file":    fileOps,
		"package": packageOps,
		"gather":  starlark.NewBuiltin("gather", d.gatherBuiltin),
		"service": starlark.NewBuiltin("service", d.serviceBuiltin),
		"shell":   starlark.NewBuiltin("shell", d.shellBuiltin),
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

	cleanPkgs, manager, isCask := parsePackagesWithPrefix(packages)
	node := &execution.Node{
		ID:         darwinGenerateNodeID("package-install", cleanPkgs...),
		Operations: []string{"package-install"},
		Project:    d.project,
	}
	node.SetSlotImmediate("packages", strings.Join(cleanPkgs, ","))
	if manager != "" {
		node.SetSlotImmediate("manager", manager)
	}
	if isCask {
		node.SetSlotImmediate("cask", "true")
	}

	d.graph.Nodes = append(d.graph.Nodes, node)
	return loreStar.NewOutput(node, d.graph, ""), nil
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

	cleanPkgs, manager, isCask := parsePackagesWithPrefix(packages)
	node := &execution.Node{
		ID:         darwinGenerateNodeID("package-upgrade", cleanPkgs...),
		Operations: []string{"package-upgrade"},
		Project:    d.project,
	}
	node.SetSlotImmediate("packages", strings.Join(cleanPkgs, ","))
	if manager != "" {
		node.SetSlotImmediate("manager", manager)
	}
	if isCask {
		node.SetSlotImmediate("cask", "true")
	}

	d.graph.Nodes = append(d.graph.Nodes, node)
	return loreStar.NewOutput(node, d.graph, ""), nil
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

	cleanPkgs, manager, isCask := parsePackagesWithPrefix(packages)
	node := &execution.Node{
		ID:         darwinGenerateNodeID("package-remove", cleanPkgs...),
		Operations: []string{"package-remove"},
		Project:    d.project,
	}
	node.SetSlotImmediate("packages", strings.Join(cleanPkgs, ","))
	if manager != "" {
		node.SetSlotImmediate("manager", manager)
	}
	if isCask {
		node.SetSlotImmediate("cask", "true")
	}

	d.graph.Nodes = append(d.graph.Nodes, node)
	return starlark.None, nil
}

func (d *DarwinPlanBindings) packageUpdateBuiltin(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	node := &execution.Node{
		ID:         darwinGenerateNodeID("package-update"),
		Operations: []string{"package-update"},
		Project:    d.project,
	}
	d.graph.Nodes = append(d.graph.Nodes, node)
	return loreStar.NewOutput(node, d.graph, ""), nil
}

func (d *DarwinPlanBindings) configureBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var source, path starlark.Value
	if err := starlark.UnpackArgs("configure", args, kwargs, "source", &source, "path", &path); err != nil {
		return nil, err
	}

	node := &execution.Node{
		ID:         darwinGenerateNodeID("configure"),
		Operations: []string{"render", "copy"},
		Project:    d.project,
	}

	if err := loreStar.FillSlot(node, d.graph, "source", source); err != nil {
		return nil, fmt.Errorf("configure: source: %w", err)
	}
	if err := loreStar.FillSlot(node, d.graph, "path", path); err != nil {
		return nil, fmt.Errorf("configure: path: %w", err)
	}

	d.graph.Nodes = append(d.graph.Nodes, node)
	return loreStar.NewOutput(node, d.graph, ""), nil
}

func (d *DarwinPlanBindings) linkBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var source, path starlark.Value
	if err := starlark.UnpackArgs("link", args, kwargs, "source", &source, "path", &path); err != nil {
		return nil, err
	}

	node := &execution.Node{
		ID:         darwinGenerateNodeID("link"),
		Operations: []string{"link"},
		Project:    d.project,
	}

	if err := loreStar.FillSlot(node, d.graph, "source", source); err != nil {
		return nil, fmt.Errorf("link: source: %w", err)
	}
	if err := loreStar.FillSlot(node, d.graph, "path", path); err != nil {
		return nil, fmt.Errorf("link: path: %w", err)
	}

	d.graph.Nodes = append(d.graph.Nodes, node)
	return loreStar.NewOutput(node, d.graph, ""), nil
}

func (d *DarwinPlanBindings) copyBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var source, path starlark.Value
	if err := starlark.UnpackArgs("copy", args, kwargs, "source", &source, "path", &path); err != nil {
		return nil, err
	}

	node := &execution.Node{
		ID:         darwinGenerateNodeID("copy"),
		Operations: []string{"copy"},
		Project:    d.project,
	}

	if err := loreStar.FillSlot(node, d.graph, "source", source); err != nil {
		return nil, fmt.Errorf("copy: source: %w", err)
	}
	if err := loreStar.FillSlot(node, d.graph, "path", path); err != nil {
		return nil, fmt.Errorf("copy: path: %w", err)
	}

	d.graph.Nodes = append(d.graph.Nodes, node)
	return loreStar.NewOutput(node, d.graph, ""), nil
}

func (d *DarwinPlanBindings) writeBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var content, path starlark.Value
	if err := starlark.UnpackArgs("write", args, kwargs, "content", &content, "path", &path); err != nil {
		return nil, err
	}

	node := &execution.Node{
		ID:         darwinGenerateNodeID("write"),
		Operations: []string{"write"},
		Project:    d.project,
	}

	if err := loreStar.FillSlot(node, d.graph, "content", content); err != nil {
		return nil, fmt.Errorf("write: content: %w", err)
	}
	if err := loreStar.FillSlot(node, d.graph, "path", path); err != nil {
		return nil, fmt.Errorf("write: path: %w", err)
	}

	d.graph.Nodes = append(d.graph.Nodes, node)
	return loreStar.NewOutput(node, d.graph, ""), nil
}

func (d *DarwinPlanBindings) serviceBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name, action starlark.Value
	if err := starlark.UnpackArgs("service", args, kwargs, "name", &name, "action", &action); err != nil {
		return nil, err
	}

	// Validate action
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
		ID:         darwinGenerateNodeID("launchd"),
		Operations: []string{"launchd-" + actionStr},
		Project:    d.project,
	}

	if err := loreStar.FillSlot(node, d.graph, "name", name); err != nil {
		return nil, fmt.Errorf("service: name: %w", err)
	}
	if err := loreStar.FillSlot(node, d.graph, "action", action); err != nil {
		return nil, fmt.Errorf("service: action: %w", err)
	}

	d.graph.Nodes = append(d.graph.Nodes, node)
	return loreStar.NewOutput(node, d.graph, ""), nil
}

func (d *DarwinPlanBindings) shellBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var command starlark.Value
	if err := starlark.UnpackArgs("shell", args, kwargs, "command", &command); err != nil {
		return nil, err
	}

	node := &execution.Node{
		ID:         darwinGenerateNodeID("shell"),
		Operations: []string{"shell"},
		Project:    d.project,
	}

	if err := loreStar.FillSlot(node, d.graph, "command", command); err != nil {
		return nil, fmt.Errorf("shell: command: %w", err)
	}

	d.graph.Nodes = append(d.graph.Nodes, node)
	return loreStar.NewOutput(node, d.graph, ""), nil
}

func (d *DarwinPlanBindings) gatherBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
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

	return loreStar.NewGather(d.graph, outputs...), nil
}

// Go API methods for internal use (not exposed to Starlark scripts)

func (d *DarwinPlanBindings) PackageInstall(packages ...string) *execution.Node {
	cleanPkgs, manager, isCask := parsePackagesWithPrefix(packages)
	node := &execution.Node{
		ID:         darwinGenerateNodeID("package-install", cleanPkgs...),
		Operations: []string{"package-install"},
		Project:    d.project,
	}
	node.SetSlotImmediate("packages", strings.Join(cleanPkgs, ","))
	if manager != "" {
		node.SetSlotImmediate("manager", manager)
	}
	if isCask {
		node.SetSlotImmediate("cask", "true")
	}
	d.graph.Nodes = append(d.graph.Nodes, node)
	return node
}

func (d *DarwinPlanBindings) PackageUpgrade(packages ...string) *execution.Node {
	cleanPkgs, manager, isCask := parsePackagesWithPrefix(packages)
	node := &execution.Node{
		ID:         darwinGenerateNodeID("package-upgrade", cleanPkgs...),
		Operations: []string{"package-upgrade"},
		Project:    d.project,
	}
	node.SetSlotImmediate("packages", strings.Join(cleanPkgs, ","))
	if manager != "" {
		node.SetSlotImmediate("manager", manager)
	}
	if isCask {
		node.SetSlotImmediate("cask", "true")
	}
	d.graph.Nodes = append(d.graph.Nodes, node)
	return node
}

func (d *DarwinPlanBindings) PackageRemove(packages ...string) *execution.Node {
	cleanPkgs, manager, isCask := parsePackagesWithPrefix(packages)
	node := &execution.Node{
		ID:         darwinGenerateNodeID("package-remove", cleanPkgs...),
		Operations: []string{"package-remove"},
		Project:    d.project,
	}
	node.SetSlotImmediate("packages", strings.Join(cleanPkgs, ","))
	if manager != "" {
		node.SetSlotImmediate("manager", manager)
	}
	if isCask {
		node.SetSlotImmediate("cask", "true")
	}
	d.graph.Nodes = append(d.graph.Nodes, node)
	return node
}

func (d *DarwinPlanBindings) PackageUpdate() *execution.Node {
	node := &execution.Node{
		ID:         darwinGenerateNodeID("package-update"),
		Operations: []string{"package-update"},
		Project:    d.project,
	}
	d.graph.Nodes = append(d.graph.Nodes, node)
	return node
}

func (d *DarwinPlanBindings) Configure(source, target string) *execution.Node {
	node := &execution.Node{
		ID:         darwinGenerateNodeID("configure"),
		Operations: []string{"render", "copy"},
		Project:    d.project,
	}
	node.SetSlotImmediate("source", source)
	node.SetSlotImmediate("path", d.host.ExpandPath(target))
	d.graph.Nodes = append(d.graph.Nodes, node)
	return node
}

func (d *DarwinPlanBindings) Link(source, target string) *execution.Node {
	node := &execution.Node{
		ID:         darwinGenerateNodeID("link"),
		Operations: []string{"link"},
		Project:    d.project,
	}
	node.SetSlotImmediate("source", source)
	node.SetSlotImmediate("path", d.host.ExpandPath(target))
	d.graph.Nodes = append(d.graph.Nodes, node)
	return node
}

func (d *DarwinPlanBindings) Copy(source, target string) *execution.Node {
	node := &execution.Node{
		ID:         darwinGenerateNodeID("copy"),
		Operations: []string{"copy"},
		Project:    d.project,
	}
	node.SetSlotImmediate("source", source)
	node.SetSlotImmediate("path", d.host.ExpandPath(target))
	d.graph.Nodes = append(d.graph.Nodes, node)
	return node
}

func (d *DarwinPlanBindings) Write(target, content string) *execution.Node {
	node := &execution.Node{
		ID:         darwinGenerateNodeID("write"),
		Operations: []string{"write"},
		Project:    d.project,
	}
	node.SetSlotImmediate("content", content)
	node.SetSlotImmediate("path", d.host.ExpandPath(target))
	d.graph.Nodes = append(d.graph.Nodes, node)
	return node
}

func (d *DarwinPlanBindings) Service(name string, action loreStar.ServiceAction) *execution.Node {
	node := &execution.Node{
		ID:         darwinGenerateNodeID("launchd", name, action.String()),
		Operations: []string{"launchd-" + action.String()},
		Project:    d.project,
	}
	node.SetSlotImmediate("name", name)
	node.SetSlotImmediate("action", action.String())
	d.graph.Nodes = append(d.graph.Nodes, node)
	return node
}

func (d *DarwinPlanBindings) Shell(command string) *execution.Node {
	node := &execution.Node{
		ID:         darwinGenerateNodeID("shell"),
		Operations: []string{"shell"},
		Project:    d.project,
	}
	node.SetSlotImmediate("command", command)
	d.graph.Nodes = append(d.graph.Nodes, node)
	return node
}

func (d *DarwinPlanBindings) DependsOn(from, to *execution.Node) {
	d.graph.Edges = append(d.graph.Edges, execution.Edge{
		From: to.ID,
		To:   from.ID,
	})
}
