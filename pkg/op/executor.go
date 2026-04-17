// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"time"

	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/pkg/iox"
	"github.com/NobleFactor/devlore-cli/pkg/op/sops"
)

var (
	ErrNilGraph = errors.New("expected non-nil Graph")
)

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

// Options configures GraphExecutor behavior.
type Options struct {

	// Root is the authority boundary for provider operations. When set, a recovery.Site is created and placed on the
	// execution ExecutionContext.
	Root string

	// BackupSuffix is appended to back up filenames (default: ".<program-name>-backup").
	BackupSuffix string

	// ConflictResolution specifies how to handle conflicts detected during preflight.
	ConflictResolution ConflictResolution

	// Data holds tool-provided context (template vars, etc.).
	Data map[string]any

	// DryRun prevents changes from being applied.
	DryRun bool

	// SopsClient provides SOPS operations. Nil when SOPS is not configured.
	SopsClient *sops.Client

	// Writer receives user-facing output.
	Writer io.Writer
}

// GraphExecutor executes action graphs.
type GraphExecutor struct {
	hooks   *HookRegistry
	options Options
}

// NewGraphExecutor creates an executor with the given options.
//
// Parameters:
//   - opts: the executor configuration.
//
// Returns:
//   - *GraphExecutor: the configured executor.
func NewGraphExecutor(programName string, options Options) (executor *GraphExecutor, err error) {

	if programName == "" {
		programName = path.Base(os.Args[0])
	}

	if options.BackupSuffix == "" {
		options.BackupSuffix = "." + programName + "-backup"
	}

	if options.Writer == nil {
		options.Writer = os.Stdout
	}

	return &GraphExecutor{
		options: options,
	}, nil
}

// newContext creates an execution ExecutionContext from the executor's options.
//
// Root is mandatory — an Root is opened for OS-enforced confinement and a RecoverySite is created. The caller must
// defer Root.Close().
//
// Parameters:
//   - ctx: the parent context for cancellation.
//
// Returns:
//   - *ExecutionContext: the execution context.
//   - error: non-nil if Root is empty or the confined root cannot be opened.
func (e *GraphExecutor) newContext() (*ExecutionContext, error) {

	if e.options.Root == "" {
		return nil, fmt.Errorf("executor: Root is required")
	}

	root, err := NewConfinedRoot(e.options.Root)
	if err != nil {
		return nil, fmt.Errorf("open root %s: %w", e.options.Root, err)
	}

	ctx := NewExecutionContext(root)

	ctx.Registry = NewReceiverRegistry()
	ctx.Data = e.options.Data
	ctx.DryRun = e.options.DryRun
	ctx.RecoverySite = NewRecoverySite(&ctx)
	ctx.Sops = e.options.SopsClient
	ctx.Thread = *e.newThread()
	ctx.Writer = e.options.Writer

	return &ctx, nil
}

// newThread creates a Starlark thread for callable initialization during
// execution. Print output goes to the executor's writer.
//
// Returns:
//   - *starlark.Thread: the configured thread.
func (e *GraphExecutor) newThread() *starlark.Thread {
	return &starlark.Thread{
		Name: "executor",
		Print: func(_ *starlark.Thread, msg string) {
			_, _ = fmt.Fprintln(e.options.Writer, msg)
		},
	}
}

// SetHooks sets the lifecycle hook registry for this executor.
//
// Parameters:
//   - hooks: the hook registry to install.
func (e *GraphExecutor) SetHooks(hooks *HookRegistry) {
	e.hooks = hooks
}

