// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package migrate

import (
	"os"
	"strconv"
	"sync"

	"github.com/NobleFactor/devlore-cli/pkg/assert"
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
		graph:   op.NewGraph(),
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

// mustAction looks up a registered action by short name; asserts on failure. Used by the builder's
// per-action methods which hard-code the action names — a missing action is a programming error,
// not a runtime condition.
func (p *planBuilder) mustAction(name string) op.Action {
	a, err := p.reg.BuildAction(name)
	assert.NoError("planBuilder.mustAction("+name+")", err)
	return a
}

// Mkdir adds a directory creation action.
func (p *planBuilder) Mkdir(path string) *op.Node {
	p.mu.Lock()
	defer p.mu.Unlock()

	node := op.NewNode(p.nextID("mkdir"), p.mustAction("file.mkdir"))
	node.Origin = p.project
	node.SetSlot("path", op.ImmediateValue{Value: path})
	node.SetSlot("mode", op.ImmediateValue{Value: os.FileMode(0o755)})
	p.graph.AddNode(node)
	return node
}

// Link adds a symlink creation action.
func (p *planBuilder) Link(source, path string) *op.Node {
	p.mu.Lock()
	defer p.mu.Unlock()

	node := op.NewNode(p.nextID("link"), p.mustAction("file.link"))
	node.Origin = p.project
	node.SetSlot("source", op.ImmediateValue{Value: source})
	node.SetSlot("path", op.ImmediateValue{Value: path})
	p.graph.AddNode(node)
	return node
}

// Copy adds a file copy action.
func (p *planBuilder) Copy(source, path string) *op.Node {
	p.mu.Lock()
	defer p.mu.Unlock()

	node := op.NewNode(p.nextID("copy"), p.mustAction("file.copy"))
	node.Origin = p.project
	node.SetSlot("source", op.ImmediateValue{Value: source})
	node.SetSlot("path", op.ImmediateValue{Value: path})
	node.SetSlot("mode", op.ImmediateValue{Value: os.FileMode(0o644)})
	p.graph.AddNode(node)
	return node
}

// CopyWithMode adds a file copy action with explicit permissions.
func (p *planBuilder) CopyWithMode(source, path string, mode os.FileMode) *op.Node {
	p.mu.Lock()
	defer p.mu.Unlock()

	node := op.NewNode(p.nextID("copy"), p.mustAction("file.copy"))
	node.Origin = p.project
	node.SetSlot("source", op.ImmediateValue{Value: source})
	node.SetSlot("path", op.ImmediateValue{Value: path})
	node.SetSlot("mode", op.ImmediateValue{Value: mode})
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

	node := op.NewNode(p.nextID("render"), p.mustAction("template.render_bytes"))
	node.Origin = p.project
	if source != "" {
		node.SetSlot("source", op.ImmediateValue{Value: source})
	}
	p.graph.AddNode(node)
	return node
}

// Decrypt adds a decryption action.
func (p *planBuilder) Decrypt(source string) *op.Node {
	p.mu.Lock()
	defer p.mu.Unlock()

	node := op.NewNode(p.nextID("decrypt"), p.mustAction("encryption.decrypt"))
	node.Origin = p.project
	if source != "" {
		node.SetSlot("source", op.ImmediateValue{Value: source})
	}
	p.graph.AddNode(node)
	return node
}

// Remove adds a file/directory removal action.
func (p *planBuilder) Remove(path string) *op.Node {
	p.mu.Lock()
	defer p.mu.Unlock()

	node := op.NewNode(p.nextID("remove"), p.mustAction("file.remove"))
	node.Origin = p.project
	node.SetSlot("path", op.ImmediateValue{Value: path})
	p.graph.AddNode(node)
	return node
}

// Unlink adds a symlink removal action.
func (p *planBuilder) Unlink(path string) *op.Node {
	p.mu.Lock()
	defer p.mu.Unlock()

	node := op.NewNode(p.nextID("unlink"), p.mustAction("file.unlink"))
	node.Origin = p.project
	node.SetSlot("path", op.ImmediateValue{Value: path})
	p.graph.AddNode(node)
	return node
}

// Backup adds a backup action for an existing file.
func (p *planBuilder) Backup(path string) *op.Node {
	p.mu.Lock()
	defer p.mu.Unlock()

	node := op.NewNode(p.nextID("backup"), p.mustAction("file.backup"))
	node.Origin = p.project
	node.SetSlot("path", op.ImmediateValue{Value: path})
	p.graph.AddNode(node)
	return node
}

// Rename adds a file/directory move action (git mv when possible).
func (p *planBuilder) Rename(source, path string) *op.Node {
	p.mu.Lock()
	defer p.mu.Unlock()

	node := op.NewNode(p.nextID("move"), p.mustAction("file.move"))
	node.Origin = p.project
	node.SetSlot("source", op.ImmediateValue{Value: source})
	node.SetSlot("path", op.ImmediateValue{Value: path})
	p.graph.AddNode(node)
	return node
}

// DependsOn adds an ordering edge: from must complete before to begins.
func (p *planBuilder) DependsOn(from, to *op.Node) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.graph.Root.AddEdge(op.Edge{
		From: from.ID(),
		To:   to.ID(),
	})
}
