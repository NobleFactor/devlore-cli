//go:build linux

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
	loreStar "github.com/NobleFactor/devlore-cli/internal/starlark"
)

// linuxNodeCounter provides unique node IDs for Linux plan bindings.
var linuxNodeCounter uint64

func linuxGenerateNodeID(prefix string, components ...string) string {
	id := atomic.AddUint64(&linuxNodeCounter, 1)
	if len(components) > 0 {
		return fmt.Sprintf("%s-%s-%d", prefix, strings.Join(components, "-"), id)
	}
	return fmt.Sprintf("%s-%d", prefix, id)
}

// packageManagerType identifies the Linux package manager family.
type packageManagerType int

const (
	pmAPT    packageManagerType = iota // Debian, Ubuntu
	pmDNF                              // Fedora, RHEL, CentOS, Rocky, Alma
	pmPacman                           // Arch, Manjaro
	pmZypper                           // openSUSE
)

// LinuxPlanBindings provides Linux-specific plan bindings.
// Uses runtime distro detection to select the appropriate package manager.
type LinuxPlanBindings struct {
	*basePlanBindings
	pmType packageManagerType
	pmName string
	distro string
}

// NewPlanBindings creates a new Linux-specific PlanBindings.
// The appropriate package manager is selected based on the detected distro.
func NewPlanBindings(graph *execution.Graph, h host.Host, project string) PlatformPlanBindings {
	p := h.Platform()
	distro := strings.ToLower(p.Distro)

	var pmType packageManagerType
	var pmName string

	switch distro {
	case "debian", "ubuntu", "linuxmint", "pop":
		pmType = pmAPT
		pmName = "apt"
	case "fedora", "rhel", "centos", "rocky", "almalinux", "oracle":
		pmType = pmDNF
		pmName = "dnf"
	case "arch", "manjaro", "endeavouros":
		pmType = pmPacman
		pmName = "pacman"
	case "opensuse", "suse":
		pmType = pmZypper
		pmName = "zypper"
	default:
		// Default to apt for unknown distros
		pmType = pmAPT
		pmName = "apt"
	}

	return &LinuxPlanBindings{
		basePlanBindings: newBasePlanBindings(graph, h, project),
		pmType:           pmType,
		pmName:           pmName,
		distro:           distro,
	}
}

// PlatformName returns "linux".
func (l *LinuxPlanBindings) PlatformName() string {
	return "linux"
}

// PackageManagerName returns the detected package manager name.
func (l *LinuxPlanBindings) PackageManagerName() string {
	return l.pmName
}

// PackageInstall adds a package installation node using the platform's package manager.
func (l *LinuxPlanBindings) PackageInstall(packages ...string) *execution.Node {
	node := &execution.Node{
		ID:         linuxGenerateNodeID("package-install", packages...),
		Action: "package-install",
		Project:    l.project,
	}
	node.SetSlotImmediate("packages", strings.Join(packages, ","))
	l.graph.Nodes = append(l.graph.Nodes, node)
	return node
}

// PackageUpgrade adds a package upgrade node using the platform's package manager.
func (l *LinuxPlanBindings) PackageUpgrade(packages ...string) *execution.Node {
	node := &execution.Node{
		ID:         linuxGenerateNodeID("package-upgrade", packages...),
		Action: "package-upgrade",
		Project:    l.project,
	}
	node.SetSlotImmediate("packages", strings.Join(packages, ","))
	l.graph.Nodes = append(l.graph.Nodes, node)
	return node
}

// PackageRemove adds a package removal node using the platform's package manager.
func (l *LinuxPlanBindings) PackageRemove(packages ...string) *execution.Node {
	node := &execution.Node{
		ID:         linuxGenerateNodeID("package-remove", packages...),
		Action: "package-remove",
		Project:    l.project,
	}
	node.SetSlotImmediate("packages", strings.Join(packages, ","))
	l.graph.Nodes = append(l.graph.Nodes, node)
	return node
}

// PackageUpdate adds a package index update node using the platform's package manager.
func (l *LinuxPlanBindings) PackageUpdate() *execution.Node {
	node := &execution.Node{
		ID:         linuxGenerateNodeID("package-update"),
		Action: "package-update",
		Project:    l.project,
	}
	l.graph.Nodes = append(l.graph.Nodes, node)
	return node
}

// Configure adds a configuration file node (render→copy chain).
func (l *LinuxPlanBindings) Configure(source, target string) *execution.Node {
	renderNode := &execution.Node{
		ID:        linuxGenerateNodeID("render"),
		Action: "render",
		Project:   l.project,
	}
	renderNode.SetSlotImmediate("source", source)
	l.graph.Nodes = append(l.graph.Nodes, renderNode)

	copyNode := &execution.Node{
		ID:        linuxGenerateNodeID("configure"),
		Action: "copy",
		Project:   l.project,
	}
	copyNode.SetSlotImmediate("path", l.host.ExpandPath(target))
	l.graph.Nodes = append(l.graph.Nodes, copyNode)

	l.graph.Edges = append(l.graph.Edges, execution.Edge{
		From: renderNode.ID,
		To:   copyNode.ID,
	})

	return copyNode
}

