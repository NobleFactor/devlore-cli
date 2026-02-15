//go:build darwin

// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package platform

import (
	"fmt"
	"strings"
	"sync/atomic"

	"go.starlark.net/starlark"

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

// ToStarlark converts the plan bindings to a Starlark receiver.
func (d *DarwinPlanBindings) ToStarlark() starlark.Value {
	return &darwinPlanReceiver{
		Receiver: loreStar.NewReceiver("plan"),
		d:        d,
		pkg:      &darwinPackageReceiver{Receiver: loreStar.NewReceiver("plan.package"), d: d},
		file:     &darwinFileReceiver{Receiver: loreStar.NewReceiver("plan.file"), d: d},
	}
}

type darwinPlanReceiver struct {
	loreStar.Receiver
	d    *DarwinPlanBindings
	pkg  *darwinPackageReceiver
	file *darwinFileReceiver
}

func (r *darwinPlanReceiver) Attr(name string) (starlark.Value, error) {
	switch name {
	case "file":
		return r.file, nil
	case "package":
		return r.pkg, nil
	case "gather":
		return loreStar.MakeAttr("plan.gather", r.d.gatherBuiltin), nil
	case "service":
		return loreStar.MakeAttr("plan.service", r.d.serviceBuiltin), nil
	case "shell":
		return loreStar.MakeAttr("plan.shell", r.d.shellBuiltin), nil
	default:
		return nil, loreStar.NoSuchAttrError("plan", name)
	}
}

func (r *darwinPlanReceiver) AttrNames() []string {
	return []string{"file", "gather", "package", "service", "shell"}
}

type darwinPackageReceiver struct {
	loreStar.Receiver
	d *DarwinPlanBindings
}

func (r *darwinPackageReceiver) Attr(name string) (starlark.Value, error) {
	switch name {
	case "install":
		return loreStar.MakeAttr("plan.package.install", r.d.packageInstallBuiltin), nil
	case "remove":
		return loreStar.MakeAttr("plan.package.remove", r.d.packageRemoveBuiltin), nil
	case "update":
		return loreStar.MakeAttr("plan.package.update", r.d.packageUpdateBuiltin), nil
	case "upgrade":
		return loreStar.MakeAttr("plan.package.upgrade", r.d.packageUpgradeBuiltin), nil
	default:
		return nil, loreStar.NoSuchAttrError("plan.package", name)
	}
}

func (r *darwinPackageReceiver) AttrNames() []string {
	return []string{"install", "remove", "update", "upgrade"}
}

type darwinFileReceiver struct {
	loreStar.Receiver
	d *DarwinPlanBindings
}

func (r *darwinFileReceiver) Attr(name string) (starlark.Value, error) {
	switch name {
	case "configure":
		return loreStar.MakeAttr("plan.file.configure", r.d.configureBuiltin), nil
	case "copy":
		return loreStar.MakeAttr("plan.file.copy", r.d.copyBuiltin), nil
	case "link":
		return loreStar.MakeAttr("plan.file.link", r.d.linkBuiltin), nil
	case "write":
		return loreStar.MakeAttr("plan.file.write", r.d.writeBuiltin), nil
	default:
		return nil, loreStar.NoSuchAttrError("plan.file", name)
	}
}

func (r *darwinFileReceiver) AttrNames() []string {
	return []string{"configure", "copy", "link", "write"}
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
		Action: "package-install",
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
		Action: "package-upgrade",
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
		Action: "package-remove",
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
		Action: "package-update",
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

	renderNode := &execution.Node{
		ID:        darwinGenerateNodeID("render"),
		Action: "render",
		Project:   d.project,
	}
	if err := loreStar.FillSlot(renderNode, d.graph, "source", source); err != nil {
		return nil, fmt.Errorf("configure: source: %w", err)
	}
	d.graph.Nodes = append(d.graph.Nodes, renderNode)

	copyNode := &execution.Node{
		ID:        darwinGenerateNodeID("configure"),
		Action: "copy",
		Project:   d.project,
	}
	if err := loreStar.FillSlot(copyNode, d.graph, "path", path); err != nil {
		return nil, fmt.Errorf("configure: path: %w", err)
	}
	d.graph.Nodes = append(d.graph.Nodes, copyNode)

	d.graph.Edges = append(d.graph.Edges, execution.Edge{
		From: renderNode.ID,
		To:   copyNode.ID,
	})

	return loreStar.NewOutput(copyNode, d.graph, ""), nil
}

func (d *DarwinPlanBindings) linkBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var source, path starlark.Value
	if err := starlark.UnpackArgs("link", args, kwargs, "source", &source, "path", &path); err != nil {
		return nil, err
	}

	node := &execution.Node{
		ID:         darwinGenerateNodeID("link"),
		Action: "link",
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
		Action: "copy",
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
		Action: "write",
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
		Action: "service-" + actionStr,
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
		Action: "shell",
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
		Action: "package-install",
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
		Action: "package-upgrade",
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
		Action: "package-remove",
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
		Action: "package-update",
		Project:    d.project,
	}
	d.graph.Nodes = append(d.graph.Nodes, node)
	return node
}

func (d *DarwinPlanBindings) Configure(source, target string) *execution.Node {
	renderNode := &execution.Node{
		ID:        darwinGenerateNodeID("render"),
		Action: "render",
		Project:   d.project,
	}
	renderNode.SetSlotImmediate("source", source)
	d.graph.Nodes = append(d.graph.Nodes, renderNode)

	copyNode := &execution.Node{
		ID:        darwinGenerateNodeID("configure"),
		Action: "copy",
		Project:   d.project,
	}
	copyNode.SetSlotImmediate("path", d.host.ExpandPath(target))
	d.graph.Nodes = append(d.graph.Nodes, copyNode)

	d.graph.Edges = append(d.graph.Edges, execution.Edge{
		From: renderNode.ID,
		To:   copyNode.ID,
	})

	return copyNode
}

func (d *DarwinPlanBindings) Link(source, target string) *execution.Node {
	node := &execution.Node{
		ID:         darwinGenerateNodeID("link"),
		Action: "link",
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
		Action: "copy",
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
		Action: "write",
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
		Action: "service-" + action.String(),
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
		Action: "shell",
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
