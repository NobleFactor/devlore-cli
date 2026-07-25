// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"context"
	"reflect"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/application"
)

// TestVariableByName_ResolvesSourceSetAfterRegistration verifies the cache-on-found (sync.OnceValues) semantics:
// a parameter registered before its source is supplied still resolves once the source is set. VariableByName
// resolves lazily on first read, so the application need not order source population before provider construction —
// the ordering bug that hid the "config" override behind cfg.Load() and made cfg.lint unresolvable.
func TestVariableByName_ResolvesSourceSetAfterRegistration(t *testing.T) {

	app := &application.Application{Name: "test"}
	spec := NewRuntimeEnvironmentSpec(app.Name).WithApplication(app)
	env := NewRuntimeEnvironment(context.Background(), spec)

	// Register the parameter BEFORE any source supplies it (mirrors provider construction at NewRuntime).
	if err := env.RegisterParameter(Parameter{Name: "late", Type: reflect.TypeFor[string]()}); err != nil {
		t.Fatalf("RegisterParameter: %v", err)
	}

	// The application supplies the value AFTER registration (mirrors app.Overrides["config"] set after the runtime
	// is built). No read happens before this point, so the lazy resolver has not cached an absence.
	app.Overrides = map[string]any{"late": "set-after-registration"}

	// First read resolves it lazily and caches.
	got, ok := env.VariableByName("late")
	if !ok || got.Value != "set-after-registration" {
		t.Fatalf("VariableByName(late) = (%v, %v), want (%q, true)", got.Value, ok, "set-after-registration")
	}

	// Subsequent reads return the cached value.
	if again, ok := env.VariableByName("late"); !ok || again.Value != "set-after-registration" {
		t.Errorf("second VariableByName(late) = (%v, %v), want the cached value", again.Value, ok)
	}
}

// TestVariableByName_ReadBeforeSourceSet pins the sharp edge of the sync.OnceValues semantic: because the first read
// is memoized, reading a variable BEFORE its source is set caches the absence permanently — a later set is not
// picked up. This is acceptable only because no provider reads a variable at construction (the first read always
// follows source population). The test documents the contract, so a future read-before-set is a deliberate choice
// and not a silent regression of the config-resolution fix.
func TestVariableByName_ReadBeforeSourceSet(t *testing.T) {

	app := &application.Application{Name: "test"}
	spec := NewRuntimeEnvironmentSpec(app.Name).WithApplication(app)
	env := NewRuntimeEnvironment(context.Background(), spec)

	if err := env.RegisterParameter(Parameter{Name: "early", Type: reflect.TypeFor[string]()}); err != nil {
		t.Fatalf("RegisterParameter: %v", err)
	}

	// Read BEFORE the source is set — resolves absent, and OnceValues memoizes that result.
	if _, ok := env.VariableByName("early"); ok {
		t.Fatal("expected 'early' unresolved before its source is set")
	}

	// Set the source AFTER the first read.
	app.Overrides = map[string]any{"early": "too-late"}

	// The memoized absence is NOT overturned — documents the compute-once trade-off.
	if v, ok := env.VariableByName("early"); ok {
		t.Errorf("VariableByName(early) = (%v, %v); want still-absent (read-before-set is memoized)", v.Value, ok)
	}
}
