// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package flow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

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

func TestChoose_ReturnsActivationStack(t *testing.T) {

	p := testProvider(t)
	activation := subgraphActivation(t)
	activation.Context = context.Background()

	result, stack, err := p.Choose(activation, nil)
	if err != nil {
		t.Fatalf("Choose() error = %v", err)
	}
	if result != nil {
		t.Errorf("result = %v, want nil (childless choose has nothing to run)", result)
	}
	if stack != activation.Stack {
		t.Error("Choose() returned a stack other than activation.Stack; the complement must be the choose's own stack")
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
	activation := subgraphActivation(t)
	activation.Context = context.Background()

	_, stack, err := p.Choose(activation, nil)
	if err != nil {
		t.Fatalf("Choose() error = %v", err)
	}
	if compensateErr := p.CompensateChoose(&op.ActivationRecord{}, stack); compensateErr != nil {
		t.Errorf("CompensateChoose() error = %v, want nil (empty-stack unwind is a no-op)", compensateErr)
	}
}

func TestWaitUntil_TimeoutOnFalsyBody(t *testing.T) {

	p := testProvider(t)
	activation := subgraphActivation(t)
	activation.Context = context.Background()

	// A childless body walks to a nil result — falsy — on every poll, so the tiny budget expires across polls.
	_, stack, err := p.WaitUntil(activation, 30*time.Millisecond, 10*time.Millisecond, nil)
	if err == nil || !strings.Contains(err.Error(), "timeout after") {
		t.Fatalf("WaitUntil error = %v, want a timeout error", err)
	}
	if stack != activation.Stack {
		t.Error("WaitUntil returned a stack other than activation.Stack")
	}
	if stack.Len() != 0 {
		t.Errorf("stack.Len() = %d, want 0 (falsy polls drop unrecorded)", stack.Len())
	}
}

func TestWaitUntil_TimeoutRequired(t *testing.T) {

	p := testProvider(t)
	activation := subgraphActivation(t)
	activation.Context = context.Background()

	if _, _, err := p.WaitUntil(activation, 0, 0, nil); err == nil {
		t.Error("WaitUntil(timeout=0) error = nil, want timeout-required error")
	}
}

func TestCompensateWaitUntil_NilStack_NoOp(t *testing.T) {

	p := testProvider(t)

	if err := p.CompensateWaitUntil(&op.ActivationRecord{}, nil); err != nil {
		t.Errorf("CompensateWaitUntil(nil) error = %v, want nil", err)
	}
}

func TestSubgraph_ReturnsActivationStack(t *testing.T) {

	p := testProvider(t)
	activation := subgraphActivation(t)

	result, stack, err := p.Subgraph(activation, nil)
	if err != nil {
		t.Fatalf("Subgraph() error = %v", err)
	}

	// The saga-shape contract finalized in phase-8 step 31.2: Subgraph walks its children on, and returns, the
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

// TestGather_Resume_ReplaysCompletedIterations proves gather resume: a gather whose iterations all completed on a prior
// run — the exact stamped-substack shape a success leaves behind — replays each result from its stamp and re-runs
// nothing, even after the trace is serialized and reloaded.
func TestGather_Resume_ReplaysCompletedIterations(t *testing.T) {

	p := testProvider(t)
	activation := subgraphActivation(t)
	activation.Context = context.Background()

	items := []any{"a", "b", "c"}

	// Simulate the prior run: one stamped substack per iteration on the gather's own stack, each a completed (nil-err)
	// run carrying a distinct result.
	for i := range items {
		sub := op.NewChildRecoveryStack(activation.Stack)
		sub.Stamp(gatherIterationID(activation.Unit, i), fmt.Sprintf("result-%d", i), nil)
		activation.Stack.PushNested(sub)
	}

	// Round-trip the stack through JSON — the save/reload a resume goes through — then resume Gather against the reload.
	data, err := json.Marshal(activation.Stack)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	reloaded := op.NewRecoveryStack()
	if err := json.Unmarshal(data, reloaded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	activation.Stack = reloaded

	result, stack, err := p.Gather(activation, items, map[string]any{"limit": 2})
	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}

	// Every result is replayed from its stamp — no iteration re-executed.
	results, ok := result.([]any)
	if !ok || len(results) != len(items) {
		t.Fatalf("Gather() result = %v, want []any of length %d", result, len(items))
	}
	for i := range items {
		want := fmt.Sprintf("result-%d", i)
		if results[i] != want {
			t.Errorf("results[%d] = %v, want %q (replayed from the stamp)", i, results[i], want)
		}
	}

	// No iteration re-ran, so no new substack was nested: the reloaded stack still holds exactly N.
	if stack.Len() != len(items) {
		t.Errorf("stack.Len() = %d after resume, want %d (completed iterations skip, not re-run + re-nest)",
			stack.Len(), len(items))
	}
}

// TestGather_Resume_ReentersPausedIteration proves the mixed case: on resume a completed iteration is skipped and
// replayed, while an iteration that was in-progress (a non-nil-status substack) is adopted in place and re-entered to
// completion — not duplicated.
func TestGather_Resume_ReentersPausedIteration(t *testing.T) {

	p := testProvider(t)
	activation := subgraphActivation(t)
	activation.Context = context.Background()

	items := []any{"a", "b"}

	done := op.NewChildRecoveryStack(activation.Stack)
	done.Stamp(gatherIterationID(activation.Unit, 0), "done-0", nil)
	activation.Stack.PushNested(done)

	paused := op.NewChildRecoveryStack(activation.Stack)
	paused.Stamp(gatherIterationID(activation.Unit, 1), nil, errors.New("paused"))
	activation.Stack.PushNested(paused)

	result, stack, err := p.Gather(activation, items, map[string]any{"limit": 2})
	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}

	results := result.([]any)
	if results[0] != "done-0" {
		t.Errorf("results[0] = %v, want done-0 (skipped and replayed)", results[0])
	}

	sub1, ok := stack.NestedStackByUnitID(gatherIterationID(activation.Unit, 1))
	if !ok {
		t.Fatal("iteration 1 substack missing after resume")
	}
	if sub1.Err() != nil {
		t.Errorf("iteration 1 Err() = %v after re-entry, want nil (re-entered and completed)", sub1.Err())
	}

	// The paused iteration was adopted in place, not re-nested: still exactly N substacks.
	if stack.Len() != len(items) {
		t.Errorf("stack.Len() = %d, want %d (paused iteration adopted in place, not duplicated)", stack.Len(), len(items))
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
