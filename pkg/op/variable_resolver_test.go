// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/NobleFactor/devlore-cli/pkg/application"
)

// region TEST FIXTURES

// newTestApp builds an [application.Application] with the supplied source maps and program name.
func newTestApp(name string, flags, config, overrides map[string]any) *application.Application {

	return &application.Application{
		Name:      name,
		Flags:     flags,
		Config:    config,
		Overrides: overrides,
	}
}

// endregion

func TestVariableResolver_EnvPrefix(t *testing.T) {

	t.Run("non-empty name produces uppercase prefix", func(t *testing.T) {
		r := NewVariableResolver(newTestApp("writ", nil, nil, nil))
		if got, want := r.EnvPrefix(), "WRIT_"; got != want {
			t.Errorf("EnvPrefix() = %q, want %q", got, want)
		}
	})

	t.Run("empty name produces empty prefix", func(t *testing.T) {
		r := NewVariableResolver(newTestApp("", nil, nil, nil))
		if got := r.EnvPrefix(); got != "" {
			t.Errorf("EnvPrefix() = %q, want empty", got)
		}
	})

	t.Run("nil app produces empty prefix", func(t *testing.T) {
		r := NewVariableResolver(nil)
		if got := r.EnvPrefix(); got != "" {
			t.Errorf("EnvPrefix() = %q, want empty", got)
		}
	})
}

func TestVariableResolver_GetPanicsBeforeResolve(t *testing.T) {

	r := NewVariableResolver(newTestApp("writ", nil, nil, nil))

	defer func() {
		if recover() == nil {
			t.Fatal("Get before Resolve should panic")
		}
	}()
	_, _ = r.Get("anything")
}

func TestVariableResolver_VariablesPanicsBeforeResolve(t *testing.T) {

	r := NewVariableResolver(newTestApp("writ", nil, nil, nil))

	defer func() {
		if recover() == nil {
			t.Fatal("Variables before Resolve should panic")
		}
	}()
	_ = r.Variables()
}

func TestVariableResolver_Cascade_Override(t *testing.T) {

	r := NewVariableResolver(newTestApp(
		"writ",
		map[string]any{"target_root": "from-flag"},
		map[string]any{"target_root": "from-config"},
		map[string]any{"target_root": "from-override"},
	))

	errs := r.Resolve(nil, []Parameter{
		{Name: "target_root", Type: reflect.TypeFor[string]()},
	})
	if len(errs) != 0 {
		t.Fatalf("Resolve errors: %v", errs)
	}

	v, ok := r.Get("target_root")
	if !ok {
		t.Fatal("target_root not resolved")
	}
	if v.Value != "from-override" {
		t.Errorf("value = %q, want from-override", v.Value)
	}
	if v.Source.Kind != VariableSourceKindOverride {
		t.Errorf("kind = %v, want Override", v.Source.Kind)
	}
}

func TestVariableResolver_Cascade_Flag(t *testing.T) {

	r := NewVariableResolver(newTestApp(
		"writ",
		map[string]any{"target_root": "from-flag"},
		map[string]any{"target_root": "from-config"},
		nil,
	))

	errs := r.Resolve(nil, []Parameter{
		{Name: "target_root", Type: reflect.TypeFor[string]()},
	})
	if len(errs) != 0 {
		t.Fatalf("Resolve errors: %v", errs)
	}

	v, _ := r.Get("target_root")
	if v.Value != "from-flag" {
		t.Errorf("value = %q, want from-flag", v.Value)
	}
	if v.Source.Kind != VariableSourceKindFlag {
		t.Errorf("kind = %v, want Flag", v.Source.Kind)
	}
}

