// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlark_test

import (
	"bytes"
	"reflect"
	"testing"

	"go.starlark.net/starlark"

	loreStar "github.com/NobleFactor/devlore-cli/internal/starlark"
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// ---------------------------------------------------------------------------
// Test provider types implementing the ReceiverFactory / PlanningReceiverFactory /
// ExecutingReceiverFactory interfaces used by the announce-and-callback model.
// ---------------------------------------------------------------------------

// rtTestActionProvider registers a single test action.
type rtTestActionProvider struct {
	actionName string
}

func (p *rtTestActionProvider) ReceiverName() string                                { return "_test_actions" }
func (p *rtTestActionProvider) GetOrCreateProvider(_ op.Context) op.ContextProvider { return nil }
func (p *rtTestActionProvider) ProviderType() reflect.Type {
	return reflect.TypeOf((*rtTestActionProvider)(nil)).Elem()
}
func (p *rtTestActionProvider) Register(reg *op.ActionRegistry, _ op.Context) {
	reg.Register(&testAction{name: p.actionName})
}

// rtTestAllActsProvider registers a single test action (for the "always registers all" test).
type rtTestAllActsProvider struct{}

func (p *rtTestAllActsProvider) ReceiverName() string                                { return "_test_all_acts" }
func (p *rtTestAllActsProvider) GetOrCreateProvider(_ op.Context) op.ContextProvider { return nil }
func (p *rtTestAllActsProvider) ProviderType() reflect.Type {
	return reflect.TypeOf((*rtTestAllActsProvider)(nil)).Elem()
}
func (p *rtTestAllActsProvider) Register(reg *op.ActionRegistry, _ op.Context) {
	reg.Register(&testAction{name: "_test_all_acts.do"})
}

// rtTestPlannedProvider implements PlanningReceiverFactory.
type rtTestPlannedProvider struct{ name string }

func (p *rtTestPlannedProvider) ReceiverName() string                                { return p.name }
func (p *rtTestPlannedProvider) GetOrCreateProvider(_ op.Context) op.ContextProvider { return nil }
func (p *rtTestPlannedProvider) ProviderType() reflect.Type {
	return reflect.TypeOf((*rtTestPlannedProvider)(nil)).Elem()
}
func (p *rtTestPlannedProvider) Register(_ *op.ActionRegistry, _ op.Context) {}
func (p *rtTestPlannedProvider) NewPlanning(_ *op.Graph, _ string, _ *op.ActionRegistry) starlark.Value {
	return starlark.String("test-plan-value")
}

// rtTestImmediateProvider implements ExecutingReceiverFactory.
type rtTestImmediateProvider struct{ name string }

func (p *rtTestImmediateProvider) ReceiverName() string                                { return p.name }
func (p *rtTestImmediateProvider) GetOrCreateProvider(_ op.Context) op.ContextProvider { return nil }
func (p *rtTestImmediateProvider) ProviderType() reflect.Type {
	return reflect.TypeOf((*rtTestImmediateProvider)(nil)).Elem()
}
func (p *rtTestImmediateProvider) Register(_ *op.ActionRegistry, _ op.Context) {}
func (p *rtTestImmediateProvider) NewExecuting(_ op.Context) starlark.Value {
	return starlark.String("test-imm-value")
}

// rtTestCountingImmProvider implements ExecutingReceiverFactory and counts calls.
type rtTestCountingImmProvider struct {
	name      string
	callCount *int
}

func (p *rtTestCountingImmProvider) ReceiverName() string                                { return p.name }
func (p *rtTestCountingImmProvider) GetOrCreateProvider(_ op.Context) op.ContextProvider { return nil }
func (p *rtTestCountingImmProvider) ProviderType() reflect.Type {
	return reflect.TypeOf((*rtTestCountingImmProvider)(nil)).Elem()
}
func (p *rtTestCountingImmProvider) Register(_ *op.ActionRegistry, _ op.Context) {}
func (p *rtTestCountingImmProvider) NewExecuting(_ op.Context) starlark.Value {
	*p.callCount++
	return starlark.String("cached-value")
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestRuntimeRegisterActions(t *testing.T) {
	op.AnnounceReceiver(&rtTestActionProvider{actionName: "_test_actions.do"})

	rt := loreStar.NewRuntime(
		op.NewBindingConfig("test").
			WithWriter(&bytes.Buffer{}),
	)

	reg := op.NewActionRegistry()
	rt.RegisterActions(reg, op.Context{})

	if _, ok := reg.Get("_test_actions.do"); !ok {
		t.Error("expected _test_actions.do action to be registered")
	}
}

func TestRuntimeRegisterActionsAlwaysRegistersAll(t *testing.T) {
	op.AnnounceReceiver(&rtTestAllActsProvider{})

	// No Receivers — but actions should still be registered.
	rt := loreStar.NewRuntime(
		op.NewBindingConfig("test").
			WithWriter(&bytes.Buffer{}),
	)

	reg := op.NewActionRegistry()
	rt.RegisterActions(reg, op.Context{})

	if _, ok := reg.Get("_test_all_acts.do"); !ok {
		t.Error("expected _test_all_acts.do to be registered even without With()")
	}
}

func TestRuntimeBuildGlobalsWithPlanAndImmediate(t *testing.T) {
	immProv := &rtTestImmediateProvider{name: "_test_imm2"}
	op.AnnounceReceiver(&rtTestPlannedProvider{name: "_test_plan2"})
	op.AnnounceReceiver(immProv)

	rt := loreStar.NewRuntime(
		op.NewBindingConfig("test").
			WithGraphBuilder().
			WithReceivers(immProv).
			WithWriter(&bytes.Buffer{}),
	)

	graph := &op.Graph{}
	reg := op.NewActionRegistry()
	globals := rt.BuildGlobals(graph, "test-project", reg)

	// "plan" should be present (requested via WithGraphBuilder).
	if _, ok := globals["plan"]; !ok {
		t.Error("expected 'plan' in globals")
	}

	// "_test_imm2" should be present (requested via WithReceivers()).
	if _, ok := globals["_test_imm2"]; !ok {
		t.Error("expected '_test_imm2' in globals")
	}

	// Verify PlanRoot has the test sub-namespace.
	planRoot, ok := globals["plan"].(*op.PlanRoot)
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

func TestRuntimeBuildGlobalsOnlyIncludesProviders(t *testing.T) {
	op.AnnounceReceiver(&rtTestImmediateProvider{name: "_test_not_included"})

	// Pass a different provider — "_test_not_included" should be excluded.
	otherProv := &rtTestImmediateProvider{name: "_test_other"}
	op.AnnounceReceiver(otherProv)

	rt := loreStar.NewRuntime(
		op.NewBindingConfig("test").
			WithReceivers(otherProv).
			WithWriter(&bytes.Buffer{}),
	)

	graph := &op.Graph{}
	reg := op.NewActionRegistry()
	globals := rt.BuildGlobals(graph, "test-project", reg)

	if _, ok := globals["_test_not_included"]; ok {
		t.Error("expected '_test_not_included' to NOT be in globals (not in Receivers)")
	}

	// plan should also not be present (WithGraphBuilder not called).
	if _, ok := globals["plan"]; ok {
		t.Error("expected 'plan' to NOT be in globals (WithGraphBuilder not called)")
	}
}

func TestRuntimeConfigureThreadEnablesLoad(t *testing.T) {
	op.AnnounceReceiver(&rtTestImmediateProvider{name: "_test_loadable"})

	rt := loreStar.NewRuntime(
		op.NewBindingConfig("test").
			WithWriter(&bytes.Buffer{}),
	)

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
	rt := loreStar.NewRuntime(
		op.NewBindingConfig("test").
			WithWriter(&bytes.Buffer{}),
	)

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
	rt := loreStar.NewRuntime(
		op.NewBindingConfig("test").
			WithWriter(&bytes.Buffer{}),
	)

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
	op.AnnounceReceiver(&rtTestCountingImmProvider{name: "_test_cached", callCount: &callCount})

	rt := loreStar.NewRuntime(
		op.NewBindingConfig("test").
			WithWriter(&bytes.Buffer{}),
	)

	graph := &op.Graph{}
	reg := op.NewActionRegistry()
	thread := &starlark.Thread{Name: "test"}
	rt.ConfigureThread(thread, graph, "test-project", reg)

	// Load the same module twice.
	_, _ = thread.Load(thread, "@devlore//_test_cached")
	_, _ = thread.Load(thread, "@devlore//_test_cached")

	if callCount != 1 {
		t.Errorf("expected NewExecuting called once (cached), got %d", callCount)
	}
}

func TestRuntimeLoaderLoadsPlan(t *testing.T) {
	op.AnnounceReceiver(&rtTestPlannedProvider{name: "_test_plan_load"})

	rt := loreStar.NewRuntime(
		op.NewBindingConfig("test").
			WithWriter(&bytes.Buffer{}),
	)

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
	if _, ok := plan.(*op.PlanRoot); !ok {
		t.Errorf("expected *PlanRoot, got %T", plan)
	}
}

// testAction is a minimal Action for testing registration.
type testAction struct{ name string }

func (a *testAction) Name() string           { return a.name }
func (a *testAction) Params() []op.ParamInfo { return nil }
func (a *testAction) Do(_ *op.Context, _ map[string]any) (op.Result, op.Complement, error) {
	return nil, nil, nil
}
