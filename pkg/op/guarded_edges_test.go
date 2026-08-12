// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package op

import (
	"strings"
	"testing"
)

// edgeTestSubgraph seals a subgraph bound by `actionName` with childless by-name children and the given edges — the
// minimum construction the edge-validation scenarios need.
func edgeTestSubgraph(t *testing.T, actionName ActionName, childIDs []string, edges []Edge) *Subgraph {
	t.Helper()

	children := make([]ExecutableUnit, 0, len(childIDs))
	for _, id := range childIDs {
		child, err := NewSubgraph(NewSubgraphSpec().WithID(id).WithActionNamed("flow.subgraph"))
		if err != nil {
			t.Fatalf("edgeTestSubgraph: child %q: %v", id, err)
		}
		children = append(children, child)
	}

	subgraph, err := NewSubgraph(NewSubgraphSpec().
		WithID("under-test").
		WithActionNamed(actionName).
		WithChildren(children...).
		WithEdges(edges...))
	if err != nil {
		t.Fatalf("edgeTestSubgraph: %v", err)
	}

	return subgraph
}

// wantEdgeViolation asserts validateEdges rejects the subgraph with a message containing `fragment`.
func wantEdgeViolation(t *testing.T, subgraph *Subgraph, fragment string) {
	t.Helper()

	err := subgraph.validateEdges()
	if err == nil {
		t.Fatalf("validateEdges = nil, want a violation containing %q", fragment)
	}
	if !strings.Contains(err.Error(), fragment) {
		t.Errorf("validateEdges = %q, want it to contain %q", err, fragment)
	}
}

func TestValidateEdges_WellFormedDecisionTree(t *testing.T) {

	subgraph := edgeTestSubgraph(t, "flow.choose",
		[]string{"when-1", "then-1", "when-2", "then-2", "default"},
		[]Edge{
			{From: "when-1", To: "then-1", Guard: GuardTruthy},
			{From: "when-1", To: "when-2", Guard: GuardFalsy},
			{From: "when-2", To: "then-2", Guard: GuardTruthy},
			{From: "when-2", To: "default", Guard: GuardFalsy},
		})

	if err := subgraph.validateEdges(); err != nil {
		t.Errorf("validateEdges = %v, want nil for a well-formed decision tree", err)
	}
}

func TestValidateEdges_DoubleTruthyEdge(t *testing.T) {

	subgraph := edgeTestSubgraph(t, "flow.choose",
		[]string{"when-1", "then-1", "then-2", "default"},
		[]Edge{
			{From: "when-1", To: "then-1", Guard: GuardTruthy},
			{From: "when-1", To: "then-2", Guard: GuardTruthy},
			{From: "when-1", To: "default", Guard: GuardFalsy},
		})

	wantEdgeViolation(t, subgraph, "want exactly 1 of each")
}

func TestValidateEdges_GuardedCycle(t *testing.T) {

	// when-2's falsy edge points back at when-1: a loop with no construct to legalize it.
	subgraph := edgeTestSubgraph(t, "flow.choose",
		[]string{"when-1", "then-1", "when-2", "then-2"},
		[]Edge{
			{From: "when-1", To: "then-1", Guard: GuardTruthy},
			{From: "when-1", To: "when-2", Guard: GuardFalsy},
			{From: "when-2", To: "then-2", Guard: GuardTruthy},
			{From: "when-2", To: "when-1", Guard: GuardFalsy},
		})

	wantEdgeViolation(t, subgraph, "cycle")
}

func TestValidateEdges_MixedGuardedAndUnguarded(t *testing.T) {

	subgraph := edgeTestSubgraph(t, "flow.choose",
		[]string{"when-1", "then-1", "default", "extra"},
		[]Edge{
			{From: "when-1", To: "then-1", Guard: GuardTruthy},
			{From: "when-1", To: "default", Guard: GuardFalsy},
			{From: "default", To: "extra"},
		})

	wantEdgeViolation(t, subgraph, "mixes guarded and unguarded edges")
}

func TestValidateEdges_UnreachableChild(t *testing.T) {

	// "orphan" is targeted by nothing and targets nothing — a second root.
	subgraph := edgeTestSubgraph(t, "flow.choose",
		[]string{"when-1", "then-1", "default", "orphan"},
		[]Edge{
			{From: "when-1", To: "then-1", Guard: GuardTruthy},
			{From: "when-1", To: "default", Guard: GuardFalsy},
		})

	wantEdgeViolation(t, subgraph, "roots")
}

func TestValidateEdges_MultiChildGuardlessChoose(t *testing.T) {

	subgraph := edgeTestSubgraph(t, "flow.choose", []string{"a", "b", "c"}, nil)

	wantEdgeViolation(t, subgraph, "must be a decision tree or the single-default degenerate form")
}

func TestValidateEdges_ZeroCaseChooseDegenerateForm(t *testing.T) {

	subgraph := edgeTestSubgraph(t, "flow.choose", []string{"default"}, nil)

	if err := subgraph.validateEdges(); err != nil {
		t.Errorf("validateEdges = %v, want nil for the single-default degenerate form", err)
	}
}

func TestValidateEdges_OrderingEdgeCycle(t *testing.T) {

	subgraph := edgeTestSubgraph(t, "flow.subgraph", []string{"a", "b"},
		[]Edge{
			{From: "a", To: "b"},
			{From: "b", To: "a"},
		})

	wantEdgeViolation(t, subgraph, "edge cycle among children")
}

func TestValidateEdges_OrderingEdgesRemainValid(t *testing.T) {

	subgraph := edgeTestSubgraph(t, "flow.subgraph", []string{"a", "b", "c"},
		[]Edge{
			{From: "a", To: "b"},
			{From: "b", To: "c"},
		})

	if err := subgraph.validateEdges(); err != nil {
		t.Errorf("validateEdges = %v, want nil for acyclic ordering edges", err)
	}
}

func TestValidateEdges_DanglingEndpointStillRejected(t *testing.T) {

	subgraph := edgeTestSubgraph(t, "flow.subgraph", []string{"a"},
		[]Edge{{From: "a", To: "ghost"}})

	wantEdgeViolation(t, subgraph, "not a direct child")
}
