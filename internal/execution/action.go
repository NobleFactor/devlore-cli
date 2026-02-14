// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package execution

import (
	"context"
	"fmt"
	"io"
	"os"
)

// Result is data that flows to downstream nodes via edges (e.g., a checksum,
// a rendered template, a query result). The executor stores this on the node
// for edge-based slot resolution.
type Result = any

// UndoState is the state captured by Do and passed to Undo during saga
// rollback. Each action defines its own state shape. Actions with no rollback
// return nil from Do; their Undo ignores the state parameter.
type UndoState = any

// Action is the interface for all executable actions.
// Each action is self-contained: it reads its own inputs via ctx and node,
// performs its work, and stores any outputs back on ctx.
type Action interface {
	// Name returns the action identifier (e.g., "link", "copy").
	Name() string

	// Do performs the forward action using context and node state.
	// Returns a result (flows to downstream nodes) and undo state
	// (stored on recovery stack for rollback).
	Do(ctx *Context, node *Node) (Result, UndoState, error)

	// Undo performs the compensating action using the state captured by Do.
	Undo(ctx *Context, node *Node, state UndoState) error
}

// Context provides execution context to actions.
type Context struct {
	context.Context

	// DryRun prevents filesystem modifications when true.
	DryRun bool

	// Logger receives action output messages.
	Logger io.Writer

	// Data holds tool-provided context: template variables, SOPS config,
	// identities, segment maps, etc. Each tool populates this before
	// calling GraphExecutor.Run().
	Data map[string]any

	// Content pipeline (set by executor before each execution loop).
	Edges   []Edge
	Outputs map[string][]byte

	// Per-node checksums (written by actions, read by executor).
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
