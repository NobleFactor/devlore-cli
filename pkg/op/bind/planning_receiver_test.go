// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package bind

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"go.starlark.net/starlark"
)

// --- Test factory for planned mode ---

// testProviderFactory wraps testProvider for planned-mode tests.
type testProviderFactory struct {
	params map[string][]string
}

func (f *testProviderFactory) ReceiverName() string                                { return "test" }
func (f *testProviderFactory) GetOrCreateProvider(_ op.Context) op.ContextProvider { return nil }
func (f *testProviderFactory) MethodParams() map[string][]string                   { return f.params }
func (f *testProviderFactory) MethodParamsFor(name string) []string                { return f.params[name] }
func (f *testProviderFactory) ProviderType() reflect.Type {
	return reflect.TypeOf((*testProvider)(nil)).Elem()
}
func (f *testProviderFactory) Register(_ op.Context, _ *op.ReceiverRegistry) {}

// --- Test actions for planned mode ---

type stubWriteAction struct{}

func (a *stubWriteAction) Name() string           { return "test.write" }
func (a *stubWriteAction) Params() []op.ParamInfo { return nil }
func (a *stubWriteAction) Do(_ *op.Context, _ map[string]any) (op.Result, op.Complement, error) {
	return nil, nil, nil
}

type stubReadAction struct{}

func (a *stubReadAction) Name() string           { return "test.read" }
func (a *stubReadAction) Params() []op.ParamInfo { return nil }
func (a *stubReadAction) Do(_ *op.Context, _ map[string]any) (op.Result, op.Complement, error) {
	return nil, nil, nil
}

type stubValidateAction struct{}

func (a *stubValidateAction) Name() string           { return "test.validate" }
func (a *stubValidateAction) Params() []op.ParamInfo { return nil }
func (a *stubValidateAction) Do(_ *op.Context, _ map[string]any) (op.Result, op.Complement, error) {
	return nil, nil, nil
}

// scopedFactory wraps an actionFactory with a custom subset of method params.
// This lets planned-mode tests restrict which methods WrapProviderInPlanningReceiver
// exposes, since the function now reads params from the factory interface.
type scopedFactory struct {
	*actionFactory
	params map[string][]string
}

func (f *scopedFactory) MethodParams() map[string][]string  { return f.params }
func (f *scopedFactory) MethodParamsFor(name string) []string { return f.params[name] }

func newScopedFactory(params map[string][]string) *scopedFactory {
	return &scopedFactory{actionFactory: newActionFactory(), params: params}
}

func TestWrapProviderInPlanningReceiver_MethodFiltering(t *testing.T) {
	reg := op.NewActionRegistry()
	reg.Register(&stubWriteAction{})
	// Note: "test.validate" NOT registered, so validate should not appear.

	graph := &op.Graph{}
	p := WrapProviderInPlanningReceiver(&testProviderFactory{params: MethodParams{
		"Write":    {"path", "content"},
		"Validate": {"s"},
		"Greet":    {"name"}, // No action registered.
	}}, graph, "proj", reg)

	names := p.AttrNames()
	if len(names) != 1 || names[0] != "write" {
		t.Errorf("AttrNames() = %v, want [write]", names)
	}
}

func TestWrapProviderInPlanningReceiver_CreatesNode(t *testing.T) {
	reg := op.NewActionRegistry()
	reg.Register(&stubWriteAction{})

	graph := &op.Graph{}
	p := WrapProviderInPlanningReceiver(&testProviderFactory{params: MethodParams{
		"Write": {"path", "content"},
	}}, graph, "myproject", reg)

	attr, err := p.Attr("write")
	if err != nil {
		t.Fatalf("Attr(write) error: %v", err)
	}

	builtin := attr.(*starlark.Builtin)
	result, err := builtin.CallInternal(nil, nil, []starlark.Tuple{
		{starlark.String("path"), starlark.String("/tmp/file")},
		{starlark.String("content"), starlark.String("hello")},
	})
	if err != nil {
		t.Fatalf("write() error: %v", err)
	}

	// Result should be an Promise (promise).
	output, ok := result.(*Promise)
	if !ok {
		t.Fatalf("result = %T, want *Promise", result)
	}

	// Verify node was created.
	if len(graph.Nodes) != 1 {
		t.Fatalf("graph has %d nodes, want 1", len(graph.Nodes))
	}

	node := graph.Nodes[0]
	if node.Action.Name() != "test.write" {
		t.Errorf("action = %q, want 'test.write'", node.Action.Name())
	}
	if node.Project != "myproject" {
		t.Errorf("project = %q, want 'myproject'", node.Project)
	}
	if output.Node() != node {
		t.Error("output.Node() does not match graph node")
	}
}

