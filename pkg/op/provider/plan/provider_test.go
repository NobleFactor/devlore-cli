// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package plan

import (
	"reflect"
	"strings"
	"testing"

	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// resolutionProvider builds a plan Provider against the test binary's registry (file + flow + plan announced by the
// API tests' blank imports and the plan_announce_test.go shim).
func resolutionProvider(t *testing.T) *Provider {
	t.Helper()
	return NewProvider(&op.RuntimeEnvironment{})
}

// tierCollisionRootProvider is a synthetic root provider whose one method ("Variable" → snake "variable") is aimed
// at whatever tier a test seeds into promoteRootMethods' name sets. Built as a real receiver type via
// [op.NewProviderReceiverType]; never announced, so the process registry stays clean.
type tierCollisionRootProvider struct{}

func (tierCollisionRootProvider) Variable() string { return "" }

// collisionRootReceiverType constructs the synthetic root provider's receiver type.
func collisionRootReceiverType(t *testing.T) op.ProviderReceiverType {
	t.Helper()

	receiverType, err := op.NewProviderReceiverType(
		reflect.TypeFor[tierCollisionRootProvider](),
		func(*op.RuntimeEnvironment) (any, error) { return nil, nil },
		op.RoleAction|op.RoleRoot,
		map[string][]op.Parameter{"Variable": {}},
		nil,
	)
	if err != nil {
		t.Fatalf("NewProviderReceiverType: %v", err)
	}

	return receiverType
}

// wantPanicContaining runs `body` and asserts it panics with a message containing `fragment`.
func wantPanicContaining(t *testing.T, fragment string, body func()) {
	t.Helper()

	defer func() {
		r := recover()
		if r == nil {
			t.Errorf("no panic; want one containing %q", fragment)
			return
		}
		if message, ok := r.(string); !ok || !strings.Contains(message, fragment) {
			t.Errorf("panic = %v, want message containing %q", r, fragment)
		}
	}()

	body()
}

func TestProvider_ResolveAttr_Tier2_PromotedBuiltin(t *testing.T) {

	p := resolutionProvider(t)

	for _, name := range []string{"choose", "gather", "subgraph", "wait_until"} {
		resolved := p.ResolveAttr(name)
		if _, ok := resolved.(*starlark.Builtin); !ok {
			t.Errorf("ResolveAttr(%q) = %T, want *starlark.Builtin (Tier 2 promoted from flow)", name, resolved)
		}
	}
}

func TestProvider_ResolveAttr_Tier1_SubNamespaceAdapter(t *testing.T) {

	p := resolutionProvider(t)

	resolved := p.ResolveAttr("file")
	if _, ok := resolved.(*adapter); !ok {
		t.Errorf("ResolveAttr(%q) = %T, want *adapter (Tier 1 sub-namespace)", "file", resolved)
	}
}

func TestProvider_ResolveAttr_Tier3_OwnMethod(t *testing.T) {

	// Tier 3 (plan's own methods) deliberately never reaches ResolveAttr — the bridge's goReceiver resolves them
	// against the announced receiver type first. Prove both halves: ResolveAttr declines the name, and the receiver
	// path can serve it.
	p := resolutionProvider(t)

	if resolved := p.ResolveAttr("assemble_definition"); resolved != nil {
		t.Errorf("ResolveAttr(%q) = %T, want nil (own methods ride the goReceiver path, not ResolveAttr)",
			"assemble_definition", resolved)
	}

	rt, ok := op.ReceiverRegistry().Type("plan")
	if !ok {
		t.Fatal(`registry has no "plan" receiver type (plan_announce_test.go shim missing?)`)
	}
	if _, found := rt.MethodByName("AssembleDefinition"); !found {
		t.Error(`plan receiver type lacks AssembleDefinition; the goReceiver path could not serve plan.assemble_definition`)
	}
}

func TestProvider_ResolveAttr_RootProviderExcludedFromTier1(t *testing.T) {

	p := resolutionProvider(t)

	if resolved := p.ResolveAttr("flow"); resolved != nil {
		t.Errorf("ResolveAttr(%q) = %T, want nil (root providers are not nested sub-namespaces)", "flow", resolved)
	}
}

func TestProvider_ResolveAttr_UnknownReturnsNil(t *testing.T) {

	p := resolutionProvider(t)

	if resolved := p.ResolveAttr("no_such_namespace_or_method"); resolved != nil {
		t.Errorf("ResolveAttr(unknown) = %T, want nil", resolved)
	}
}

func TestProvider_ResolveAttr_TierOrder(t *testing.T) {

	// Collisions across tiers panic at construction (invariant I4), so no real name can occupy two tiers. Prove the
	// precedence structurally by injecting a synthetic promoted entry over an existing Tier-1 name on this instance —
	// the promoted map must win before the adapter path is consulted.
	p := resolutionProvider(t)

	p.promotedBuiltins["file"] = starlark.None
	if resolved := p.ResolveAttr("file"); resolved != starlark.None {
		t.Errorf("ResolveAttr(%q) with injected promoted entry = %T, want the promoted value (Tier 2 wins)",
			"file", resolved)
	}
}

func TestProvider_BuildPromotedBuiltins_PanicsOnCollision_PromotedVsOwn(t *testing.T) {

	p := resolutionProvider(t)
	roots := []op.ProviderReceiverType{collisionRootReceiverType(t)}

	wantPanicContaining(t, "collides with plan.Provider's own method", func() {
		p.promoteRootMethods(map[string]struct{}{"variable": {}}, map[string]struct{}{}, roots)
	})
}

func TestProvider_BuildPromotedBuiltins_PanicsOnCollision_PromotedVsSubNamespace(t *testing.T) {

	p := resolutionProvider(t)
	roots := []op.ProviderReceiverType{collisionRootReceiverType(t)}

	wantPanicContaining(t, "collides with sub-namespace adapter name", func() {
		p.promoteRootMethods(map[string]struct{}{}, map[string]struct{}{"variable": {}}, roots)
	})
}

func TestProvider_PromoteRootMethods_PanicsOnDuplicateRootMethod(t *testing.T) {

	// Extra-matrix companion to rows 7–8: the third collision case — the same method promoted from two root
	// providers.
	p := resolutionProvider(t)
	duplicate := collisionRootReceiverType(t)

	wantPanicContaining(t, "collides with another root provider's method", func() {
		p.promoteRootMethods(map[string]struct{}{}, map[string]struct{}{},
			[]op.ProviderReceiverType{duplicate, duplicate})
	})
}

func TestAdapter_Attr_RoutesToMethod(t *testing.T) {

	p := resolutionProvider(t)

	fileReceiverType, ok := op.ReceiverRegistry().PlannerByName("file")
	if !ok {
		t.Fatal(`registry has no "file" planner (file/gen announce missing?)`)
	}

	a := p.adapterFor(fileReceiverType)

	attr, err := a.Attr("write_text")
	if err != nil || attr == nil {
		t.Fatalf(`adapter.Attr("write_text") = (%v, %v), want a builtin`, attr, err)
	}
	if _, ok := attr.(*starlark.Builtin); !ok {
		t.Errorf(`adapter.Attr("write_text") = %T, want *starlark.Builtin`, attr)
	}

	missing, err := a.Attr("no_such_method")
	if missing != nil || err == nil {
		t.Errorf("adapter.Attr(unknown) = (%v, %v), want (nil, NoSuchAttrError)", missing, err)
	}
}
