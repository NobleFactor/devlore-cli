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
	"time"
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
// When the graph has phases, execution is delegated to RunPhased which
// implements the saga pattern with retry and rollback. Otherwise, nodes
// are processed in topological order (flat execution).
func (e *GraphExecutor) Run(ctx context.Context, g *Graph) error {
	if g.State != StatePending {
		return fmt.Errorf("graph already executed (state: %s)", g.State)
	}

	if len(g.Phases) > 0 {
		return e.RunPhased(ctx, g)
	}

	return e.runFlat(ctx, g)
}

// runFlat executes all nodes in topological order without phase boundaries.
// This is the original execution path for non-phased graphs.
func (e *GraphExecutor) runFlat(ctx context.Context, g *Graph) error {
	ordered := e.orderNodes(g.Nodes, g.Edges)

	execCtx := &Context{
		Context: ctx,
		DryRun:  e.options.DryRun,
		Logger:  e.options.Logger,
		Data:    e.options.Data,
	}

	outputs := make(map[string][]byte)
	var results []*Result
	for _, node := range ordered {
		result := e.executeNodeWithOutputs(execCtx, node, g.Edges, outputs)
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

// RunPhased executes a phased graph using the saga pattern.
// Phases are executed in order. Each phase runs its inner nodes via
// topological sort. On failure, completed phases are compensated in
// LIFO order (rollback).
//
// Phases referenced as compensating actions (via Compensate fields) are
// skipped during the forward pass — they execute only during rollback.
func (e *GraphExecutor) RunPhased(ctx context.Context, g *Graph) error {
	execCtx := &Context{
		Context: ctx,
		DryRun:  e.options.DryRun,
		Logger:  e.options.Logger,
		Data:    e.options.Data,
	}

	// Collect compensating phase IDs so we skip them in the forward pass.
	compensateIDs := make(map[string]bool)
	for _, p := range g.Phases {
		if p.Compensate != "" {
			compensateIDs[p.Compensate] = true
		}
	}

	// Build the forward phase list (excludes compensating phases).
	var forwardPhases []*Phase
	for _, p := range g.Phases {
		if !compensateIDs[p.ID] {
			forwardPhases = append(forwardPhases, p)
		}
	}

	stack := &RecoveryStack{}
	var rollbackLog []RollbackEntry

	for _, phase := range forwardPhases {
		err := e.executePhase(execCtx, g, phase, stack)
		if err != nil {
			// Phase failed after retries — unwind completed phases
			phase.Status = PhaseFailed

			// Mark remaining forward phases as skipped
			started := false
			for _, p := range forwardPhases {
				if p.ID == phase.ID {
					started = true
					continue
				}
				if started && p.Status == PhasePending {
					p.Status = PhaseSkipped
				}
			}

			// Unwind the recovery stack
			rollbackLog = e.unwindStack(execCtx, g, stack)

			g.Rollback = rollbackLog
			g.ComputeSummary()
			g.State = StateFailed
			return fmt.Errorf("phase %s failed: %w", phase.Name, err)
		}
	}

	g.ComputeSummary()
	g.State = StateExecuted
	return nil
}

// executePhase runs a single phase with retry logic.
// On success, pushes a recovery entry onto the stack.
// On failure (retries exhausted), returns the error.
func (e *GraphExecutor) executePhase(ctx *Context, g *Graph, phase *Phase, stack *RecoveryStack) error {
	maxAttempts := 1
	if phase.Retry != nil {
		maxAttempts += phase.Retry.MaxAttempts
	}

	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		// Apply backoff delay before retries (not before first attempt)
		if attempt > 0 && phase.Retry != nil {
			delay := phase.Retry.ComputeDelay(attempt - 1)
			if delay > 0 {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(delay):
				}
			}
		}

		// Reset inner node statuses for retry
		if attempt > 0 {
			e.resetPhaseNodes(g, phase)
		}

		err := e.ExecutePhaseInner(ctx, g, phase)

		attemptRecord := Attempt{
			Number:    attempt + 1,
			Timestamp: time.Now().Format(time.RFC3339),
		}

		if err == nil {
			attemptRecord.Status = "completed"
			phase.Attempts = append(phase.Attempts, attemptRecord)
			phase.Status = PhaseCompleted

			// Push recovery entry if phase has a compensating action
			if phase.Compensate != "" {
				compensateID := phase.Compensate
				phaseName := phase.Name
				phaseState := phase.State

				stack.Push(RecoveryEntry{
					PhaseID:   phase.ID,
					PhaseName: phaseName,
					State:     phaseState,
					Compensate: func(ctx *Context) error {
						// Find the compensating phase in the graph
						for _, p := range g.Phases {
							if p.ID == compensateID {
								return e.ExecutePhaseInner(ctx, g, p)
							}
						}
						return fmt.Errorf("compensating phase %s not found", compensateID)
					},
				})
			}

			return nil
		}

		attemptRecord.Status = "failed"
		attemptRecord.Error = err.Error()
		phase.Attempts = append(phase.Attempts, attemptRecord)
		lastErr = err
	}

	return lastErr
}

