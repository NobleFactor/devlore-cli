// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package execution

import (
	"os"
	"sync"
)

// Plan provides binding functions for building an execution graph.
// Graph producers (writ tree builder, lore pipeline executor, LLM graph builder)
// use Plan to add actions to the graph. Each method returns the created node
// for edge construction.
//
// In Starlark scripts, plan is a global:
//
//	def install(package, phase):
//	    plan.file.mkdir("/usr/local/bin")
//	    plan.file.link("/usr/local/bin/foo", source="/path/to/foo")
type Plan struct {
	mu      sync.Mutex
	reg     *ActionRegistry
	graph   *Graph
	project string // default project for new nodes
	nodeID  int    // auto-incrementing node ID
}

// NewPlan creates a new plan for building an execution graph.
func NewPlan(reg *ActionRegistry, project string) *Plan {
	return &Plan{
		reg:     reg,
		graph:   &Graph{Nodes: []*Node{}, Edges: []Edge{}},
		project: project,
	}
}

// Graph returns the built execution graph.
func (p *Plan) Graph() *Graph {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.graph
}

// nextID generates a unique node ID.
func (p *Plan) nextID(prefix string) string {
	p.nodeID++
	return prefix + "-" + itoa(p.nodeID)
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [20]byte
	n := len(b)
	for i > 0 {
		n--
		b[n] = byte('0' + i%10)
		i /= 10
	}
	return string(b[n:])
}

// Mkdir adds a directory creation action.
func (p *Plan) Mkdir(path string) *Node {
	p.mu.Lock()
	defer p.mu.Unlock()

	node := &Node{
		ID:      p.nextID("mkdir"),
		Action:  p.reg.MustGet("file.mkdir"),
		Project: p.project,
	}
	node.SetSlotImmediate("path", path)
	node.SetSlotImmediate("mode", os.FileMode(0755))
	p.graph.Nodes = append(p.graph.Nodes, node)
	return node
}

// Link adds a symlink creation action.
func (p *Plan) Link(source, path string) *Node {
	p.mu.Lock()
	defer p.mu.Unlock()

	node := &Node{
		ID:      p.nextID("link"),
		Action:  p.reg.MustGet("file.link"),
		Project: p.project,
	}
	node.SetSlotImmediate("source", source)
	node.SetSlotImmediate("path", path)
	p.graph.Nodes = append(p.graph.Nodes, node)
	return node
}

// Copy adds a file copy action.
func (p *Plan) Copy(source, path string) *Node {
	p.mu.Lock()
	defer p.mu.Unlock()

	node := &Node{
		ID:      p.nextID("copy"),
		Action:  p.reg.MustGet("file.copy"),
		Project: p.project,
	}
	node.SetSlotImmediate("source", source)
	node.SetSlotImmediate("path", path)
	node.SetSlotImmediate("mode", os.FileMode(0644))
	p.graph.Nodes = append(p.graph.Nodes, node)
	return node
}

// CopyWithMode adds a file copy action with explicit permissions.
func (p *Plan) CopyWithMode(source, path string, mode os.FileMode) *Node {
	p.mu.Lock()
	defer p.mu.Unlock()

	node := &Node{
		ID:      p.nextID("copy"),
		Action:  p.reg.MustGet("file.copy"),
		Project: p.project,
	}
	node.SetSlotImmediate("source", source)
	node.SetSlotImmediate("path", path)
	node.SetSlotImmediate("mode", mode)
	p.graph.Nodes = append(p.graph.Nodes, node)
	return node
}

// Render adds a template rendering action.
func (p *Plan) Render(source string) *Node {
	p.mu.Lock()
	defer p.mu.Unlock()

	node := &Node{
		ID:      p.nextID("render"),
		Action:  p.reg.MustGet("template.render"),
		Project: p.project,
	}
	if source != "" {
		node.SetSlotImmediate("source", source)
	}
	p.graph.Nodes = append(p.graph.Nodes, node)
	return node
}

// Decrypt adds a decryption action.
func (p *Plan) Decrypt(source string) *Node {
	p.mu.Lock()
	defer p.mu.Unlock()

	node := &Node{
		ID:      p.nextID("decrypt"),
		Action:  p.reg.MustGet("encryption.decrypt"),
		Project: p.project,
	}
	if source != "" {
		node.SetSlotImmediate("source", source)
	}
	p.graph.Nodes = append(p.graph.Nodes, node)
	return node
}

// Remove adds a file/directory removal action.
func (p *Plan) Remove(path string) *Node {
	p.mu.Lock()
	defer p.mu.Unlock()

	node := &Node{
		ID:      p.nextID("remove"),
		Action:  p.reg.MustGet("file.remove"),
		Project: p.project,
	}
	node.SetSlotImmediate("path", path)
	p.graph.Nodes = append(p.graph.Nodes, node)
	return node
}

// Unlink adds a symlink removal action.
func (p *Plan) Unlink(path string) *Node {
	p.mu.Lock()
	defer p.mu.Unlock()

	node := &Node{
		ID:      p.nextID("unlink"),
		Action:  p.reg.MustGet("file.unlink"),
		Project: p.project,
	}
	node.SetSlotImmediate("path", path)
	p.graph.Nodes = append(p.graph.Nodes, node)
	return node
}

// Backup adds a backup action for an existing file.
func (p *Plan) Backup(path string) *Node {
	p.mu.Lock()
	defer p.mu.Unlock()

	node := &Node{
		ID:      p.nextID("backup"),
		Action:  p.reg.MustGet("file.backup"),
		Project: p.project,
	}
	node.SetSlotImmediate("path", path)
	p.graph.Nodes = append(p.graph.Nodes, node)
	return node
}

// Rename adds a file/directory move action (git mv when possible).
func (p *Plan) Rename(source, path string) *Node {
	p.mu.Lock()
	defer p.mu.Unlock()

	node := &Node{
		ID:      p.nextID("move"),
		Action:  p.reg.MustGet("file.move"),
		Project: p.project,
	}
	node.SetSlotImmediate("source", source)
	node.SetSlotImmediate("path", path)
	p.graph.Nodes = append(p.graph.Nodes, node)
	return node
}

// DependsOn adds an ordering edge: from must complete before to begins.
func (p *Plan) DependsOn(from, to *Node) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.graph.Edges = append(p.graph.Edges, Edge{
		From: from.ID,
		To:   to.ID,
	})
}

// Orders adds an ordering constraint between nodes.
func (p *Plan) Orders(from, to *Node) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.graph.Edges = append(p.graph.Edges, Edge{
		From: from.ID,
		To:   to.ID,
	})
}

