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

// ---------------------------------------------------------------------------
// Test provider types implementing the Provider / PlannedProvider /
// ImmediateProvider interfaces used by the announce-and-callback model.
// ---------------------------------------------------------------------------

// rtTestActionProvider registers a single test action.
type rtTestActionProvider struct {
	actionName string
}

func (p *rtTestActionProvider) Name() string { return "_test_actions" }
func (p *rtTestActionProvider) Register(reg *op.ActionRegistry, _ op.Context) {
	reg.Register(testAction{name: p.actionName})
}

// rtTestAllActsProvider registers a single test action (for the "always registers all" test).
type rtTestAllActsProvider struct{}

func (p *rtTestAllActsProvider) Name() string { return "_test_all_acts" }
func (p *rtTestAllActsProvider) Register(reg *op.ActionRegistry, _ op.Context) {
	reg.Register(testAction{name: "_test_all_acts.do"})
}

// rtTestPlannedProvider implements PlannedProvider.
type rtTestPlannedProvider struct{ name string }

func (p *rtTestPlannedProvider) Name() string                                { return p.name }
func (p *rtTestPlannedProvider) Register(_ *op.ActionRegistry, _ op.Context) {}
func (p *rtTestPlannedProvider) NewPlanned(_ *op.Graph, _ string, _ *op.ActionRegistry) starlark.Value {
	return starlark.String("test-plan-value")
}

// rtTestImmediateProvider implements ImmediateProvider.
type rtTestImmediateProvider struct{ name string }

func (p *rtTestImmediateProvider) Name() string                                { return p.name }
func (p *rtTestImmediateProvider) Register(_ *op.ActionRegistry, _ op.Context) {}
func (p *rtTestImmediateProvider) NewImmediate(_ op.BindingConfig) starlark.Value {
	return starlark.String("test-imm-value")
}

// rtTestCountingImmProvider implements ImmediateProvider and counts calls.
type rtTestCountingImmProvider struct {
	name      string
	callCount *int
}

