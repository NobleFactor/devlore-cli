// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package execution

import (
	"context"
	"io"
)

// Operation is the base interface for all executable actions.
type Operation interface {
	// Name returns the operation identifier (e.g., "file.link", "file.decrypt").
	Name() string
}

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
