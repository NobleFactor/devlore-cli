// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package execution

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

// ResultStatus represents the execution status of a node.
type ResultStatus int

const (
	// ResultPending means the node has not been processed yet.
	ResultPending ResultStatus = iota
	// ResultRunning means the node is currently executing.
	ResultRunning
	// ResultCompleted means the node executed successfully.
	ResultCompleted
	// ResultFailed means the node encountered an error.
	ResultFailed
	// ResultSkipped means the node was skipped (conflict, already deployed, etc.).
	ResultSkipped
)

// String returns a human-readable status label.
func (s ResultStatus) String() string {
	switch s {
	case ResultPending:
		return "pending"
	case ResultRunning:
		return "running"
	case ResultCompleted:
		return "completed"
	case ResultFailed:
		return "failed"
	case ResultSkipped:
		return "skipped"
	default:
		return "unknown"
	}
}

// Result represents the outcome of executing a single node.
type Result struct {
	NodeID         string
	Status         ResultStatus
	Error          error
	Message        string
	SourceChecksum string
	TargetChecksum string
}

// ConflictResolution specifies how to handle conflicts during execution.
type ConflictResolution int

const (
	// ResolutionStop aborts execution on first conflict.
	ResolutionStop ConflictResolution = iota
	// ResolutionBackup moves conflicting files to timestamped backups.
	ResolutionBackup
	// ResolutionOverwrite removes conflicting files without backup.
	ResolutionOverwrite
	// ResolutionSkip skips conflicting files and continues.
	ResolutionSkip
)

// ExecutorOptions configures GraphExecutor behavior.
type ExecutorOptions struct {
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

// GraphExecutor executes operation graphs.
type GraphExecutor struct {
	registry *OperationRegistry
	options  ExecutorOptions
}

// NewGraphExecutor creates an executor with the given registry and options.
func NewGraphExecutor(registry *OperationRegistry, opts ExecutorOptions) *GraphExecutor {
	if opts.Logger == nil {
		opts.Logger = os.Stdout
	}
	if opts.BackupSuffix == "" {
		opts.BackupSuffix = ".writ-backup"
	}
	return &GraphExecutor{
		registry: registry,
		options:  opts,
	}
}

// Run executes all nodes in the graph, respecting ordering constraints.
// Nodes are processed in topological order when edges define dependencies.
// Returns results for each node.
func (e *GraphExecutor) Run(ctx context.Context, g *Graph) error {
	if g.State != StatePending {
		return fmt.Errorf("graph already executed (state: %s)", g.State)
	}

	ordered := e.orderNodes(g.Nodes, g.Edges)

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

		if result.Status == ResultFailed && e.options.ConflictResolution == ResolutionStop {
			g.ApplyResults(results)
			g.ComputeSummary()
			g.State = StateFailed
			return result.Error
		}
	}

	// Apply results and update state
	g.ApplyResults(results)
	g.ComputeSummary()

	if g.Summary.Failed > 0 {
		g.State = StateFailed
	} else {
		g.State = StateExecuted
	}

	return nil
}

// RunNodes executes a slice of executables with the given edges.
// This is a lower-level API for callers that don't have a full Graph.
func (e *GraphExecutor) RunNodes(ctx context.Context, nodes []Executable, edges []Edge) ([]*Result, error) {
	// Convert to internal node pointers for ordering
	ordered := e.orderExecutables(nodes, edges)

	execCtx := &Context{
		Context: ctx,
		DryRun:  e.options.DryRun,
		Logger:  e.options.Logger,
		Data:    e.options.Data,
	}

	var results []*Result
	for _, node := range ordered {
		result := e.executeExecutable(execCtx, node)
		results = append(results, result)

		if result.Status == ResultFailed && e.options.ConflictResolution == ResolutionStop {
			return results, result.Error
		}
	}

	return results, nil
}

// executeNode processes a single node through its operation pipeline.
func (e *GraphExecutor) executeNode(ctx *Context, node *Node) *Result {
	return e.executeExecutable(ctx, node)
}

// executeExecutable processes any Executable through its operation pipeline.
func (e *GraphExecutor) executeExecutable(ctx *Context, node Executable) *Result {
	var content []byte
	var sourceChecksum, targetChecksum string

	// Pre-read source content if pipeline needs it
	source := node.GetSlot("source")
	if source != "" && e.needsContent(node) {
		var err error
		content, err = os.ReadFile(source)
		if err != nil {
			return &Result{
				NodeID: node.GetID(),
				Status: ResultFailed,
				Error:  fmt.Errorf("read source %s: %w", source, err),
			}
		}
		sourceChecksum = ChecksumBytes(content)
	}

	// Execute operation pipeline
	for _, opName := range node.GetOperations() {
		op, ok := e.registry.Get(opName)
		if !ok {
			return &Result{
				NodeID: node.GetID(),
				Status: ResultFailed,
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
				NodeID: node.GetID(),
				Status: ResultFailed,
				Error:  fmt.Errorf("%s: %w", opName, err),
			}
		}
	}

	return &Result{
		NodeID:         node.GetID(),
		Status:         ResultCompleted,
		SourceChecksum: sourceChecksum,
		TargetChecksum: targetChecksum,
	}
}

