// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package tree provides deployment tree building for writ.
package tree

// Operation represents a file processing operation.
type Operation int

const (
	// OpLink creates a symlink from target to source.
	OpLink Operation = iota

	// OpExpand processes a .template file with Go text/template.
	OpExpand

	// OpCopy writes content to target (used after expand or decrypt).
	OpCopy

	// OpDecrypt decrypts a .age file using age encryption.
	OpDecrypt

	// OpPackages marks a packages-manifest.yaml file that requires processing
	// by the Package Graph Builder to add package installation nodes to the
	// execution graph. This is NOT a delegation to another tool—writ and lore
	// share the same execution engine.
	//
	// NOT YET IMPLEMENTED: The Package Graph Builder (internal/lore/graph) does
	// not exist yet. When implemented, it will parse the manifest and produce
	// install/configure/verify nodes that the shared engine executes.
	OpPackages
)

// String returns the operation name.
func (o Operation) String() string {
	switch o {
	case OpLink:
		return "link"
	case OpExpand:
		return "expand"
	case OpCopy:
		return "copy"
	case OpDecrypt:
		return "decrypt"
	case OpPackages:
		return "packages"
	default:
		return "unknown"
	}
}

// Operations is a slice of operations for JSON marshaling.
type Operations []Operation

// Strings returns the operation names.
func (ops Operations) Strings() []string {
	result := make([]string, len(ops))
	for i, op := range ops {
		result[i] = op.String()
	}
	return result
}

// HasCopy returns true if the operations include a copy operation.
// Files with copy are written to disk (templates, secrets) rather than symlinked.
func (ops Operations) HasCopy() bool {
	for _, op := range ops {
		if op == OpCopy {
			return true
		}
	}
	return false
}

// HasPackages returns true if the operations include a packages operation.
// This indicates a packages-manifest.yaml file that needs processing by the
// Package Graph Builder (NOT YET IMPLEMENTED).
func (ops Operations) HasPackages() bool {
	for _, op := range ops {
		if op == OpPackages {
			return true
		}
	}
	return false
}
