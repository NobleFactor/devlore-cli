// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/application"
)

// The two fixture providers below exercise the executor's failure-terminal split (phase-8 step 21): each pairs a
// compensable Produce (whose complement is a bare [*ReceiptBase]) with an always-failing Explode. They differ only
// in their CompensateProduce companion — one errors (the unwind fails → stopped × [ConditionCompensationFailed]),
// one succeeds (the unwind is clean → stopped × [ConditionExecutionFailed]). Announced at init (ahead of the
// registry singleton's snapshot); inert to
// every other test because only these tests name their actions.

type compensationFailingFixture struct{ ProviderBase }

func (p *compensationFailingFixture) Produce() (string, *ReceiptBase, error) {
	return "made", &ReceiptBase{}, nil
}

func (p *compensationFailingFixture) CompensateProduce(*ReceiptBase) error {
	return errors.New("undo exploded: resource is stuck")
}

func (p *compensationFailingFixture) Explode(input string) error {
	return errors.New("forward failure after " + input)
}

type compensationCleanFixture struct{ ProviderBase }

func (p *compensationCleanFixture) Produce() (string, *ReceiptBase, error) {
	return "made", &ReceiptBase{}, nil
}

func (p *compensationCleanFixture) CompensateProduce(*ReceiptBase) error {
	return nil
}

func (p *compensationCleanFixture) Explode(input string) error {
	return errors.New("forward failure after " + input)
}

func init() {

	AnnounceProvider(reflect.TypeFor[compensationFailingFixture](), RoleAction,
		func(runtimeEnvironment *RuntimeEnvironment) (any, error) {
			return &compensationFailingFixture{ProviderBase: NewProviderBase(runtimeEnvironment)}, nil
		},
		map[string]MethodMetadata{
			"Produce": {},
			"Explode": {ParameterNames: []string{"input"}},
		})

	AnnounceProvider(reflect.TypeFor[compensationCleanFixture](), RoleAction,
		func(runtimeEnvironment *RuntimeEnvironment) (any, error) {
			return &compensationCleanFixture{ProviderBase: NewProviderBase(runtimeEnvironment)}, nil
		},
		map[string]MethodMetadata{
			"Produce": {},
			"Explode": {ParameterNames: []string{"input"}},
		})
}

// retryHandlingFixture drives the OnRetry gate (phase-8 step 41 slice 2): Fail always errors — the action under
// retry — and Veto returns a falsy value, the OnRetry handler body that vetoes the loop. Announced at init; inert to
// every other test because only its test names these actions.
type retryHandlingFixture struct{ ProviderBase }

func (p *retryHandlingFixture) Fail() error {
	return errors.New("always fails")
}

func (p *retryHandlingFixture) Veto() (bool, error) {
	return false, nil
}

func (p *retryHandlingFixture) Recover() (string, error) {
	return "recovered", nil
}

// Degrade and Halt are condition-flip drivers (like flow.Degraded / flow.Failed): each receives the framework-injected
// activation record and submits a Transition on its own boundary, exercising the TransitionPolicy reaction consumption.

func (p *retryHandlingFixture) Degrade(activationRecord *ActivationRecord) error {
	_ = activationRecord.Transition(ConditionDegraded, ReasonDegraded, "degrade fixture executed")
	return nil
}

func (p *retryHandlingFixture) Halt(activationRecord *ActivationRecord) error {
	_ = activationRecord.Transition(ConditionExecutionFailed, ReasonFailed, "halt fixture executed")
	return nil
}

func init() {

	AnnounceProvider(reflect.TypeFor[retryHandlingFixture](), RoleAction,
		func(runtimeEnvironment *RuntimeEnvironment) (any, error) {
			return &retryHandlingFixture{ProviderBase: NewProviderBase(runtimeEnvironment)}, nil
		},
		map[string]MethodMetadata{
			"Fail":    {},
			"Veto":    {},
			"Recover": {},
			"Degrade": {},
			"Halt":    {},
		})
}

