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

// region TEST FIXTURES

// asJoinedError unwraps `err` into its joined leaves so callers can inspect aggregated entries
// individually. Returns a single-entry slice when `err` is not an [errors.Join] aggregate.
//
// Parameters:
//   - `t`: the test handle; marked as a helper for accurate failure line attribution.
//   - `err`: the joined or single error to inspect.
//
// Returns:
//   - []error: the leaves; never empty when `err` is non-nil.
func asJoinedError(t *testing.T, err error) []error {

	t.Helper()
	type unwrapper interface{ Unwrap() []error }
	if u, ok := err.(unwrapper); ok {
		return u.Unwrap()
	}
	return []error{err}
}

// graphWithVariableSlot constructs a minimal Graph whose root has one node with a single
// VariableValue slot. Useful for exercising the bubble-up surface against the preflight resolver.
//
// Parameters:
//   - `varName`: the slot's variable name, surfaced through [Graph.Parameters].
//   - `t`: the reflect.Type recorded on the slot, used by the resolver's type-check pass.
//
// Returns:
//   - *Graph: the constructed graph with a single root child; unbound from any env.
func graphWithVariableSlot(varName string, t reflect.Type) *Graph {

	n := nodeWithSlots("n", slotSpec{name: "p", typ: t, value: VariableValue{Name: varName}})
	g, err := NewGraph(Origin{}, []ExecutableUnit{n}, nil, nil, nil, nil, nil)
	if err != nil {
		panic("graphWithVariableSlot: " + err.Error())
	}
	return g
}

// newExecutorForTest constructs a GraphExecutor wired with a minimal RuntimeEnvironment backed by
// `app`. The env is built up-front (rather than per-Run as [GraphExecutor.Run] does in production)
// so tests can call `e.bindVariables(g, ...)` directly against a live env without going through
// full dispatch. A placeholder graph satisfies the executor's construction-time non-nil invariant;
// tests pass their real graph to bindVariables.
//
// Parameters:
//   - `t`: the test handle; the env's Close is registered via [testing.T.Cleanup].
//   - `app`: the [application.Application] backing the env's flag and dry-run lookups.
//
// Returns:
//   - *GraphExecutor: the executor with `e.environment` pre-bound; safe to call `bindVariables` on.
func newExecutorForTest(t *testing.T, app *application.Application) *GraphExecutor {

	t.Helper()
	spec := NewRuntimeEnvironmentSpec(app.Name, NewReceiverRegistry()).WithApplication(app)
	g, err := NewGraph(Origin{}, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewGraph: %v", err)
	}
	e := NewGraphExecutor(g, spec)
	e.environment = NewRuntimeEnvironment(context.Background(), spec)
	t.Cleanup(func() {
		_ = e.environment.Close()
		e.environment = nil
	})
	return e
}

// endregion

// --- bindVariables ---

func TestBindVariables_AggregatesMultipleErrors(t *testing.T) {

	app := &application.Application{Name: "writ"}
	e := newExecutorForTest(t, app)

	g := newTestGraph(t, nodeWithSlots("n1",
		stringSlot("p", VariableValue{Name: "missing_a"}),
		intSlot("q", VariableValue{Name: "missing_b"}),
	))

	g.Rebind(e.environment)
	defer g.Unbind()

	err := e.bindVariables(g, nil)
	if err == nil {
		t.Fatal("expected aggregated errors; got nil")
	}

	msg := err.Error()
	for _, name := range []string{"missing_a", "missing_b"} {
		if !strings.Contains(msg, name) {
			t.Errorf("aggregated error should mention %q: %v", name, err)
		}
	}
}

func TestBindVariables_CallerSuppliedOverridesResolver(t *testing.T) {

	app := &application.Application{
		Name:  "writ",
		Flags: map[string]any{"target_root": "/from/flag"},
	}
	e := newExecutorForTest(t, app)
	g := graphWithVariableSlot("target_root", reflect.TypeFor[string]())

	g.Rebind(e.environment)
	defer g.Unbind()

	caller := map[string]Variable{
		"target_root": {
			Name:   "target_root",
			Value:  "/from/caller",
			Source: VariableSource{Kind: VariableSourceKindOverride, Name: "test"},
		},
	}

	if err := e.bindVariables(g, caller); err != nil {
		t.Fatalf("bindVariables: %v", err)
	}

	got, _ := e.environment.VariableByName("target_root")
	if got.Value != "/from/caller" {
		t.Errorf("Value = %q, want /from/caller (caller wins over flag)", got.Value)
	}
}

