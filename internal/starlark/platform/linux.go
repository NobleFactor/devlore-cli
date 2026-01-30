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
		Metadata: map[string]string{
			"packages": strings.Join(packages, ","),
		},
	}
	l.graph.Nodes = append(l.graph.Nodes, node)
	return node
}

// PackageUpgrade adds a package upgrade node using the platform's package manager.
func (l *LinuxPlanBindings) PackageUpgrade(packages ...string) *execution.Node {
	node := &execution.Node{
		ID:         linuxGenerateNodeID("package-upgrade", packages...),
		Operations: []string{"package-upgrade"},
		Project:    l.project,
		Metadata: map[string]string{
			"packages": strings.Join(packages, ","),
		},
	}
	l.graph.Nodes = append(l.graph.Nodes, node)
	return node
}

// PackageRemove adds a package removal node using the platform's package manager.
func (l *LinuxPlanBindings) PackageRemove(packages ...string) *execution.Node {
	node := &execution.Node{
		ID:         linuxGenerateNodeID("package-remove", packages...),
		Operations: []string{"package-remove"},
		Project:    l.project,
		Metadata: map[string]string{
			"packages": strings.Join(packages, ","),
		},
	}
	l.graph.Nodes = append(l.graph.Nodes, node)
	return node
}

// PackageUpdate adds a package index update node using the platform's package manager.
func (l *LinuxPlanBindings) PackageUpdate() *execution.Node {
	node := &execution.Node{
		ID:         linuxGenerateNodeID("package-update"),
		Operations: []string{"package-update"},
		Project:    l.project,
		Metadata:   map[string]string{},
	}
	l.graph.Nodes = append(l.graph.Nodes, node)
	return node
}

// Configure adds a configuration file node.
func (l *LinuxPlanBindings) Configure(source, target string) *execution.Node {
	node := &execution.Node{
		ID:         linuxGenerateNodeID("configure"),
		Operations: []string{"expand", "copy"},
		Source:     source,
		Target:     l.host.ExpandPath(target),
		Project:    l.project,
	}
	l.graph.Nodes = append(l.graph.Nodes, node)
	return node
}

// Link adds a symlink creation node.
func (l *LinuxPlanBindings) Link(source, target string) *execution.Node {
	node := &execution.Node{
		ID:         linuxGenerateNodeID("link"),
		Operations: []string{"link"},
		Source:     source,
		Target:     l.host.ExpandPath(target),
		Project:    l.project,
	}
	l.graph.Nodes = append(l.graph.Nodes, node)
	return node
}

// Copy adds a file copy node.
func (l *LinuxPlanBindings) Copy(source, target string) *execution.Node {
	node := &execution.Node{
		ID:         linuxGenerateNodeID("copy"),
		Operations: []string{"copy"},
		Source:     source,
		Target:     l.host.ExpandPath(target),
		Project:    l.project,
	}
	l.graph.Nodes = append(l.graph.Nodes, node)
	return node
}

// Mkdir adds a directory creation node.
func (l *LinuxPlanBindings) Mkdir(target string) *execution.Node {
	node := &execution.Node{
		ID:         linuxGenerateNodeID("mkdir"),
		Operations: []string{"mkdir"},
		Target:     l.host.ExpandPath(target),
		Project:    l.project,
	}
	l.graph.Nodes = append(l.graph.Nodes, node)
	return node
}

// Write adds a file write node (write content directly to target).
func (l *LinuxPlanBindings) Write(target, content string) *execution.Node {
	node := &execution.Node{
		ID:         linuxGenerateNodeID("write"),
		Operations: []string{"file-write"},
		Target:     l.host.ExpandPath(target),
		Project:    l.project,
		Metadata: map[string]string{
			"content": content,
		},
	}
	l.graph.Nodes = append(l.graph.Nodes, node)
	return node
}

// Service adds a systemd service management node.
func (l *LinuxPlanBindings) Service(name string, action loreStar.ServiceAction) *execution.Node {
	node := &execution.Node{
		ID:         linuxGenerateNodeID("systemd", name, action.String()),
		Operations: []string{"systemd-" + action.String()},
		Project:    l.project,
		Metadata: map[string]string{
			"service": name,
			"action":  action.String(),
		},
	}
	l.graph.Nodes = append(l.graph.Nodes, node)
	return node
}

// Shell adds a shell command execution node.
func (l *LinuxPlanBindings) Shell(command string) *execution.Node {
	node := &execution.Node{
		ID:         linuxGenerateNodeID("shell"),
		Operations: []string{"shell"},
		Project:    l.project,
		Metadata: map[string]string{
			"command": command,
		},
	}
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
		"mkdir":     starlark.NewBuiltin("mkdir", l.mkdirBuiltin),
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
	return linuxNodeToStarlark(node), nil
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
	return linuxNodeToStarlark(node), nil
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
	node := l.PackageRemove(packages...)
	return linuxNodeToStarlark(node), nil
}

func (l *LinuxPlanBindings) packageUpdateBuiltin(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	node := l.PackageUpdate()
	return linuxNodeToStarlark(node), nil
}

func (l *LinuxPlanBindings) configureBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var source, target string
	if err := starlark.UnpackArgs("configure", args, kwargs, "source", &source, "target", &target); err != nil {
		return nil, err
	}
	node := l.Configure(source, target)
	return linuxNodeToStarlark(node), nil
}

func (l *LinuxPlanBindings) linkBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var source, target string
	if err := starlark.UnpackArgs("link", args, kwargs, "source", &source, "target", &target); err != nil {
		return nil, err
	}
	node := l.Link(source, target)
	return linuxNodeToStarlark(node), nil
}

func (l *LinuxPlanBindings) copyBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var source, target string
	if err := starlark.UnpackArgs("copy", args, kwargs, "source", &source, "target", &target); err != nil {
		return nil, err
	}
	node := l.Copy(source, target)
	return linuxNodeToStarlark(node), nil
}

func (l *LinuxPlanBindings) mkdirBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var target string
	if err := starlark.UnpackArgs("mkdir", args, kwargs, "target", &target); err != nil {
		return nil, err
	}
	node := l.Mkdir(target)
	return linuxNodeToStarlark(node), nil
}

func (l *LinuxPlanBindings) writeBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var target, content string
	if err := starlark.UnpackArgs("write", args, kwargs, "target", &target, "content", &content); err != nil {
		return nil, err
	}
	node := l.Write(target, content)
	return linuxNodeToStarlark(node), nil
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
	return linuxNodeToStarlark(node), nil
}

func (l *LinuxPlanBindings) shellBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var command string
	if err := starlark.UnpackArgs("shell", args, kwargs, "command", &command); err != nil {
		return nil, err
	}
	node := l.Shell(command)
	return linuxNodeToStarlark(node), nil
}

func (l *LinuxPlanBindings) dependsOnBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
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

	l.graph.Edges = append(l.graph.Edges, execution.Edge{
		From:     toIDStr,
		To:       fromIDStr,
		Relation: "depends_on",
	})

	return starlark.None, nil
}

// linuxNodeToStarlark converts an execution.Node to a Starlark struct.
func linuxNodeToStarlark(node *execution.Node) starlark.Value {
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
