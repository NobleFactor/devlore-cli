// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package flow

import (
	"context"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// testProvider builds a flow.Provider with a minimal RuntimeEnvironment suitable for the saga-shape
// signature tests that don't dispatch into child ExecutableUnits.
func testProvider(t *testing.T) *Provider {
	t.Helper()
	runtimeEnvironment := &op.RuntimeEnvironment{}
	return &Provider{ProviderBase: op.NewProviderBase(runtimeEnvironment)}
}

// stubAction is the minimum op.Action implementation needed to construct a Subgraph in tests that
// never dispatch through the action (Do is unreachable from the test paths that use it).
type stubAction struct{}

func (stubAction) FullName() string       { return "stub.action" }
func (stubAction) Name() string           { return "action" }
func (stubAction) Method() *op.Method     { return nil }
func (stubAction) Params() []op.Parameter { return nil }
func (stubAction) Do(*op.ActivationRecord) (op.Result, op.Complement, error) {
	return nil, nil, nil
}

// subgraphActivation builds an empty Subgraph + an activation pointing at it, suitable for the
// saga-shape tests below. The activation's `dispatchChild` is nil (would be installed by the
// executor on the bound-action path); these tests do not exercise the children-walk.
func subgraphActivation(t *testing.T) *op.ActivationRecord {
	t.Helper()
	subgraph, err := op.NewSubgraph(op.NewSubgraphSpec().WithID("test").WithAction(stubAction{}))
	if err != nil {
		t.Fatalf("subgraphActivation: %v", err)
	}
	// The executor supplies a non-nil recovery stack on the activation before dispatching a subgraph's bound action
	// (subgraph.Execute sets it to the child executor's stack). Mirror that contract so the combinator under test reads
	// a real stack rather than nil.
	activation := op.NewActivationRecord(nil, subgraph, &op.RuntimeEnvironment{})
	activation.Stack = op.NewRecoveryStack()
	return activation
}

func TestChoose_ReturnsRecoveryStack(t *testing.T) {

	p := testProvider(t)

	chosen, stack, err := p.Choose(nil, "default-value")
	if err != nil {
		t.Fatalf("Choose() error = %v", err)
	}
	if chosen != "default-value" {
		t.Errorf("chosen = %v, want \"default-value\"", chosen)
	}
	if stack == nil {
		t.Fatal("Choose() returned nil *RecoveryStack; want empty stack per the saga-shape contract")
	}
	if stack.Len() != 0 {
		t.Errorf("Choose() returned stack with %d entries; want 0 (empty stub stack)", stack.Len())
	}
}

func TestChoose_TruthyCaseReturnsThen(t *testing.T) {

	p := testProvider(t)

	chosen, stack, err := p.Choose(
		nil, "default",
		Case{When: false, Then: "skip-1"},
		Case{When: true, Then: "winner"},
		Case{When: true, Then: "skip-2"},
	)
	if err != nil {
		t.Fatalf("Choose() error = %v", err)
	}
	if chosen != "winner" {
		t.Errorf("chosen = %v, want \"winner\"", chosen)
	}
	if stack == nil {
		t.Fatal("Choose() returned nil *RecoveryStack")
	}
}

func TestCompensateChoose_NilStack_NoOp(t *testing.T) {

	p := testProvider(t)

	if err := p.CompensateChoose(&op.ActivationRecord{}, nil); err != nil {
		t.Errorf("CompensateChoose(nil) error = %v, want nil", err)
	}
}

func TestCompensateChoose_EmptyStack_NoOp(t *testing.T) {

	p := testProvider(t)

	if err := p.CompensateChoose(&op.ActivationRecord{}, op.NewRecoveryStack()); err != nil {
		t.Errorf("CompensateChoose(empty) error = %v, want nil", err)
	}
}

func TestChoose_CompensateChoose_RoundTrip(t *testing.T) {

	p := testProvider(t)

	_, stack, err := p.Choose(nil, "default", Case{When: true, Then: "winner"})
	if err != nil {
		t.Fatalf("Choose() error = %v", err)
	}
	if compensateErr := p.CompensateChoose(&op.ActivationRecord{}, stack); compensateErr != nil {
		t.Errorf("CompensateChoose() error = %v, want nil (empty-stack unwind is a no-op)", compensateErr)
	}
}

func TestSubgraph_ReturnsActivationStack(t *testing.T) {

	p := testProvider(t)
	activation := subgraphActivation(t)

	result, stack, err := p.Subgraph(activation, nil)
	if err != nil {
		t.Fatalf("Subgraph() error = %v", err)
	}

	// The saga-shape contract finalized in phase-8 step 28.2: Subgraph walks its children on, and returns, the
	// executor-owned stack supplied as activation.Stack — so the executor nests that same stack onto the parent as the
	// subgraph's complement. Subgraph is the base case of its family; Choose/Gather/WaitUntil quantify over it. Full
	// build/save/load/execute + pause/resume + fail/rollback coverage lives in the plan package's lifecycle suite
	// (TestLifecycle_ViaGoAPI / _ViaStarlark, TestGraphSaveLoadResume / _ResumeThenFail), whose graph root is a subgraph
	// whose complement is the *RecoveryStack rollback cascades through.
	if stack != activation.Stack {
		t.Error("Subgraph() returned a stack other than activation.Stack; the complement must be the subgraph's own stack")
	}
	if result != nil {
		t.Errorf("Subgraph() result = %v; want nil (an empty container has no terminal output of its own)", result)
	}
	if stack.Len() != 0 {
		t.Errorf("Subgraph() stack has %d entries; want 0 (an empty subgraph dispatched zero children)", stack.Len())
	}
}

func TestCompensateSubgraph_NilStack_NoOp(t *testing.T) {

	p := testProvider(t)

	if err := p.CompensateSubgraph(&op.ActivationRecord{}, nil); err != nil {
		t.Errorf("CompensateSubgraph(nil) error = %v, want nil", err)
	}
}

func TestSubgraph_CompensateSubgraph_RoundTrip(t *testing.T) {

	p := testProvider(t)

	_, stack, err := p.Subgraph(subgraphActivation(t), nil)
	if err != nil {
		t.Fatalf("Subgraph() error = %v", err)
	}
	if compensateErr := p.CompensateSubgraph(&op.ActivationRecord{}, stack); compensateErr != nil {
		t.Errorf("CompensateSubgraph() error = %v, want nil (empty-stack unwind is a no-op)", compensateErr)
	}
}

func TestGather_StampsIterationSubstacks(t *testing.T) {

	p := testProvider(t)
	activation := subgraphActivation(t)
	activation.Context = context.Background()

	items := []any{"a", "b", "c"}

	result, stack, err := p.Gather(activation, items, map[string]any{"limit": 2})
	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}
	if stack != activation.Stack {
		t.Error("Gather() returned a stack other than activation.Stack; the complement must be the gather's own stack")
	}

	results, ok := result.([]any)
	if !ok || len(results) != len(items) {
		t.Fatalf("Gather() result = %v, want []any of length %d", result, len(items))
	}

	// One stamped substack per iteration, keyed "<gatherID>#<i>", each a completed (nil-err) run over the empty body.
	for i := range items {
		id := gatherIterationID(activation.Unit, i)
		sub, found := stack.NestedStackByUnitID(id)
		if !found {
			t.Errorf("no stamped substack for iteration %d (%q)", i, id)
			continue
		}
		if sub.Err() != nil {
			t.Errorf("iteration %d substack Err() = %v, want nil", i, sub.Err())
		}
	}
}

func TestCompensateGather_NilStack_NoOp(t *testing.T) {

	p := testProvider(t)

	if err := p.CompensateGather(&op.ActivationRecord{}, nil); err != nil {
		t.Errorf("CompensateGather(nil) error = %v, want nil", err)
	}
}

func TestCompensateGather_EmptyStack_NoOp(t *testing.T) {

	p := testProvider(t)

	if err := p.CompensateGather(&op.ActivationRecord{}, op.NewRecoveryStack()); err != nil {
		t.Errorf("CompensateGather(empty) error = %v, want nil", err)
	}
}
