// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlarkbridge

import (
	"os"
	"reflect"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"go.starlark.net/starlark"
)

// region Plan-mode integration fixtures

// pipelineProvider is a registered Provider used to drive plan-mode dispatch end-to-end. Its methods cover
// the shapes the conversion pipeline must handle: scalar primitive, Resource argument, slice argument.
type pipelineProvider struct {
	op.ProviderBase
}

// NewPipelineProvider constructs a *pipelineProvider for the registered ProviderConstructor.
func NewPipelineProvider(ctx *op.RuntimeEnvironment) *pipelineProvider {
	return &pipelineProvider{ProviderBase: op.NewProviderBase(ctx)}
}

// Echo accepts a string parameter and returns it. Smallest-possible primitive method.
func (p *pipelineProvider) Echo(s string) (string, error) { return s, nil }

// EchoResource accepts a *pipelineResource (registered Resource type) and returns its path. Exercises the
// string → Resource construction path through op.Convert step 7.
func (p *pipelineProvider) EchoResource(r *pipelineResource) (string, error) { return r.Path, nil }

// EchoStrings accepts a []string and returns the first element. Exercises slice element conversion.
func (p *pipelineProvider) EchoStrings(items []string) (string, error) {
	if len(items) == 0 {
		return "", nil
	}
	return items[0], nil
}

// EchoVariadic accepts variadic string items. Exercises the *args path in goReceiver.dispatch.
func (p *pipelineProvider) EchoVariadic(items []string) (string, error) {
	if len(items) == 0 {
		return "", nil
	}
	return items[0], nil
}

// EchoKwargs accepts a kwargs catch-all. Exercises the **kwargs path in goReceiver.dispatch.
func (p *pipelineProvider) EchoKwargs(opts map[string]any) (string, error) {
	if v, ok := opts["key"]; ok {
		if s, ok := v.(string); ok {
			return s, nil
		}
	}
	return "", nil
}

// EchoOptional accepts an optional parameter; passing None should skip it (NoneType short-circuit).
func (p *pipelineProvider) EchoOptional(label string) (string, error) {
	if label == "" {
		return "default", nil
	}
	return label, nil
}

// EchoMode accepts an [os.FileMode] parameter and returns it as the natural uint32. Drives the
// directive-default integration tests (announced as "mode?=0o755") — when the kwarg is omitted, slot-fill
// must reach the announced default and the method receives os.FileMode(0o755) at the parameter's exact type.
func (p *pipelineProvider) EchoMode(mode os.FileMode) (uint32, error) { return uint32(mode), nil }

// init registers pipelineProvider at test-binary load time. Lives alongside pipelineResource registration in
// starlark_to_go_typed_test.go's init.
func init() {
	op.AnnounceProvider(
		reflect.TypeFor[pipelineProvider](),
		op.RoleModule|op.RoleAction,
		func(ctx *op.RuntimeEnvironment) (any, error) { return NewPipelineProvider(ctx), nil },
		map[string][]string{
			"Echo":         {"s"},
			"EchoResource": {"r"},
			"EchoStrings":  {"items"},
			"EchoVariadic": {"*items"},
			"EchoKwargs":   {"**opts"},
			"EchoOptional": {"label?"},
			"EchoMode":     {"mode?=0o755"},
		},
	)
}

// makePlanNodeBuilder returns a NodeBuilder for the registered pipelineProvider, ready to drive plan-mode
// dispatch.
func makePlanNodeBuilder(t *testing.T) (*NodeBuilder, *op.RuntimeEnvironment) {
	t.Helper()

	reg := op.NewReceiverRegistry()
	ctx := &op.RuntimeEnvironment{
		Registry: reg,
		Catalog:  op.NewResourceCatalog(),
	}

	rt, ok := reg.ModuleByName("pipelineProvider")
	if !ok {
		t.Fatalf("pipelineProvider not in registry")
	}

	return NewNodeBuilder(rt, ctx, ctx.Catalog, NewInvocationRegistry()), ctx
}

// endregion

// region Plan-mode dispatch — end-to-end starlark → catalog interning

