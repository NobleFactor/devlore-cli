// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package execution

import (
	"context"
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

// NodeResult represents the outcome of executing a single node.
type NodeResult struct {
	NodeID  string
	Status  ResultStatus
	Error   error
	Message string
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

	// Writer receives user-facing output.
	Writer io.Writer

	// Data holds tool-provided context (template vars, SOPS config, etc.).
	Data map[string]any

	// ConflictResolution specifies how to handle conflicts detected during preflight.
	ConflictResolution ConflictResolution

	// BackupSuffix is appended to backup filenames (default: ".writ-backup").
	BackupSuffix string
}

// GraphExecutor executes action graphs.
type GraphExecutor struct {
	options ExecutorOptions
	hooks   *HookRegistry
}

// NewGraphExecutor creates an executor with the given options.
func NewGraphExecutor(opts ExecutorOptions) *GraphExecutor {
	if opts.Writer == nil {
		opts.Writer = os.Stdout
	}
	if opts.BackupSuffix == "" {
		opts.BackupSuffix = ".writ-backup"
	}
	return &GraphExecutor{
		options: opts,
	}
}

// SetHooks sets the lifecycle hook registry for this executor.
func (e *GraphExecutor) SetHooks(hooks *HookRegistry) {
	e.hooks = hooks
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
func (e *GraphExecutor) runFlat(ctx context.Context, g *Graph) error {
	ordered := OrderNodes(g.Nodes, g.Edges)

	execCtx := &Context{
		Context: ctx,
		DryRun:  e.options.DryRun,
		Writer:  e.options.Writer,
		Data:    e.options.Data,
		Graph:   g,
	}

	results := make(map[string]any)
	stack := &RecoveryStack{}
	var nodeResults []*NodeResult

	for _, node := range ordered {
		result := e.executeNode(execCtx, node, results, stack)
		nodeResults = append(nodeResults, result)

		if result.Status == ResultFailed && e.options.ConflictResolution == ResolutionStop {
			g.ApplyResults(nodeResults)
			g.ComputeSummary()
			g.State = StateFailed
			return result.Error
		}
	}

	// Apply results and update state
	g.ApplyResults(nodeResults)
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
		Writer:  e.options.Writer,
		Data:    e.options.Data,
		Graph:   g,
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

	// Phase-level recovery stack for compensating phases.
	type phaseRecovery struct {
		phaseID      string
		phaseName    string
		compensateID string
		state        map[string]any
	}
	var phaseStack []phaseRecovery

	// Node-level recovery stack spans all phases.
	nodeStack := &RecoveryStack{}
	results := make(map[string]any)

	for _, phase := range forwardPhases {
		err := e.executePhase(execCtx, g, phase, results, nodeStack)
		if err != nil {
			// Phase failed after retries — unwind
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

			// Unwind node-level recovery stack
			nodeStack.Unwind(execCtx)

			// Execute compensating phases in LIFO order
			var rollbackLog []RollbackEntry
			for i := len(phaseStack) - 1; i >= 0; i-- {
				pr := phaseStack[i]
				rollback := RollbackEntry{
					Phase:      pr.phaseName,
					Compensate: pr.compensateID,
				}

				if pr.compensateID == "" {
					rollback.Status = "skipped"
					rollbackLog = append(rollbackLog, rollback)
					continue
				}

				compPhase := g.PhaseByID(pr.compensateID)
				if compPhase == nil {
					rollback.Status = "failed"
					rollback.Error = fmt.Sprintf("compensating phase %s not found", pr.compensateID)
					rollbackLog = append(rollbackLog, rollback)
					continue
				}

				compResults := make(map[string]any)
				compStack := &RecoveryStack{}
				if compErr := e.ExecutePhaseInner(execCtx, g, compPhase, compResults, compStack); compErr != nil {
					rollback.Status = "failed"
					rollback.Error = compErr.Error()
				} else {
					rollback.Status = "completed"
				}

				for _, p := range g.Phases {
					if p.ID == pr.phaseID {
						p.Status = PhaseRolledBack
						break
					}
				}

				rollbackLog = append(rollbackLog, rollback)
			}

			g.Rollback = rollbackLog
			g.ComputeSummary()
			g.State = StateFailed
			return fmt.Errorf("phase %s failed: %w", phase.Name, err)
		}

		// Push phase-level recovery entry
		if phase.Compensate != "" {
			phaseStack = append(phaseStack, phaseRecovery{
				phaseID:      phase.ID,
				phaseName:    phase.Name,
				compensateID: phase.Compensate,
				state:        phase.State,
			})
		}
	}

	g.ComputeSummary()
	g.State = StateExecuted
	return nil
}

// executePhase runs a single phase with retry logic.
func (e *GraphExecutor) executePhase(ctx *Context, g *Graph, phase *Phase, results map[string]any, stack *RecoveryStack) error {
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

		err := e.ExecutePhaseInner(ctx, g, phase, results, stack)

		attemptRecord := Attempt{
			Number:    attempt + 1,
			Timestamp: time.Now().Format(time.RFC3339),
		}

		if err == nil {
			attemptRecord.Status = "completed"
			phase.Attempts = append(phase.Attempts, attemptRecord)
			phase.Status = PhaseCompleted
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

// ExecutePhaseInner runs the inner nodes of a phase.
func (e *GraphExecutor) ExecutePhaseInner(ctx *Context, g *Graph, phase *Phase, results map[string]any, stack *RecoveryStack) error {
	phaseNodes, phaseEdges := g.CollectPhaseNodes(phase)
	ordered := OrderNodes(phaseNodes, phaseEdges)

	e.hooks.FirePhaseStart(ctx, phase.ID)

	for _, node := range ordered {
		result := e.executeNode(ctx, node, results, stack)
		if result.Status == ResultFailed {
			e.hooks.FirePhaseComplete(ctx, phase.ID, result.Error)
			return result.Error
		}
	}

	e.hooks.FirePhaseComplete(ctx, phase.ID, nil)
	return nil
}

// RunNodes executes a slice of nodes with the given edges.
// This is a lower-level API for callers that don't have a full Graph.
func (e *GraphExecutor) RunNodes(ctx context.Context, nodes []*Node, edges []Edge) ([]*NodeResult, error) {
	ordered := OrderNodes(nodes, edges)

	execCtx := &Context{
		Context: ctx,
		DryRun:  e.options.DryRun,
		Writer:  e.options.Writer,
		Data:    e.options.Data,
	}

	results := make(map[string]any)
	stack := &RecoveryStack{}
	var nodeResults []*NodeResult

	for _, node := range ordered {
		result := e.executeNode(execCtx, node, results, stack)
		nodeResults = append(nodeResults, result)

		if result.Status == ResultFailed && e.options.ConflictResolution == ResolutionStop {
			return nodeResults, result.Error
		}
	}

	return nodeResults, nil
}

// executeNode resolves slots, calls Do, stores the result, and pushes a recovery entry.
func (e *GraphExecutor) executeNode(ctx *Context, node *Node, results map[string]any, stack *RecoveryStack) *NodeResult {
	if node.Action == nil {
		node.Status = StatusFailed
		return &NodeResult{
			NodeID: node.ID,
			Status: ResultFailed,
			Error:  fmt.Errorf("node %s has no action", node.ID),
		}
	}

	actionName := node.ActionName()

	// Resolve promise slots from upstream results
	slots := node.ResolvedSlots(results)

	// Fill unfilled slots from Context.Data
	FillSlotsFromData(slots, e.options.Data)

	ctx.NodeID = node.ID

	e.hooks.FireNodeStart(ctx, node.ID, slots)

	result, undoState, err := node.Action.Do(ctx, slots)
	if err != nil {
		e.hooks.FireNodeComplete(ctx, node.ID, nil, err)
		node.Status = StatusFailed
		return &NodeResult{
			NodeID: node.ID,
			Status: ResultFailed,
			Error:  fmt.Errorf("%s: %w", actionName, err),
		}
	}

	e.hooks.FireNodeComplete(ctx, node.ID, result, nil)

	// Store result for downstream promise resolution
	if result != nil {
		results[node.ID] = result
	}

	// Push recovery entry only for compensable actions.
	if _, ok := node.Action.(CompensableAction); ok {
		stack.Push(RecoveryEntry{Node: node, UndoState: undoState})
	}

	node.Status = StatusCompleted

	return &NodeResult{
		NodeID: node.ID,
		Status: ResultCompleted,
	}
}

// FillSlotsFromData fills unfilled slots from Context.Data.
// Slots already set by the caller or resolved from promises are not overwritten.
func FillSlotsFromData(slots map[string]any, data map[string]any) {
	for key, value := range data {
		if _, exists := slots[key]; !exists {
			slots[key] = value
		}
	}
}

// OrderNodes returns nodes in execution order.
// Nodes with edges are topologically sorted; nodes without edges are sorted by path depth.
func OrderNodes(nodes []*Node, edges []Edge) []*Node {
	if len(edges) > 0 {
		return topologicalSortNodes(nodes, edges)
	}
	return sortNodesByDepth(nodes)
}

// topologicalSortNodes orders nodes respecting edge constraints (Kahn's algorithm).
func topologicalSortNodes(nodes []*Node, edges []Edge) []*Node {
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

// sortNodesByDepth sorts nodes by target path depth.
func sortNodesByDepth(nodes []*Node) []*Node {
	sorted := make([]*Node, len(nodes))
	copy(sorted, nodes)

	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			pathI, _ := sorted[i].GetSlot("path").(string)
			pathJ, _ := sorted[j].GetSlot("path").(string)
			depthI := pathDepth(pathI)
			depthJ := pathDepth(pathJ)
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