func TestWrapProviderInPlanningReceiver_SlotsPopulated(t *testing.T) {
	reg := op.NewActionRegistry()
	reg.Register(&stubWriteAction{})

	graph := &op.Graph{}
	p := WrapProviderInPlanningReceiver(&testProviderFactory{params: MethodParams{
		"Write": {"path", "content"},
	}}, graph, "proj", reg)

	attr, _ := p.Attr("write")
	builtin := attr.(*starlark.Builtin)
	_, err := builtin.CallInternal(nil, nil, []starlark.Tuple{
		{starlark.String("path"), starlark.String("/tmp/x")},
		{starlark.String("content"), starlark.String("data")},
	})
	if err != nil {
		t.Fatalf("write() error: %v", err)
	}

	node := graph.Nodes[0]
	path := node.GetSlot("path")
	if path != "/tmp/x" {
		t.Errorf("slot path = %v, want '/tmp/x'", path)
	}
	content := node.GetSlot("content")
	if content != "data" {
		t.Errorf("slot content = %v, want 'data'", content)
	}
}

func TestWrapProviderInPlanningReceiver_PromiseChaining(t *testing.T) {
	reg := op.NewActionRegistry()
	reg.Register(&stubWriteAction{})
	reg.Register(&stubValidateAction{})

	graph := &op.Graph{}
	p := WrapProviderInPlanningReceiver(&testProviderFactory{params: MethodParams{
		"Write":    {"path", "content"},
		"Validate": {"s"},
	}}, graph, "proj", reg)

	// First call: write.
	writeAttr, _ := p.Attr("write")
	writeBuiltin := writeAttr.(*starlark.Builtin)
	writeResult, err := writeBuiltin.CallInternal(nil, nil, []starlark.Tuple{
		{starlark.String("path"), starlark.String("/tmp/f")},
		{starlark.String("content"), starlark.String("hello")},
	})
	if err != nil {
		t.Fatalf("write() error: %v", err)
	}

	// Second call: validate, passing write output as promise.
	validateAttr, _ := p.Attr("validate")
	validateBuiltin := validateAttr.(*starlark.Builtin)
	_, err = validateBuiltin.CallInternal(nil, nil, []starlark.Tuple{
		{starlark.String("s"), writeResult},
	})
	if err != nil {
		t.Fatalf("validate() error: %v", err)
	}

	// Verify graph has 2 nodes and 1 edge.
	if len(graph.Nodes) != 2 {
		t.Fatalf("graph has %d nodes, want 2", len(graph.Nodes))
	}
	if len(graph.Edges) != 1 {
		t.Fatalf("graph has %d edges, want 1", len(graph.Edges))
	}

	edge := graph.Edges[0]
	if edge.From != graph.Nodes[0].ID {
		t.Errorf("edge from = %q, want %q", edge.From, graph.Nodes[0].ID)
	}
	if edge.To != graph.Nodes[1].ID {
		t.Errorf("edge to = %q, want %q", edge.To, graph.Nodes[1].ID)
	}
}

