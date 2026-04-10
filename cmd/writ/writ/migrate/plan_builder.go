// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package migrate

import (
	"os"
	"strconv"
	"sync"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// planBuilder provides binding functions for building a migration execution graph.
// Each method returns the created node for edge construction.
type planBuilder struct {
	mu      sync.Mutex
	reg     *op.ReceiverRegistry
	graph   *op.Graph
	project string // default project for new nodes
	nodeID  int    // auto-incrementing node ID
}

// newPlanBuilder creates a new plan builder for migration graph construction.
func newPlanBuilder(reg *op.ReceiverRegistry, project string) *planBuilder {
	return &planBuilder{
		reg:     reg,
		graph:   &op.Graph{},
		project: project,
	}
}

// Graph returns the built execution graph.
func (p *planBuilder) Graph() *op.Graph {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.graph
}

// nextID generates a unique node ID.
func (p *planBuilder) nextID(prefix string) string {
	p.nodeID++
	return prefix + "-" + strconv.Itoa(p.nodeID)
}

// Mkdir adds a directory creation action.
func (p *planBuilder) Mkdir(path string) *op.Node {
	p.mu.Lock()
	defer p.mu.Unlock()

	node := &op.Node{
		ID: p.nextID("mkdir"), Receiver: "file.mkdir",
		Origin: p.project,
	}
	node.SetSlotImmediate("path", path)
	node.SetSlotImmediate("mode", os.FileMode(0o755))
	p.graph.AddNode(node)
	return node
}

// Link adds a symlink creation action.
func (p *planBuilder) Link(source, path string) *op.Node {
	p.mu.Lock()
	defer p.mu.Unlock()

	node := &op.Node{
		ID:     p.nextID("link"),
		Receiver: "file.link",
		Origin: p.project,
	}
	node.SetSlotImmediate("source", source)
	node.SetSlotImmediate("path", path)
	p.graph.AddNode(node)
	return node
}

// Copy adds a file copy action.
func (p *planBuilder) Copy(source, path string) *op.Node {
	p.mu.Lock()
	defer p.mu.Unlock()

	node := &op.Node{
		ID:     p.nextID("copy"),
		Receiver: "file.copy",
		Origin: p.project,
	}
	node.SetSlotImmediate("source", source)
	node.SetSlotImmediate("path", path)
	node.SetSlotImmediate("mode", os.FileMode(0o644))
	p.graph.AddNode(node)
	return node
}

// CopyWithMode adds a file copy action with explicit permissions.
func (p *planBuilder) CopyWithMode(source, path string, mode os.FileMode) *op.Node {
	p.mu.Lock()
	defer p.mu.Unlock()

	node := &op.Node{
		ID:     p.nextID("copy"),
		Receiver: "file.copy",
		Origin: p.project,
	}
	node.SetSlotImmediate("source", source)
	node.SetSlotImmediate("path", path)
	node.SetSlotImmediate("mode", mode)
	p.graph.AddNode(node)
	return node
}

// Render adds a template rendering action.
//
// Callers inject Source, Target, and Origin into the template_data slot if needed — the provider no longer accepts them
// as separate parameters.
func (p *planBuilder) Render(source string) *op.Node {
	p.mu.Lock()
	defer p.mu.Unlock()

	node := &op.Node{
		ID:     p.nextID("render"),
		Receiver: "template.render_bytes",
		Origin: p.project,
	}
	if source != "" {
		node.SetSlotImmediate("source", source)
	}
	p.graph.AddNode(node)
	return node
}

// Decrypt adds a decryption action.
func (p *planBuilder) Decrypt(source string) *op.Node {
	p.mu.Lock()
	defer p.mu.Unlock()

	node := &op.Node{
		ID:     p.nextID("decrypt"),
		Receiver: "encryption.decrypt",
		Origin: p.project,
	}
	if source != "" {
		node.SetSlotImmediate("source", source)
	}
	p.graph.AddNode(node)
	return node
}

// Remove adds a file/directory removal action.
func (p *planBuilder) Remove(path string) *op.Node {
	p.mu.Lock()
	defer p.mu.Unlock()

	node := &op.Node{
		ID:     p.nextID("remove"),
		Receiver: "file.remove",
		Origin: p.project,
	}
	node.SetSlotImmediate("path", path)
	p.graph.AddNode(node)
	return node
}

// Unlink adds a symlink removal action.
func (p *planBuilder) Unlink(path string) *op.Node {
	p.mu.Lock()
	defer p.mu.Unlock()

	node := &op.Node{
		ID:     p.nextID("unlink"),
		Receiver: "file.unlink",
		Origin: p.project,
	}
	node.SetSlotImmediate("path", path)
	p.graph.AddNode(node)
	return node
}

// Backup adds a backup action for an existing file.
func (p *planBuilder) Backup(path string) *op.Node {
	p.mu.Lock()
	defer p.mu.Unlock()

	node := &op.Node{
		ID:     p.nextID("backup"),
		Receiver: "file.backup",
		Origin: p.project,
	}
	node.SetSlotImmediate("path", path)
	p.graph.AddNode(node)
	return node
}

// Rename adds a file/directory move action (git mv when possible).
func (p *planBuilder) Rename(source, path string) *op.Node {
	p.mu.Lock()
	defer p.mu.Unlock()

	node := &op.Node{
		ID:     p.nextID("move"),
		Receiver: "file.move",
		Origin: p.project,
	}
	node.SetSlotImmediate("source", source)
	node.SetSlotImmediate("path", path)
	p.graph.AddNode(node)
	return node
}

// DependsOn adds an ordering edge: from must complete before to begins.
func (p *planBuilder) DependsOn(from, to *op.Node) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.graph.Edges = append(p.graph.Edges, op.Edge{
		From: from.ID,
		To:   to.ID,
	})
}
