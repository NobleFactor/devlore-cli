// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package extension

import (
	"fmt"
	"sort"
	"sync"
)

// Registry holds registered extensions. Used for debugging and
// listing loaded extensions from the command line.
type Registry struct {
	mu   sync.RWMutex
	exts map[string]*Extension
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		exts: make(map[string]*Extension),
	}
}

// Register adds an extension to the registry.
// Returns an error if an extension with the same name is already registered.
func (r *Registry) Register(ext *Extension) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.exts[ext.Name]; exists {
		return fmt.Errorf("extension %q already registered", ext.Name)
	}

	r.exts[ext.Name] = ext
	return nil
}

// Get returns an extension by name, or nil if not found.
func (r *Registry) Get(name string) *Extension {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.exts[name]
}

// All returns a copy of all registered extensions.
func (r *Registry) All() map[string]*Extension {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]*Extension, len(r.exts))
	for k, v := range r.exts {
		result[k] = v
	}
	return result
}

// Names returns a sorted list of registered extension names.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.exts))
	for name := range r.exts {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Count returns the number of registered extensions.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.exts)
}

// Clear removes all extensions from the registry.
func (r *Registry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.exts = make(map[string]*Extension)
}