func TestWrapProviderInPlanningReceiver_OptionalParams(t *testing.T) {
	reg := op.NewActionRegistry()
	reg.Register(&stubWriteAction{})

	graph := &op.Graph{}
	p := WrapProviderInPlanningReceiver(&testProviderFactory{params: MethodParams{
		"Write": {"path", "content?"},
	}}, graph, "proj", reg)

	attr, _ := p.Attr("write")
	builtin := attr.(*starlark.Builtin)
	_, err := builtin.CallInternal(nil, nil, []starlark.Tuple{
		{starlark.String("path"), starlark.String("/tmp/x")},
	})
	if err != nil {
		t.Fatalf("write(path) error: %v", err)
	}

	node := graph.Nodes[0]
	path := node.GetSlot("path")
	if path != "/tmp/x" {
		t.Errorf("slot path = %v, want '/tmp/x'", path)
	}
	// content was optional and not provided — no slot.
	content := node.GetSlot("content")
	if content != nil {
		t.Errorf("slot content = %v, want nil", content)
	}
}

func TestWrapProviderInPlanningReceiver_ResolvesResourceParams(t *testing.T) {
	// Register plan-time constructor for actionResource.
	op.RegisterConstructor(func(v any) (actionResource, error) {
		s, ok := v.(string)
		if !ok {
			return actionResource{}, fmt.Errorf("expected string, got %T", v)
		}
		r := actionResource{SourcePath: s}
		r.SetURI("file://" + s)
		return r, nil
	})
	defer op.ResetResourceRegistry()

	reg := op.NewActionRegistry()
	factory := newActionFactory()
	RegisterActions(reg, factory)

	graph := op.NewGraph("test")

	// Touch takes an actionResource parameter — should trigger catalog resolution.
	scoped := newScopedFactory(MethodParams{"Touch": {"res"}})
	p := WrapProviderInPlanningReceiver(scoped, graph, "proj", reg)

	attr, _ := p.Attr("touch")
	builtin := attr.(*starlark.Builtin)
	_, err := builtin.CallInternal(nil, nil, []starlark.Tuple{
		{starlark.String("res"), starlark.String("/tmp/new-file")},
	})
	if err != nil {
		t.Fatalf("touch() error: %v", err)
	}

	// The res parameter is actionResource-typed. The plan-time constructor
	// should have created a URI-only Resource and resolved it in the catalog.
	uri := "file:///tmp/new-file"
	id := graph.Catalog.Current(uri)
	if id == "" {
		t.Errorf("catalog has no entry for %q — resolveResourceParam did not fire", uri)
	}
}

func TestWrapProviderInPlanningReceiver_SkipsPromiseResolution(t *testing.T) {
	reg := op.NewActionRegistry()
	factory := newActionFactory()
	RegisterActions(reg, factory)

	graph := op.NewGraph("test")

	scoped := newScopedFactory(MethodParams{"Create": {"path"}, "Delete": {"path"}})
	p := WrapProviderInPlanningReceiver(scoped, graph, "proj", reg)

	// First call: create returns a promise.
	createAttr, _ := p.Attr("create")
	createBuiltin := createAttr.(*starlark.Builtin)
	promise, err := createBuiltin.CallInternal(nil, nil, []starlark.Tuple{
		{starlark.String("path"), starlark.String("/tmp/x")},
	})
	if err != nil {
		t.Fatalf("create() error: %v", err)
	}

	// Second call: pass promise to delete — should NOT resolve.
	catalogBefore := graph.Catalog.Len()
	deleteAttr, _ := p.Attr("delete")
	deleteBuiltin := deleteAttr.(*starlark.Builtin)
	_, err = deleteBuiltin.CallInternal(nil, nil, []starlark.Tuple{
		{starlark.String("path"), promise},
	})
	if err != nil {
		t.Fatalf("delete() error: %v", err)
	}

	// Catalog should not have grown (promise is deferred, not resolved).
	if graph.Catalog.Len() != catalogBefore {
		t.Errorf("catalog grew from %d to %d — promise should not be resolved at plan time",
			catalogBefore, graph.Catalog.Len())
	}
}

