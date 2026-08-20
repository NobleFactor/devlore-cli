// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package op

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/application"
)

// The two fixture providers below exercise the executor's failure-terminal split (phase-8 step 21): each pairs a
// compensable Produce (whose compensator is a bare [*ReceiptBase]) with an always-failing Explode. They differ only
// in their CompensateProduce companion — one errors (the unwind fails → stopped × [ConditionCompensationFailed]),
// one succeeds (the unwind is clean → stopped × [ConditionExecutionFailed]). Announced at init (ahead of the
// registry singleton's snapshot); inert to
// every other test because only these tests name their actions.

type compensationFailingFixture struct{ ProviderBase }

func (p *compensationFailingFixture) Produce(*ActivationRecord) (string, *ReceiptBase, error) {
	return "made", &ReceiptBase{}, nil
}

func (p *compensationFailingFixture) CompensateProduce(*ActivationRecord, *ReceiptBase) error {
	return errors.New("undo exploded: resource is stuck")
}

func (p *compensationFailingFixture) Explode(input string) error {
	return errors.New("forward failure after " + input)
}

// compensationFlakyAttempts counts CompensateProduce attempts across executor instances for the flaky fixture —
// the resumed-unwind tests reset it; the graph-executor tests do not run in parallel.
var compensationFlakyAttempts int

// compensationFlakyFixture fails its first CompensateProduce and succeeds afterward — the "operator cleared the
// blocker" shape the step-21 resumed state-checked unwind exercises.
type compensationFlakyFixture struct{ ProviderBase }

func (p *compensationFlakyFixture) Produce(*ActivationRecord) (string, *ReceiptBase, error) {
	return "made", &ReceiptBase{}, nil
}

func (p *compensationFlakyFixture) CompensateProduce(*ActivationRecord, *ReceiptBase) error {
	compensationFlakyAttempts++
	if compensationFlakyAttempts == 1 {
		return errors.New("undo blocked: clear the blocker and resume")
	}
	return nil
}

func (p *compensationFlakyFixture) Explode(input string) error {
	return errors.New("forward failure after " + input)
}

type compensationCleanFixture struct{ ProviderBase }

func (p *compensationCleanFixture) Produce(*ActivationRecord) (string, *ReceiptBase, error) {
	return "made", &ReceiptBase{}, nil
}

