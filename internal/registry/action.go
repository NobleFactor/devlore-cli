// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

// action.go defines PhaseAction types for uniform pipeline phases.
//
// PhaseAction is the core abstraction: both Starlark scripts (ScriptAction)
// and native PM operations (NativePMAction) implement this interface.
// The engine's package-manifest delegate queries LorePackage.PhaseActions()
// and adds the results to the execution graph.
//
// TODO: Implement graph optimization that batches NativePMAction nodes with
// the same manager and operation. See docs/plans/uniform-pipeline-interface.md.

package registry

// PhaseActionType distinguishes between script and native PM actions.
type PhaseActionType int

const (
	// ActionScript is a Starlark phase script.
	ActionScript PhaseActionType = iota
	// ActionNativePM is a native package manager operation.
	ActionNativePM
)

// PMOperation represents a native package manager operation.
type PMOperation int

const (
	// PMInstall installs packages.
	PMInstall PMOperation = iota
	// PMRemove removes packages.
	PMRemove
	// PMUpdate refreshes the package index.
	PMUpdate
	// PMUpgrade upgrades installed packages.
	PMUpgrade
)

// String returns the operation name.
func (op PMOperation) String() string {
	switch op {
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
// This provides a uniform interface for both Starlark scripts and native PM operations.
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

// NativePMAction executes a native package manager operation.
type NativePMAction struct {
	// Manager identifies which package manager to use.
	Manager PackageSource

	// Operation is the PM operation (install, remove, update).
	Operation PMOperation

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
// Native PM install/upgrade/remove operations are batchable; update (index refresh) is not.
func (a *NativePMAction) Batchable() bool {
	return a.Operation == PMInstall || a.Operation == PMUpgrade || a.Operation == PMRemove
}

// CanBatchWith returns true if this action can be batched with another.
// Actions can be batched if they have the same manager, operation, and phase.
func (a *NativePMAction) CanBatchWith(other *NativePMAction) bool {
	return a.Manager == other.Manager &&
		a.Operation == other.Operation &&
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
		Operation: a.Operation,
		Packages:  packages,
		PhaseName: a.PhaseName,
	}
}