func TestWrapProviderInPlanningReceiver_ShadowsOutputResource(t *testing.T) {
	// Register plan-time constructor for actionResource.
	op.RegisterConstructor(func(v any) (actionResource, error) {
		s, ok := v.(string)
		if !ok {
			return actionResource{}, fmt.Errorf("expected string, got %T", v)
		}
		r := actionResource{SourcePath: s}
		r.SetURI("file://" + s)
		return r, nil
	})
	defer op.ResetResourceRegistry()

	reg := op.NewActionRegistry()
	factory := newActionFactory()
	RegisterActions(reg, factory)

	graph := op.NewGraph("test")

	// Transfer takes 2 actionResource params (source, dest) and returns
	// (actionResource, map[string]any, error) — compensable.
	scoped := newScopedFactory(MethodParams{"Transfer": {"source", "dest"}})
	p := WrapProviderInPlanningReceiver(scoped, graph, "proj", reg)

	attr, _ := p.Attr("transfer")
	builtin := attr.(*starlark.Builtin)
	_, err := builtin.CallInternal(nil, nil, []starlark.Tuple{
		{starlark.String("source"), starlark.String("/tmp/src")},
		{starlark.String("dest"), starlark.String("/tmp/dst")},
	})
	if err != nil {
		t.Fatalf("transfer() error: %v", err)
	}

	// Source should be resolved (discovery entry — no originID).
	srcURI := "file:///tmp/src"
	srcID := graph.Catalog.Current(srcURI)
	if srcID == "" {
		t.Fatalf("catalog has no entry for %q", srcURI)
	}
	srcEntry, _ := graph.Catalog.Lookup(srcID)
	srcOrigin, _ := op.ExtractResource(srcEntry)
	if srcOrigin != "" {
		t.Errorf("source originID = %q, want empty (discovery entry)", srcOrigin)
	}

	// Destination should be shadowed (has originID = node ID).
	dstURI := "file:///tmp/dst"
	dstID := graph.Catalog.Current(dstURI)
	if dstID == "" {
		t.Fatalf("catalog has no entry for %q", dstURI)
	}
	dstEntry, _ := graph.Catalog.Lookup(dstID)
	dstOrigin, dstFound := op.ExtractResource(dstEntry)
	if !dstFound {
		t.Error("dest originID is empty — shadowOutputParam did not fire")
	}
	if dstOrigin != graph.Nodes[0].ID {
		t.Errorf("dest originID = %q, want node ID %q", dstOrigin, graph.Nodes[0].ID)
	}

	// Destination should NOT appear in DiscoveryURIs (it's shadowed).
	for _, uri := range graph.Catalog.DiscoveryURIs() {
		if uri == dstURI {
			t.Error("destination URI appears in DiscoveryURIs — should be shadowed, not discovered")
		}
	}
}

func TestWrapProviderInPlanningReceiver_ShadowsSingleResourceOutput(t *testing.T) {
	// Register plan-time constructor for actionResource.
	op.RegisterConstructor(func(v any) (actionResource, error) {
		s, ok := v.(string)
		if !ok {
			return actionResource{}, fmt.Errorf("expected string, got %T", v)
		}
		r := actionResource{SourcePath: s}
		r.SetURI("file://" + s)
		return r, nil
	})
	defer op.ResetResourceRegistry()

	reg := op.NewActionRegistry()
	factory := newActionFactory()
	RegisterActions(reg, factory)

	graph := op.NewGraph("test")

	// Stamp takes 1 actionResource param (dest) and returns
	// (actionResource, map[string]any, error) — compensable.
	scoped := newScopedFactory(MethodParams{"Stamp": {"dest"}})
	p := WrapProviderInPlanningReceiver(scoped, graph, "proj", reg)

	attr, _ := p.Attr("stamp")
	builtin := attr.(*starlark.Builtin)
	_, err := builtin.CallInternal(nil, nil, []starlark.Tuple{
		{starlark.String("dest"), starlark.String("/tmp/stamped")},
	})
	if err != nil {
		t.Fatalf("stamp() error: %v", err)
	}

	// Destination should be shadowed (has originID = node ID).
	dstURI := "file:///tmp/stamped"
	dstID := graph.Catalog.Current(dstURI)
	if dstID == "" {
		t.Fatalf("catalog has no entry for %q", dstURI)
	}
	dstEntry, _ := graph.Catalog.Lookup(dstID)
	dstOrigin, dstFound := op.ExtractResource(dstEntry)
	if !dstFound {
		t.Error("dest originID is empty — single-Resource output was not shadowed")
	}
	if dstOrigin != graph.Nodes[0].ID {
		t.Errorf("dest originID = %q, want node ID %q", dstOrigin, graph.Nodes[0].ID)
	}

	// Destination should NOT appear in DiscoveryURIs (it's shadowed).
	for _, uri := range graph.Catalog.DiscoveryURIs() {
		if uri == dstURI {
			t.Error("destination URI appears in DiscoveryURIs — should be shadowed, not discovered")
		}
	}
}

