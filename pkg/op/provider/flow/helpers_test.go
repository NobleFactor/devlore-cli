// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package flow

import (
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// --- decision tree: root / branch / hasConditionalEdges ---

// namedSubgraph builds a childless by-name subgraph for tree-topology tests.
func namedSubgraph(t *testing.T, id string) *op.Subgraph {
	t.Helper()

	subgraph, err := op.NewSubgraph(op.NewSubgraphSpec().WithID(id).WithActionNamed(Subgraph))
	if err != nil {
		t.Fatalf("namedSubgraph(%q): %v", id, err)
	}

	return subgraph
}

// decisionTree builds a two-case choose topology:
//
//	when-1 ───truthy───► then-1
//	  └─────falsy────► when-2 ───truthy───► then-2
//	                     └─────falsy────► default
func decisionTree(t *testing.T) *op.Subgraph {
	t.Helper()

	children := []op.ExecutableUnit{
		namedSubgraph(t, "when-1"), namedSubgraph(t, "then-1"),
		namedSubgraph(t, "when-2"), namedSubgraph(t, "then-2"),
		namedSubgraph(t, "default"),
	}
	edges := []op.Edge{
		{From: "when-1", To: "then-1", Guard: op.GuardTruthy},
		{From: "when-1", To: "when-2", Guard: op.GuardFalsy},
		{From: "when-2", To: "then-2", Guard: op.GuardTruthy},
		{From: "when-2", To: "default", Guard: op.GuardFalsy},
	}

	subgraph, err := op.NewSubgraph(op.NewSubgraphSpec().
		WithID("choose-1").
		WithAction(stubAction{}).
		WithChildren(children...).
		WithEdges(edges...))
	if err != nil {
		t.Fatalf("decisionTree: %v", err)
	}

	return subgraph
}

func TestHasConditionalEdges(t *testing.T) {

	if !hasConditionalEdges(decisionTree(t)) {
		t.Error("hasConditionalEdges(decision tree) = false, want true")
	}
	if hasConditionalEdges(namedSubgraph(t, "plain")) {
		t.Error("hasConditionalEdges(plain subgraph) = true, want false")
	}
}

func TestRoot_FindsEntryNode(t *testing.T) {

	entry, err := root(decisionTree(t))
	if err != nil {
		t.Fatalf("root: %v", err)
	}
	if entry.ID() != "when-1" {
		t.Errorf("root = %q, want %q", entry.ID(), "when-1")
	}
}

func TestBranch_TruthyRoutesToThen(t *testing.T) {

	next, err := branch(decisionTree(t), "when-1", "non-empty")
	if err != nil {
		t.Fatalf("branch: %v", err)
	}
	if next.ID() != "then-1" {
		t.Errorf("next = %q, want %q", next.ID(), "then-1")
	}
}

func TestBranch_FalsyRoutesToNextWhen(t *testing.T) {

	next, err := branch(decisionTree(t), "when-1", "")
	if err != nil {
		t.Fatalf("branch: %v", err)
	}
	if next.ID() != "when-2" {
		t.Errorf("next = %q, want %q", next.ID(), "when-2")
	}
}

func TestBranch_LastFalsyRoutesToDefault(t *testing.T) {

	next, err := branch(decisionTree(t), "when-2", nil)
	if err != nil {
		t.Fatalf("branch: %v", err)
	}
	if next.ID() != "default" {
		t.Errorf("next = %q, want %q", next.ID(), "default")
	}
}

func TestBranch_LeafEndsWalk(t *testing.T) {

	next, err := branch(decisionTree(t), "then-1", "anything")
	if err != nil {
		t.Fatalf("branch: %v", err)
	}
	if next != nil {
		t.Errorf("next = %v, want nil (a leaf ends the walk)", next.ID())
	}
}

func TestBranch_AmbiguousEdgesError(t *testing.T) {

	children := []op.ExecutableUnit{
		namedSubgraph(t, "when-1"), namedSubgraph(t, "then-1"), namedSubgraph(t, "then-2"),
	}
	edges := []op.Edge{
		{From: "when-1", To: "then-1", Guard: op.GuardTruthy},
		{From: "when-1", To: "then-2", Guard: op.GuardTruthy},
	}

	malformed, err := op.NewSubgraph(op.NewSubgraphSpec().
		WithID("malformed").
		WithAction(stubAction{}).
		WithChildren(children...).
		WithEdges(edges...))
	if err != nil {
		t.Fatalf("NewSubgraph: %v", err)
	}

	if _, branchErr := branch(malformed, "when-1", true); branchErr == nil {
		t.Error("branch(two truthy out-edges) error = nil, want ambiguity error (defense in depth behind validation)")
	}
}

// --- NewCase ---

func TestNewCase_SealsBodies(t *testing.T) {

	caseValue, err := NewCase([]any{}, []any{})
	if err != nil {
		t.Fatalf("NewCase: %v", err)
	}
	if caseValue.When.ID() == "" || caseValue.Then.ID() == "" {
		t.Error("NewCase produced a subgraph without an ID")
	}
	if caseValue.When.ID() == caseValue.Then.ID() {
		t.Error("when- and then-subgraphs share an ID, want distinct IDs")
	}
}

func TestNewCase_MalformedBodyErrors(t *testing.T) {

	if _, err := NewCase("not-a-list", []any{}); err == nil {
		t.Error("NewCase(non-list when) error = nil, want error")
	}
	if _, err := NewCase([]any{}, 42); err == nil {
		t.Error("NewCase(non-list then) error = nil, want error")
	}
}
