// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlarkbridge

import (
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"go.starlark.net/starlark"
)

// region Immediate-mode integration

// makeImmediateReceiver returns a goReceiver around a fresh pipelineProvider, ready to drive immediate-mode
// dispatch through goReceiver.Attr → builtin → goReceiver.dispatch → Method.Invoke.
//
// The receiver is bound to the registered ProviderReceiverType (mirroring Runtime.buildOne), so the announced
// parameter names — including *items / **opts variadic markers — survive into goReceiver.dispatch's classifier.
func makeImmediateReceiver(t *testing.T) (starlark.HasAttrs, *op.RuntimeEnvironment) {
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

	provider := NewPipelineProvider(ctx)
	return newGoReceiver(rt, provider), ctx
}

// callBuiltin resolves a builtin by name, calls it with the given args, and returns the starlark result.
func callBuiltin(t *testing.T, w starlark.HasAttrs, name string, args ...starlark.Value) starlark.Value {
	t.Helper()

	attr, err := w.Attr(name)
	if err != nil {
		t.Fatalf("Attr(%q): %v", name, err)
	}
	builtin, ok := attr.(*starlark.Builtin)
	if !ok {
		t.Fatalf("Attr(%q): got %T, want *starlark.Builtin", name, attr)
	}

	thread := &starlark.Thread{}
	result, err := starlark.Call(thread, builtin, starlark.Tuple(args), nil)
	if err != nil {
		t.Fatalf("Call(%q): %v", name, err)
	}
	return result
}

// endregion

// region Immediate-mode dispatch

func TestImmediatePipeline_PrimitiveString(t *testing.T) {

	w, _ := makeImmediateReceiver(t)

	got := callBuiltin(t, w, "echo", starlark.String("hello"))

	s, ok := got.(starlark.String)
	if !ok {
		t.Fatalf("got %T, want starlark.String", got)
	}
	if string(s) != "hello" {
		t.Errorf("got %q, want \"hello\"", s)
	}
}

func TestImmediatePipeline_StringToResource(t *testing.T) {

	w, _ := makeImmediateReceiver(t)

	got := callBuiltin(t, w, "echo_resource", starlark.String("/etc/foo"))

	// EchoResource returns the Path as a string.
	s, ok := got.(starlark.String)
	if !ok {
		t.Fatalf("got %T, want starlark.String", got)
	}
	if string(s) != "/etc/foo" {
		t.Errorf("got %q, want \"/etc/foo\"", s)
	}
}

func TestImmediatePipeline_ListToTypedSlice(t *testing.T) {

	w, _ := makeImmediateReceiver(t)

	list := starlark.NewList([]starlark.Value{
		starlark.String("alpha"),
		starlark.String("beta"),
	})
	got := callBuiltin(t, w, "echo_strings", list)

	s, ok := got.(starlark.String)
	if !ok {
		t.Fatalf("got %T, want starlark.String", got)
	}
	if string(s) != "alpha" {
		t.Errorf("got %q, want \"alpha\"", s)
	}
}

func TestImmediatePipeline_Variadic(t *testing.T) {

	w, _ := makeImmediateReceiver(t)

	// Variadic positional: pass strings positionally.
	got := callBuiltin(t, w, "echo_variadic", starlark.String("alpha"), starlark.String("beta"))
	s, ok := got.(starlark.String)
	if !ok {
		t.Fatalf("got %T, want starlark.String", got)
	}
	if string(s) != "alpha" {
		t.Errorf("got %q, want \"alpha\"", s)
	}
}

func TestImmediatePipeline_VariadicAsKwargList(t *testing.T) {

	w, _ := makeImmediateReceiver(t)

	// Variadic via kwarg with a list. Pass items=[...] as kwargs to the builtin.
	attr, _ := w.Attr("echo_variadic")
	builtin := attr.(*starlark.Builtin)
	thread := &starlark.Thread{}
	list := starlark.NewList([]starlark.Value{
		starlark.String("kw-alpha"),
		starlark.String("kw-beta"),
	})
	got, err := starlark.Call(thread, builtin, nil, []starlark.Tuple{
		{starlark.String("items"), list},
	})
	if err != nil {
		t.Fatalf("starlark.Call: %v", err)
	}
	s, ok := got.(starlark.String)
	if !ok {
		t.Fatalf("got %T, want starlark.String", got)
	}
	if string(s) != "kw-alpha" {
		t.Errorf("got %q, want \"kw-alpha\"", s)
	}
}

