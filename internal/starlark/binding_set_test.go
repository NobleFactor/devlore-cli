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

func TestBindingSetRegisterActionsAlwaysRegistersAll(t *testing.T) {
	// RegisterActions registers all providers' actions regardless of With().
	op.RegisterBinding(&op.ProviderBinding{
		Name:   "_test_all_acts",
		Access: op.AccessPlanned,
		ActionRegistrar: func(reg *op.ActionRegistry) {
			reg.Register(testAction{name: "_test_all_acts.do"})
		},
	})

	// No With() — but actions should still be registered.
	bs := loreStar.NewBindingSet(op.BindingConfig{
		Writer:      &bytes.Buffer{},
		ProgramName: "test",
		Color:       false,
	})

	reg := op.NewActionRegistry()
	bs.RegisterActions(reg)

	if _, ok := reg.Get("_test_all_acts.do"); !ok {
		t.Error("expected _test_all_acts.do to be registered even without With()")
	}
}

func TestBindingSetBuildGlobalsWithPlanAndImmediate(t *testing.T) {
	op.RegisterBinding(&op.ProviderBinding{
		Name:   "_test_plan2",
		Access: op.AccessPlanned,
		PlannedFactory: func(graph *op.Graph, project string, reg *op.ActionRegistry) starlark.Value {
			return starlark.String("test-plan-value")
		},
	})

	op.RegisterBinding(&op.ProviderBinding{
		Name:   "_test_imm2",
		Access: op.AccessImmediate,
		ImmediateFactory: func(cfg op.BindingConfig) starlark.Value {
			return starlark.String("test-imm-value")
		},
	})

	bs := loreStar.NewBindingSet(op.BindingConfig{
		Writer:      &bytes.Buffer{},
		ProgramName: "test",
		Color:       false,
	}).With("plan", "_test_imm2")

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
	op.RegisterBinding(&op.ProviderBinding{
		Name:   "_test_not_included",
		Access: op.AccessImmediate,
		ImmediateFactory: func(cfg op.BindingConfig) starlark.Value {
			return starlark.String("should-not-appear")
		},
	})

	// Don't include "_test_not_included" in With().
	bs := loreStar.NewBindingSet(op.BindingConfig{
		Writer:      &bytes.Buffer{},
		ProgramName: "test",
		Color:       false,
	}).With("ui")

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
	op.RegisterBinding(&op.ProviderBinding{
		Name:   "_test_loadable",
		Access: op.AccessImmediate,
		ImmediateFactory: func(cfg op.BindingConfig) starlark.Value {
			return starlark.String("loaded-value")
		},
	})

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
	if v.String() != `"loaded-value"` {
		t.Errorf("expected loaded-value, got %s", v.String())
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
	op.RegisterBinding(&op.ProviderBinding{
		Name:   "_test_cached",
		Access: op.AccessImmediate,
		ImmediateFactory: func(cfg op.BindingConfig) starlark.Value {
			callCount++
			return starlark.String("cached-value")
		},
	})

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
		t.Errorf("expected ImmediateFactory called once (cached), got %d", callCount)
	}
}

func TestBindingSetLoaderLoadsPlan(t *testing.T) {
	op.RegisterBinding(&op.ProviderBinding{
		Name:   "_test_plan_load",
		Access: op.AccessPlanned,
		PlannedFactory: func(graph *op.Graph, project string, reg *op.ActionRegistry) starlark.Value {
			return starlark.String("plan-loaded")
		},
	})

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

func (a testAction) Name() string { return a.name }
func (a testAction) Do(_ *op.Context, _ map[string]any) (result op.Result, undo op.UndoState, err error) {
	return nil, nil, nil
}
