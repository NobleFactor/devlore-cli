// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package flow

import (
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// testProvider builds a flow.Provider with a minimal RuntimeEnvironment suitable for the saga-shape
// signature tests that don't dispatch into child ExecutableUnits.
func testProvider(t *testing.T) *Provider {
	t.Helper()
	ctx := &op.RuntimeEnvironment{}
	return &Provider{ProviderBase: op.NewProviderBase(ctx)}
}

func TestChoose_ReturnsRecoveryStack(t *testing.T) {

	p := testProvider(t)

	chosen, stack, err := p.Choose("default-value")
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

	chosen, stack, err := p.Choose("default", Case{When: false, Then: "skip-1"}, Case{When: true, Then: "winner"}, Case{When: true, Then: "skip-2"})
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

	if err := p.CompensateChoose(nil); err != nil {
		t.Errorf("CompensateChoose(nil) error = %v, want nil", err)
	}
}

func TestCompensateChoose_EmptyStack_NoOp(t *testing.T) {

	p := testProvider(t)

	if err := p.CompensateChoose(op.NewRecoveryStack()); err != nil {
		t.Errorf("CompensateChoose(empty) error = %v, want nil", err)
	}
}

func TestChoose_CompensateChoose_RoundTrip(t *testing.T) {

	p := testProvider(t)

	_, stack, err := p.Choose("default", Case{When: true, Then: "winner"})
	if err != nil {
		t.Fatalf("Choose() error = %v", err)
	}
	if compensateErr := p.CompensateChoose(stack); compensateErr != nil {
		t.Errorf("CompensateChoose() error = %v, want nil (empty-stack unwind is a no-op)", compensateErr)
	}
}

func TestSubgraph_ReturnsRecoveryStack(t *testing.T) {

	p := testProvider(t)

	results, stack, err := p.Subgraph()
	if err != nil {
		t.Fatalf("Subgraph() error = %v", err)
	}
	if len(results) != 0 {
		t.Errorf("Subgraph() with no children returned %d results; want 0", len(results))
	}
	if stack == nil {
		t.Fatal("Subgraph() returned nil *RecoveryStack; want empty stack per the saga-shape contract")
	}
	if stack.Len() != 0 {
		t.Errorf("Subgraph() returned stack with %d entries; want 0 (empty stub stack)", stack.Len())
	}
}

func TestCompensateSubgraph_NilStack_NoOp(t *testing.T) {

	p := testProvider(t)

	if err := p.CompensateSubgraph(nil); err != nil {
		t.Errorf("CompensateSubgraph(nil) error = %v, want nil", err)
	}
}

func TestSubgraph_CompensateSubgraph_RoundTrip(t *testing.T) {

	p := testProvider(t)

	_, stack, err := p.Subgraph()
	if err != nil {
		t.Fatalf("Subgraph() error = %v", err)
	}
	if compensateErr := p.CompensateSubgraph(stack); compensateErr != nil {
		t.Errorf("CompensateSubgraph() error = %v, want nil (empty-stack unwind is a no-op)", compensateErr)
	}
}

func TestCompensateGather_NilStack_NoOp(t *testing.T) {

	p := testProvider(t)

	if err := p.CompensateGather(nil); err != nil {
		t.Errorf("CompensateGather(nil) error = %v, want nil", err)
	}
}

func TestCompensateGather_EmptyStack_NoOp(t *testing.T) {

	p := testProvider(t)

	if err := p.CompensateGather(op.NewRecoveryStack()); err != nil {
		t.Errorf("CompensateGather(empty) error = %v, want nil", err)
	}
}