func TestWrapProviderInPlanningReceiver_ConflictDetection(t *testing.T) {
	// Register plan-time constructor for actionResource.
	op.RegisterConstructor(func(v any) (actionResource, error) {
		s, ok := v.(string)
		if !ok {
			return actionResource{}, fmt.Errorf("expected string, got %T", v)
		}
		r := actionResource{SourcePath: s}
		r.SetURI("file://" + s)
		return r, nil
	})
	defer op.ResetResourceRegistry()

	reg := op.NewActionRegistry()
	factory := newActionFactory()
	RegisterActions(reg, factory)

	graph := op.NewGraph("test")

	// Transfer takes 2 Resource params — the last (dest) is shadowed.
	scoped := newScopedFactory(MethodParams{"Transfer": {"source", "dest"}})
	p := WrapProviderInPlanningReceiver(scoped, graph, "proj", reg)

	attr, _ := p.Attr("transfer")
	builtin := attr.(*starlark.Builtin)

	// First transfer to /tmp/dst — should succeed.
	_, err := builtin.CallInternal(nil, nil, []starlark.Tuple{
		{starlark.String("source"), starlark.String("/tmp/src1")},
		{starlark.String("dest"), starlark.String("/tmp/dst")},
	})
	if err != nil {
		t.Fatalf("first transfer() error: %v", err)
	}

	// Second transfer to same /tmp/dst from a different node — should conflict.
	_, err = builtin.CallInternal(nil, nil, []starlark.Tuple{
		{starlark.String("source"), starlark.String("/tmp/src2")},
		{starlark.String("dest"), starlark.String("/tmp/dst")},
	})
	if err == nil {
		t.Fatal("expected conflict error for second transfer to same dest, got nil")
	}
	if !strings.Contains(err.Error(), "resource conflict") {
		t.Errorf("error = %q, want 'resource conflict' substring", err)
	}
}

func TestWrapProviderInPlanningReceiver_TypeValidation_RejectsWrongType(t *testing.T) {
	reg := op.NewActionRegistry()
	factory := newActionFactory()
	RegisterActions(reg, factory)

	graph := op.NewGraph("test")

	// Validate takes a string param "path".
	scoped := newScopedFactory(MethodParams{"Validate": {"path"}})
	p := WrapProviderInPlanningReceiver(scoped, graph, "proj", reg)

	attr, _ := p.Attr("validate")
	builtin := attr.(*starlark.Builtin)

	// Pass a dict where a string is expected — should fail at plan time.
	dict := starlark.NewDict(1)
	_ = dict.SetKey(starlark.String("bad"), starlark.True)
	_, err := builtin.CallInternal(nil, nil, []starlark.Tuple{
		{starlark.String("path"), dict},
	})
	if err == nil {
		t.Fatal("expected type validation error, got nil")
	}
	if !strings.Contains(err.Error(), "cannot coerce") {
		t.Errorf("error = %q, want 'cannot coerce' substring", err)
	}
	if !strings.Contains(err.Error(), "path") {
		t.Errorf("error = %q, want 'path' mentioned", err)
	}
}

