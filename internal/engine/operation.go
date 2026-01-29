// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package engine provides a common execution engine for graph-based operations.
// Both writ and lore build execution graphs and hand them to this engine for
// processing. The engine dispatches operations to registered handlers, threads
// content through transform pipelines, and produces receipts.
package engine

import (
	"context"
	"io"
)

// Operation is the base interface for all executable actions.
type Operation interface {
	// Name returns the operation identifier (e.g., "link", "decrypt").
	Name() string

	// Category returns the operation category for pipeline validation.
	Category() OpCategory
}

// OpCategory classifies operations by their data flow behavior.
type OpCategory int

const (
	// OpTransform reads content, produces transformed content.
	OpTransform OpCategory = iota

	// OpWriter reads content, writes to filesystem, produces checksum.
	OpWriter

	// OpDirect manages its own I/O, no content flow.
	OpDirect
)

// Transform operations read content and produce transformed content.
// Used for: decrypt, expand.
type Transform interface {
	Operation
	Transform(ctx *Context, node *Node, content []byte) ([]byte, error)
}

// Writer operations read content and write to the filesystem.
// Used for: copy.
type Writer interface {
	Operation
	Write(ctx *Context, node *Node, content []byte) (targetChecksum string, err error)
}

// Direct operations manage their own I/O with no content flow.
// Used for: link, mkdir, backup, unlink, remove, validate, rename.
type Direct interface {
	Operation
	Execute(ctx *Context, node *Node) error
}

// PipelineInput holds the input data for a pipeline.
type PipelineInput struct {
	// Content is the source file content (read by engine before pipeline starts).
	Content []byte

	// SourceChecksum is the SHA256 of the original content.
	SourceChecksum string
}

// PipelineOutput holds the output data from a pipeline.
type PipelineOutput struct {
	// Content is the final transformed content (after all transforms).
	Content []byte

	// TargetChecksum is the SHA256 of the written target file.
	TargetChecksum string
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