func TestVariableResolver_Cascade_Env(t *testing.T) {

	t.Setenv("WRIT_TARGET_ROOT", "from-env")

	r := NewVariableResolver(newTestApp(
		"writ",
		nil,
		map[string]any{"target_root": "from-config"},
		nil,
	))

	errs := r.Resolve(nil, []Parameter{
		{Name: "target_root", Type: reflect.TypeFor[string]()},
	})
	if len(errs) != 0 {
		t.Fatalf("Resolve errors: %v", errs)
	}

	v, _ := r.Get("target_root")
	if v.Value != "from-env" {
		t.Errorf("value = %q, want from-env", v.Value)
	}
	if v.Source.Kind != VariableSourceKindEnv {
		t.Errorf("kind = %v, want Env", v.Source.Kind)
	}
	if v.Source.Name != "WRIT_TARGET_ROOT" {
		t.Errorf("name = %q, want WRIT_TARGET_ROOT", v.Source.Name)
	}
}

func TestVariableResolver_Cascade_Env_Typed(t *testing.T) {

	t.Setenv("WRIT_TIMEOUT", "30s")
	t.Setenv("WRIT_MODE", "0o755")
	t.Setenv("WRIT_VERBOSE", "true")

	r := NewVariableResolver(newTestApp("writ", nil, nil, nil))

	errs := r.Resolve(nil, []Parameter{
		{Name: "timeout", Type: reflect.TypeFor[time.Duration]()},
		{Name: "mode", Type: reflect.TypeFor[os.FileMode]()},
		{Name: "verbose", Type: reflect.TypeFor[bool]()},
	})
	if len(errs) != 0 {
		t.Fatalf("Resolve errors: %v", errs)
	}

	timeout, _ := r.Get("timeout")
	if timeout.Value != 30*time.Second {
		t.Errorf("timeout = %v, want 30s", timeout.Value)
	}

	mode, _ := r.Get("mode")
	if mode.Value != os.FileMode(0o755) {
		t.Errorf("mode = %v, want 0o755", mode.Value)
	}

	verbose, _ := r.Get("verbose")
	if verbose.Value != true {
		t.Errorf("verbose = %v, want true", verbose.Value)
	}
}

func TestVariableResolver_Cascade_Config(t *testing.T) {

	r := NewVariableResolver(newTestApp(
		"writ",
		nil,
		map[string]any{"target_root": "from-config"},
		nil,
	))

	errs := r.Resolve(nil, []Parameter{
		{Name: "target_root", Type: reflect.TypeFor[string]()},
	})
	if len(errs) != 0 {
		t.Fatalf("Resolve errors: %v", errs)
	}

	v, _ := r.Get("target_root")
	if v.Value != "from-config" {
		t.Errorf("value = %q, want from-config", v.Value)
	}
	if v.Source.Kind != VariableSourceKindConfig {
		t.Errorf("kind = %v, want Config", v.Source.Kind)
	}
}

func TestVariableResolver_Cascade_Default(t *testing.T) {

	r := NewVariableResolver(newTestApp("writ", nil, nil, nil))

	errs := r.Resolve(nil, []Parameter{
		{Name: "target_root", Type: reflect.TypeFor[string](), Optional: true, Default: "fallback"},
	})
	if len(errs) != 0 {
		t.Fatalf("Resolve errors: %v", errs)
	}

	v, _ := r.Get("target_root")
	if v.Value != "fallback" {
		t.Errorf("value = %q, want fallback", v.Value)
	}
	if v.Source.Kind != VariableSourceKindDefault {
		t.Errorf("kind = %v, want Default", v.Source.Kind)
	}
}

func TestVariableResolver_MissingRequired(t *testing.T) {

	r := NewVariableResolver(newTestApp("writ", nil, nil, nil))

	errs := r.Resolve(nil, []Parameter{
		{Name: "target_root", Type: reflect.TypeFor[string]()},
	})
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0].Error(), "target_root") {
		t.Errorf("error should name the parameter: %v", errs[0])
	}
	if !strings.Contains(errs[0].Error(), "WRIT_TARGET_ROOT") {
		t.Errorf("error should name the env key it tried: %v", errs[0])
	}
}