func TestWrapProviderInPlanningReceiver_TypeValidation_AcceptsValid(t *testing.T) {
	reg := op.NewActionRegistry()
	factory := newActionFactory()
	RegisterActions(reg, factory)

	graph := op.NewGraph("test")

	// Mkdir takes (string, os.FileMode) — int is convertible to FileMode.
	scoped := newScopedFactory(MethodParams{"Mkdir": {"path", "mode"}})
	p := WrapProviderInPlanningReceiver(scoped, graph, "proj", reg)

	attr, _ := p.Attr("mkdir")
	builtin := attr.(*starlark.Builtin)

	_, err := builtin.CallInternal(nil, nil, []starlark.Tuple{
		{starlark.String("path"), starlark.String("/tmp/dir")},
		{starlark.String("mode"), starlark.MakeInt(0o755)},
	})
	if err != nil {
		t.Fatalf("mkdir with valid types failed: %v", err)
	}
}

func TestWrapProviderInPlanningReceiver_TypeValidation_AcceptsConstructable(t *testing.T) {
	op.RegisterConstructor(func(v any) (actionResource, error) {
		s, ok := v.(string)
		if !ok {
			return actionResource{}, fmt.Errorf("expected string, got %T", v)
		}
		r := actionResource{SourcePath: s}
		r.SetURI("file://" + s)
		return r, nil
	})
	defer op.ResetResourceRegistry()

	reg := op.NewActionRegistry()
	factory := newActionFactory()
	RegisterActions(reg, factory)

	graph := op.NewGraph("test")

	// Touch takes an actionResource — string should be accepted (constructor exists).
	scoped := newScopedFactory(MethodParams{"Touch": {"res"}})
	p := WrapProviderInPlanningReceiver(scoped, graph, "proj", reg)

	attr, _ := p.Attr("touch")
	builtin := attr.(*starlark.Builtin)

	_, err := builtin.CallInternal(nil, nil, []starlark.Tuple{
		{starlark.String("res"), starlark.String("/tmp/file")},
	})
	if err != nil {
		t.Fatalf("touch with constructable type failed: %v", err)
	}
}

func TestWrapProviderInPlanningReceiver_TypeValidation_SkipsPromises(t *testing.T) {
	reg := op.NewActionRegistry()
	factory := newActionFactory()
	RegisterActions(reg, factory)

	graph := op.NewGraph("test")

	scoped := newScopedFactory(MethodParams{"Create": {"path"}, "Validate": {"path"}})
	p := WrapProviderInPlanningReceiver(scoped, graph, "proj", reg)

	// Create returns a promise.
	createAttr, _ := p.Attr("create")
	createBuiltin := createAttr.(*starlark.Builtin)
	promise, err := createBuiltin.CallInternal(nil, nil, []starlark.Tuple{
		{starlark.String("path"), starlark.String("/tmp/x")},
	})
	if err != nil {
		t.Fatalf("create() error: %v", err)
	}

	// Pass promise to validate — should NOT trigger type validation.
	validateAttr, _ := p.Attr("validate")
	validateBuiltin := validateAttr.(*starlark.Builtin)
	_, err = validateBuiltin.CallInternal(nil, nil, []starlark.Tuple{
		{starlark.String("path"), promise},
	})
	if err != nil {
		t.Fatalf("validate with promise should not fail, got: %v", err)
	}
}

func TestWrapProviderInPlanningReceiver_NoSuchAttr(t *testing.T) {
	reg := op.NewActionRegistry()
	graph := &op.Graph{}
	p := WrapProviderInPlanningReceiver(&testProviderFactory{}, graph, "proj", reg)

	_, err := p.Attr("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent attr")
	}
}

func TestPlanningReceiver_StarlarkValue(t *testing.T) {
	reg := op.NewActionRegistry()
	graph := &op.Graph{}
	p := WrapProviderInPlanningReceiver(&testProviderFactory{}, graph, "proj", reg)

	if p.String() != "plan.test" {
		t.Errorf("String() = %q, want 'plan.test'", p.String())
	}
	if p.Type() != "plan.test" {
		t.Errorf("ProviderType() = %q, want 'plan.test'", p.Type())
	}
}
