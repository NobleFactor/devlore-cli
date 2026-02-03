//go:build linux

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
		Operations: []string{"package-install"},
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
		Operations: []string{"package-upgrade"},
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
		Operations: []string{"package-remove"},
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
		Operations: []string{"package-update"},
		Project:    l.project,
	}
	l.graph.Nodes = append(l.graph.Nodes, node)
	return node
}

// Configure adds a configuration file node.
func (l *LinuxPlanBindings) Configure(source, target string) *execution.Node {
	node := &execution.Node{
		ID:         linuxGenerateNodeID("configure"),
		Operations: []string{"render", "copy"},
		Project:    l.project,
	}
	node.SetSlotImmediate("source", source)
	node.SetSlotImmediate("path", l.host.ExpandPath(target))
	l.graph.Nodes = append(l.graph.Nodes, node)
	return node
}

// Link adds a symlink creation node.
func (l *LinuxPlanBindings) Link(source, target string) *execution.Node {
	node := &execution.Node{
		ID:         linuxGenerateNodeID("link"),
		Operations: []string{"link"},
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
		Operations: []string{"copy"},
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
		Operations: []string{"write"},
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
		Operations: []string{"systemd-" + action.String()},
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
		Operations: []string{"shell"},
		Project:    l.project,
	}
	node.SetSlotImmediate("command", command)
	l.graph.Nodes = append(l.graph.Nodes, node)
	return node
}

// DependsOn creates a dependency edge between nodes.
func (l *LinuxPlanBindings) DependsOn(from, to *execution.Node) {
	l.graph.Edges = append(l.graph.Edges, execution.Edge{
		From:     to.ID,
		To:       from.ID,
		Relation: "depends_on",
	})
}

// ToStarlark converts the plan bindings to a Starlark struct.
// Uses nested structs: plan.package.install(), plan.file.copy(), etc.
func (l *LinuxPlanBindings) ToStarlark() starlark.Value {
	// Package operations namespace: plan.package.*
	packageOps := starlarkstruct.FromStringDict(starlark.String("package"), starlark.StringDict{
		"install": starlark.NewBuiltin("install", l.packageInstallBuiltin),
		"upgrade": starlark.NewBuiltin("upgrade", l.packageUpgradeBuiltin),
		"remove":  starlark.NewBuiltin("remove", l.packageRemoveBuiltin),
		"update":  starlark.NewBuiltin("update", l.packageUpdateBuiltin),
		// Linux-specific: expose package manager info
		"manager": starlark.String(l.pmName),
	})

	// File operations namespace: plan.file.*
	fileOps := starlarkstruct.FromStringDict(starlark.String("file"), starlark.StringDict{
		"configure": starlark.NewBuiltin("configure", l.configureBuiltin),
		"copy":      starlark.NewBuiltin("copy", l.copyBuiltin),
		"link":      starlark.NewBuiltin("link", l.linkBuiltin),
		"write":     starlark.NewBuiltin("write", l.writeBuiltin),
	})

	return starlarkstruct.FromStringDict(starlark.String("plan"), starlark.StringDict{
		// Namespaces
		"file":    fileOps,
		"package": packageOps,
		// Global functions (at root of plan)
		"depends_on": starlark.NewBuiltin("depends_on", l.dependsOnBuiltin),
		"service":    starlark.NewBuiltin("service", l.serviceBuiltin),
		"shell":      starlark.NewBuiltin("shell", l.shellBuiltin),
		// Linux-specific: expose distro info at top level
		"distro": starlark.String(l.distro),
	})
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
	return loreStar.NewOutput(node, l.graph, packagesStr, loreStar.OutputPackage), nil
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
	return loreStar.NewOutput(node, l.graph, packagesStr, loreStar.OutputPackage), nil
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
	return loreStar.NewOutput(node, l.graph, "<index>", loreStar.OutputPackage), nil
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
	return loreStar.NewOutput(node, l.graph, node.GetSlot("path"), loreStar.OutputFile), nil
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
	return loreStar.NewOutput(node, l.graph, node.GetSlot("path"), loreStar.OutputSymlink), nil
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
	return loreStar.NewOutput(node, l.graph, node.GetSlot("path"), loreStar.OutputFile), nil
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
		Operations: []string{"write"},
		Project:    l.project,
	}
	node.SetSlotImmediate("source", input.Path())
	node.SetSlotImmediate("path", expandedOut)
	l.graph.Nodes = append(l.graph.Nodes, node)
	input.DependOn(node)
	return loreStar.NewOutput(node, l.graph, expandedOut, loreStar.OutputFile), nil
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
	return loreStar.NewOutput(node, l.graph, name, loreStar.OutputService), nil
}

func (l *LinuxPlanBindings) shellBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var command string
	if err := starlark.UnpackArgs("shell", args, kwargs, "command", &command); err != nil {
		return nil, err
	}
	node := l.Shell(command)
	return loreStar.NewOutput(node, l.graph, command, loreStar.OutputCommand), nil
}

func (l *LinuxPlanBindings) dependsOnBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("depends_on: expected 2 arguments, got %d", len(args))
	}

	// Extract node ID from first argument (consumer - depends ON the second)
	fromIDStr, err := linuxExtractNodeID(args[0], "first")
	if err != nil {
		return nil, fmt.Errorf("depends_on: %w", err)
	}

	// Extract node ID from second argument (producer - depended upon)
	toIDStr, err := linuxExtractNodeID(args[1], "second")
	if err != nil {
		return nil, fmt.Errorf("depends_on: %w", err)
	}

	// Create edge in the graph: consumer depends_on producer
	l.graph.Edges = append(l.graph.Edges, execution.Edge{
		From:     toIDStr,
		To:       fromIDStr,
		Relation: "depends_on",
	})

	return starlark.None, nil
}

// linuxExtractNodeID extracts the node ID from an argument that may be Output or struct.
func linuxExtractNodeID(arg starlark.Value, position string) (string, error) {
	// Check if it's an Output
	if output, ok := arg.(*loreStar.Output); ok {
		return output.Node().ID, nil
	}

	// Check if it's a struct with an id attribute
	if st, ok := arg.(*starlarkstruct.Struct); ok {
		idVal, err := st.Attr("id")
		if err != nil {
			return "", fmt.Errorf("%s argument has no id", position)
		}
		idStr, ok := starlark.AsString(idVal)
		if !ok {
			return "", fmt.Errorf("%s argument id is not a string", position)
		}
		return idStr, nil
	}

	return "", fmt.Errorf("%s argument must be an Output or node struct", position)
}
