// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package binding

import (
	"reflect"
	"testing"
)

func TestNewVariableResolver_NoOptions(t *testing.T) {

	r := NewVariableResolver()
	if r == nil {
		t.Fatal("NewVariableResolver() returned nil")
	}
}

func TestNewVariableResolver_WithAllOptions(t *testing.T) {

	r := NewVariableResolver(
		WithOverrides(map[string]any{"force": true}),
		WithFlags(map[string]any{"layer": "personal"}),
		WithEnvPrefix("DEVLORE_WRIT"),
		WithConfig(map[string]any{"project": "noblefactor"}),
	)

	if r.overrides["force"] != true {
		t.Errorf("WithOverrides did not record force=true; got %v", r.overrides)
	}
	if r.flags["layer"] != "personal" {
		t.Errorf("WithFlags did not record layer=personal; got %v", r.flags)
	}
	if r.envPrefix != "DEVLORE_WRIT" {
		t.Errorf("WithEnvPrefix did not record program prefix; got %q", r.envPrefix)
	}
	if r.globalPrefix != "DEVLORE" {
		t.Errorf("WithEnvPrefix did not derive global prefix; got %q want %q", r.globalPrefix, "DEVLORE")
	}
	if r.config["project"] != "noblefactor" {
		t.Errorf("WithConfig did not record project=noblefactor; got %v", r.config)
	}
}

func TestVariableResolver_GetPanicsBeforeResolve(t *testing.T) {

	r := NewVariableResolver()
	defer func() {
		if rec := recover(); rec == nil {
			t.Errorf("Get before Resolve should panic")
		}
	}()
	r.Get("anything")
}

func TestVariableResolver_VariablesPanicsBeforeResolve(t *testing.T) {

	r := NewVariableResolver()
	defer func() {
		if rec := recover(); rec == nil {
			t.Errorf("Variables before Resolve should panic")
		}
	}()
	r.Variables()
}

func TestVariableResolver_Resolve_SkeletonProducesEmptyMap(t *testing.T) {

	r := NewVariableResolver()
	errs := r.Resolve([]Parameter{
		{Name: "x", Type: reflect.TypeOf("")},
	})
	if len(errs) != 0 {
		t.Errorf("Phase 1 skeleton Resolve should return no errors; got %v", errs)
	}
	vars := r.Variables()
	if len(vars) != 0 {
		t.Errorf("Phase 1 skeleton Resolve should produce empty map; got %v", vars)
	}
}

func TestVariableResolver_GetReturnsFalseAfterEmptyResolve(t *testing.T) {

	r := NewVariableResolver()
	_ = r.Resolve(nil)
	if _, ok := r.Get("missing"); ok {
		t.Errorf("Get on missing name should return false after empty resolve")
	}
}

func TestDerivedGlobalPrefix(t *testing.T) {

	tests := []struct {
		in   string
		want string
	}{
		{"DEVLORE_WRIT", "DEVLORE"},
		{"DEVLORE_LORE", "DEVLORE"},
		{"DEVLORE_TEST", "DEVLORE"},
		{"FOO_BAR_BAZ", "FOO_BAR"},
		{"NOUNDERSCORE", ""},
		{"", ""},
	}

	for _, tc := range tests {
		if got := derivedGlobalPrefix(tc.in); got != tc.want {
			t.Errorf("derivedGlobalPrefix(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
