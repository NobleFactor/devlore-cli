// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Status represents the execution status of a node.
type Status int

const (
	// StatusPending means the node has not been processed yet.
	StatusPending Status = iota
	// StatusRunning means the node is currently executing.
	StatusRunning
	// StatusCompleted means the node executed successfully.
	StatusCompleted
	// StatusFailed means the node encountered an error.
	StatusFailed
	// StatusSkipped means the node was skipped (conflict, already deployed, etc.).
	StatusSkipped
)

// String returns a human-readable status label.
func (s Status) String() string {
	switch s {
	case StatusPending:
		return "pending"
	case StatusRunning:
		return "running"
	case StatusCompleted:
		return "completed"
	case StatusFailed:
		return "failed"
	case StatusSkipped:
		return "skipped"
	default:
		return "unknown"
	}
}

// Graph represents an execution graph: nodes with operations and edges
// defining ordering constraints.
type Graph struct {
	Nodes []*Node
	Edges []Edge
}

// Node represents a unit of work in the execution graph.
type Node struct {
	// ID uniquely identifies this node (e.g., relative target path for writ,
	// package name for lore).
	ID string

	// Operations is the pipeline of operations to execute on this node.
	// Examples: ["link"], ["decrypt", "expand", "copy"], ["install"].
	Operations []string

	// Source is the source path (for file operations).
	Source string

	// Target is the target path (for file operations).
	Target string

	// Project is the grouping key (writ: project name, lore: package name).
	Project string

	// Mode is the target file permissions (0 means use default 0644).
	Mode os.FileMode

	// DelegateTo is DEPRECATED - there is no delegation between tools.
	// writ and lore share the same execution engine. Retained for backwards
	// compatibility with old receipts.
	DelegateTo string

	// Metadata holds tool-specific extensions.
	Metadata map[string]string
}

// Edge defines an ordering constraint between nodes.
type Edge struct {
	From     string // Source node ID
	To       string // Target node ID
	Relation string // "depends_on", "orders"
}

// Result represents the outcome of executing a single node.
type Result struct {
	NodeID         string
	Status         Status
	Error          error
	Message        string
	SourceChecksum string
	TargetChecksum string
}

// Options configures engine behavior.
type Options struct {
	// DryRun prevents filesystem modifications.
	DryRun bool

	// Logger receives operation output.
	Logger io.Writer

	// Data holds tool-provided context (template vars, SOPS config, etc.).
	Data map[string]any

	// ConflictResolution specifies how to handle conflicts detected during preflight.
	ConflictResolution ConflictResolution

	// BackupSuffix is appended to backup filenames (default: ".writ-backup").
	BackupSuffix string
}

// Engine executes operation graphs.
type Engine struct {
	registry *Registry
	options  Options
}

// New creates an engine with the given registry and options.
func New(registry *Registry, opts Options) *Engine {
	if opts.Logger == nil {
		opts.Logger = os.Stdout
	}
	if opts.BackupSuffix == "" {
		opts.BackupSuffix = ".writ-backup"
	}
	return &Engine{
		registry: registry,
		options:  opts,
	}
}

// Run executes all nodes in the graph, respecting ordering constraints.
// Nodes are processed in topological order when edges define dependencies.
// Returns results for each node.
//
// TODO: Add graph optimization pass before execution. Native PM operations
// (e.g., multiple "install" nodes with the same package manager) should be
// batched into a single operation to reduce PM invocations. See
// docs/plans/uniform-pipeline-interface.md for design details.
func (e *Engine) Run(ctx context.Context, graph *Graph) ([]*Result, error) {
	// TODO: graph = e.optimize(graph)
	ordered := e.orderNodes(graph)

	execCtx := &Context{
		Context: ctx,
		DryRun:  e.options.DryRun,
		Logger:  e.options.Logger,
		Data:    e.options.Data,
	}

	var results []*Result
	for _, node := range ordered {
		result := e.executeNode(execCtx, node)
		results = append(results, result)

		if result.Status == StatusFailed && e.options.ConflictResolution == ResolutionStop {
			return results, result.Error
		}
	}

	return results, nil
}