// needsContent returns true if the node's first operation requires pre-read
// source content (transforms and writers, but not direct operations like link).
func (e *GraphExecutor) needsContent(node Executable) bool {
	ops := node.GetOperations()
	if len(ops) == 0 {
		return false
	}
	first := ops[0]
	op, ok := e.registry.Get(first)
	if !ok {
		return false
	}
	return op.Category() != OpDirect
}

// orderNodes returns nodes in execution order.
func (e *GraphExecutor) orderNodes(nodes []*Node, edges []Edge) []*Node {
	if len(edges) > 0 {
		return e.topologicalSortNodes(nodes, edges)
	}
	return e.sortNodesByDepth(nodes)
}

// orderExecutables returns executables in execution order.
func (e *GraphExecutor) orderExecutables(nodes []Executable, edges []Edge) []Executable {
	if len(edges) > 0 {
		return e.topologicalSort(nodes, edges)
	}
	return e.sortByDepth(nodes)
}

// topologicalSortNodes orders nodes respecting edge constraints (Kahn's algorithm).
func (e *GraphExecutor) topologicalSortNodes(nodes []*Node, edges []Edge) []*Node {
	nodeMap := make(map[string]*Node)
	inDegree := make(map[string]int)
	adj := make(map[string][]string)

	for _, node := range nodes {
		nodeMap[node.ID] = node
		inDegree[node.ID] = 0
	}

	for _, edge := range edges {
		if _, fromOK := nodeMap[edge.From]; !fromOK {
			continue
		}
		if _, toOK := nodeMap[edge.To]; !toOK {
			continue
		}
		adj[edge.From] = append(adj[edge.From], edge.To)
		inDegree[edge.To]++
	}

	var queue []string
	for _, node := range nodes {
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

	if len(sorted) < len(nodes) {
		visited := make(map[string]bool)
		for _, node := range sorted {
			visited[node.ID] = true
		}
		for _, node := range nodes {
			if !visited[node.ID] {
				sorted = append(sorted, node)
			}
		}
	}

	return sorted
}

// topologicalSort orders executables respecting edge constraints.
func (e *GraphExecutor) topologicalSort(nodes []Executable, edges []Edge) []Executable {
	nodeMap := make(map[string]Executable)
	inDegree := make(map[string]int)
	adj := make(map[string][]string)

	for _, node := range nodes {
		nodeMap[node.GetID()] = node
		inDegree[node.GetID()] = 0
	}

	for _, edge := range edges {
		if _, fromOK := nodeMap[edge.From]; !fromOK {
			continue
		}
		if _, toOK := nodeMap[edge.To]; !toOK {
			continue
		}
		adj[edge.From] = append(adj[edge.From], edge.To)
		inDegree[edge.To]++
	}

	var queue []string
	for _, node := range nodes {
		if inDegree[node.GetID()] == 0 {
			queue = append(queue, node.GetID())
		}
	}

	var sorted []Executable
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

	if len(sorted) < len(nodes) {
		visited := make(map[string]bool)
		for _, node := range sorted {
			visited[node.GetID()] = true
		}
		for _, node := range nodes {
			if !visited[node.GetID()] {
				sorted = append(sorted, node)
			}
		}
	}

	return sorted
}

// sortNodesByDepth sorts nodes by target path depth.
func (e *GraphExecutor) sortNodesByDepth(nodes []*Node) []*Node {
	sorted := make([]*Node, len(nodes))
	copy(sorted, nodes)

	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			depthI := pathDepth(sorted[i].GetSlot("path"))
			depthJ := pathDepth(sorted[j].GetSlot("path"))
			if depthI > depthJ {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	return sorted
}

// sortByDepth sorts executables by target path depth.
func (e *GraphExecutor) sortByDepth(nodes []Executable) []Executable {
	sorted := make([]Executable, len(nodes))
	copy(sorted, nodes)

	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			depthI := pathDepth(sorted[i].GetSlot("path"))
			depthJ := pathDepth(sorted[j].GetSlot("path"))
			if depthI > depthJ {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	return sorted
}

// pathDepth returns the directory depth of a path.
func pathDepth(path string) int {
	if path == "" {
		return 0
	}
	return strings.Count(path, string(filepath.Separator))
}

// ChecksumBytes computes SHA256 of content and returns "sha256:<hex>".
func ChecksumBytes(content []byte) string {
	hash := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(hash[:])
}

// ChecksumFile computes SHA256 of a file and returns "sha256:<hex>".
// Returns empty string if the file cannot be read.
func ChecksumFile(path string) string {
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return ChecksumBytes(content)
}
