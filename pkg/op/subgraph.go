// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"time"
)

// SubgraphStatus represents the execution state of a subgraph.
type SubgraphStatus string

// SubgraphStatus constants define the possible states of a subgraph.
const (
	// SubgraphPending indicates the subgraph has not yet been executed.
	SubgraphPending SubgraphStatus = "pending"
	// SubgraphCompleted indicates the subgraph executed successfully.
	SubgraphCompleted SubgraphStatus = "completed"
	// SubgraphFailed indicates the subgraph failed during execution.
	SubgraphFailed SubgraphStatus = "failed"
	// SubgraphRolledBack indicates the subgraph was rolled back after failure.
	SubgraphRolledBack SubgraphStatus = "rolled_back"
	// SubgraphSkipped indicates the subgraph was skipped.
	SubgraphSkipped SubgraphStatus = "skipped"
)
// Subgraph is a subsystem of the graph — a functional, structural, and transactional boundary.
// Subgraphs are recursive: a subgraph contains nodes and child subgraphs, forming a tree.
// The graph is the root of the tree.
//
// All subgraphs participate in the saga pattern: retry, compensation, status tracking.
// Nodes and subgraphs are peers at any level — both are vertices in the same topological sort.
type Subgraph struct {
	// ID is the unique identifier (e.g., "install").
	ID string `json:"id" yaml:"id"`

	// Name is the subgraph name (e.g., "install").
	Name string `json:"name" yaml:"name"`

	// Children are the nodes and child subgraphs in declaration order.
	// Execution proceeds through this list after topological sorting by edges at this level.
	Children []SubgraphChild `json:"children" yaml:"children"`

	// Edges are ordering constraints between children at this level.
	// Each edge references children by ID (both node IDs and subgraph IDs).
	Edges []Edge `json:"edges,omitempty" yaml:"edges,omitempty"`

	// Status of this subgraph: pending, completed, failed, rolled_back, skipped.
	Status SubgraphStatus `json:"status" yaml:"status"`

	// Retry governs retry behavior when inner children fail.
	Retry *RetryPolicy `json:"retry,omitempty" yaml:"retry,omitempty"`

	// Compensate is the ID of the compensating subgraph for rollback.
	Compensate string `json:"compensate,omitempty" yaml:"compensate,omitempty"`

	// Attempts records retry history (populated during execution).
	Attempts []Attempt `json:"attempts,omitempty" yaml:"attempts,omitempty"`

	// State holds execution metadata captured during the forward pass.
	// The compensating subgraph reads this to know what to undo.
	State map[string]any `json:"state,omitempty" yaml:"state,omitempty"`

	// Branch marks this subgraph as a conditional branch owned by a choose action.
	// Branch subgraphs are not executed directly by the top-level executor; they
	// are dispatched by the choose action's Do method.
	Branch bool `json:"branch,omitempty" yaml:"branch,omitempty"`
}

// SubgraphChild is a child of a subgraph or graph — either a node or a nested subgraph.
// Exactly one field is set.
type SubgraphChild struct {
	Node     *Node     `json:"node,omitempty" yaml:"node,omitempty"`
	Subgraph *Subgraph `json:"subgraph,omitempty" yaml:"subgraph,omitempty"`
}

// ChildID returns the ID of this child, whether it is a node or a subgraph.
func (c SubgraphChild) ChildID() string {
	if c.Node != nil {
		return c.Node.ID
	}
	if c.Subgraph != nil {
		return c.Subgraph.ID
	}
	return ""
}

// Attempt records one execution attempt of a subgraph.
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

// BackoffStrategy constants define the available retry backoff strategies.
const (
	// BackoffNone applies no delay between retries.
	BackoffNone BackoffStrategy = "none"
	// BackoffLinear increases delay linearly between retries.
	BackoffLinear BackoffStrategy = "linear"
	// BackoffExponential doubles the delay between each retry.
	BackoffExponential BackoffStrategy = "exponential"
)

// RetryPolicy configures retry behavior for a subgraph.
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
func (r RetryPolicy) ParseInitialDelay() time.Duration {
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
func (r RetryPolicy) ParseMaxDelay() time.Duration {
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
func (r RetryPolicy) ComputeDelay(attempt int) time.Duration {
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
	// Subgraph is the subgraph name that was rolled back.
	Subgraph string `json:"subgraph" yaml:"subgraph"`

	// Compensate is the ID of the compensating subgraph.
	Compensate string `json:"compensate" yaml:"compensate"`

	// Status is "completed" or "failed".
	Status string `json:"status" yaml:"status"`

	// Error is the error message if the compensating action failed.
	Error string `json:"error,omitempty" yaml:"error,omitempty"`
}
