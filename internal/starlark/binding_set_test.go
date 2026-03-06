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
// Test provider types implementing the new Provider / PlannedProvider /
// ImmediateProvider interfaces used by the announce-and-callback model.
// ---------------------------------------------------------------------------

// bsTestActionProvider registers a single test action.
type bsTestActionProvider struct {
	actionName string
}

func (p *bsTestActionProvider) Name() string { return "_test_actions" }
func (p *bsTestActionProvider) Register(reg *op.ActionRegistry, _ op.Context) {
	reg.Register(testAction{name: p.actionName})
}

// bsTestAllActsProvider registers a single test action (for the "always registers all" test).
type bsTestAllActsProvider struct{}

func (p *bsTestAllActsProvider) Name() string { return "_test_all_acts" }
func (p *bsTestAllActsProvider) Register(reg *op.ActionRegistry, _ op.Context) {
	reg.Register(testAction{name: "_test_all_acts.do"})
}

// bsTestPlannedProvider implements PlannedProvider.
type bsTestPlannedProvider struct{ name string }

func (p *bsTestPlannedProvider) Name() string                                { return p.name }
func (p *bsTestPlannedProvider) Register(_ *op.ActionRegistry, _ op.Context) {}
func (p *bsTestPlannedProvider) NewPlanned(_ *op.Graph, _ string, _ *op.ActionRegistry) starlark.Value {
	return starlark.String("test-plan-value")
}

// bsTestImmediateProvider implements ImmediateProvider.
type bsTestImmediateProvider struct{ name string }

func (p *bsTestImmediateProvider) Name() string                                { return p.name }
func (p *bsTestImmediateProvider) Register(_ *op.ActionRegistry, _ op.Context) {}
func (p *bsTestImmediateProvider) NewImmediate(_ op.BindingConfig) starlark.Value {
	return starlark.String("test-imm-value")
}

// bsTestCountingImmProvider implements ImmediateProvider and counts calls.
type bsTestCountingImmProvider struct {
	name      string
	callCount *int
}