func TestPlanPipeline_StringToResource(t *testing.T) {

	nb, ctx := makePlanNodeBuilder(t)

	// Get the EchoResource builtin from the NodeBuilder.
	attr, err := nb.Attr("echo_resource")
	if err != nil {
		t.Fatalf("Attr(echo_resource): %v", err)
	}
	builtin, ok := attr.(*starlark.Builtin)
	if !ok {
		t.Fatalf("got %T, want *starlark.Builtin", attr)
	}

	// Invoke with a string argument; expect an *Invocation back. The slot should hold the canonical Resource.
	thread := &starlark.Thread{}
	result, err := starlark.Call(thread, builtin, starlark.Tuple{starlark.String("/etc/foo")}, nil)
	if err != nil {
		t.Fatalf("starlark.Call: %v", err)
	}

	inv, ok := result.(*Invocation)
	if !ok {
		t.Fatalf("got %T, want *Invocation", result)
	}

	// Verify the slot holds a *pipelineResource with the right Path.
	slot := inv.Target.(*op.Node).SlotByName("r")
	if slot == nil {
		t.Fatalf("slot \"r\" missing on target node")
	}
	got := slot.Immediate()
	r, ok := got.(*pipelineResource)
	if !ok {
		t.Fatalf("slot value: got %T, want *pipelineResource", got)
	}
	if r.Path != "/etc/foo" {
		t.Errorf("Path = %q, want \"/etc/foo\"", r.Path)
	}

	// Catalog must have an entry for the URI.
	if id := ctx.Catalog.Current(r.URI()); id == "" {
		t.Errorf("catalog has no entry for URI %q", r.URI())
	}
}

func TestPlanPipeline_PrimitiveString(t *testing.T) {

	nb, _ := makePlanNodeBuilder(t)

	attr, err := nb.Attr("echo")
	if err != nil {
		t.Fatalf("Attr(echo): %v", err)
	}
	builtin := attr.(*starlark.Builtin)

	thread := &starlark.Thread{}
	result, err := starlark.Call(thread, builtin, starlark.Tuple{starlark.String("hello")}, nil)
	if err != nil {
		t.Fatalf("starlark.Call: %v", err)
	}
	inv := result.(*Invocation)

	slot := inv.Target.(*op.Node).SlotByName("s")
	got := slot.Immediate()
	if got != "hello" {
		t.Errorf("slot value: got %v, want \"hello\"", got)
	}
}

func TestPlanPipeline_ListToTypedSlice(t *testing.T) {

	nb, _ := makePlanNodeBuilder(t)

	attr, err := nb.Attr("echo_strings")
	if err != nil {
		t.Fatalf("Attr(echo_strings): %v", err)
	}
	builtin := attr.(*starlark.Builtin)

	list := starlark.NewList([]starlark.Value{
		starlark.String("a"),
		starlark.String("b"),
	})
	thread := &starlark.Thread{}
	result, err := starlark.Call(thread, builtin, starlark.Tuple{list}, nil)
	if err != nil {
		t.Fatalf("starlark.Call: %v", err)
	}
	inv := result.(*Invocation)

	slot := inv.Target.(*op.Node).SlotByName("items")
	got := slot.Immediate()
	want := []string{"a", "b"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("slot value: got %#v, want %#v", got, want)
	}
}

func TestPlanPipeline_NoneTypeShortCircuit(t *testing.T) {

	nb, _ := makePlanNodeBuilder(t)

	attr, err := nb.Attr("echo_optional")
	if err != nil {
		t.Fatalf("Attr(echo_optional): %v", err)
	}
	builtin := attr.(*starlark.Builtin)

	thread := &starlark.Thread{}
	// Pass None for the optional parameter; fillSlot should short-circuit and not set the slot.
	result, err := starlark.Call(thread, builtin, starlark.Tuple{starlark.None}, nil)
	if err != nil {
		t.Fatalf("starlark.Call: %v", err)
	}
	inv := result.(*Invocation)

	// The optional slot should be unset (returns nil from SlotByName).
	slot := inv.Target.(*op.Node).SlotByName("label")
	if slot != nil {
		t.Errorf("slot \"label\" should be nil for None input, got %v", slot)
	}
}

func TestPlanPipeline_InvocationShortCircuit(t *testing.T) {

	nb, _ := makePlanNodeBuilder(t)

	// First, create an Invocation by calling Echo with a string.
	echoBuiltin, _ := nb.Attr("echo")
	thread := &starlark.Thread{}
	first, err := starlark.Call(thread, echoBuiltin.(*starlark.Builtin), starlark.Tuple{starlark.String("upstream")}, nil)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	upstreamInv := first.(*Invocation)

	// Now pass that Invocation as input to another Echo. fillSlot's *Invocation branch should fire.
	second, err := starlark.Call(thread, echoBuiltin.(*starlark.Builtin), starlark.Tuple{upstreamInv}, nil)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	consumerInv := second.(*Invocation)

	// The slot should hold a PromiseValue (since target is string, not ExecutableUnit).
	slot := consumerInv.Target.(*op.Node).SlotByName("s")
	if slot == nil {
		t.Fatalf("slot \"s\" missing")
	}
	if _, ok := slot.Value.(op.PromiseValue); !ok {
		t.Errorf("slot value: got %T, want op.PromiseValue", slot.Value)
	}
}

