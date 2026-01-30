// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package execution

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
	Transform(ctx *Context, node Executable, content []byte) ([]byte, error)
}

// Writer operations read content and write to the filesystem.
// Used for: copy.
type Writer interface {
	Operation
	Write(ctx *Context, node Executable, content []byte) (targetChecksum string, err error)
}

// Direct operations manage their own I/O with no content flow.
// Used for: link, mkdir, backup, unlink, remove, validate, rename, install.
type Direct interface {
	Operation
	Execute(ctx *Context, node Executable) error
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
	// calling GraphExecutor.Run().
	Data map[string]any
}

// pipelineState holds mutable state threaded through a node's operation pipeline.
// The executor pre-reads source content into Content when the pipeline begins
// with a transform or writer operation.
type pipelineState struct {
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
