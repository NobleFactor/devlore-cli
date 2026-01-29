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

	// OpDelegate passes the file to another tool (e.g., lore for packages.manifest).
	OpDelegate
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
	case OpDelegate:
		return "delegate"
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

// HasDelegate returns true if the operations include a delegate operation.
func (ops Operations) HasDelegate() bool {
	for _, op := range ops {
		if op == OpDelegate {
			return true
		}
	}
	return false
}