// runFailingFixtureGraph builds and runs a two-node graph against the named fixture provider: "producer" completes
// (pushing its compensable receipt), then "exploder" — consuming the producer's promise, so toposort orders them —
// fails, forcing the executor to unwind. Returns the executor and Run's error.
func runFailingFixtureGraph(t *testing.T, providerName string) (*GraphExecutor, error) {

	t.Helper()

	produceAction, err := ReceiverRegistry().BuildAction(providerName + ".produce")
	if err != nil {
		t.Fatalf("BuildAction(produce): %v", err)
	}
	explodeAction, err := ReceiverRegistry().BuildAction(providerName + ".explode")
	if err != nil {
		t.Fatalf("BuildAction(explode): %v", err)
	}

	producer, err := NewNode(NewNodeSpec().WithID("producer").WithAction(produceAction))
	if err != nil {
		t.Fatalf("NewNode(producer): %v", err)
	}

	exploderSpec := NewNodeSpec().WithID("exploder").WithAction(explodeAction)
	exploderSpec.WithSlot("input", NewPromiseBinding("producer"))
	exploder, err := NewNode(exploderSpec)
	if err != nil {
		t.Fatalf("NewNode(exploder): %v", err)
	}

	graph, err := NewGraph(NewGraphSpec().WithOrigin(OriginBase{}).WithUnits(producer, exploder))
	if err != nil {
		t.Fatalf("NewGraph: %v", err)
	}

	executor := NewGraphExecutor(graph, NewRuntimeEnvironmentSpec("test").
		WithApplication(&application.Application{Name: "test"}))

	_, runErr := executor.Run(context.Background(), nil)
	return executor, runErr
}

func TestRun_CompensationFailure_ReachesFailedCompensation(t *testing.T) {

	executor, err := runFailingFixtureGraph(t, "compensationFailingFixture")

	if err == nil {
		t.Fatal("Run returned no error; want the joined forward + compensation failure")
	}

	// Fail loud (contract R2): the error names the forward failure AND the failed compensation.
	for _, want := range []string{"forward failure", "undo exploded"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Run error %q does not name %q", err, want)
		}
	}

	got := executor.RunStatus()
	if got.Phase != PhaseStopped || got.Condition != ConditionCompensationFailed {
		t.Errorf("RunStatus() = %v, want stopped/compensation_failed (unwind failed — the system is dirty)", got)
	}
}

func TestRun_CleanUnwind_ReachesFailed(t *testing.T) {

	executor, err := runFailingFixtureGraph(t, "compensationCleanFixture")

	if err == nil {
		t.Fatal("Run returned no error; want the forward failure")
	}
	if strings.Contains(err.Error(), "compensation:") {
		t.Errorf("Run error %q reports a compensation failure; the unwind was clean", err)
	}

	got := executor.RunStatus()
	if got.Phase != PhaseStopped || got.Condition != ConditionExecutionFailed {
		t.Errorf("RunStatus() = %v, want stopped/execution_failed (clean unwind — back at the pre-run state)", got)
	}
}

// TestRun_OnRetryVeto_StampsRetryVetoed proves the OnRetry gate (phase-8 step 41 slice 2): a failing node with a
// RetryPolicy and an OnRetry handler whose result is falsy vetoes the retry loop, so the run's terminal Reason is
// retry_vetoed rather than the objective action_failed default.
func TestRun_OnRetryVeto_StampsRetryVetoed(t *testing.T) {

	failAction, err := ReceiverRegistry().BuildAction("retryHandlingFixture.fail")
	if err != nil {
		t.Fatalf("BuildAction(fail): %v", err)
	}
	vetoAction, err := ReceiverRegistry().BuildAction("retryHandlingFixture.veto")
	if err != nil {
		t.Fatalf("BuildAction(veto): %v", err)
	}

	// The OnRetry handler is a subgraph whose bound action returns a falsy value — a falsy verdict vetoes the loop.
	handler, err := NewSubgraph(NewSubgraphSpec().WithID("on-retry").WithAction(vetoAction))
	if err != nil {
		t.Fatalf("NewSubgraph(handler): %v", err)
	}

	failNode, err := NewNode(NewNodeSpec().
		WithID("faller").
		WithAction(failAction).
		WithRetryPolicy(&RetryPolicy{MaxAttempts: 1}).
		WithOnRetry(handler))
	if err != nil {
		t.Fatalf("NewNode(faller): %v", err)
	}

	graph, err := NewGraph(NewGraphSpec().WithOrigin(OriginBase{}).WithUnits(failNode))
	if err != nil {
		t.Fatalf("NewGraph: %v", err)
	}

	executor := NewGraphExecutor(graph, NewRuntimeEnvironmentSpec("test").
		WithApplication(&application.Application{Name: "test"}))

	if _, runErr := executor.Run(context.Background(), nil); runErr == nil {
		t.Fatal("Run returned no error; want the vetoed forward failure")
	}

	got := executor.RunStatus()
	if got.Phase != PhaseStopped || got.Condition != ConditionExecutionFailed {
		t.Errorf("RunStatus() = %v, want stopped/execution_failed", got)
	}
	if got.Reason != ReasonRetryVetoed {
		t.Errorf("RunStatus().Reason = %v, want retry_vetoed (the OnRetry veto reason)", got.Reason)
	}
}