// Link adds a symlink creation node.
func (l *LinuxPlanBindings) Link(source, target string) *execution.Node {
	node := &execution.Node{
		ID:         linuxGenerateNodeID("link"),
		Action: "link",
		Project:    l.project,
	}
	node.SetSlotImmediate("source", source)
	node.SetSlotImmediate("path", l.host.ExpandPath(target))
	l.graph.Nodes = append(l.graph.Nodes, node)
	return node
}

// Copy adds a file copy node.
func (l *LinuxPlanBindings) Copy(source, target string) *execution.Node {
	node := &execution.Node{
		ID:         linuxGenerateNodeID("copy"),
		Action: "copy",
		Project:    l.project,
	}
	node.SetSlotImmediate("source", source)
	node.SetSlotImmediate("path", l.host.ExpandPath(target))
	l.graph.Nodes = append(l.graph.Nodes, node)
	return node
}

// Write adds a file write node (write content directly to target).
func (l *LinuxPlanBindings) Write(target, content string) *execution.Node {
	node := &execution.Node{
		ID:         linuxGenerateNodeID("write"),
		Action: "write",
		Project:    l.project,
	}
	node.SetSlotImmediate("content", content)
	node.SetSlotImmediate("path", l.host.ExpandPath(target))
	l.graph.Nodes = append(l.graph.Nodes, node)
	return node
}

// Service adds a systemd service management node.
func (l *LinuxPlanBindings) Service(name string, action loreStar.ServiceAction) *execution.Node {
	node := &execution.Node{
		ID:         linuxGenerateNodeID("systemd", name, action.String()),
		Action: "service-" + action.String(),
		Project:    l.project,
	}
	node.SetSlotImmediate("name", name)
	node.SetSlotImmediate("action", action.String())
	l.graph.Nodes = append(l.graph.Nodes, node)
	return node
}

// Shell adds a shell command execution node.
func (l *LinuxPlanBindings) Shell(command string) *execution.Node {
	node := &execution.Node{
		ID:         linuxGenerateNodeID("shell"),
		Action: "shell",
		Project:    l.project,
	}
	node.SetSlotImmediate("command", command)
	l.graph.Nodes = append(l.graph.Nodes, node)
	return node
}

// DependsOn creates a dependency edge between nodes.
func (l *LinuxPlanBindings) DependsOn(from, to *execution.Node) {
	l.graph.Edges = append(l.graph.Edges, execution.Edge{
		From: to.ID,
		To:   from.ID,
	})
}

// ToStarlark converts the plan bindings to a Starlark receiver.
func (l *LinuxPlanBindings) ToStarlark() starlark.Value {
	return &linuxPlanReceiver{
		Receiver: loreStar.NewReceiver("plan"),
		l:        l,
		pkg:      &linuxPackageReceiver{Receiver: loreStar.NewReceiver("plan.package"), l: l},
		file:     &linuxFileReceiver{Receiver: loreStar.NewReceiver("plan.file"), l: l},
	}
}

type linuxPlanReceiver struct {
	loreStar.Receiver
	l    *LinuxPlanBindings
	pkg  *linuxPackageReceiver
	file *linuxFileReceiver
}

func (r *linuxPlanReceiver) Attr(name string) (starlark.Value, error) {
	switch name {
	case "file":
		return r.file, nil
	case "package":
		return r.pkg, nil
	case "gather":
		return loreStar.MakeAttr("plan.gather", r.l.gatherBuiltin), nil
	case "service":
		return loreStar.MakeAttr("plan.service", r.l.serviceBuiltin), nil
	case "shell":
		return loreStar.MakeAttr("plan.shell", r.l.shellBuiltin), nil
	case "distro":
		return starlark.String(r.l.distro), nil
	default:
		return nil, loreStar.NoSuchAttrError("plan", name)
	}
}

func (r *linuxPlanReceiver) AttrNames() []string {
	return []string{"distro", "file", "gather", "package", "service", "shell"}
}

type linuxPackageReceiver struct {
	loreStar.Receiver
	l *LinuxPlanBindings
}

func (r *linuxPackageReceiver) Attr(name string) (starlark.Value, error) {
	switch name {
	case "install":
		return loreStar.MakeAttr("plan.package.install", r.l.packageInstallBuiltin), nil
	case "upgrade":
		return loreStar.MakeAttr("plan.package.upgrade", r.l.packageUpgradeBuiltin), nil
	case "remove":
		return loreStar.MakeAttr("plan.package.remove", r.l.packageRemoveBuiltin), nil
	case "update":
		return loreStar.MakeAttr("plan.package.update", r.l.packageUpdateBuiltin), nil
	case "manager":
		return starlark.String(r.l.pmName), nil
	default:
		return nil, loreStar.NoSuchAttrError("plan.package", name)
	}
}

