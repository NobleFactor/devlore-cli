// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package execution

import (
	"fmt"
	"time"
)

// PhaseStatus represents the execution state of a phase.
type PhaseStatus string

const (
	PhasePending    PhaseStatus = "pending"
	PhaseCompleted  PhaseStatus = "completed"
	PhaseFailed     PhaseStatus = "failed"
	PhaseRolledBack PhaseStatus = "rolled_back"
	PhaseSkipped    PhaseStatus = "skipped"
)

// Phase represents a lifecycle phase in the execution graph.
// Each phase owns a set of inner nodes and acts as an error boundary
// with retry and compensating action support (the Saga Pattern).
type Phase struct {
	// ID is the unique identifier (e.g., "phase.install").
	ID string `json:"id" yaml:"id"`

	// Name is the phase name (e.g., "install").
	Name string `json:"name" yaml:"name"`

	// Status of this phase: pending, completed, failed, rolled_back, skipped.
	Status PhaseStatus `json:"status" yaml:"status"`

	// Retry governs retry behavior when inner nodes fail.
	Retry *RetryPolicy `json:"retry,omitempty" yaml:"retry,omitempty"`

	// NodeIDs lists the IDs of inner nodes belonging to this phase.
	NodeIDs []string `json:"nodes,omitempty" yaml:"nodes,omitempty"`

	// Compensate is the ID of the compensating action for rollback.
	Compensate string `json:"compensate,omitempty" yaml:"compensate,omitempty"`

	// Attempts records retry history (populated during execution).
	Attempts []Attempt `json:"attempts,omitempty" yaml:"attempts,omitempty"`

	// State holds execution metadata captured during the forward action.
	// The compensating action reads this to know what to undo.
	State map[string]any `json:"state,omitempty" yaml:"state,omitempty"`
}

// Attempt records one execution attempt of a phase.
type Attempt struct {
	// Number is the 1-based attempt number.
	Number int `json:"number" yaml:"number"`

	// Status is "completed" or "failed".
	Status string `json:"status" yaml:"status"`

	// Error is the error message if the attempt failed.
	Error string `json:"error,omitempty" yaml:"error,omitempty"`

	// Timestamp is when this attempt completed (RFC3339).
	Timestamp string `json:"timestamp" yaml:"timestamp"`
}

// BackoffStrategy defines how delays increase between retries.
type BackoffStrategy string

const (
	BackoffNone        BackoffStrategy = "none"
	BackoffLinear      BackoffStrategy = "linear"
	BackoffExponential BackoffStrategy = "exponential"
)

// RetryPolicy configures retry behavior for a phase.
type RetryPolicy struct {
	// MaxAttempts is the maximum number of retries (0 = no retry, fail immediately).
	MaxAttempts int `json:"max_attempts" yaml:"max_attempts"`

	// Backoff is the delay strategy: none, linear, exponential.
	Backoff BackoffStrategy `json:"backoff" yaml:"backoff"`

	// InitialDelay is the delay before the first retry (Go duration string, e.g. "1s").
	InitialDelay string `json:"initial_delay,omitempty" yaml:"initial_delay,omitempty"`

	// MaxDelay caps the delay between retries (Go duration string, e.g. "30s").
	MaxDelay string `json:"max_delay,omitempty" yaml:"max_delay,omitempty"`
}

// ParseInitialDelay parses the InitialDelay string into a time.Duration.
// Returns 0 if the string is empty or unparseable.
func (r *RetryPolicy) ParseInitialDelay() time.Duration {
	if r.InitialDelay == "" {
		return 0
	}
	d, err := time.ParseDuration(r.InitialDelay)
	if err != nil {
		return 0
	}
	return d
}

// ParseMaxDelay parses the MaxDelay string into a time.Duration.
// Returns 0 if the string is empty or unparseable.
func (r *RetryPolicy) ParseMaxDelay() time.Duration {
	if r.MaxDelay == "" {
		return 0
	}
	d, err := time.ParseDuration(r.MaxDelay)
	if err != nil {
		return 0
	}
	return d
}

// ComputeDelay returns the delay for a given attempt number (0-based).
func (r *RetryPolicy) ComputeDelay(attempt int) time.Duration {
	initial := r.ParseInitialDelay()
	if initial == 0 {
		return 0
	}

	var delay time.Duration
	switch r.Backoff {
	case BackoffNone:
		delay = initial
	case BackoffLinear:
		delay = initial * time.Duration(attempt+1)
	case BackoffExponential:
		delay = initial
		for i := 0; i < attempt; i++ {
			delay *= 2
		}
	default:
		delay = initial
	}

	if maxDelay := r.ParseMaxDelay(); maxDelay > 0 && delay > maxDelay {
		delay = maxDelay
	}

	return delay
}

// RollbackEntry records a compensating action executed during rollback.
type RollbackEntry struct {
	// Phase is the phase name that was rolled back.
	Phase string `json:"phase" yaml:"phase"`

	// Compensate is the ID of the compensating action.
	Compensate string `json:"compensate" yaml:"compensate"`

	// Status is "completed" or "failed".
	Status string `json:"status" yaml:"status"`

	// Error is the error message if the compensating action failed.
	Error string `json:"error,omitempty" yaml:"error,omitempty"`
}

// ExecutePhaseInner runs all inner nodes of a phase using the executor.
// It extracts the phase's nodes from the graph and runs them in topological order.
// Any node failure immediately fails the phase.
func (e *GraphExecutor) ExecutePhaseInner(ctx *Context, g *Graph, phase *Phase) error {
	// Collect the phase's inner nodes
	nodeSet := make(map[string]bool, len(phase.NodeIDs))
	for _, id := range phase.NodeIDs {
		nodeSet[id] = true
	}

	var phaseNodes []*Node
	for _, n := range g.Nodes {
		if nodeSet[n.ID] {
			phaseNodes = append(phaseNodes, n)
		}
	}

	if len(phaseNodes) == 0 {
		return nil
	}

	// Filter edges to only those between phase nodes
	var phaseEdges []Edge
	for _, edge := range g.Edges {
		if nodeSet[edge.From] && nodeSet[edge.To] {
			phaseEdges = append(phaseEdges, edge)
		}
	}

	ordered := e.topologicalSortNodes(phaseNodes, phaseEdges)

	// Set content pipeline on context for this phase
	ctx.Edges = phaseEdges
	ctx.Outputs = make(map[string][]byte)

	for _, node := range ordered {
		result := e.executeNode(ctx, node)
		if result.Status == ResultFailed {
			// Apply this result to the node
			node.Status = StatusFailed
			if result.Error != nil {
				node.Error = result.Error.Error()
			}
			node.Timestamp = time.Now().Format(time.RFC3339)
			return fmt.Errorf("node %s failed: %w", node.ID, result.Error)
		}

		// Apply successful result
		node.Status = StatusCompleted
		node.Timestamp = time.Now().Format(time.RFC3339)
		node.SourceChecksum = result.SourceChecksum
		node.TargetChecksum = result.TargetChecksum
	}

	return nil
}