// TestFailureReason_DefaultsToActionFailed proves that a plain propagating error carries no dispatch reason, so the
// boundary unwind stamps the objective action_failed default.
func TestFailureReason_DefaultsToActionFailed(t *testing.T) {

	if got := failureReason(errors.New("bare")); got != ReasonActionFailed {
		t.Errorf("failureReason(bare error) = %v, want action_failed", got)
	}
}

// TestFailureReason_CarriesReasonThroughWrapping proves that a dispatchFailure carries its Reason to the boundary
// even after the subgraph walk re-wraps the error with %w, and that it unwraps to its cause for errors.Is.
func TestFailureReason_CarriesReasonThroughWrapping(t *testing.T) {

	cause := errors.New("action boom")
	failure := &dispatchFailure{reason: ReasonRetryVetoed, cause: cause}

	if got := failureReason(failure); got != ReasonRetryVetoed {
		t.Errorf("failureReason(dispatchFailure) = %v, want retry_vetoed", got)
	}

	// The boundary unwind sees the error after the subgraph walk re-wraps it — failureReason must traverse wrapping.
	wrapped := fmt.Errorf("subgraph x: child y: %w", failure)
	if got := failureReason(wrapped); got != ReasonRetryVetoed {
		t.Errorf("failureReason(wrapped) = %v, want retry_vetoed (must traverse %%w wrapping)", got)
	}

	if !errors.Is(wrapped, cause) {
		t.Error("errors.Is(wrapped, cause) = false; dispatchFailure must Unwrap to its cause")
	}
	if failure.Error() != "action boom" {
		t.Errorf("dispatchFailure.Error() = %q, want the cause's message %q", failure.Error(), "action boom")
	}
}

// TestFailureReason_HandlerFailed proves the handler-failure reason is carried the same way as a veto.
func TestFailureReason_HandlerFailed(t *testing.T) {

	failure := &dispatchFailure{reason: ReasonHandlerFailed, cause: errors.New("handler boom")}
	if got := failureReason(failure); got != ReasonHandlerFailed {
		t.Errorf("failureReason(handler-failed) = %v, want handler_failed", got)
	}
}

// TestRun_OnErrorAbsorb_NodeSucceeds proves the OnError verdict (phase-8 step 41 slice 3): a failing node whose
// OnError handler returns a truthy value absorbs the failure — the handler's return becomes the node's result, no flip
// fires, and the run completes healthy.
func TestRun_OnErrorAbsorb_NodeSucceeds(t *testing.T) {

	failAction, err := ReceiverRegistry().BuildAction("retryHandlingFixture.fail")
	if err != nil {
		t.Fatalf("BuildAction(fail): %v", err)
	}
	recoverAction, err := ReceiverRegistry().BuildAction("retryHandlingFixture.recover")
	if err != nil {
		t.Fatalf("BuildAction(recover): %v", err)
	}

	// The OnError handler is a subgraph whose bound action returns a truthy value — a truthy verdict absorbs.
	handler, err := NewSubgraph(NewSubgraphSpec().WithID("on-error").WithAction(recoverAction))
	if err != nil {
		t.Fatalf("NewSubgraph(handler): %v", err)
	}

	failNode, err := NewNode(NewNodeSpec().WithID("faller").WithAction(failAction).WithOnError(handler))
	if err != nil {
		t.Fatalf("NewNode(faller): %v", err)
	}

	graph, err := NewGraph(NewGraphSpec().WithOrigin(OriginBase{}).WithUnits(failNode))
	if err != nil {
		t.Fatalf("NewGraph: %v", err)
	}

	executor := NewGraphExecutor(graph, NewRuntimeEnvironmentSpec("test").
		WithApplication(&application.Application{Name: "test"}))

	result, runErr := executor.Run(context.Background(), nil)
	if runErr != nil {
		t.Fatalf("Run returned %v; want nil (the OnError handler absorbs the failure)", runErr)
	}
	if result != "recovered" {
		t.Errorf("Run result = %v, want %q (the handler's return becomes the node's result)", result, "recovered")
	}

	got := executor.RunStatus()
	if got.Phase != PhaseCompleted || got.Condition != ConditionHealthy {
		t.Errorf("RunStatus() = %v, want completed/healthy (an absorb rejects the flip)", got)
	}
}

