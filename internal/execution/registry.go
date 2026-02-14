// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package execution

// ActionRegistry maps action names to their implementations.
// Each tool registers its actions before calling GraphExecutor.Run().
type ActionRegistry struct {
	actions map[string]Action
}

// NewActionRegistry creates an empty action registry.
func NewActionRegistry() *ActionRegistry {
	return &ActionRegistry{actions: make(map[string]Action)}
}

// Register adds an action to the registry. If an action with the same
// name already exists, it is replaced.
func (r *ActionRegistry) Register(action Action) {
	r.actions[action.Name()] = action
}

// Get returns the action registered under the given name.
func (r *ActionRegistry) Get(name string) (Action, bool) {
	action, ok := r.actions[name]
	return action, ok
}

// Names returns all registered action names.
func (r *ActionRegistry) Names() []string {
	names := make([]string, 0, len(r.actions))
	for name := range r.actions {
		names = append(names, name)
	}
	return names
}
