// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

// ReceiverRegistry stores receiver factories and their methods.
type ReceiverRegistry struct {
	factories map[string]ReceiverFactory // by receiver name
	methods   map[string]*Method         // by action name
}

// NewReceiverRegistry creates an empty registry.
func NewReceiverRegistry() *ReceiverRegistry {
	return &ReceiverRegistry{
		factories: make(map[string]ReceiverFactory),
		methods:   make(map[string]*Method),
	}
}

// NewActionRegistry creates an empty registry. Alias for NewReceiverRegistry
// during migration.
func NewActionRegistry() *ReceiverRegistry {
	return NewReceiverRegistry()
}

// RegisterFactory stores a factory by its receiver name.
func (r *ReceiverRegistry) RegisterFactory(factory ReceiverFactory) {
	r.factories[factory.ReceiverName()] = factory
}

// Factory returns the factory for the given receiver name.
func (r *ReceiverRegistry) Factory(name string) (ReceiverFactory, bool) {
	f, ok := r.factories[name]
	return f, ok
}

// RegisterMethod stores a method by its action name.
func (r *ReceiverRegistry) RegisterMethod(method *Method) {
	r.methods[method.ActionName] = method
}

// Register adds an action to the registry. If an action with the same
// name already exists, it is replaced.
// TODO: migrate callers to RegisterMethod, then remove.
func (r *ReceiverRegistry) Register(action Action) {
	if m, ok := action.(*Method); ok {
		r.methods[m.ActionName] = m
		return
	}
	// Legacy path for handwritten actions (flow.Choose, etc.)
	r.methods[action.Name()] = &Method{
		ActionName: action.Name(),
		DoFunc: func(ctx *Context, slots map[string]any) (Result, Complement, error) {
			return action.Do(ctx, slots)
		},
	}
}

// Get returns the method registered under the given action name.
func (r *ReceiverRegistry) Get(name string) (Action, bool) {
	m, ok := r.methods[name]
	return m, ok
}

// Method returns the Method for the given action name.
func (r *ReceiverRegistry) Method(name string) (*Method, bool) {
	m, ok := r.methods[name]
	return m, ok
}

// MustGet returns the action registered under the given name.
// Panics if not registered.
func (r *ReceiverRegistry) MustGet(name string) Action {
	m, ok := r.methods[name]
	if !ok {
		panic("unregistered action: " + name)
	}
	return m
}

// Names returns all registered action names.
func (r *ReceiverRegistry) Names() []string {
	names := make([]string, 0, len(r.methods))
	for name := range r.methods {
		names = append(names, name)
	}
	return names
}
