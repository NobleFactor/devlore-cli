// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package execution

// OperationRegistry maps operation names to their implementations.
// Each tool registers its operations before calling GraphExecutor.Run().
type OperationRegistry struct {
	ops map[string]Operation
}

// NewOperationRegistry creates an empty operation registry.
func NewOperationRegistry() *OperationRegistry {
	return &OperationRegistry{ops: make(map[string]Operation)}
}

// Register adds an operation to the registry. If an operation with the same
// name already exists, it is replaced.
func (r *OperationRegistry) Register(op Operation) {
	r.ops[op.Name()] = op
}

// Get returns the operation registered under the given name.
func (r *OperationRegistry) Get(name string) (Operation, bool) {
	op, ok := r.ops[name]
	return op, ok
}

// Names returns all registered operation names.
func (r *OperationRegistry) Names() []string {
	names := make([]string, 0, len(r.ops))
	for name := range r.ops {
		names = append(names, name)
	}
	return names
}
