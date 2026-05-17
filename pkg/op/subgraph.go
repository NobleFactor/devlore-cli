// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"sort"
	"time"

	"github.com/NobleFactor/devlore-cli/pkg/assert"
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
	executableUnit

	// Name is the subgraph name (e.g., "install").
	Name string

	// Children are the nodes and child subgraphs in declaration order.
	// Execution proceeds through this list after topological sorting by edges at this level.
	Children []SubgraphChild

	// Edges are ordering constraints between children at this level.
	// Each edge references children by ID (both node IDs and subgraph IDs).
	Edges []Edge

	// Status of this subgraph: pending, completed, failed, rolled_back, skipped.
	Status SubgraphStatus

	// Compensate is the ID of the compensating subgraph for rollback.
	Compensate string

	// Attempts records retry history (populated during execution).
	Attempts []Attempt

	// State holds execution metadata captured during the forward pass.
	// The compensating subgraph reads this to know what to undo.
	State map[string]any

	// Branch marks this subgraph as a conditional branch owned by a choose action.
	// Branch subgraphs are not executed directly by the top-level executor; they
	// are dispatched by the choose action's Do method.
	Branch bool
}

// NewSubgraph constructs a Subgraph with the given identifier. Additional fields may be set on the returned
// pointer. The parameter surface is computed lazily by [Subgraph.Parameters] via a graph-walk over children's
// slots (plan-doc D3); no precomputation needed.
func NewSubgraph(id string) *Subgraph {
	return &Subgraph{executableUnit: executableUnit{id: id}}
}

// region EXPORTED METHODS

// region State management

// AddChild appends a child (Node or Subgraph) to this subgraph's Children and stamps the child's parentID
// to this subgraph's ID. Centralizing wiring through this method keeps ownership accurate (plan-doc D11)
// without callers having to remember to maintain the back-reference themselves.
//
// Idempotent on parentID under multi-Graph reuse: the same Invocation referenced from two different
// Graphs' assemblies both stamp `parentID = "root"` (constant Root ID) — silent success. Adding the same
// child to a Subgraph with a different ID panics (a unit cannot belong to two different Subgraphs at the
// same time within a single Graph context).
//
// Parameters:
//   - `child`: the [SubgraphChild] variant to attach. Exactly one of Node or Subgraph must be set; AddChild
//     stamps parentID on whichever is present.
func (s *Subgraph) AddChild(child SubgraphChild) {

	s.Children = append(s.Children, child)

	if child.Node != nil {
		child.Node.stampParent(s.ID())
	}

	if child.Subgraph != nil {
		child.Subgraph.stampParent(s.ID())
	}
}

// endregion

// region Behaviors

// Parameters returns the bubble-up variable surface of this subgraph — the deduplicated set of
// [VariableValue] references walked across every child's slots, recursing into nested subgraphs (plan-doc
// D3). This shadows the embedded [executableUnit.Parameters] for *Subgraph callers and for interface
// dispatch through [ExecutableUnit] on *Subgraph.
//
// Discovery is a graph-walk: for each child node, iterate its slots; for each slot whose Value is a
// [VariableValue], contribute a [Parameter] under the variable's Name, carrying the slot's declared Type
// and Default. For each child subgraph, recurse — its [Subgraph.Parameters] already returns deduped,
// type-checked entries; merge them into the parent's working set. [ImmediateValue] and [PromiseValue]
// slot fills do not contribute (they are intrinsically resolved at execute time).
//
// Same-name + same-Type collapses to one entry. Same name + different Type is a plan-time error
// (panic via [assert.Failf]) because the variable map at runtime is keyed by name and carries one value.
//
// Returns:
//   - []Parameter: the deduplicated bubble-up surface, in stable order by Name.
func (s *Subgraph) Parameters() []Parameter {

	seen := make(map[string]Parameter)

	for _, child := range s.Children {

		if child.Node != nil {
			for _, slot := range child.Node.Slots {

				vv, ok := slot.Value.(VariableValue)
				if !ok {
					continue
				}

				bubbled := Parameter{
					Name:    vv.Name,
					Type:    slot.Parameter.Type,
					Default: slot.Parameter.Default,
				}

				s.mergeBubbled(seen, bubbled)
			}
			continue
		}

		if child.Subgraph != nil {
			for _, bubbled := range child.Subgraph.Parameters() {
				s.mergeBubbled(seen, bubbled)
			}
		}
	}

	out := make([]Parameter, 0, len(seen))
	names := make([]string, 0, len(seen))

	for name := range seen {
		names = append(names, name)
	}

	sort.Strings(names)
	for _, name := range names {
		out = append(out, seen[name])
	}

	return out
}

// endregion

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// mergeBubbled merges a single bubbled [Parameter] into the seen map, panicking via [assert.Failf] on a
// same-name + different-Type collision (plan-doc D3 plan-time error).
//
// Parameters:
//   - `seen`: the accumulating dedup map keyed by variable name.
//   - `bubbled`: the candidate entry to merge.
func (s *Subgraph) mergeBubbled(seen map[string]Parameter, bubbled Parameter) {

	existing, dup := seen[bubbled.Name]
	if !dup {
		seen[bubbled.Name] = bubbled
		return
	}

	if existing.Type != bubbled.Type {
		assert.Failf(
			"subgraph %q: variable %q declared with incompatible types %s and %s across slots",
			s.ID(), bubbled.Name, existing.Type, bubbled.Type)
	}
}

// endregion

// endregion

// SubgraphChild is a child of a subgraph or graph — either a node or a nested subgraph.
// Exactly one field is set.
type SubgraphChild struct {
	Node     *Node     `json:"node,omitempty" yaml:"node,omitempty"`
	Subgraph *Subgraph `json:"subgraph,omitempty" yaml:"subgraph,omitempty"`
}

// ChildID returns the ID of this child, whether it is a node or a subgraph.
func (c SubgraphChild) ChildID() string {
	if c.Node != nil {
		return c.Node.ID()
	}
	if c.Subgraph != nil {
		return c.Subgraph.ID()
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