func (r *linuxPackageReceiver) AttrNames() []string {
	return []string{"install", "manager", "remove", "update", "upgrade"}
}

type linuxFileReceiver struct {
	loreStar.Receiver
	l *LinuxPlanBindings
}

func (r *linuxFileReceiver) Attr(name string) (starlark.Value, error) {
	switch name {
	case "configure":
		return loreStar.MakeAttr("plan.file.configure", r.l.configureBuiltin), nil
	case "copy":
		return loreStar.MakeAttr("plan.file.copy", r.l.copyBuiltin), nil
	case "link":
		return loreStar.MakeAttr("plan.file.link", r.l.linkBuiltin), nil
	case "write":
		return loreStar.MakeAttr("plan.file.write", r.l.writeBuiltin), nil
	default:
		return nil, loreStar.NoSuchAttrError("plan.file", name)
	}
}

func (r *linuxFileReceiver) AttrNames() []string {
	return []string{"configure", "copy", "link", "write"}
}

func (l *LinuxPlanBindings) packageInstallBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
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
	node := l.PackageInstall(packages...)
	packagesStr := strings.Join(packages, ",")
	return loreStar.NewOutput(node, l.graph, packagesStr), nil
}

func (l *LinuxPlanBindings) packageUpgradeBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
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
	node := l.PackageUpgrade(packages...)
	packagesStr := strings.Join(packages, ",")
	return loreStar.NewOutput(node, l.graph, packagesStr), nil
}

func (l *LinuxPlanBindings) packageRemoveBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
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
	_ = l.PackageRemove(packages...)
	// Remove produces no output
	return starlark.None, nil
}

func (l *LinuxPlanBindings) packageUpdateBuiltin(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	node := l.PackageUpdate()
	return loreStar.NewOutput(node, l.graph, "<index>"), nil
}

func (l *LinuxPlanBindings) configureBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var inputArg starlark.Value
	var out string
	if err := starlark.UnpackArgs("configure", args, kwargs, "input", &inputArg, "out", &out); err != nil {
		return nil, err
	}
	input, err := loreStar.ResolveInput(inputArg)
	if err != nil {
		return nil, fmt.Errorf("configure: input: %w", err)
	}
	node := l.Configure(input.Path(), out)
	input.DependOn(node)
	return loreStar.NewOutput(node, l.graph, node.GetSlot("path").(string)), nil
}

func (l *LinuxPlanBindings) linkBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var inputArg starlark.Value
	var out string
	if err := starlark.UnpackArgs("link", args, kwargs, "input", &inputArg, "out", &out); err != nil {
		return nil, err
	}
	input, err := loreStar.ResolveInput(inputArg)
	if err != nil {
		return nil, fmt.Errorf("link: input: %w", err)
	}
	node := l.Link(input.Path(), out)
	input.DependOn(node)
	return loreStar.NewOutput(node, l.graph, node.GetSlot("path").(string)), nil
}

func (l *LinuxPlanBindings) copyBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var inputArg starlark.Value
	var out string
	if err := starlark.UnpackArgs("copy", args, kwargs, "input", &inputArg, "out", &out); err != nil {
		return nil, err
	}
	input, err := loreStar.ResolveInput(inputArg)
	if err != nil {
		return nil, fmt.Errorf("copy: input: %w", err)
	}
	node := l.Copy(input.Path(), out)
	input.DependOn(node)
	return loreStar.NewOutput(node, l.graph, node.GetSlot("path").(string)), nil
}

func (l *LinuxPlanBindings) writeBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var inputArg starlark.Value
	var out string
	if err := starlark.UnpackArgs("write", args, kwargs, "input", &inputArg, "out", &out); err != nil {
		return nil, err
	}
	input, err := loreStar.ResolveInput(inputArg)
	if err != nil {
		return nil, fmt.Errorf("write: input: %w", err)
	}
	// Create write node directly - content comes from input artifact via edge
	expandedOut := l.host.ExpandPath(out)
	node := &execution.Node{
		ID:         linuxGenerateNodeID("write"),
		Action: "write",
		Project:    l.project,
	}
	node.SetSlotImmediate("source", input.Path())
	node.SetSlotImmediate("path", expandedOut)
	l.graph.Nodes = append(l.graph.Nodes, node)
	input.DependOn(node)
	return loreStar.NewOutput(node, l.graph, expandedOut), nil
}

func (l *LinuxPlanBindings) serviceBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
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

	node := l.Service(name, serviceAction)
	return loreStar.NewOutput(node, l.graph, name), nil
}

func (l *LinuxPlanBindings) shellBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var command string
	if err := starlark.UnpackArgs("shell", args, kwargs, "command", &command); err != nil {
		return nil, err
	}
	node := l.Shell(command)
	return loreStar.NewOutput(node, l.graph, command), nil
}

func (l *LinuxPlanBindings) gatherBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
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

	return loreStar.NewGather(l.graph, outputs...), nil
}
