// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package execution

import (
	"context"
	"fmt"
	"io"
	"os"
)

// Operation is the interface for all executable actions.
// Each operation is self-contained: it reads its own inputs via ctx and node,
// performs its work, and stores any outputs back on ctx.
type Operation interface {
	// Name returns the operation identifier (e.g., "file.link", "file.decrypt").
	Name() string

	// Execute performs the operation using context and node state.
	Execute(ctx *Context, node *Node) error
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

	// Content pipeline (set by executor before each execution loop).
	Edges   []Edge
	Outputs map[string][]byte

	// Per-node checksums (written by ops, read by executor).
	SourceChecksum string
	TargetChecksum string
}

// ContentFor resolves input content for a node. It checks the upstream
// chain via edges first, then falls back to reading the source file.
func (c *Context) ContentFor(node *Node) ([]byte, error) {
	// Check upstream chain via edges
	for _, edge := range c.Edges {
		if edge.To == node.ID {
			if content, ok := c.Outputs[edge.From]; ok {
				return content, nil
			}
		}
	}
	// Read source file
	source, _ := node.GetSlot("source").(string)
	if source != "" {
		content, err := os.ReadFile(source)
		if err != nil {
			return nil, fmt.Errorf("read source %s: %w", source, err)
		}
		c.SourceChecksum = ChecksumBytes(content)
		return content, nil
	}
	return nil, nil
}

// StoreContent stores output content for a node so downstream nodes can read it.
func (c *Context) StoreContent(node *Node, data []byte) {
	if c.Outputs == nil {
		c.Outputs = make(map[string][]byte)
	}
	c.Outputs[node.ID] = data
}
