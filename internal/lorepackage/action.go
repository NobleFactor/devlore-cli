// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// action.go defines PhaseAction types for uniform pipeline phases.
//
// PhaseAction is the core abstraction: both Starlark scripts (ScriptAction)
// and native PM commands (NativePMAction) implement this interface.
// The engine's package-manifest delegate queries Release.PhaseActions()
// and adds the results to the execution graph.
//
// TODO: Implement graph optimization that batches NativePMAction nodes with
// the same manager and command. See docs/plans/uniform-pipeline-interface.md.

package lorepackage

// PhaseActionType distinguishes between script and native PM actions.
type PhaseActionType int

const (
	// ActionScript is a Starlark phase script.
	ActionScript PhaseActionType = iota
	// ActionNativePM is a native package manager command.
	ActionNativePM
)

// PMCommand represents a native package manager command (install, remove, etc.).
type PMCommand int

const (
	// PMInstall installs packages.
	PMInstall PMCommand = iota
	// PMRemove removes packages.
	PMRemove
	// PMUpdate refreshes the package index.
	PMUpdate
	// PMUpgrade upgrades installed packages.
	PMUpgrade
)

// String returns the command name.
func (c PMCommand) String() string {
	switch c {
	case PMInstall:
		return "install"
	case PMRemove:
		return "remove"
	case PMUpdate:
		return "update"
	case PMUpgrade:
		return "upgrade"
	default:
		return "unknown"
	}
}

// PhaseAction represents an executable phase action.
// This provides a uniform interface for both Starlark scripts and native PM commands.
type PhaseAction interface {
	// Type returns the action type (script or native PM).
	Type() PhaseActionType

	// Phase returns the phase name this action belongs to.
	Phase() string
}

// ScriptAction executes a Starlark phase script.
type ScriptAction struct {
	// Path is the absolute path to the .star file.
	Path string

	// PhaseName is the phase name (function to call in the script).
	PhaseName string

	// Platform is the platform directory this script came from.
	Platform string
}

// Type returns ActionScript.
func (a *ScriptAction) Type() PhaseActionType {
	return ActionScript
}

// Phase returns the phase name.
func (a *ScriptAction) Phase() string {
	return a.PhaseName
}

// NativePMAction executes a native package manager command.
type NativePMAction struct {
	// Manager identifies which package manager to use.
	Manager PackageSource

	// Command is the native PM command (install, remove, update, upgrade).
	Command PMCommand

	// Packages is the list of package names to operate on.
	Packages []string

	// PhaseName is the phase this action belongs to.
	PhaseName string
}

// Type returns ActionNativePM.
func (a *NativePMAction) Type() PhaseActionType {
	return ActionNativePM
}

// Phase returns the phase name.
func (a *NativePMAction) Phase() string {
	return a.PhaseName
}

// Batchable returns true if this action can be batched with others.
// Native PM install/upgrade/remove commands are batchable; update (index refresh) is not.
func (a *NativePMAction) Batchable() bool {
	return a.Command == PMInstall || a.Command == PMUpgrade || a.Command == PMRemove
}

// CanBatchWith returns true if this action can be batched with another.
// Actions can be batched if they have the same manager, command, and phase.
func (a *NativePMAction) CanBatchWith(other *NativePMAction) bool {
	return a.Manager == other.Manager &&
		a.Command == other.Command &&
		a.PhaseName == other.PhaseName
}

// Merge combines this action with another, returning a new batched action.
// Returns nil if the actions cannot be batched.
func (a *NativePMAction) Merge(other *NativePMAction) *NativePMAction {
	if !a.CanBatchWith(other) {
		return nil
	}

	// Combine package lists
	packages := make([]string, 0, len(a.Packages)+len(other.Packages))
	packages = append(packages, a.Packages...)
	packages = append(packages, other.Packages...)

	return &NativePMAction{
		Manager:   a.Manager,
		Command:   a.Command,
		Packages:  packages,
		PhaseName: a.PhaseName,
	}
}