// resetPhaseNodes resets inner node statuses back to pending for retry.
func (e *GraphExecutor) resetPhaseNodes(g *Graph, phase *Phase) {
	nodeSet := make(map[string]bool, len(phase.NodeIDs))
	for _, id := range phase.NodeIDs {
		nodeSet[id] = true
	}
	for _, n := range g.Nodes {
		if nodeSet[n.ID] {
			n.Status = StatusPending
			n.Error = ""
			n.Timestamp = ""
		}
	}
}

// unwindStack pops the recovery stack and executes compensating actions in LIFO order.
// Returns a log of rollback entries for the receipt.
func (e *GraphExecutor) unwindStack(ctx *Context, g *Graph, stack *RecoveryStack) []RollbackEntry {
	entries := stack.Entries()
	var log []RollbackEntry

	// Process in LIFO order (most recent first)
	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]
		rollback := RollbackEntry{
			Phase:      entry.PhaseName,
			Compensate: entry.PhaseID + ".compensate",
		}

		if entry.Compensate == nil {
			rollback.Status = "skipped"
			log = append(log, rollback)
			continue
		}

		if err := entry.Compensate(ctx); err != nil {
			rollback.Status = "failed"
			rollback.Error = err.Error()
		} else {
			rollback.Status = "completed"
		}

		// Mark the original phase as rolled back
		for _, p := range g.Phases {
			if p.ID == entry.PhaseID {
				p.Status = PhaseRolledBack
				break
			}
		}

		log = append(log, rollback)
	}

	return log
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

	outputs := make(map[string][]byte)
	var results []*Result
	for _, node := range ordered {
		result := e.executeExecutableWithOutputs(execCtx, node, edges, outputs)
		results = append(results, result)

		if result.Status == ResultFailed && e.options.ConflictResolution == ResolutionStop {
			return results, result.Error
		}
	}

	return results, nil
}

// executeNodeWithOutputs processes a single node, resolving upstream content from edges.
func (e *GraphExecutor) executeNodeWithOutputs(ctx *Context, node *Node, edges []Edge, outputs map[string][]byte) *Result {
	return e.executeExecutableWithOutputs(ctx, node, edges, outputs)
}

// executeExecutableWithOutputs processes any Executable with single-op dispatch.
// Content flows between chained nodes via the outputs map.
func (e *GraphExecutor) executeExecutableWithOutputs(ctx *Context, node Executable, edges []Edge, outputs map[string][]byte) *Result {
	opName := node.GetOperation()
	if opName == "" {
		return &Result{
			NodeID: node.GetID(),
			Status: ResultFailed,
			Error:  fmt.Errorf("node %s has no operation", node.GetID()),
		}
	}

	op, ok := e.registry.Get(opName)
	if !ok {
		return &Result{
			NodeID: node.GetID(),
			Status: ResultFailed,
			Error:  fmt.Errorf("unknown operation: %s", opName),
		}
	}

	var content []byte
	var sourceChecksum, targetChecksum string

	// Resolve input content: check upstream chain first, then read source file
	if upstream := findContentUpstream(node.GetID(), edges, outputs); upstream != nil {
		content = upstream
	} else if op.Category() != OpDirect {
		source := node.GetSlot("source")
		if source != "" {
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
	}

	// Single operation dispatch
	var err error
	switch typed := op.(type) {
	case Transform:
		content, err = typed.Transform(ctx, node, content)
		if err == nil {
			outputs[node.GetID()] = content
		}
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

	return &Result{
		NodeID:         node.GetID(),
		Status:         ResultCompleted,
		SourceChecksum: sourceChecksum,
		TargetChecksum: targetChecksum,
	}
}

// findContentUpstream walks incoming edges to find content from an upstream node.
func findContentUpstream(nodeID string, edges []Edge, outputs map[string][]byte) []byte {
	for _, edge := range edges {
		if edge.To == nodeID {
			if content, ok := outputs[edge.From]; ok {
				return content
			}
		}
	}
	return nil
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