func (p *bsTestCountingImmProvider) Name() string                                { return p.name }
func (p *bsTestCountingImmProvider) Register(_ *op.ActionRegistry, _ op.Context) {}
func (p *bsTestCountingImmProvider) NewImmediate(_ op.BindingConfig) starlark.Value {
	*p.callCount++
	return starlark.String("cached-value")
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestBindingSetRegisterActions(t *testing.T) {
	op.Announce(&bsTestActionProvider{actionName: "_test_actions.do"})

	bs := loreStar.NewBindingSet(op.BindingConfig{
		Writer:      &bytes.Buffer{},
		ProgramName: "test",
		Color:       false,
	})

	reg := op.NewActionRegistry()
	bs.RegisterActions(reg, op.Context{})

	if _, ok := reg.Get("_test_actions.do"); !ok {
		t.Error("expected _test_actions.do action to be registered")
	}
}

func TestBindingSetRegisterActionsAlwaysRegistersAll(t *testing.T) {
	op.Announce(&bsTestAllActsProvider{})

	// No Receivers — but actions should still be registered.
	bs := loreStar.NewBindingSet(op.BindingConfig{
		Writer:      &bytes.Buffer{},
		ProgramName: "test",
		Color:       false,
	})

	reg := op.NewActionRegistry()
	bs.RegisterActions(reg, op.Context{})

	if _, ok := reg.Get("_test_all_acts.do"); !ok {
		t.Error("expected _test_all_acts.do to be registered even without With()")
	}
}

func TestBindingSetBuildGlobalsWithPlanAndImmediate(t *testing.T) {
	op.Announce(&bsTestPlannedProvider{name: "_test_plan2"})
	op.Announce(&bsTestImmediateProvider{name: "_test_imm2"})

	bs := loreStar.NewBindingSet(op.BindingConfig{
		Writer:      &bytes.Buffer{},
		ProgramName: "test",
		Color:       false,
		Receivers:   []string{"plan", "_test_imm2"},
	})

	graph := &op.Graph{}
	reg := op.NewActionRegistry()
	globals := bs.BuildGlobals(graph, "test-project", reg)

	// "plan" should be present (requested via With).
	if _, ok := globals["plan"]; !ok {
		t.Error("expected 'plan' in globals")
	}

	// "_test_imm2" should be present (requested via With).
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

func TestBindingSetBuildGlobalsOnlyIncludesWithProviders(t *testing.T) {
	op.Announce(&bsTestImmediateProvider{name: "_test_not_included"})

	// Don't include "_test_not_included" in Receivers.
	bs := loreStar.NewBindingSet(op.BindingConfig{
		Writer:      &bytes.Buffer{},
		ProgramName: "test",
		Color:       false,
		Receivers:   []string{"ui"},
	})

	graph := &op.Graph{}
	reg := op.NewActionRegistry()
	globals := bs.BuildGlobals(graph, "test-project", reg)

	if _, ok := globals["_test_not_included"]; ok {
		t.Error("expected '_test_not_included' to NOT be in globals (not in With())")
	}

	// plan should also not be present (not in With).
	if _, ok := globals["plan"]; ok {
		t.Error("expected 'plan' to NOT be in globals (not in With())")
	}
}

func TestBindingSetConfigureThreadEnablesLoad(t *testing.T) {
	op.Announce(&bsTestImmediateProvider{name: "_test_loadable"})

	bs := loreStar.NewBindingSet(op.BindingConfig{
		Writer:      &bytes.Buffer{},
		ProgramName: "test",
		Color:       false,
	})

	graph := &op.Graph{}
	reg := op.NewActionRegistry()
	thread := &starlark.Thread{Name: "test"}
	bs.ConfigureThread(thread, graph, "test-project", reg)

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

func TestBindingSetLoaderRejectsUnknownPrefix(t *testing.T) {
	bs := loreStar.NewBindingSet(op.BindingConfig{
		Writer:      &bytes.Buffer{},
		ProgramName: "test",
		Color:       false,
	})

	graph := &op.Graph{}
	reg := op.NewActionRegistry()
	thread := &starlark.Thread{Name: "test"}
	bs.ConfigureThread(thread, graph, "test-project", reg)

	_, err := thread.Load(thread, "unknown_module")
	if err == nil {
		t.Fatal("expected error for unknown module prefix")
	}
}

func TestBindingSetLoaderRejectsUnknownProvider(t *testing.T) {
	bs := loreStar.NewBindingSet(op.BindingConfig{
		Writer:      &bytes.Buffer{},
		ProgramName: "test",
		Color:       false,
	})

	graph := &op.Graph{}
	reg := op.NewActionRegistry()
	thread := &starlark.Thread{Name: "test"}
	bs.ConfigureThread(thread, graph, "test-project", reg)

	_, err := thread.Load(thread, "@devlore//nonexistent_provider")
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
}

func TestBindingSetLoaderCachesResults(t *testing.T) {
	callCount := 0
	op.Announce(&bsTestCountingImmProvider{name: "_test_cached", callCount: &callCount})

	bs := loreStar.NewBindingSet(op.BindingConfig{
		Writer:      &bytes.Buffer{},
		ProgramName: "test",
		Color:       false,
	})

	graph := &op.Graph{}
	reg := op.NewActionRegistry()
	thread := &starlark.Thread{Name: "test"}
	bs.ConfigureThread(thread, graph, "test-project", reg)

	// Load the same module twice.
	_, _ = thread.Load(thread, "@devlore//_test_cached")
	_, _ = thread.Load(thread, "@devlore//_test_cached")

	if callCount != 1 {
		t.Errorf("expected NewImmediate called once (cached), got %d", callCount)
	}
}

func TestBindingSetLoaderLoadsPlan(t *testing.T) {
	op.Announce(&bsTestPlannedProvider{name: "_test_plan_load"})

	bs := loreStar.NewBindingSet(op.BindingConfig{
		Writer:      &bytes.Buffer{},
		ProgramName: "test",
		Color:       false,
	})

	graph := &op.Graph{}
	reg := op.NewActionRegistry()
	thread := &starlark.Thread{Name: "test"}
	bs.ConfigureThread(thread, graph, "test-project", reg)

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