// TestRun_OnErrorFalsy_FailureStands proves that a falsy OnError verdict lets the failure stand as action_failed.
func TestRun_OnErrorFalsy_FailureStands(t *testing.T) {

	failAction, err := ReceiverRegistry().BuildAction("retryHandlingFixture.fail")
	if err != nil {
		t.Fatalf("BuildAction(fail): %v", err)
	}
	vetoAction, err := ReceiverRegistry().BuildAction("retryHandlingFixture.veto")
	if err != nil {
		t.Fatalf("BuildAction(veto): %v", err)
	}

	handler, err := NewSubgraph(NewSubgraphSpec().WithID("on-error").WithAction(vetoAction))
	if err != nil {
		t.Fatalf("NewSubgraph(handler): %v", err)
	}

	failNode, err := NewNode(NewNodeSpec().WithID("faller").WithAction(failAction).WithOnError(handler))
	if err != nil {
		t.Fatalf("NewNode(faller): %v", err)
	}

	graph, err := NewGraph(NewGraphSpec().WithOrigin(OriginBase{}).WithUnits(failNode))
	if err != nil {
		t.Fatalf("NewGraph: %v", err)
	}

	executor := NewGraphExecutor(graph, NewRuntimeEnvironmentSpec("test").
		WithApplication(&application.Application{Name: "test"}))

	if _, runErr := executor.Run(context.Background(), nil); runErr == nil {
		t.Fatal("Run returned no error; want the standing failure (falsy OnError)")
	}

	got := executor.RunStatus()
	if got.Phase != PhaseStopped || got.Condition != ConditionExecutionFailed {
		t.Errorf("RunStatus() = %v, want stopped/execution_failed", got)
	}
	if got.Reason != ReasonActionFailed {
		t.Errorf("RunStatus().Reason = %v, want action_failed (a falsy OnError lets the failure stand)", got.Reason)
	}
}

// TestRun_OnErrorHandlerFails_HandlerFailed proves that an OnError handler that itself errors fails as handler_failed.
func TestRun_OnErrorHandlerFails_HandlerFailed(t *testing.T) {

	failAction, err := ReceiverRegistry().BuildAction("retryHandlingFixture.fail")
	if err != nil {
		t.Fatalf("BuildAction(fail): %v", err)
	}

	// The OnError handler's bound action itself errors — the verdict is handler_failed, not the action's failure.
	handlerAction, err := ReceiverRegistry().BuildAction("retryHandlingFixture.fail")
	if err != nil {
		t.Fatalf("BuildAction(fail) for handler: %v", err)
	}
	handler, err := NewSubgraph(NewSubgraphSpec().WithID("on-error").WithAction(handlerAction))
	if err != nil {
		t.Fatalf("NewSubgraph(handler): %v", err)
	}

	failNode, err := NewNode(NewNodeSpec().WithID("faller").WithAction(failAction).WithOnError(handler))
	if err != nil {
		t.Fatalf("NewNode(faller): %v", err)
	}

	graph, err := NewGraph(NewGraphSpec().WithOrigin(OriginBase{}).WithUnits(failNode))
	if err != nil {
		t.Fatalf("NewGraph: %v", err)
	}

	executor := NewGraphExecutor(graph, NewRuntimeEnvironmentSpec("test").
		WithApplication(&application.Application{Name: "test"}))

	if _, runErr := executor.Run(context.Background(), nil); runErr == nil {
		t.Fatal("Run returned no error; want the handler-failed failure")
	}

	got := executor.RunStatus()
	if got.Reason != ReasonHandlerFailed {
		t.Errorf("RunStatus().Reason = %v, want handler_failed (the OnError handler itself errored)", got.Reason)
	}
}