func (p *rtTestCountingImmProvider) Name() string                                { return p.name }
func (p *rtTestCountingImmProvider) Register(_ *op.ActionRegistry, _ op.Context) {}
func (p *rtTestCountingImmProvider) NewImmediate(_ op.BindingConfig) starlark.Value {
	*p.callCount++
	return starlark.String("cached-value")
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestRuntimeRegisterActions(t *testing.T) {
	op.Announce(&rtTestActionProvider{actionName: "_test_actions.do"})

	rt := loreStar.NewRuntime(op.BindingConfig{
		Writer:      &bytes.Buffer{},
		ProgramName: "test",
		Color:       false,
	})

	reg := op.NewActionRegistry()
	rt.RegisterActions(reg, op.Context{})

	if _, ok := reg.Get("_test_actions.do"); !ok {
		t.Error("expected _test_actions.do action to be registered")
	}
}

func TestRuntimeRegisterActionsAlwaysRegistersAll(t *testing.T) {
	op.Announce(&rtTestAllActsProvider{})

	// No Receivers — but actions should still be registered.
	rt := loreStar.NewRuntime(op.BindingConfig{
		Writer:      &bytes.Buffer{},
		ProgramName: "test",
		Color:       false,
	})

	reg := op.NewActionRegistry()
	rt.RegisterActions(reg, op.Context{})

	if _, ok := reg.Get("_test_all_acts.do"); !ok {
		t.Error("expected _test_all_acts.do to be registered even without With()")
	}
}

func TestRuntimeBuildGlobalsWithPlanAndImmediate(t *testing.T) {
	op.Announce(&rtTestPlannedProvider{name: "_test_plan2"})
	op.Announce(&rtTestImmediateProvider{name: "_test_imm2"})

	rt := loreStar.NewRuntime(op.BindingConfig{
		Writer:      &bytes.Buffer{},
		ProgramName: "test",
		Color:       false,
		Receivers:   []string{"plan", "_test_imm2"},
	})

	graph := &op.Graph{}
	reg := op.NewActionRegistry()
	globals := rt.BuildGlobals(graph, "test-project", reg)

	// "plan" should be present (requested via Receivers).
	if _, ok := globals["plan"]; !ok {
		t.Error("expected 'plan' in globals")
	}

	// "_test_imm2" should be present (requested via Receivers).
	if _, ok := globals["_test_imm2"]; !ok {
		t.Error("expected '_test_imm2' in globals")
	}

	// Verify PlanRoot has the test sub-namespace.
	planRoot, ok := globals["plan"].(*loreStar.PlanRoot)
	if !ok {
		t.Fatalf("expected globals['plan'] to be *PlanRoot, got %T", globals["plan"])
	}
	attr, err := planRoot.Attr("_test_plan2")
	if err != nil {
		t.Fatalf("PlanRoot.Attr('_test_plan2') error: %v", err)
	}
	if attr.String() != `"test-plan-value"` {
		t.Errorf("expected test-plan-value, got %s", attr.String())
	}
}

func TestRuntimeBuildGlobalsOnlyIncludesReceivers(t *testing.T) {
	op.Announce(&rtTestImmediateProvider{name: "_test_not_included"})

	// Don't include "_test_not_included" in Receivers.
	rt := loreStar.NewRuntime(op.BindingConfig{
		Writer:      &bytes.Buffer{},
		ProgramName: "test",
		Color:       false,
		Receivers:   []string{"ui"},
	})

	graph := &op.Graph{}
	reg := op.NewActionRegistry()
	globals := rt.BuildGlobals(graph, "test-project", reg)

	if _, ok := globals["_test_not_included"]; ok {
		t.Error("expected '_test_not_included' to NOT be in globals (not in Receivers)")
	}

	// plan should also not be present (not in Receivers).
	if _, ok := globals["plan"]; ok {
		t.Error("expected 'plan' to NOT be in globals (not in Receivers)")
	}
}

func TestRuntimeConfigureThreadEnablesLoad(t *testing.T) {
	op.Announce(&rtTestImmediateProvider{name: "_test_loadable"})

	rt := loreStar.NewRuntime(op.BindingConfig{
		Writer:      &bytes.Buffer{},
		ProgramName: "test",
		Color:       false,
	})

	graph := &op.Graph{}
	reg := op.NewActionRegistry()
	thread := &starlark.Thread{Name: "test"}
	rt.ConfigureThread(thread, graph, "test-project", reg)

	// thread.Load should now work for @devlore// modules.
	if thread.Load == nil {
		t.Fatal("expected thread.Load to be set after ConfigureThread")
	}

	globals, err := thread.Load(thread, "@devlore//_test_loadable")
	if err != nil {
		t.Fatalf("load @devlore//_test_loadable: %v", err)
	}
	v, ok := globals["_test_loadable"]
	if !ok {
		t.Fatal("expected '_test_loadable' in loaded globals")
	}
	if v.String() != `"test-imm-value"` {
		t.Errorf("expected test-imm-value, got %s", v.String())
	}
}

func TestRuntimeLoaderRejectsUnknownPrefix(t *testing.T) {
	rt := loreStar.NewRuntime(op.BindingConfig{
		Writer:      &bytes.Buffer{},
		ProgramName: "test",
		Color:       false,
	})

	graph := &op.Graph{}
	reg := op.NewActionRegistry()
	thread := &starlark.Thread{Name: "test"}
	rt.ConfigureThread(thread, graph, "test-project", reg)

	_, err := thread.Load(thread, "unknown_module")
	if err == nil {
		t.Fatal("expected error for unknown module prefix")
	}
}

func TestRuntimeLoaderRejectsUnknownProvider(t *testing.T) {
	rt := loreStar.NewRuntime(op.BindingConfig{
		Writer:      &bytes.Buffer{},
		ProgramName: "test",
		Color:       false,
	})

	graph := &op.Graph{}
	reg := op.NewActionRegistry()
	thread := &starlark.Thread{Name: "test"}
	rt.ConfigureThread(thread, graph, "test-project", reg)

	_, err := thread.Load(thread, "@devlore//nonexistent_provider")
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
}

func TestRuntimeLoaderCachesResults(t *testing.T) {
	callCount := 0
	op.Announce(&rtTestCountingImmProvider{name: "_test_cached", callCount: &callCount})

	rt := loreStar.NewRuntime(op.BindingConfig{
		Writer:      &bytes.Buffer{},
		ProgramName: "test",
		Color:       false,
	})

	graph := &op.Graph{}
	reg := op.NewActionRegistry()
	thread := &starlark.Thread{Name: "test"}
	rt.ConfigureThread(thread, graph, "test-project", reg)

	// Load the same module twice.
	_, _ = thread.Load(thread, "@devlore//_test_cached")
	_, _ = thread.Load(thread, "@devlore//_test_cached")

	if callCount != 1 {
		t.Errorf("expected NewImmediate called once (cached), got %d", callCount)
	}
}

func TestRuntimeLoaderLoadsPlan(t *testing.T) {
	op.Announce(&rtTestPlannedProvider{name: "_test_plan_load"})

	rt := loreStar.NewRuntime(op.BindingConfig{
		Writer:      &bytes.Buffer{},
		ProgramName: "test",
		Color:       false,
	})

	graph := &op.Graph{}
	reg := op.NewActionRegistry()
	thread := &starlark.Thread{Name: "test"}
	rt.ConfigureThread(thread, graph, "test-project", reg)

	globals, err := thread.Load(thread, "@devlore//plan")
	if err != nil {
		t.Fatalf("load @devlore//plan: %v", err)
	}
	plan, ok := globals["plan"]
	if !ok {
		t.Fatal("expected 'plan' in loaded globals")
	}
	if _, ok := plan.(*loreStar.PlanRoot); !ok {
		t.Errorf("expected *PlanRoot, got %T", plan)
	}
}

// testAction is a minimal Action for testing registration.
type testAction struct{ name string }

func (a testAction) Name() string           { return a.name }
func (a testAction) Params() []op.ParamInfo { return nil }
func (a testAction) Do(_ *op.Context, _ map[string]any) (op.Result, op.Complement, error) {
	return nil, nil, nil
}
