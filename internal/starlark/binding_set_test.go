// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlark_test

import (
	"bytes"
	"testing"

	"go.starlark.net/starlark"

	loreStar "github.com/NobleFactor/devlore-cli/internal/starlark"
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

func TestBindingSetRegisterActions(t *testing.T) {
	// Register a test binding with an action registrar.
	op.RegisterBinding(&op.ProviderBinding{
		Name:   "_test_actions",
		Access: op.AccessPlanned,
		ActionRegistrar: func(reg *op.ActionRegistry) {
			reg.Register(testAction{name: "_test_actions.do"})
		},
	})

	bs := loreStar.NewBindingSet(op.BindingConfig{
		Writer:      &bytes.Buffer{},
		ProgramName: "test",
		Color:       false,
	})

	reg := op.NewActionRegistry()
	bs.RegisterActions(reg)

	if _, ok := reg.Get("_test_actions.do"); !ok {
		t.Error("expected _test_actions.do action to be registered")
	}
}

func TestBindingSetWithoutExcludesActions(t *testing.T) {
	op.RegisterBinding(&op.ProviderBinding{
		Name:   "_test_excl_act",
		Access: op.AccessPlanned,
		ActionRegistrar: func(reg *op.ActionRegistry) {
			reg.Register(testAction{name: "_test_excl_act.do"})
		},
	})

	bs := loreStar.NewBindingSet(op.BindingConfig{
		Writer:      &bytes.Buffer{},
		ProgramName: "test",
		Color:       false,
	}).Without("_test_excl_act")

	reg := op.NewActionRegistry()
	bs.RegisterActions(reg)

	if _, ok := reg.Get("_test_excl_act.do"); ok {
		t.Error("expected _test_excl_act.do to be excluded")
	}
}

func TestBindingSetBuildGlobals(t *testing.T) {
	op.RegisterBinding(&op.ProviderBinding{
		Name:   "_test_plan",
		Access: op.AccessPlanned,
		PlannedFactory: func(graph *op.Graph, project string, reg *op.ActionRegistry) starlark.Value {
			return starlark.String("test-plan-value")
		},
	})

	op.RegisterBinding(&op.ProviderBinding{
		Name:   "_test_imm",
		Access: op.AccessImmediate,
		ImmediateFactory: func(cfg op.BindingConfig) starlark.Value {
			return starlark.String("test-imm-value")
		},
	})

	bs := loreStar.NewBindingSet(op.BindingConfig{
		Writer:      &bytes.Buffer{},
		ProgramName: "test",
		Color:       false,
	})

	graph := &op.Graph{}
	reg := op.NewActionRegistry()
	globals := bs.BuildGlobals(graph, "test-project", reg)

	// "plan" should be present (from planned factories).
	if _, ok := globals["plan"]; !ok {
		t.Error("expected 'plan' in globals")
	}

	// "_test_imm" should be present (from immediate factory).
	if _, ok := globals["_test_imm"]; !ok {
		t.Error("expected '_test_imm' in globals")
	}

	// Verify PlanRoot has the test sub-namespace.
	planRoot, ok := globals["plan"].(*loreStar.PlanRoot)
	if !ok {
		t.Fatalf("expected globals['plan'] to be *PlanRoot, got %T", globals["plan"])
	}
	attr, err := planRoot.Attr("_test_plan")
	if err != nil {
		t.Fatalf("PlanRoot.Attr('_test_plan') error: %v", err)
	}
	if attr.String() != `"test-plan-value"` {
		t.Errorf("expected test-plan-value, got %s", attr.String())
	}
}

func TestBindingSetWithoutExcludesFromGlobals(t *testing.T) {
	op.RegisterBinding(&op.ProviderBinding{
		Name:   "_test_exclude",
		Access: op.AccessImmediate,
		ImmediateFactory: func(cfg op.BindingConfig) starlark.Value {
			return starlark.String("should-be-excluded")
		},
	})

	bs := loreStar.NewBindingSet(op.BindingConfig{
		Writer:      &bytes.Buffer{},
		ProgramName: "test",
		Color:       false,
	}).Without("_test_exclude")

	graph := &op.Graph{}
	reg := op.NewActionRegistry()
	globals := bs.BuildGlobals(graph, "test-project", reg)

	if _, ok := globals["_test_exclude"]; ok {
		t.Error("expected '_test_exclude' to be excluded from globals")
	}
}

// testAction is a minimal Action for testing registration.
type testAction struct{ name string }

func (a testAction) Name() string { return a.name }
func (a testAction) Do(_ *op.Context, _ map[string]any) (result op.Result, undo op.UndoState, err error) {
	return nil, nil, nil
}
