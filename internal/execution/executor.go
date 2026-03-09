// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package execution

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/pkg/op"
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
	// Root is the authority boundary for provider operations. When set, a
	// recovery.Site is created and placed on the execution Context.
	Root string

	// DryRun prevents filesystem modifications.
	DryRun bool

	// Writer receives user-facing output.
	Writer io.Writer

	// Data holds tool-provided context (template vars, SOPS config, etc.).
	Data map[string]any

	// Platform provides platform abstractions (package manager, service manager)
	// to action providers. Create with platform.New().
	Platform *op.Platform

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

// newContext creates an execution Context from the executor's options.
// Root is mandatory — an op.Root is opened for OS-enforced confinement
// and a RecoverySite is created. The caller must defer Root.Close().
func (e *GraphExecutor) newContext(ctx context.Context) (*op.Context, error) {

	if e.options.Root == "" {
		return nil, fmt.Errorf("executor: Root is required")
	}

	root, err := op.NewConfinedRoot(e.options.Root)
	if err != nil {
		return nil, fmt.Errorf("open root %s: %w", e.options.Root, err)
	}

	execCtx := &op.Context{
		Context:  ctx,
		Root:     root,
		DryRun:   e.options.DryRun,
		Writer:   e.options.Writer,
		Data:     e.options.Data,
		Platform: e.options.Platform,
		Thread:   e.newThread(),
	}
	execCtx.RecoverySite = op.NewRecoverySite(*execCtx)

	return execCtx, nil
}