func TestVariableResolver_OptionalMissingProducesNoEntry(t *testing.T) {

	r := NewVariableResolver(newTestApp("writ", nil, nil, nil))

	errs := r.Resolve(nil, []Parameter{
		{Name: "target_root", Type: reflect.TypeFor[string](), Optional: true},
	})
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}

	if _, ok := r.Get("target_root"); ok {
		t.Error("optional missing parameter should not produce a resolved entry")
	}
}

func TestVariableResolver_TypeMismatch_Flag(t *testing.T) {

	r := NewVariableResolver(newTestApp(
		"writ",
		map[string]any{"port": "not-an-int"},
		nil,
		nil,
	))

	errs := r.Resolve(nil, []Parameter{
		{Name: "port", Type: reflect.TypeFor[int]()},
	})
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0].Error(), "port") {
		t.Errorf("error should name parameter: %v", errs[0])
	}
}

func TestVariableResolver_TypeMismatch_Env_ProducesError(t *testing.T) {

	t.Setenv("WRIT_PORT", "not-an-int")

	r := NewVariableResolver(newTestApp("writ", nil, nil, nil))

	errs := r.Resolve(nil, []Parameter{
		{Name: "port", Type: reflect.TypeFor[int]()},
	})
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0].Error(), "WRIT_PORT") {
		t.Errorf("error should name env key: %v", errs[0])
	}
}

func TestVariableResolver_CamelCase_EnvKey(t *testing.T) {

	// targetRoot camelCase → TARGET_ROOT via op.CamelToSnake + uppercase.
	t.Setenv("WRIT_TARGET_ROOT", "from-env")

	r := NewVariableResolver(newTestApp("writ", nil, nil, nil))

	errs := r.Resolve(nil, []Parameter{
		{Name: "targetRoot", Type: reflect.TypeFor[string]()},
	})
	if len(errs) != 0 {
		t.Fatalf("Resolve errors: %v", errs)
	}

	v, _ := r.Get("targetRoot")
	if v.Value != "from-env" {
		t.Errorf("camelCase name should resolve via env key WRIT_TARGET_ROOT; got %q", v.Value)
	}
}

func TestVariableResolver_NoEnvWhenPrefixEmpty(t *testing.T) {

	// app with empty Name → EnvPrefix is "" → env step is skipped, so a stray TARGET_ROOT in the
	// process env must not be picked up.
	t.Setenv("TARGET_ROOT", "leaked-from-shell")

	r := NewVariableResolver(newTestApp("", nil, nil, nil))

	errs := r.Resolve(nil, []Parameter{
		{Name: "target_root", Type: reflect.TypeFor[string](), Optional: true},
	})
	if len(errs) != 0 {
		t.Fatalf("Resolve errors: %v", errs)
	}

	if v, ok := r.Get("target_root"); ok {
		t.Errorf("env step should be skipped when prefix empty; got %v", v)
	}
}

func TestVariableResolver_MultipleErrors_Aggregated(t *testing.T) {

	r := NewVariableResolver(newTestApp("writ", nil, nil, nil))

	errs := r.Resolve(nil, []Parameter{
		{Name: "a", Type: reflect.TypeFor[string]()},
		{Name: "b", Type: reflect.TypeFor[int]()},
	})
	if len(errs) != 2 {
		t.Fatalf("expected 2 aggregated errors, got %d: %v", len(errs), errs)
	}
}

func TestVariableResolver_NilApp_OnlyEnv(t *testing.T) {

	t.Setenv("TARGET_ROOT_X", "nope") // nil app → empty prefix → env skipped.

	r := NewVariableResolver(nil)

	errs := r.Resolve(nil, []Parameter{
		{Name: "target_root_x", Type: reflect.TypeFor[string](), Optional: true},
	})
	if len(errs) != 0 {
		t.Fatalf("Resolve errors: %v", errs)
	}

	if _, ok := r.Get("target_root_x"); ok {
		t.Error("nil app + no default → no entry")
	}
}
