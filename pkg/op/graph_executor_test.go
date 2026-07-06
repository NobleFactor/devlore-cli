// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/application"
)

// The two fixture providers below exercise the executor's failure-terminal split (phase-8 step 21): each pairs a
// compensable Produce (whose complement is a bare [*ReceiptBase]) with an always-failing Explode. They differ only
// in their CompensateProduce companion — one errors (the unwind fails → [RunStateFailedCompensation]), one succeeds
// (the unwind is clean → [RunStateFailed]). Announced at init (ahead of the registry singleton's snapshot); inert to
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

	if got := executor.State(); got != RunStateFailedCompensation {
		t.Errorf("State() = %v, want %v (unwind failed — the system is dirty)", got, RunStateFailedCompensation)
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

	if got := executor.State(); got != RunStateFailed {
		t.Errorf("State() = %v, want %v (clean unwind — back at the pre-run state)", got, RunStateFailed)
	}
}
