// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

// Package engine provides a common execution engine for graph-based operations.
// Both writ and lore build execution graphs and hand them to this engine for
// processing. The engine dispatches operations to registered handlers, threads
// content through transform pipelines, and produces receipts.
package engine

import (
	"context"
	"io"
)

// Operation defines an executable action. Implementations fall into three
// categories: transforms (modify state.Content), writers (produce filesystem
// side effects from state.Content), and direct operations (manage their own I/O).
type Operation interface {
	// Execute performs the operation on the given node with pipeline state.
	// Transforms modify state.Content in place. Writers consume state.Content
	// and produce filesystem output. Direct operations ignore state.Content.
	Execute(ctx *Context, node *Node, state *PipelineState) error

	// Name returns the operation identifier (e.g., "link", "decrypt").
	Name() string
}

// PipelineState holds mutable state threaded through a node's operation pipeline.
// The engine pre-reads source content into Content when the pipeline begins
// with a transform or writer operation.
type PipelineState struct {
	// Content is the current file content. Transforms modify this in place;
	// writers consume it to produce output files.
	Content []byte

	// SourceChecksum is computed from the original source file content
	// before any transforms are applied. Format: "sha256:<hex>".
	SourceChecksum string

	// TargetChecksum is set by writer operations after writing content
	// to the target path. Format: "sha256:<hex>".
	TargetChecksum string

	// Metadata holds per-node extensible state that operations can read/write.
	Metadata map[string]string
}

// Context provides execution context to operations.
type Context struct {
	context.Context

	// DryRun prevents filesystem modifications when true.
	DryRun bool

	// Logger receives operation output messages.
	Logger io.Writer

	// Data holds tool-provided context: template variables, SOPS config,
	// identities, segment maps, etc. Each tool populates this before
	// calling Engine.Run().
	Data map[string]any
}

// Registry maps operation names to implementations. Each tool registers its
// operations before calling Engine.Run().
type Registry struct {
	ops map[string]Operation
}

// NewRegistry creates an empty operation registry.
func NewRegistry() *Registry {
	return &Registry{ops: make(map[string]Operation)}
}

// Register adds an operation to the registry. If an operation with the same
// name already exists, it is replaced.
func (r *Registry) Register(op Operation) {
	r.ops[op.Name()] = op
}

// Get returns the operation registered under the given name.
func (r *Registry) Get(name string) (Operation, bool) {
	op, ok := r.ops[name]
	return op, ok
}

// Names returns all registered operation names.
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.ops))
	for name := range r.ops {
		names = append(names, name)
	}
	return names
}