// Run executes all nodes in the graph, respecting ordering constraints.
//
// The graph root is treated as an implicit subgraph. The executor calls executeChildren on the root's children,
// applying Kahn's algorithm at each level and recursing into child subgraphs.
//
// Parameters:
//   - graph: the execution graph to run.
//
// Returns:
//   - any: the terminal node's output value, or nil if no node produced output.
//   - error: non-nil if any node or subgraph fails.
func (e *GraphExecutor) Run(graph *Graph) (any, error) {

	if graph.State != StatePending {
		return nil, fmt.Errorf("graph already executed (state: %s)", graph.State)
	}

	ctx, err := e.newContext()

	if err != nil {
		return nil, err
	}

	defer iox.Close(&err, ctx.Root)
	graph.Rebind(ctx)

	// Pre-flight resolution pass. Iterate the catalog's discovery entries and
	// fail fast if any source resource does not exist on the target machine.
	// Shadowed entries (outputs of nodes in this graph) are skipped — their
	// producer will create them at execution time.
	//
	// This is the link-time symbol resolution pass. See the resource management
	// architecture doc §6.5.
	//
	// Skipped in dry-run mode: dry-run validates graph structure without
	// asserting target-machine state.
	if !ctx.DryRun {
		if err = ResolveResources(graph.Catalog); err != nil {
			graph.State = StateFailed
			return nil, err
		}
	}

	ctx.Results = make(map[string]any)
	stack := NewRecoveryStack()

	result, err := graph.dispatch(e, stack, graph.Root, ctx.Results, nil)

	summary := graph.Summary()

	if err != nil {
		// Unwind the recovery stack in LIFO order so every action that
		// successfully completed before the failure gets its Compensate
		// companion called. The stack was populated on each successful
		// executeNode. Without this, TestCompensation-style rollback
		// never runs.
		if unwindErr := stack.Unwind(); unwindErr != nil {
			err = fmt.Errorf("%w; compensation: %w", err, unwindErr)
		}
		graph.State = StateFailed
		return nil, err
	}

	if summary.Failed() > 0 {
		graph.State = StateFailed
	} else {
		graph.State = StateExecuted
	}

	return result, nil
}

// executeChildren walks a sorted children list, executing each child.
// Node children are executed directly. Subgraph children are entered recursively via executeSubgraph.
//
// Parameters:
//   - graph: the root graph (for context access).
//   - children: the children to execute (declaration order).
//   - edges: ordering constraints between children at this level.
//   - results: the accumulated node results for promise resolution.
//   - stack: the recovery stack for compensation.
//
// Returns:
//   - any: the last node's output value, or nil if no node produced output.
//   - error: non-nil if any child fails.
func (e *GraphExecutor) executeChildren(graph *Graph, children []SubgraphChild, edges []Edge, results map[string]any, stack *RecoveryStack, overrides map[string]SlotValue) (any, error) {

	sorted := SortChildren(children, edges)

	// Topological roots — children with no incoming edges at this level —
	// receive overrides. Non-root children consume their inputs via promises
	// resolved from the results map, so overrides bypass them.
	hasIncoming := make(map[string]bool, len(edges))
	for _, edge := range edges {
		hasIncoming[edge.To] = true
	}

	var lastResult any

	for _, child := range sorted {
		var childOverrides map[string]SlotValue
		if !hasIncoming[child.ChildID()] {
			childOverrides = overrides
		}

		var unit ExecutableUnit
		switch {
		case child.Node != nil:
			unit = child.Node
		case child.Subgraph != nil:
			unit = child.Subgraph
		default:
			continue
		}

		childResult, err := graph.dispatch(e, stack, unit, results, childOverrides)
		if err != nil {
			return nil, err
		}
		if childResult != nil {
			lastResult = childResult
		}
	}

	return lastResult, nil
}

// executeSubgraph runs a subgraph with retry logic, recursively executing its children.
//
// Parameters:
//   - graph: the root graph (for context access and compensation lookup).
//   - sg: the subgraph to execute.
//   - results: the accumulated node results for promise resolution.
//   - stack: the recovery stack for compensation.
//
// Returns:
//   - any: the last node's output value within the subgraph, or nil.
//   - error: non-nil if the subgraph fails after all retry attempts.
func (e *GraphExecutor) executeSubgraph(graph *Graph, sg *Subgraph, results map[string]any, stack *RecoveryStack, overrides map[string]SlotValue) (any, error) {

	maxAttempts := 1

	if sg.Retry != nil {
		maxAttempts += sg.Retry.MaxAttempts
	}

	ctx := graph.ExecutionContext()
	var lastErr error

	for attempt := 0; attempt < maxAttempts; attempt++ {

		// Apply backoff delay before retries (not before first attempt)
		if attempt > 0 && sg.Retry != nil {
			delay := sg.Retry.ComputeDelay(attempt - 1)
			if delay > 0 {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(delay):
				}
			}
		}

		// Reset inner node statuses for retry
		if attempt > 0 {
			resetSubgraphNodes(sg)
		}

		e.hooks.FireSubgraphStart(ctx, sg.ID())

		childResult, innerErr := e.executeChildren(graph, sg.Children, sg.Edges, results, stack, overrides)

		e.hooks.FireSubgraphComplete(ctx, sg.ID(), innerErr)

		attemptRecord := Attempt{
			Number:    attempt + 1,
			Timestamp: time.Now().Format(time.RFC3339),
		}

		if innerErr == nil {
			attemptRecord.Status = "completed"
			sg.Attempts = append(sg.Attempts, attemptRecord)
			sg.Status = SubgraphCompleted
			return childResult, nil
		}

		attemptRecord.Status = "failed"
		attemptRecord.Error = innerErr.Error()
		sg.Attempts = append(sg.Attempts, attemptRecord)
		lastErr = innerErr
	}

	return nil, lastErr
}