func TestBindVariables_DryRun_StillRuns(t *testing.T) {

	// Variable resolution is pure (no side effects); it must run in dry-run so dry-run output can
	// render resolved slot values. This diverges from D10's "skip in dry-run" framing.
	app := &application.Application{
		Name:  "writ",
		Flags: map[string]any{"dry_run": true, "target_root": "/tmp/x"},
	}
	if !app.DryRun() {
		t.Fatal("test fixture: app.DryRun() should be true")
	}

	e := newExecutorForTest(t, app)
	g := graphWithVariableSlot("target_root", reflect.TypeFor[string]())

	g.Rebind(e.environment)
	defer g.Unbind()

	if err := e.bindVariables(g, nil); err != nil {
		t.Fatalf("bindVariables in dry-run: %v", err)
	}

	got, ok := e.environment.VariableByName("target_root")
	if !ok || got.Value != "/tmp/x" {
		t.Errorf("dry-run should still resolve variables; got (%v, %v)", got, ok)
	}
}

func TestBindVariables_EmptyParameters_NoError(t *testing.T) {

	app := &application.Application{Name: "writ"}
	e := newExecutorForTest(t, app)

	g := newTestGraph(t, nodeWithSlots("n",
		stringSlot("p", ImmediateValue{Value: "x"}),
	))

	g.Rebind(e.environment)
	defer g.Unbind()

	if err := e.bindVariables(g, nil); err != nil {
		t.Errorf("empty parameter surface should be a no-op; got %v", err)
	}

	if _, ok := e.environment.VariableByName("p"); ok {
		t.Error("env.variables should not gain entries for non-Variable slots")
	}
}

func TestBindVariables_ErrorShape_IsJoined(t *testing.T) {

	app := &application.Application{Name: "writ"}
	e := newExecutorForTest(t, app)

	g := newTestGraph(t, nodeWithSlots("n",
		stringSlot("p1", VariableValue{Name: "a"}),
		stringSlot("p2", VariableValue{Name: "b"}),
	))

	g.Rebind(e.environment)
	defer g.Unbind()

	err := e.bindVariables(g, nil)
	if err == nil {
		t.Fatal("expected aggregated error")
	}

	parts := asJoinedError(t, err)
	if len(parts) != 2 {
		t.Errorf("expected 2 joined errors; got %d", len(parts))
	}

	for _, p := range parts {
		if !errors.Is(err, p) {
			t.Errorf("joined parent should still match leaf via errors.Is: %v", p)
		}
	}
}

func TestBindVariables_MissingRequired_ReturnsError(t *testing.T) {

	app := &application.Application{Name: "writ"}
	e := newExecutorForTest(t, app)
	g := graphWithVariableSlot("target_root", reflect.TypeFor[string]())

	g.Rebind(e.environment)
	defer g.Unbind()

	err := e.bindVariables(g, nil)
	if err == nil {
		t.Fatal("expected missing-required error; got nil")
	}
	if !strings.Contains(err.Error(), "target_root") {
		t.Errorf("error should name the parameter: %v", err)
	}
}

func TestBindVariables_PopulatesEnvVariables_FromEnv(t *testing.T) {

	t.Setenv("WRIT_TARGET_ROOT", "/from/env")

	app := &application.Application{Name: "writ"}
	e := newExecutorForTest(t, app)
	g := graphWithVariableSlot("target_root", reflect.TypeFor[string]())

	g.Rebind(e.environment)
	defer g.Unbind()

	if err := e.bindVariables(g, nil); err != nil {
		t.Fatalf("bindVariables: %v", err)
	}

	got, _ := e.environment.VariableByName("target_root")
	if got.Value != "/from/env" {
		t.Errorf("Value = %q, want /from/env", got.Value)
	}
	if got.Source.Kind != VariableSourceKindEnv {
		t.Errorf("Source.Kind = %v, want Env", got.Source.Kind)
	}
}

func TestBindVariables_PopulatesEnvVariables_FromFlag(t *testing.T) {

	app := &application.Application{
		Name:  "writ",
		Flags: map[string]any{"target_root": "/tmp/x"},
	}
	e := newExecutorForTest(t, app)
	g := graphWithVariableSlot("target_root", reflect.TypeFor[string]())

	g.Rebind(e.environment)
	defer g.Unbind()

	if err := e.bindVariables(g, nil); err != nil {
		t.Fatalf("bindVariables: %v", err)
	}

	got, ok := e.environment.VariableByName("target_root")
	if !ok {
		t.Fatal("target_root not in env.variables")
	}
	if got.Value != "/tmp/x" {
		t.Errorf("Value = %q, want /tmp/x", got.Value)
	}
	if got.Source.Kind != VariableSourceKindFlag {
		t.Errorf("Source.Kind = %v, want Flag", got.Source.Kind)
	}

	if _, ok := e.variables["target_root"]; !ok {
		t.Error("e.variables not populated; should mirror env.variables")
	}
}

func TestBindVariables_TypeMismatch_ReturnsError(t *testing.T) {

	app := &application.Application{
		Name:  "writ",
		Flags: map[string]any{"port": "not-an-int"},
	}
	e := newExecutorForTest(t, app)
	g := graphWithVariableSlot("port", reflect.TypeFor[int]())

	g.Rebind(e.environment)
	defer g.Unbind()

	err := e.bindVariables(g, nil)
	if err == nil {
		t.Fatal("expected type-mismatch error; got nil")
	}
}