func (p *compensationCleanFixture) CompensateProduce(*ActivationRecord, *ReceiptBase) error {
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

	AnnounceProvider(reflect.TypeFor[compensationFlakyFixture](), RoleAction,
		func(runtimeEnvironment *RuntimeEnvironment) (any, error) {
			return &compensationFlakyFixture{ProviderBase: NewProviderBase(runtimeEnvironment)}, nil
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

// callerStampFixture pins step 30's graph-dispatch caller id: Mint interns a resource into the session catalog
// under the activation's caller id — exactly what real providers do — and returns the stamp the catalog recorded,
// so the test observes the id flowing executor → activation → producer stamp. Announced at init; inert to every
// other test because only its test names this action.
type callerStampFixture struct{ ProviderBase }

func (p *callerStampFixture) Mint(activation *ActivationRecord) (string, error) {

	resource, err := activation.RuntimeEnvironment.ResourceCatalog.GetOrCreate(
		activation.CallerID, "mem://caller-stamp", func() (Resource, error) {
			return newFake("mem://caller-stamp", 0, ""), nil
		})
	if err != nil {
		return "", err
	}

	return resource.ProducerID(), nil
}

func init() {

	AnnounceProvider(reflect.TypeFor[callerStampFixture](), RoleAction,
		func(runtimeEnvironment *RuntimeEnvironment) (any, error) {
			return &callerStampFixture{ProviderBase: NewProviderBase(runtimeEnvironment)}, nil
		},
		map[string]MethodMetadata{
			"Mint": {},
		})
}

// runFailingFixtureGraph builds and runs a two-node graph against the named fixture provider: "producer" completes
// (pushing its compensable receipt), then "exploder" — consuming the producer's promise, so toposort orders them —
// fails, forcing the executor to unwind. Returns the executor and Run's error.
func runFailingFixtureGraph(t *testing.T, providerName string) (*GraphExecutor, error) {

	executor, _, err := runFailingFixtureGraphKeepGraph(t, providerName)
	return executor, err
}

// runFailingFixtureGraphKeepGraph is [runFailingFixtureGraph] returning the graph too, for the resume tests.
func runFailingFixtureGraphKeepGraph(t *testing.T, providerName string) (*GraphExecutor, *Graph, error) {

	t.Helper()

	produceAction, err := ReceiverRegistry().BuildAction(ActionName(providerName + ".produce"))
	if err != nil {
		t.Fatalf("BuildAction(produce): %v", err)
	}
	explodeAction, err := ReceiverRegistry().BuildAction(ActionName(providerName + ".explode"))
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
	return executor, graph, runErr
}

func TestRun_CompensationFailure_ReachesCompensationFailed(t *testing.T) {

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

	// The framework retains the compensation-failure journal (phase-8 step 21): the stack is NOT wiped, so a client can
	// persist + present it. It must carry the source (the failing node's forward error) and the diagnostics (the failed
	// compensation, recorded on its own receipt as CompensationError).
	trace := executor.Trace()
	if trace.Stack == nil || trace.Stack.Len() == 0 {
		t.Fatal("Trace().Stack is empty; the compensation-failure journal was destroyed (want it retained)")
	}

	var sawSource, sawCompensationFailure bool
	for _, receipt := range trace.Stack.Receipts() {
		if forwardErr := receipt.Err(); forwardErr != nil && strings.Contains(forwardErr.Error(), "forward failure") {
			sawSource = true
		}
		if undoErr := receipt.CompensationError(); undoErr != nil && strings.Contains(undoErr.Error(), "undo exploded") {
			sawCompensationFailure = true
		}
	}
	if !sawSource {
		t.Error("Trace().Stack carries no forward error; the source of the problem was not journaled")
	}
	if !sawCompensationFailure {
		t.Error("Trace().Stack carries no CompensationError; the failed compensation was not journaled")
	}
}

func TestRun_CleanUnwind_ReachesExecutionFailed(t *testing.T) {

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

	// A clean unwind clears the stack — the system is back at its pre-run baseline, so there is nothing to journal.
	if trace := executor.Trace(); trace.Stack != nil && trace.Stack.Len() != 0 {
		t.Errorf("Trace().Stack.Len() = %d after a clean unwind, want 0 (the journal is empty when nothing is dirty)",
			trace.Stack.Len())
	}
}

// TestRun_PreflightResolvesPendingResources proves the pre-flight resolve pass (phase-8 step 22 slice C).
//
// Pending entries of a participating type are probed exactly once through Resource.Exists, a non-enrolled type is never
// probed (the staged-rollout gate), a missing resource marks Gone without failing the run, and the graph's planning
// catalog stays pristine (the pass runs on the per-run clone).
func TestRun_PreflightResolvesPendingResources(t *testing.T) {

	presentBase, err := NewResourceBase(nil, "test:///present", reflect.TypeFor[*lifecycleResource]())
	if err != nil {
		t.Fatalf("NewResourceBase(present): %v", err)
	}
	missingBase, err := NewResourceBase(nil, "test:///missing", reflect.TypeFor[*lifecycleResource]())
	if err != nil {
		t.Fatalf("NewResourceBase(missing): %v", err)
	}

	present := &lifecycleResource{ResourceBase: presentBase, addressingMode: AddressingLocation, present: true}
	missing := &lifecycleResource{ResourceBase: missingBase, addressingMode: AddressingLocation, present: false}
	unenrolled := newLifecycle("test:///unenrolled", AddressingLocation) // bare base: type id "", not enrolled

	// Enroll the fixture type in the staged-rollout gate for the duration of this test.
	typeID := present.ResourceType()
	existenceVerifiableTypes[typeID] = struct{}{}
	t.Cleanup(func() { delete(existenceVerifiableTypes, typeID) })

	catalog := NewResourceCatalog()
	catalog.Resolve(present)
	catalog.Resolve(missing)
	catalog.Resolve(unenrolled)

	produceAction, err := ReceiverRegistry().BuildAction("compensationCleanFixture.produce")
	if err != nil {
		t.Fatalf("BuildAction(produce): %v", err)
	}
	producer, err := NewNode(NewNodeSpec().WithID("producer").WithAction(produceAction))
	if err != nil {
		t.Fatalf("NewNode(producer): %v", err)
	}
	graph, err := NewGraph(NewGraphSpec().WithOrigin(OriginBase{}).WithUnits(producer).WithResourceCatalog(catalog))
	if err != nil {
		t.Fatalf("NewGraph: %v", err)
	}

	executor := NewGraphExecutor(graph, NewRuntimeEnvironmentSpec("test").
		WithApplication(&application.Application{Name: "test"}))

	if _, err := executor.Run(context.Background(), nil); err != nil {
		t.Fatalf("Run: %v — a Gone resource is a recorded fact; the pass must mark, not fail", err)
	}

	if present.existsCalls != 1 {
		t.Errorf("present.existsCalls = %d, want 1 (a pending participating entry is probed once)", present.existsCalls)
	}
	if missing.existsCalls != 1 {
		t.Errorf("missing.existsCalls = %d, want 1 (a missing resource is probed, marked Gone, and the run proceeds)",
			missing.existsCalls)
	}
	if unenrolled.existsCalls != 0 {
		t.Errorf("unenrolled.existsCalls = %d, want 0 (the staging gate must skip non-enrolled types)",
			unenrolled.existsCalls)
	}

	// The pass ran on the per-run clone; the graph's planning catalog stays pristine for "plan once, run many."
	if got := catalog.State(present.ID()); got != Pending {
		t.Errorf("planning catalog State(present) = %v, want Pending (transitions must land on the clone only)", got)
	}
	if got := catalog.State(missing.ID()); got != Pending {
		t.Errorf("planning catalog State(missing) = %v, want Pending (transitions must land on the clone only)", got)
	}
}

// TestRun_OnRetryVeto_StampsRetryVetoed proves the OnRetry gate (phase-8 step 41 slice 2).
//
// A failing node with a RetryPolicy and an OnRetry handler whose result is falsy vetoes the retry loop, so the run's
// terminal Reason is retry_vetoed rather than the objective action_failed default.
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

// TestFailureReason_DefaultsToActionFailed proves the objective action_failed default.
//
// A plain propagating error carries no dispatch reason, so the boundary unwind stamps action_failed.
func TestFailureReason_DefaultsToActionFailed(t *testing.T) {

	if got := failureReason(errors.New("bare")); got != ReasonActionFailed {
		t.Errorf("failureReason(bare error) = %v, want action_failed", got)
	}
}

// TestFailureReason_CarriesReasonThroughWrapping proves a dispatchFailure's Reason survives error wrapping.
//
// The Reason reaches the boundary even after the subgraph walk re-wraps the error with %w, and the failure unwraps
// to its cause for errors.Is.
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

// TestRun_OnErrorAbsorb_NodeSucceeds proves the OnError verdict (phase-8 step 41 slice 3).
//
// A failing node whose OnError handler returns a truthy value absorbs the failure — the handler's return becomes the
// node's result, no flip fires, and the run completes healthy.
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
func runReactionGraph(t *testing.T, actionName ActionName, policy *TransitionPolicy) (*GraphExecutor, error) {

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

// TestRun_ReactionStop_FlipStopsRun proves the Stop reaction (phase-8 step 41 slice 17a).
//
// A driver that flips the condition to execution_failed under the floor policy (execution_failed → stop) stops the
// run, and the flip's reason rides the Stop error to the boundary (this is the mechanism item 13's flow.Failed will
// use).
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

// TestRun_ReactionPause_FlipPausesRun proves the Pause reaction.
//
// A driver that flips to degraded under a degraded → pause policy parks the run run-globally (ErrPaused, Phase
// paused).
func TestRun_ReactionPause_FlipPausesRun(t *testing.T) {

	executor, err := runReactionGraph(t, "retryHandlingFixture.degrade",
		&TransitionPolicy{Degraded: ReactionPause, ExecutionFailed: ReactionStop, CompensationFailed: ReactionStop})
	if !errors.Is(err, ErrPaused) {
		t.Fatalf("Run error = %v, want ErrPaused (the pause reaction)", err)
	}

	if got := executor.RunStatus(); got.Phase != PhasePaused || got.Condition != ConditionDegraded {
		t.Errorf("RunStatus() = %v, want paused/degraded (the flip condition bubbles up to the run level)", got)
	}
}

// TestRun_ReactionContinue_FlipKeepsWalking proves the Continue reaction.
//
// A driver that flips to degraded under the floor policy (degraded → continue) keeps walking, so the run completes —
// degraded, not healthy (slice 17b bubble-up).
func TestRun_ReactionContinue_FlipKeepsWalking(t *testing.T) {

	executor, err := runReactionGraph(t, "retryHandlingFixture.degrade", nil)
	if err != nil {
		t.Fatalf("Run returned %v; want nil (degraded → continue keeps walking)", err)
	}

	if got := executor.RunStatus(); got.Phase != PhaseCompleted || got.Condition != ConditionDegraded {
		t.Errorf("RunStatus() = %v, want completed/degraded (the flip condition bubbles up to the run level)", got)
	}
}

// TestRun_ReactionStop_DegradedStopsAtDegraded proves the boundary honors a recorded condition (slice 17b).
//
// A degraded flip under a degraded → stop policy stops the run at stopped × degraded, not execution_failed.
func TestRun_ReactionStop_DegradedStopsAtDegraded(t *testing.T) {

	executor, err := runReactionGraph(t, "retryHandlingFixture.degrade",
		&TransitionPolicy{Degraded: ReactionStop, ExecutionFailed: ReactionStop, CompensationFailed: ReactionStop})
	if err == nil {
		t.Fatal("Run returned no error; want the policy-driven stop")
	}

	got := executor.RunStatus()
	if got.Phase != PhaseStopped || got.Condition != ConditionDegraded {
		t.Errorf("RunStatus() = %v, want stopped/degraded (the boundary honors the recorded condition)", got)
	}
	if got.Reason != ReasonDegraded {
		t.Errorf("RunStatus().Reason = %v, want degraded", got.Reason)
	}
}

// TestRun_CompletedExecutionFailed_StopContract proves the stop contract's loud-but-non-fatal terminal (item 18).
//
// A flip to execution_failed under a graph-level execution_failed → continue policy completes the run — Run returns
// nil error — yet RunStatus reports completed × execution_failed. The error reflects halting; the condition reflects
// health. The policy is set graph-wide so both the flip and the bubble-up at the root continue.
func TestRun_CompletedExecutionFailed_StopContract(t *testing.T) {

	haltAction, err := ReceiverRegistry().BuildAction("retryHandlingFixture.halt")
	if err != nil {
		t.Fatalf("BuildAction(halt): %v", err)
	}
	haltNode, err := NewNode(NewNodeSpec().WithID("halt").WithAction(haltAction))
	if err != nil {
		t.Fatalf("NewNode(halt): %v", err)
	}

	policy := &TransitionPolicy{
		Degraded: ReactionContinue, ExecutionFailed: ReactionContinue, CompensationFailed: ReactionStop,
	}
	graph, err := NewGraph(NewGraphSpec().WithOrigin(OriginBase{}).WithUnits(haltNode).WithTransitionPolicy(policy))
	if err != nil {
		t.Fatalf("NewGraph: %v", err)
	}

	executor := NewGraphExecutor(graph, NewRuntimeEnvironmentSpec("test").
		WithApplication(&application.Application{Name: "test"}))

	if _, runErr := executor.Run(context.Background(), nil); runErr != nil {
		t.Fatalf("Run returned %v; want nil (execution_failed → continue runs to the end)", runErr)
	}

	got := executor.RunStatus()
	if got.Phase != PhaseCompleted || got.Condition != ConditionExecutionFailed {
		t.Errorf("RunStatus() = %v, want completed/execution_failed (ran to the end despite the failure)", got)
	}
}

// TestRun_ReactionStop_BypassesOnError proves the item-13 bypass.
//
// A policy-driven Stop (a flip to execution_failed — the mechanism flow.Failed uses) is a hard assertion, not an
// incidental failure, so it bypasses even an ancestor's OnError rather than being absorbed. The graph's root carries
// a truthy OnError handler that would absorb an ordinary
// failure; the Stop must stop the run regardless.
func TestRun_ReactionStop_BypassesOnError(t *testing.T) {

	haltAction, err := ReceiverRegistry().BuildAction("retryHandlingFixture.halt")
	if err != nil {
		t.Fatalf("BuildAction(halt): %v", err)
	}
	haltNode, err := NewNode(NewNodeSpec().WithID("halt").WithAction(haltAction))
	if err != nil {
		t.Fatalf("NewNode(halt): %v", err)
	}

	recoverAction, err := ReceiverRegistry().BuildAction("retryHandlingFixture.recover")
	if err != nil {
		t.Fatalf("BuildAction(recover): %v", err)
	}
	onError, err := NewSubgraph(NewSubgraphSpec().WithID("on-error").WithAction(recoverAction))
	if err != nil {
		t.Fatalf("NewSubgraph(on-error): %v", err)
	}

	graph, err := NewGraph(NewGraphSpec().WithOrigin(OriginBase{}).WithUnits(haltNode).WithOnError(onError))
	if err != nil {
		t.Fatalf("NewGraph: %v", err)
	}

	executor := NewGraphExecutor(graph, NewRuntimeEnvironmentSpec("test").
		WithApplication(&application.Application{Name: "test"}))

	if _, runErr := executor.Run(context.Background(), nil); runErr == nil {
		t.Fatal("Run returned nil; the root OnError absorbed a policy-driven stop, want the stop to bypass OnError")
	}

	got := executor.RunStatus()
	if got.Phase != PhaseStopped || got.Condition != ConditionExecutionFailed {
		t.Errorf("RunStatus() = %v, want stopped/execution_failed (the Stop bypassed OnError)", got)
	}
}

// TestFrameworkFailure_MarksAndJournals proves the framework-dispatch hardening (slice 17c).
//
// frameworkFailure journals an execution_failed × framework_failed flip on the executor and returns a
// dispatchFailure that reads as a framework
// failure — so the dispatch machinery bypasses OnError — and carries the reason to the boundary. A structural dispatch
// error (no action bound, action-name resolution, malformed topology) is thus never absorbed as an incidental failure.
func TestFrameworkFailure_MarksAndJournals(t *testing.T) {

	action, err := ReceiverRegistry().BuildAction("retryHandlingFixture.halt")
	if err != nil {
		t.Fatalf("BuildAction: %v", err)
	}
	node, err := NewNode(NewNodeSpec().WithID("driver").WithAction(action))
	if err != nil {
		t.Fatalf("NewNode: %v", err)
	}
	graph, err := NewGraph(NewGraphSpec().WithOrigin(OriginBase{}).WithUnits(node))
	if err != nil {
		t.Fatalf("NewGraph: %v", err)
	}
	executor := NewGraphExecutor(graph, NewRuntimeEnvironmentSpec("test").
		WithApplication(&application.Application{Name: "test"}))

	failure := executor.frameworkFailure("driver", errors.New("no action bound"))

	if !isHardFailure(failure) {
		t.Error("isHardFailure(frameworkFailure result) = false, want true (must bypass OnError)")
	}
	if got := failureReason(failure); got != ReasonFrameworkFailed {
		t.Errorf("failureReason = %v, want framework_failed", got)
	}
	if got := conditionForReason(ReasonFrameworkFailed); got != ConditionExecutionFailed {
		t.Errorf("conditionForReason(framework_failed) = %v, want execution_failed", got)
	}

	// The flip is journaled on the executor's run status.
	if got := executor.RunStatus(); got.Condition != ConditionExecutionFailed || got.Reason != ReasonFrameworkFailed {
		t.Errorf("RunStatus() = %v, want execution_failed/framework_failed (journaled flip)", got)
	}

	// A plain action failure is NOT a hard failure (OnError still adjudicates those); the marker survives wrapping.
	if isHardFailure(&dispatchFailure{reason: ReasonActionFailed, cause: errors.New("x")}) {
		t.Error("isHardFailure(action_failed) = true, want false")
	}
	if !isHardFailure(fmt.Errorf("subgraph x: %w", failure)) {
		t.Error("isHardFailure(wrapped) = false, want true (must traverse wrapping)")
	}

	// Serialized snake name.
	if name, _ := ReasonFrameworkFailed.MarshalText(); string(name) != "framework_failed" {
		t.Errorf("ReasonFrameworkFailed serialized = %q, want framework_failed", name)
	}
}

// TestResumeUnwind_CleanSecondPass_DeEscalates pins the step-21 Restart contract end to end.
//
// A run lands at stopped × compensation_failed, the operator clears the blocker (the flaky fixture succeeds on its
// second attempt), and the resumed state-checked unwind clears the journal and de-escalates to
// stopped × execution_failed with the sanctioned ReasonUnwound journal entry.
func TestResumeUnwind_CleanSecondPass_DeEscalates(t *testing.T) {

	compensationFlakyAttempts = 0

	executor, graph, err := runFailingFixtureGraphKeepGraph(t, "compensationFlakyFixture")
	if err == nil {
		t.Fatal("expected the forward failure; got nil")
	}
	if got := executor.RunStatus(); got.Phase != PhaseStopped || got.Condition != ConditionCompensationFailed {
		t.Fatalf("first run status = %s, want stopped × compensation_failed", got)
	}

	resumed, err := ResumeExecutor(graph, NewRuntimeEnvironmentSpec("test").
		WithApplication(&application.Application{Name: "test"}), executor.Trace())
	if err != nil {
		t.Fatalf("ResumeExecutor: %v", err)
	}

	if err := resumed.ResumeUnwind(context.Background()); err != nil {
		t.Fatalf("ResumeUnwind: %v", err)
	}

	status := resumed.RunStatus()
	if status.Phase != PhaseStopped || status.Condition != ConditionExecutionFailed || status.Reason != ReasonUnwound {
		t.Errorf("resumed status = %+v, want stopped × execution_failed × unwound", status)
	}
	if trace := resumed.Trace(); trace.Stack != nil && len(trace.Stack.entries) != 0 {
		t.Errorf("journal not cleared by the clean resumed unwind: %d entries", len(trace.Stack.entries))
	}
	transitions := resumed.Trace().Transitions
	if len(transitions) == 0 || transitions[len(transitions)-1].Reason != ReasonUnwound {
		t.Errorf("the de-escalation was not journaled; transitions = %+v", transitions)
	}
}

// TestResumeUnwind_StillDirty_StaysCompensationFailed pins the dirty half.
//
// A still-failing compensation leaves the run at stopped × compensation_failed with the journal retained.
func TestResumeUnwind_StillDirty_StaysCompensationFailed(t *testing.T) {

	executor, graph, err := runFailingFixtureGraphKeepGraph(t, "compensationFailingFixture")
	if err == nil {
		t.Fatal("expected the forward failure; got nil")
	}

	resumed, err := ResumeExecutor(graph, NewRuntimeEnvironmentSpec("test").
		WithApplication(&application.Application{Name: "test"}), executor.Trace())
	if err != nil {
		t.Fatalf("ResumeExecutor: %v", err)
	}

	if err := resumed.ResumeUnwind(context.Background()); err == nil {
		t.Fatal("expected the still-dirty unwind error; got nil")
	}

	status := resumed.RunStatus()
	if status.Phase != PhaseStopped || status.Condition != ConditionCompensationFailed {
		t.Errorf("resumed status = %+v, want stopped × compensation_failed retained", status)
	}
	if trace := resumed.Trace(); trace.Stack == nil || len(trace.Stack.entries) == 0 {
		t.Error("the dirty journal was not retained")
	}
}

// TestResumeUnwind_RefusesWrongState pins the precondition.
//
// Only a stopped × compensation_failed executor may resume-unwind.
func TestResumeUnwind_RefusesWrongState(t *testing.T) {

	graph, err := NewGraph(NewGraphSpec().WithOrigin(OriginBase{}))
	if err != nil {
		t.Fatalf("NewGraph: %v", err)
	}
	executor := NewGraphExecutor(graph, NewRuntimeEnvironmentSpec("test").
		WithApplication(&application.Application{Name: "test"}))

	if err := executor.ResumeUnwind(context.Background()); err == nil {
		t.Fatal("expected the wrong-state refusal; got nil")
	}
}

// TestRun_GraphDispatch_CallerIDIsUnitID pins step 30's graph half.
//
// Under graph dispatch the caller id IS the executing unit's id — the executor constructs the activation with the
// node id, and a resource the action interns
// carries that id as its producer stamp. The starlark half (a `file:line:col` call site) is pinned in
// starlarkbridge's TestGoReceiver_StarlarkDispatchStampsCallSite.
func TestRun_GraphDispatch_CallerIDIsUnitID(t *testing.T) {

	mintAction, err := ReceiverRegistry().BuildAction("callerStampFixture.mint")
	if err != nil {
		t.Fatalf("BuildAction(mint): %v", err)
	}

	node, err := NewNode(NewNodeSpec().WithID("mint-step").WithAction(mintAction))
	if err != nil {
		t.Fatalf("NewNode: %v", err)
	}

	graph, err := NewGraph(NewGraphSpec().WithOrigin(OriginBase{}).WithUnits(node))
	if err != nil {
		t.Fatalf("NewGraph: %v", err)
	}

	executor := NewGraphExecutor(graph, NewRuntimeEnvironmentSpec("test").
		WithApplication(&application.Application{Name: "test"}))

	result, err := executor.Run(context.Background(), nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	stamp, ok := result.(string)
	if !ok {
		t.Fatalf("Run result = %T, want the string producer stamp", result)
	}
	if stamp != "mint-step" {
		t.Errorf("producer stamp = %q, want the unit id %q", stamp, "mint-step")
	}
}

// TestRetryPolicyFor_TriState pins the step-35 resolution.
//
// An explicit policy wins (MaxAttempts:0 = deliberate no-retry); a structural nested subgraph (flow.subgraph, not
// the root) inherits policies.retry; the graph root, the flow combinators (flow.gather here), a node (which stamps
// an explicit MaxAttempts:0 at construction), and
// every other unit resolve to none. flow.subgraph / flow.gather / flow.complete resolve via flow_announce_test.go.
func TestRetryPolicyFor_TriState(t *testing.T) {

	graph, err := NewGraph(NewGraphSpec().WithOrigin(OriginBase{}))
	if err != nil {
		t.Fatalf("NewGraph: %v", err)
	}
	executor := NewGraphExecutor(graph, NewRuntimeEnvironmentSpec("test").
		WithApplication(&application.Application{Name: "test"}))

	// The root (flow.subgraph, but no "up" to roll back to) is exempt.
	if got := executor.retryPolicyFor(graph.Root()); got != nil {
		t.Errorf("root: retryPolicyFor = %+v, want nil (root exempt)", got)
	}

	// A structural nested subgraph inherits policies.retry.
	structural, err := NewSubgraph(NewSubgraphSpec().WithID("nested").WithActionNamed("flow.subgraph"))
	if err != nil {
		t.Fatalf("NewSubgraph(structural): %v", err)
	}
	if got := executor.retryPolicyFor(structural); got == nil || got.MaxAttempts != 3 {
		t.Errorf("structural nested subgraph: retryPolicyFor = %+v, want policies.retry (MaxAttempts 3)", got)
	}

	// A flow combinator keeps its own failure semantics — no default retry.
	combinator, err := NewSubgraph(NewSubgraphSpec().WithID("g").WithActionNamed("flow.gather"))
	if err != nil {
		t.Fatalf("NewSubgraph(combinator): %v", err)
	}
	if got := executor.retryPolicyFor(combinator); got != nil {
		t.Errorf("combinator: retryPolicyFor = %+v, want nil (combinators are excluded)", got)
	}

	// A node stamps explicit no-retry at construction, so it resolves to that (MaxAttempts:0), not the default.
	node, err := NewNode(NewNodeSpec().WithID("leaf").WithActionNamed("flow.complete"))
	if err != nil {
		t.Fatalf("NewNode: %v", err)
	}
	if got := executor.retryPolicyFor(node); got == nil || got.MaxAttempts != 0 {
		t.Errorf("node: retryPolicyFor = %+v, want explicit MaxAttempts 0", got)
	}

	// An explicit policy on a structural subgraph wins over the default.
	explicit, err := NewSubgraph(NewSubgraphSpec().WithID("x").WithActionNamed("flow.subgraph").
		WithRetryPolicy(&RetryPolicy{MaxAttempts: 7}))
	if err != nil {
		t.Fatalf("NewSubgraph(explicit): %v", err)
	}
	if got := executor.retryPolicyFor(explicit); got == nil || got.MaxAttempts != 7 {
		t.Errorf("explicit: retryPolicyFor = %+v, want the explicit MaxAttempts 7", got)
	}
}

// TestNewNode_StampsExplicitNoRetry pins the step-35 node default.
//
// Construction stamps an explicit MaxAttempts:0 (not nil), so a leaf's no-retry intent is unambiguous rather than
// colliding with a subgraph's "inherit the default".
func TestNewNode_StampsExplicitNoRetry(t *testing.T) {

	node, err := NewNode(NewNodeSpec().WithID("leaf").WithActionNamed("flow.complete"))
	if err != nil {
		t.Fatalf("NewNode: %v", err)
	}

	policy := node.RetryPolicy()
	if policy == nil {
		t.Fatal("node RetryPolicy() = nil, want an explicit no-retry policy")
	}
	if policy.MaxAttempts != 0 {
		t.Errorf("node default MaxAttempts = %d, want 0", policy.MaxAttempts)
	}
}

// newRecoverGraph builds a single-node graph bound to the always-succeeding retryHandlingFixture.recover action,
// for tests that need a trivially green dispatch.
func newRecoverGraph(t *testing.T) *Graph {

	t.Helper()

	action, err := ReceiverRegistry().BuildAction("retryHandlingFixture.recover")
	if err != nil {
		t.Fatalf("BuildAction(recover): %v", err)
	}

	node, err := NewNode(NewNodeSpec().WithID("recoverer").WithAction(action))
	if err != nil {
		t.Fatalf("NewNode(recoverer): %v", err)
	}

	graph, err := NewGraph(NewGraphSpec().WithOrigin(OriginBase{}).WithUnits(node))
	if err != nil {
		t.Fatalf("NewGraph: %v", err)
	}

	return graph
}

// TestRun_BadRootAnchor_PreflightFailed proves a Root-mint failure is a preflight failure (issue #393).
//
// Run refuses before any dispatch and lands stopped × execution_failed × preflight_failed.
func TestRun_BadRootAnchor_PreflightFailed(t *testing.T) {

	missing := filepath.Join(t.TempDir(), "absent")
	executor := NewGraphExecutor(newRecoverGraph(t), NewRuntimeEnvironmentSpec("test").
		WithApplication(&application.Application{Name: "test"}).
		WithRoot(missing))

	if _, runErr := executor.Run(context.Background(), nil); runErr == nil {
		t.Fatal("Run = nil error, want the mint failure for the missing anchor")
	}

	got := executor.RunStatus()
	if got.Phase != PhaseStopped || got.Condition != ConditionExecutionFailed || got.Reason != ReasonPreflightFailed {
		t.Errorf("RunStatus() = %v, want stopped × execution_failed × preflight_failed", got)
	}
}

// TestRun_SequentialExecutorsShareOneSpec is the closed-Root-reuse regression (issue #393, the cmd/lore loop shape).
//
// The spec carries only an anchor, so every executor's Run mints its own Root and a second executor built from the
// SAME spec runs cleanly after the first one's teardown. Under the pre-#393 design the first Run closed the spec's
// shared live handle and the second dispatched against a closed Root.
func TestRun_SequentialExecutorsShareOneSpec(t *testing.T) {

	graph := newRecoverGraph(t)
	spec := NewRuntimeEnvironmentSpec("test").
		WithApplication(&application.Application{Name: "test"}).
		WithRoot(t.TempDir())

	for i := 1; i <= 2; i++ {
		if _, runErr := NewGraphExecutor(graph, spec).Run(context.Background(), nil); runErr != nil {
			t.Fatalf("Run #%d: %v (each Run must mint its own Root from the shared spec)", i, runErr)
		}
	}
}