// TestPlanPipeline_ProjectorPath drives fillSlot's Projector branch: a *goReceiver passed as a slot value
// projects itself to the slot's Go target type via goReceiver.Project → op.Convert.
func TestPlanPipeline_ProjectorPath(t *testing.T) {

	nb, _ := makePlanNodeBuilder(t)

	wrapped, err := NewGoReceiver(&pipelineResource{Path: "/projected"})
	if err != nil {
		t.Fatalf("NewGoReceiver: %v", err)
	}

	attr, _ := nb.Attr("echo_resource")
	builtin := attr.(*starlark.Builtin)
	thread := &starlark.Thread{}
	result, err := starlark.Call(thread, builtin, starlark.Tuple{wrapped.(starlark.Value)}, nil)
	if err != nil {
		t.Fatalf("starlark.Call: %v", err)
	}
	inv := result.(*Invocation)

	slot := inv.Target.(*op.Node).SlotByName("r")
	if slot == nil {
		t.Fatalf("slot \"r\" missing")
	}
	got := slot.Immediate()
	r, ok := got.(*pipelineResource)
	if !ok {
		t.Fatalf("slot value: got %T, want *pipelineResource", got)
	}
	if r.Path != "/projected" {
		t.Errorf("Path = %q, want \"/projected\"", r.Path)
	}
}

func TestPlanPipeline_TwoCallsSameURI_SameCanonical(t *testing.T) {

	nb, ctx := makePlanNodeBuilder(t)

	attr, _ := nb.Attr("echo_resource")
	builtin := attr.(*starlark.Builtin)

	thread := &starlark.Thread{}

	r1Result, err := starlark.Call(thread, builtin, starlark.Tuple{starlark.String("/etc/x")}, nil)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	inv1 := r1Result.(*Invocation)
	r1 := inv1.Target.(*op.Node).SlotByName("r").Immediate().(*pipelineResource)

	r2Result, err := starlark.Call(thread, builtin, starlark.Tuple{starlark.String("/etc/x")}, nil)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	inv2 := r2Result.(*Invocation)
	r2 := inv2.Target.(*op.Node).SlotByName("r").Immediate().(*pipelineResource)

	if r1 != r2 {
		t.Errorf("canonical pointers differ: r1=%p r2=%p", r1, r2)
	}

	// Catalog should have exactly one entry for the URI.
	if id := ctx.Catalog.Current(r1.URI()); id == "" {
		t.Errorf("catalog has no entry for URI")
	}
}

// TestPlanPipeline_DirectiveDefault verifies 13.0(f) step 11: a plan-mode call that omits a defaulted
// kwarg lands the directive's default value on the slot at the parameter's Go type exactly. The provider
// announces EchoMode with "mode?=0o755"; calling without mode must populate the slot with
// os.FileMode(0o755) via op.ImmediateValue, bypassing the starlark-conversion fillSlot path.
func TestPlanPipeline_DirectiveDefault(t *testing.T) {

	nb, _ := makePlanNodeBuilder(t)

	attr, err := nb.Attr("echo_mode")
	if err != nil {
		t.Fatalf("Attr(echo_mode): %v", err)
	}
	builtin := attr.(*starlark.Builtin)

	thread := &starlark.Thread{}
	result, err := starlark.Call(thread, builtin, nil, nil)
	if err != nil {
		t.Fatalf("starlark.Call: %v", err)
	}

	inv, ok := result.(*Invocation)
	if !ok {
		t.Fatalf("got %T, want *Invocation", result)
	}

	slot := inv.Target.(*op.Node).SlotByName("mode")
	if slot == nil {
		t.Fatalf("slot \"mode\" missing on target node")
	}

	got, ok := slot.Immediate().(os.FileMode)
	if !ok {
		t.Fatalf("slot value: got %T, want os.FileMode", slot.Immediate())
	}
	if got != os.FileMode(0o755) {
		t.Errorf("default mode = %o, want 0o755", got)
	}
}

// endregion