// runReactionGraph builds and runs a single-node graph whose node drives a condition flip via `actionName`, under an
// optional unit-level `policy` (nil = the builtin floor), returning the executor and Run's error.
func runReactionGraph(t *testing.T, actionName string, policy *TransitionPolicy) (*GraphExecutor, error) {

	t.Helper()

	action, err := ReceiverRegistry().BuildAction(actionName)
	if err != nil {
		t.Fatalf("BuildAction(%s): %v", actionName, err)
	}

	spec := NewNodeSpec().WithID("driver").WithAction(action)
	if policy != nil {
		spec.WithTransitionPolicy(policy)
	}
	driver, err := NewNode(spec)
	if err != nil {
		t.Fatalf("NewNode(driver): %v", err)
	}

	graph, err := NewGraph(NewGraphSpec().WithOrigin(OriginBase{}).WithUnits(driver))
	if err != nil {
		t.Fatalf("NewGraph: %v", err)
	}

	executor := NewGraphExecutor(graph, NewRuntimeEnvironmentSpec("test").
		WithApplication(&application.Application{Name: "test"}))

	_, runErr := executor.Run(context.Background(), nil)
	return executor, runErr
}

// TestRun_ReactionStop_FlipStopsRun proves the Stop reaction (phase-8 step 41 slice 17a): a driver that flips the
// condition to execution_failed under the floor policy (execution_failed → stop) stops the run, and the flip's reason
// rides the Stop error to the boundary (this is the mechanism item 13's flow.Failed will use).
func TestRun_ReactionStop_FlipStopsRun(t *testing.T) {

	executor, err := runReactionGraph(t, "retryHandlingFixture.halt", nil)
	if err == nil {
		t.Fatal("Run returned no error; want the policy-driven stop")
	}

	got := executor.RunStatus()
	if got.Phase != PhaseStopped || got.Condition != ConditionExecutionFailed {
		t.Errorf("RunStatus() = %v, want stopped/execution_failed", got)
	}
	if got.Reason != ReasonFailed {
		t.Errorf("RunStatus().Reason = %v, want failed (the flip reason, carried by the Stop reaction)", got.Reason)
	}
}

// TestRun_ReactionPause_FlipPausesRun proves the Pause reaction: a driver that flips to degraded under a
// degraded → pause policy parks the run run-globally (ErrPaused, Phase paused).
func TestRun_ReactionPause_FlipPausesRun(t *testing.T) {

	executor, err := runReactionGraph(t, "retryHandlingFixture.degrade",
		&TransitionPolicy{Degraded: ReactionPause, ExecutionFailed: ReactionStop, CompensationFailed: ReactionStop})
	if !errors.Is(err, ErrPaused) {
		t.Fatalf("Run error = %v, want ErrPaused (the pause reaction)", err)
	}

	if got := executor.RunStatus(); got.Phase != PhasePaused {
		t.Errorf("RunStatus().Phase = %v, want paused", got.Phase)
	}
}

// TestRun_ReactionContinue_FlipKeepsWalking proves the Continue reaction: a driver that flips to degraded under the
// floor policy (degraded → continue) keeps walking, so the run completes rather than stopping.
func TestRun_ReactionContinue_FlipKeepsWalking(t *testing.T) {

	executor, err := runReactionGraph(t, "retryHandlingFixture.degrade", nil)
	if err != nil {
		t.Fatalf("Run returned %v; want nil (degraded → continue keeps walking)", err)
	}

	if got := executor.RunStatus(); got.Phase != PhaseCompleted {
		t.Errorf("RunStatus().Phase = %v, want completed", got.Phase)
	}
}
