// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"fmt"
	"reflect"
	"testing"

	"go.starlark.net/starlark"
)

// --- Test actions for planned mode ---

type stubWriteAction struct{}

func (a *stubWriteAction) Name() string { return "test.write" }
func (a *stubWriteAction) Do(_ *Context, _ map[string]any) (Result, UndoState, error) {
	return nil, nil, nil
}

type stubReadAction struct{}

func (a *stubReadAction) Name() string { return "test.read" }
func (a *stubReadAction) Do(_ *Context, _ map[string]any) (Result, UndoState, error) {
	return nil, nil, nil
}

type stubValidateAction struct{}

func (a *stubValidateAction) Name() string { return "test.validate" }
func (a *stubValidateAction) Do(_ *Context, _ map[string]any) (Result, UndoState, error) {
	return nil, nil, nil
}

func TestWrapPlanned_MethodFiltering(t *testing.T) {
	reg := NewActionRegistry()
	reg.Register(&stubWriteAction{})
	// Note: "test.validate" NOT registered, so validate should not appear.

	graph := &Graph{}
	providerType := reflect.TypeOf(&testProvider{})

	p := WrapPlanned("test", providerType, graph, "proj", reg, MethodParams{
		"Write":    {"path", "content"},
		"Validate": {"s"},
		"Greet":    {"name"}, // No action registered.
	})

	names := p.AttrNames()
	if len(names) != 1 || names[0] != "write" {
		t.Errorf("AttrNames() = %v, want [write]", names)
	}
}

func TestWrapPlanned_CreatesNode(t *testing.T) {
	reg := NewActionRegistry()
	reg.Register(&stubWriteAction{})

	graph := &Graph{}
	providerType := reflect.TypeOf(&testProvider{})

	p := WrapPlanned("test", providerType, graph, "myproject", reg, MethodParams{
		"Write": {"path", "content"},
	})

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

	// Result should be an Output (promise).
	output, ok := result.(*Output)
	if !ok {
		t.Fatalf("result = %T, want *Output", result)
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

func TestWrapPlanned_SlotsPopulated(t *testing.T) {
	reg := NewActionRegistry()
	reg.Register(&stubWriteAction{})

	graph := &Graph{}
	providerType := reflect.TypeOf(&testProvider{})

	p := WrapPlanned("test", providerType, graph, "proj", reg, MethodParams{
		"Write": {"path", "content"},
	})

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

func TestWrapPlanned_PromiseChaining(t *testing.T) {
	reg := NewActionRegistry()
	reg.Register(&stubWriteAction{})
	reg.Register(&stubValidateAction{})

	graph := &Graph{}
	providerType := reflect.TypeOf(&testProvider{})

	p := WrapPlanned("test", providerType, graph, "proj", reg, MethodParams{
		"Write":    {"path", "content"},
		"Validate": {"s"},
	})

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

func TestWrapPlanned_OptionalParams(t *testing.T) {
	reg := NewActionRegistry()
	reg.Register(&stubWriteAction{})

	graph := &Graph{}
	providerType := reflect.TypeOf(&testProvider{})

	p := WrapPlanned("test", providerType, graph, "proj", reg, MethodParams{
		"Write": {"path", "content?"},
	})

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

func TestWrapPlanned_Override(t *testing.T) {
	reg := NewActionRegistry()
	reg.Register(&stubWriteAction{})

	graph := &Graph{}
	providerType := reflect.TypeOf(&testProvider{})

	p := WrapPlanned("test", providerType, graph, "proj", reg, MethodParams{
		"Write": {"path", "content"},
	})

	called := false
	p.Override("write", func(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
		called = true
		return starlark.None, nil
	})

	attr, _ := p.Attr("write")
	builtin := attr.(*starlark.Builtin)
	_, _ = builtin.CallInternal(nil, nil, nil)

	if !called {
		t.Error("override was not called")
	}
}

func TestWrapPlanned_ResolvesResourceParams(t *testing.T) {
	// Register plan-time constructor for actionResource.
	RegisterPlanTimeConstructor(func(v any) (actionResource, error) {
		s, ok := v.(string)
		if !ok {
			return actionResource{}, fmt.Errorf("expected string, got %T", v)
		}
		return actionResource{SourcePath: s}, nil
	})
	defer planTimeConstructorRegistry.Delete(reflect.TypeOf(actionResource{}))

	reg := NewActionRegistry()
	RegisterReflectedActions(reg, "test", &actionProvider{}, actionParams)

	graph := NewGraph("test")
	providerType := reflect.TypeOf(&actionProvider{})

	// Touch takes an actionResource parameter — should trigger catalog resolution.
	p := WrapPlanned("test", providerType, graph, "proj", reg, MethodParams{
		"Touch": {"res"},
	})

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

func TestWrapPlanned_SkipsPromiseResolution(t *testing.T) {
	reg := NewActionRegistry()
	RegisterReflectedActions(reg, "test", &actionProvider{}, actionParams)

	graph := NewGraph("test")
	providerType := reflect.TypeOf(&actionProvider{})

	p := WrapPlanned("test", providerType, graph, "proj", reg, MethodParams{
		"Create": {"path"},
		"Delete": {"path"},
	})

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

func TestWrapPlanned_NoSuchAttr(t *testing.T) {
	reg := NewActionRegistry()
	graph := &Graph{}
	providerType := reflect.TypeOf(&testProvider{})

	p := WrapPlanned("test", providerType, graph, "proj", reg, MethodParams{})

	_, err := p.Attr("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent attr")
	}
}

func TestReflectedPlanned_StarlarkValue(t *testing.T) {
	reg := NewActionRegistry()
	graph := &Graph{}
	providerType := reflect.TypeOf(&testProvider{})

	p := WrapPlanned("test", providerType, graph, "proj", reg, MethodParams{})

	if p.String() != "plan.test" {
		t.Errorf("String() = %q, want 'plan.test'", p.String())
	}
	if p.Type() != "plan.test" {
		t.Errorf("Type() = %q, want 'plan.test'", p.Type())
	}
}