// newThread creates a Starlark thread for callable initialization during
// execution. Print output goes to the executor's writer.
func (e *GraphExecutor) newThread() *starlark.Thread {
	return &starlark.Thread{
		Name: "executor",
		Print: func(_ *starlark.Thread, msg string) {
			_, _ = fmt.Fprintln(e.options.Writer, msg)
		},
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
func (e *GraphExecutor) Run(ctx context.Context, g *op.Graph) error {
	if g.State != op.StatePending {
		return fmt.Errorf("graph already executed (state: %s)", g.State)
	}

	if len(g.Phases) > 0 {
		return e.RunPhased(ctx, g)
	}

	return e.runFlat(ctx, g)
}

// runFlat executes all nodes in topological order without phase boundaries.
func (e *GraphExecutor) runFlat(ctx context.Context, g *op.Graph) error {
	ordered := OrderNodes(g.Nodes, g.Edges)

	execCtx, err := e.newContext(ctx)
	if err != nil {
		return err
	}
	defer execCtx.Root.Close()
	execCtx.Catalog = g.Catalog
	execCtx.Graph = g

	// Create a fresh ActionRegistry with per-graph provider instances
	// and hydrate any stub actions from deserialized graphs.
	if err := e.hydrateProviders(g, *execCtx); err != nil {
		return err
	}

	results := make(map[string]any)
	stack := op.NewRecoveryStack()
	var nodeResults []*NodeResult

	for _, node := range ordered {
		result := e.executeNode(execCtx, node, results, stack)
		nodeResults = append(nodeResults, result)

		if result.Status == ResultFailed && e.options.ConflictResolution == ResolutionStop {
			ApplyResults(g, nodeResults)
			g.ComputeSummary()
			g.State = op.StateFailed
			return result.Error
		}
	}

	// Apply results and update state
	ApplyResults(g, nodeResults)
	g.ComputeSummary()

	if g.Summary.Failed > 0 {
		g.State = op.StateFailed
	} else {
		g.State = op.StateExecuted
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
func (e *GraphExecutor) RunPhased(ctx context.Context, g *op.Graph) error { //nolint:gocognit,gocyclo // complexity is inherent to the algorithm
	execCtx, err := e.newContext(ctx)
	if err != nil {
		return err
	}
	defer execCtx.Root.Close()
	execCtx.Catalog = g.Catalog
	execCtx.Graph = g

	// Create a fresh ActionRegistry with per-graph provider instances
	// and hydrate any stub actions from deserialized graphs.
	if err := e.hydrateProviders(g, *execCtx); err != nil {
		return err
	}

	// Collect compensating phase IDs so we skip them in the forward pass.
	compensateIDs := make(map[string]bool)
	for _, p := range g.Phases {
		if p.Compensate != "" {
			compensateIDs[p.Compensate] = true
		}
	}

	// Build the forward phase list (excludes compensating phases).
	var forwardPhases []*op.Phase
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
	recoveryStack := op.NewRecoveryStack()
	results := make(map[string]any)

	for _, phase := range forwardPhases {
		err := e.executePhase(execCtx, g, phase, results, recoveryStack)
		if err != nil {
			// Phase failed after retries — unwind
			phase.Status = op.PhaseFailed

			// Mark remaining forward phases as skipped
			started := false
			for _, p := range forwardPhases {
				if p.ID == phase.ID {
					started = true
					continue
				}
				if started && p.Status == op.PhasePending {
					p.Status = op.PhaseSkipped
				}
			}

			// Unwind node-level recovery stack
			unwindErr := recoveryStack.Unwind()

			// Execute compensating phases in LIFO order
			var rollbackLog []op.RollbackEntry
			for i := len(phaseStack) - 1; i >= 0; i-- {
				pr := phaseStack[i]
				rollback := op.RollbackEntry{
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
				compStack := op.NewRecoveryStack()
				if compErr := e.ExecutePhaseInner(execCtx, g, compPhase, compResults, compStack); compErr != nil {
					rollback.Status = "failed"
					rollback.Error = compErr.Error()
				} else {
					rollback.Status = "completed"
				}

				for _, p := range g.Phases {
					if p.ID == pr.phaseID {
						p.Status = op.PhaseRolledBack
						break
					}
				}

				rollbackLog = append(rollbackLog, rollback)
			}

			g.Rollback = rollbackLog
			g.ComputeSummary()
			g.State = op.StateFailed
			phaseErr := fmt.Errorf("phase %s failed: %w", phase.Name, err)
			return errors.Join(phaseErr, unwindErr)
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
	g.State = op.StateExecuted
	return nil
}

// executePhase runs a single phase with retry logic.
func (e *GraphExecutor) executePhase(ctx *op.Context, g *op.Graph, phase *op.Phase, results map[string]any, stack *op.RecoveryStack) error {
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

		attemptRecord := op.Attempt{
			Number:    attempt + 1,
			Timestamp: time.Now().Format(time.RFC3339),
		}

		if err == nil {
			attemptRecord.Status = "completed"
			phase.Attempts = append(phase.Attempts, attemptRecord)
			phase.Status = op.PhaseCompleted
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
func (e *GraphExecutor) resetPhaseNodes(g *op.Graph, phase *op.Phase) {
	nodeSet := make(map[string]bool, len(phase.NodeIDs))
	for _, id := range phase.NodeIDs {
		nodeSet[id] = true
	}
	for _, n := range g.Nodes {
		if nodeSet[n.ID] {
			n.Status = op.StatusPending
			n.Error = ""
			n.Timestamp = ""
		}
	}
}

// ExecutePhaseInner runs the inner nodes of a phase.
func (e *GraphExecutor) ExecutePhaseInner(ctx *op.Context, g *op.Graph, phase *op.Phase, results map[string]any, stack *op.RecoveryStack) error {
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
func (e *GraphExecutor) RunNodes(ctx context.Context, nodes []*op.Node, edges []op.Edge) ([]*NodeResult, error) {
	ordered := OrderNodes(nodes, edges)

	execCtx, err := e.newContext(ctx)
	if err != nil {
		return nil, err
	}
	defer execCtx.Root.Close()

	// Hydrate stub actions and inject context into provider-backed actions.
	freshReg := op.NewActionRegistry()
	op.InitAll(freshReg, *execCtx)
	for _, n := range nodes {
		if op.IsStubAction(n.Action) {
			name := n.ActionName()
			if name == "" {
				continue
			}
			if action, ok := freshReg.Get(name); ok {
				n.Action = action
			}
		} else {
			op.InitActionProvider(n.Action, *execCtx)
		}
	}

	results := make(map[string]any)
	stack := op.NewRecoveryStack()
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
func (e *GraphExecutor) executeNode(ctx *op.Context, node *op.Node, results map[string]any, stack *op.RecoveryStack) *NodeResult {
	if node.Action == nil {
		node.Status = op.StatusFailed
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

	result, complement, err := node.Action.Do(ctx, slots)
	if err != nil {
		e.hooks.FireNodeComplete(ctx, node.ID, nil, err)
		node.Status = op.StatusFailed
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

	stack.PushAction(ctx, node.Action, complement)

	node.Status = op.StatusCompleted

	return &NodeResult{
		NodeID: node.ID,
		Status: ResultCompleted,
	}
}

// FillSlotsFromData fills unfilled slots from Context.Data.
// Slots already set by the caller or resolved from promises are not overwritten.
func FillSlotsFromData(slots, data map[string]any) {
	for key, value := range data {
		if _, exists := slots[key]; !exists {
			slots[key] = value
		}
	}
}

// OrderNodes returns nodes in execution order.
// Nodes with edges are topologically sorted; nodes without edges are sorted by path depth.
func OrderNodes(nodes []*op.Node, edges []op.Edge) []*op.Node {
	if len(edges) > 0 {
		return topologicalSortNodes(nodes, edges)
	}
	return sortNodesByDepth(nodes)
}

// topologicalSortNodes orders nodes respecting edge constraints (Kahn's algorithm).
func topologicalSortNodes(nodes []*op.Node, edges []op.Edge) []*op.Node { //nolint:gocognit // complexity is inherent to the algorithm
	nodeMap := make(map[string]*op.Node)
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

	var sorted []*op.Node
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
func sortNodesByDepth(nodes []*op.Node) []*op.Node {
	sorted := make([]*op.Node, len(nodes))
	copy(sorted, nodes)

	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			pathI, _ := sorted[i].GetSlot("path").(string) //nolint:errcheck // type assertion ok
			pathJ, _ := sorted[j].GetSlot("path").(string) //nolint:errcheck // type assertion ok
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

// hydrateProviders creates a fresh ActionRegistry with per-graph provider instances.
// Stub actions (from deserialized graphs) are replaced with real actions.
// Non-stub actions (from planning) get their provider's context updated.
func (e *GraphExecutor) hydrateProviders(g *op.Graph, ctx op.Context) error {
	freshReg := op.NewActionRegistry()
	op.InitAll(freshReg, ctx)

	for _, n := range g.Nodes {
		if op.IsStubAction(n.Action) {
			name := n.ActionName()
			if name == "" {
				continue
			}
			action, ok := freshReg.Get(name)
			if !ok {
				return fmt.Errorf("hydrate: unknown action %q on node %q", name, n.ID)
			}
			n.Action = action
		} else {
			op.InitActionProvider(n.Action, ctx)
		}
	}
	return nil
}

// ApplyResults updates node states from execution results.
func ApplyResults(g *op.Graph, results []*NodeResult) {
	resultMap := make(map[string]*NodeResult)
	for _, r := range results {
		resultMap[r.NodeID] = r
	}

	for _, n := range g.Nodes {
		if r, ok := resultMap[n.ID]; ok {
			switch r.Status {
			case ResultCompleted:
				n.Status = op.StatusCompleted
			case ResultSkipped:
				n.Status = op.StatusSkipped
			case ResultFailed:
				n.Status = op.StatusFailed
				if r.Error != nil {
					n.Error = r.Error.Error()
				}
			}
			n.Timestamp = time.Now().Format(time.RFC3339)
		}
	}
}