// resetSubgraphNodes resets all node statuses within a subgraph back to pending for retry.
// Walks the subgraph tree recursively.
func resetSubgraphNodes(sg *Subgraph) {
	for _, c := range sg.Children {
		if c.Node != nil {
			c.Node.Status = StatusPending
			c.Node.Error = ""
			c.Node.Timestamp = ""
		}
		if c.Subgraph != nil {
			resetSubgraphNodes(c.Subgraph)
		}
	}
}

// executeNode resolves slots, calls Do, stores the result, and pushes a recovery entry.
//
// Parameters:
//   - ctx: the execution context.
//   - node: the node to execute.
//   - results: the accumulated results for promise resolution.
//   - stack: the recovery stack for compensation.
//
// Returns:
//   - *NodeResult: the execution outcome.
func (e *GraphExecutor) executeNode(node *Node, results map[string]any, stack *RecoveryStack, overrides map[string]SlotValue) *NodeResult {

	ctx := node.ExecutionContext()
	action, err := node.Action()

	if err != nil {
		node.Status = StatusFailed
		return &NodeResult{
			NodeID: node.ID(),
			Status: ResultFailed,
			Error:  fmt.Errorf("node %s: %w", node.ID(), err),
		}
	}

	slots := node.ResolveSlots(ctx, results, overrides)

	ctx.Results = results

	e.hooks.FireNodeStart(ctx, node.ID(), slots)

	result, complement, err := action.Do(ctx, slots)

	if err != nil {

		e.hooks.FireNodeComplete(ctx, node.ID(), nil, err)
		node.Status = StatusFailed

		return &NodeResult{
			NodeID: node.ID(),
			Status: ResultFailed,
			Error:  fmt.Errorf("%s: %w", node.Receiver, err),
		}
	}

	// Post-dispatch catalog reconciliation. See docs/architecture/4-resource-management.md §6.5, §6.8.
	//
	// If the method returned a Resource, the catalog records the resolved version so downstream nodes,
	// receipts, and compensation can observe it.
	//
	//   - Plan-time-shadowed case: the Planned companion ran during planning and the catalog already has
	//     a pending entry owned by this node. Transition fills the pending entry's metadata in place, so
	//     every outstanding pointer to the pending resource sees the populated fields.
	//   - Monadic case: no Planned companion (or the companion returned KnownAtExecution), so no pending
	//     entry exists. Shadow the real result now under this node's origin.
	//
	// A Shadow/Transition failure means the node's side effect has already happened; we push the action
	// onto the recovery stack before surfacing the catalog error so compensation can unwind it.
	if resource, isResource := result.(Resource); isResource && resource != nil && !IsKnownAtExecution(resource) {
		if node.graph.Catalog == nil {
			node.graph.Catalog = NewResourceCatalog()
		}
		catalog := node.graph.Catalog

		var catErr error
		if pendingID := catalog.Current(resource.URI()); pendingID != "" {
			if pending, ok := catalog.Lookup(pendingID); ok && pending.resourceBase().originID == node.ID() {
				catErr = catalog.Transition(resource, node.ID())
			} else {
				_, catErr = catalog.Shadow(resource, node.ID())
			}
		} else {
			_, catErr = catalog.Shadow(resource, node.ID())
		}

		if catErr != nil {
			stack.PushAction(ctx, action, complement)
			e.hooks.FireNodeComplete(ctx, node.ID(), nil, catErr)
			node.Status = StatusFailed
			return &NodeResult{
				NodeID: node.ID(),
				Status: ResultFailed,
				Error:  fmt.Errorf("%s: post-dispatch catalog: %w", node.Receiver, catErr),
			}
		}
	}

	e.hooks.FireNodeComplete(ctx, node.ID(), result, nil)

	if result != nil {
		results[node.ID()] = result
	}

	stack.PushAction(ctx, action, complement)
	node.Status = StatusCompleted

	return &NodeResult{
		NodeID: node.ID(),
		Status: ResultCompleted,
	}
}
