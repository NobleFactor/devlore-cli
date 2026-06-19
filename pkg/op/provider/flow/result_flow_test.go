// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package flow_test

import (
	"context"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/application"
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// sentinelOutput is the recognizable value the flow.complete leaf produces. The whole point of the test
// is to follow this exact value out the far end of GraphExecutor.Run.
const sentinelOutput = "result-flow-sentinel-7f3a"

// TestSubgraphBoundAction_FlowsLeafResult proves a flow.subgraph-bound subgraph propagates its child's
// terminal result up through the subgraph, the structural root, and out of GraphExecutor.Run.
//
// Topology: structural root → subgraph bound to `flow.subgraph` → single node bound to `flow.complete`
// whose `output` slot is an [op.ImmediateBinding] of [sentinelOutput]. flow.complete returns its output;
// flow.subgraph must carry that last child result out as its own result (subgraph.go:271 returns the
// bound action's result), and the structural root bubbles it to Run (graph_executor.go:281).
//
// RED before the fix: flow.Provider.Subgraph returns nil instead of the last child's result, so Run
// returns nil. GREEN after the fix: Run returns [sentinelOutput].
func TestSubgraphBoundAction_FlowsLeafResult(t *testing.T) {

	registry := op.ReceiverRegistry()

	completeAction, err := registry.BuildAction("flow.complete")
	if err != nil {
		t.Fatalf("BuildAction(flow.complete): %v", err)
	}

	subgraphAction, err := registry.BuildAction("flow.subgraph")
	if err != nil {
		t.Fatalf("BuildAction(flow.subgraph): %v", err)
	}

	leaf, err := op.NewNode(op.NewNodeSpec().
		WithID("leaf").
		WithAction(completeAction).
		WithSlot("output", op.NewImmediateBinding(sentinelOutput)))
	if err != nil {
		t.Fatalf("NewNode(leaf): %v", err)
	}

	subgraph, err := op.NewSubgraph(op.NewSubgraphSpec().
		WithID("body").
		WithAction(subgraphAction).
		WithChildren(leaf))
	if err != nil {
		t.Fatalf("NewSubgraph(body): %v", err)
	}

	graph, err := op.NewGraph(op.NewGraphSpec().WithUnits(subgraph))
	if err != nil {
		t.Fatalf("NewGraph: %v", err)
	}

	spec := op.NewRuntimeEnvironmentSpec("test").
		WithApplication(&application.Application{Name: "test"})

	result, err := op.NewGraphExecutor(graph, spec).Run(context.Background(), nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if result != sentinelOutput {
		t.Errorf("Run() result = %#v, want %#v "+
			"(flow.subgraph dropped its child's terminal result)", result, sentinelOutput)
	}

	t.Logf("GraphExecutor.Run returned %#v — flow.complete's output bubbled flow.subgraph → root → Run", result)
}

// TestBareNodeUnderRoot_FlowsLeafResult proves a bare node placed directly under the root returns its result.
//
// Topology: root (bound to flow.subgraph by name, seeded by NewGraphSpec) → single flow.complete leaf. With the
// structural child-walk gone, the root dispatches through flow.subgraph like every other subgraph; this confirms a
// leaf with no intermediate subgraph still bubbles its terminal result out of GraphExecutor.Run — the plan's
// Verification bullet ("a graph with a bare node directly under the root still returns its result"). Distinct from
// TestSubgraphBoundAction_FlowsLeafResult, which interposes an explicit child subgraph between root and leaf.
func TestBareNodeUnderRoot_FlowsLeafResult(t *testing.T) {

	registry := op.ReceiverRegistry()

	completeAction, err := registry.BuildAction("flow.complete")
	if err != nil {
		t.Fatalf("BuildAction(flow.complete): %v", err)
	}

	leaf, err := op.NewNode(op.NewNodeSpec().
		WithID("leaf").
		WithAction(completeAction).
		WithSlot("output", op.NewImmediateBinding(sentinelOutput)))
	if err != nil {
		t.Fatalf("NewNode(leaf): %v", err)
	}

	graph, err := op.NewGraph(op.NewGraphSpec().WithUnits(leaf))
	if err != nil {
		t.Fatalf("NewGraph: %v", err)
	}

	spec := op.NewRuntimeEnvironmentSpec("test").
		WithApplication(&application.Application{Name: "test"})

	result, err := op.NewGraphExecutor(graph, spec).Run(context.Background(), nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if result != sentinelOutput {
		t.Errorf("Run() result = %#v, want %#v (bare node under root dropped its result)",
			result, sentinelOutput)
	}
}