// executeNode processes a single node through its operation pipeline.
func (e *Engine) executeNode(ctx *Context, node *Node) *Result {
	var content []byte
	var sourceChecksum, targetChecksum string

	// Pre-read source content if pipeline needs it
	if node.Source != "" && e.needsContent(node) {
		var err error
		content, err = os.ReadFile(node.Source)
		if err != nil {
			return &Result{
				NodeID: node.ID,
				Status: StatusFailed,
				Error:  fmt.Errorf("read source %s: %w", node.Source, err),
			}
		}
		sourceChecksum = checksumBytes(content)
	}

	// Execute operation pipeline
	for _, opName := range node.Operations {
		op, ok := e.registry.Get(opName)
		if !ok {
			return &Result{
				NodeID: node.ID,
				Status: StatusFailed,
				Error:  fmt.Errorf("unknown operation: %s", opName),
			}
		}

		var err error
		switch typed := op.(type) {
		case Transform:
			content, err = typed.Transform(ctx, node, content)
		case Writer:
			targetChecksum, err = typed.Write(ctx, node, content)
		case Direct:
			err = typed.Execute(ctx, node)
		default:
			err = fmt.Errorf("operation %s does not implement Transform, Writer, or Direct", opName)
		}

		if err != nil {
			return &Result{
				NodeID: node.ID,
				Status: StatusFailed,
				Error:  fmt.Errorf("%s: %w", opName, err),
			}
		}
	}

	return &Result{
		NodeID:         node.ID,
		Status:         StatusCompleted,
		SourceChecksum: sourceChecksum,
		TargetChecksum: targetChecksum,
	}
}

// needsContent returns true if the node's first operation requires pre-read
// source content (transforms and writers, but not direct operations like link).
func (e *Engine) needsContent(node *Node) bool {
	if len(node.Operations) == 0 {
		return false
	}
	first := node.Operations[0]
	op, ok := e.registry.Get(first)
	if !ok {
		return false
	}
	// Direct operations don't need content pre-read; transforms and writers do
	return op.Category() != OpDirect
}

// orderNodes returns nodes in execution order. When edges define ordering
// constraints, topological sort is used. Otherwise, nodes are ordered by
// path depth (breadth-first).
func (e *Engine) orderNodes(graph *Graph) []*Node {
	if len(graph.Edges) > 0 {
		return e.topologicalSort(graph)
	}
	return e.sortByDepth(graph.Nodes)
}

// topologicalSort orders nodes respecting edge constraints (Kahn's algorithm).
func (e *Engine) topologicalSort(graph *Graph) []*Node {
	// Build adjacency and in-degree maps
	nodeMap := make(map[string]*Node)
	inDegree := make(map[string]int)
	adj := make(map[string][]string)

	for _, node := range graph.Nodes {
		nodeMap[node.ID] = node
		inDegree[node.ID] = 0
	}

	for _, edge := range graph.Edges {
		// Only consider ordering edges between nodes in this graph
		if _, fromOK := nodeMap[edge.From]; !fromOK {
			continue
		}
		if _, toOK := nodeMap[edge.To]; !toOK {
			continue
		}
		adj[edge.From] = append(adj[edge.From], edge.To)
		inDegree[edge.To]++
	}

	// Start with nodes that have no incoming edges
	var queue []string
	for _, node := range graph.Nodes {
		if inDegree[node.ID] == 0 {
			queue = append(queue, node.ID)
		}
	}

	var sorted []*Node
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		sorted = append(sorted, nodeMap[id])

		for _, neighbor := range adj[id] {
			inDegree[neighbor]--
			if inDegree[neighbor] == 0 {
				queue = append(queue, neighbor)
			}
		}
	}

	// If not all nodes were sorted (cycle), append remaining
	if len(sorted) < len(graph.Nodes) {
		visited := make(map[string]bool)
		for _, node := range sorted {
			visited[node.ID] = true
		}
		for _, node := range graph.Nodes {
			if !visited[node.ID] {
				sorted = append(sorted, node)
			}
		}
	}

	return sorted
}

// sortByDepth sorts nodes by target path depth (breadth-first order).
func (e *Engine) sortByDepth(nodes []*Node) []*Node {
	sorted := make([]*Node, len(nodes))
	copy(sorted, nodes)

	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			depthI := pathDepth(sorted[i])
			depthJ := pathDepth(sorted[j])
			if depthI > depthJ {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	return sorted
}

// pathDepth returns the directory depth of a node's target path.
func pathDepth(node *Node) int {
	if node.Target == "" {
		return 0
	}
	return strings.Count(node.Target, string(filepath.Separator))
}

// checksumBytes computes SHA256 of content and returns "sha256:<hex>".
func checksumBytes(content []byte) string {
	hash := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(hash[:])
}