func TestImmediatePipeline_Kwargs(t *testing.T) {

	w, _ := makeImmediateReceiver(t)

	attr, _ := w.Attr("echo_kwargs")
	builtin := attr.(*starlark.Builtin)
	thread := &starlark.Thread{}
	got, err := starlark.Call(thread, builtin, nil, []starlark.Tuple{
		{starlark.String("key"), starlark.String("value-from-kwargs")},
		{starlark.String("other"), starlark.MakeInt64(42)},
	})
	if err != nil {
		t.Fatalf("starlark.Call: %v", err)
	}
	s, ok := got.(starlark.String)
	if !ok {
		t.Fatalf("got %T, want starlark.String", got)
	}
	if string(s) != "value-from-kwargs" {
		t.Errorf("got %q, want \"value-from-kwargs\"", s)
	}
}

func TestImmediatePipeline_NoneArg(t *testing.T) {

	w, _ := makeImmediateReceiver(t)

	// Pass None for an optional parameter; the method should receive zero-value (empty string).
	got := callBuiltin(t, w, "echo_optional", starlark.None)
	s, ok := got.(starlark.String)
	if !ok {
		t.Fatalf("got %T, want starlark.String", got)
	}
	if string(s) != "default" {
		t.Errorf("got %q, want \"default\" (zero-value branch)", s)
	}
}

func TestImmediatePipeline_UnknownAttr(t *testing.T) {

	w, _ := makeImmediateReceiver(t)

	_, err := w.Attr("nonexistent_method")
	if err == nil {
		t.Fatal("want error for unknown attribute")
	}
}

func TestImmediatePipeline_StringToResource_BadValue(t *testing.T) {

	w, _ := makeImmediateReceiver(t)

	// Pass an Int where the method expects a *pipelineResource. The constructor errors on non-string.
	attr, _ := w.Attr("echo_resource")
	builtin := attr.(*starlark.Builtin)
	thread := &starlark.Thread{}
	_, err := starlark.Call(thread, builtin, starlark.Tuple{starlark.MakeInt64(42)}, nil)
	if err == nil {
		t.Fatal("want error from constructor")
	}
}

// endregion

// region Immediate-mode error paths

// TestImmediatePipeline_VariadicConflict drives the "multiple values" error: positional variadic and a
// keyword spelling of the same name must not coexist.
func TestImmediatePipeline_VariadicConflict(t *testing.T) {

	w, _ := makeImmediateReceiver(t)

	attr, _ := w.Attr("echo_variadic")
	builtin := attr.(*starlark.Builtin)
	thread := &starlark.Thread{}
	list := starlark.NewList([]starlark.Value{starlark.String("kw")})
	_, err := starlark.Call(thread, builtin,
		starlark.Tuple{starlark.String("pos")},
		[]starlark.Tuple{{starlark.String("items"), list}})
	if err == nil {
		t.Fatal("want error for positional+kw variadic conflict")
	}
}

// TestImmediatePipeline_VariadicKwargNotList drives the "must be a list" error: kw spelling of the variadic
// parameter must carry a starlark list value.
func TestImmediatePipeline_VariadicKwargNotList(t *testing.T) {

	w, _ := makeImmediateReceiver(t)

	attr, _ := w.Attr("echo_variadic")
	builtin := attr.(*starlark.Builtin)
	thread := &starlark.Thread{}
	_, err := starlark.Call(thread, builtin, nil, []starlark.Tuple{
		{starlark.String("items"), starlark.String("not-a-list")},
	})
	if err == nil {
		t.Fatal("want error for non-list kw variadic")
	}
}

// TestImmediatePipeline_UnexpectedKwarg drives the unexpected-kwarg path on a method without a **kwargs
// catch-all. echo accepts only a single named "s"; passing "stranger" must reject before UnpackArgs.
func TestImmediatePipeline_UnexpectedKwarg(t *testing.T) {

	w, _ := makeImmediateReceiver(t)

	attr, _ := w.Attr("echo")
	builtin := attr.(*starlark.Builtin)
	thread := &starlark.Thread{}
	_, err := starlark.Call(thread, builtin, nil, []starlark.Tuple{
		{starlark.String("stranger"), starlark.String("uninvited")},
	})
	if err == nil {
		t.Fatal("want error for unexpected kwarg")
	}
}

// TestImmediatePipeline_DirectiveDefault verifies 13.0(f) step 11 in immediate-mode: a call that omits a
// defaulted kwarg flows the directive's default through to the underlying Go method. The provider announces
// EchoMode with "mode?=0o755"; calling without mode must invoke EchoMode with os.FileMode(0o755), which the
// method returns as a uint32 for assertion.
func TestImmediatePipeline_DirectiveDefault(t *testing.T) {

	w, _ := makeImmediateReceiver(t)

	got := callBuiltin(t, w, "echo_mode")

	n, ok := got.(starlark.Int)
	if !ok {
		t.Fatalf("got %T, want starlark.Int", got)
	}
	v, _ := n.Uint64()
	if v != 0o755 {
		t.Errorf("default mode = %o, want 0o755", v)
	}
}

// endregion
