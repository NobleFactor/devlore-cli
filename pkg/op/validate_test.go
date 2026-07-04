// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"reflect"
	"strings"
	"testing"
)

// region TEST FIXTURES

// methodSpec describes a synthesized [*Method]'s parameters for tests that need typed declarations
// without going through the receiver-registry plumbing. Each entry maps a parameter name to its type
// and its (Optional | Variadic | Kwargs) flags.
type paramSpec struct {
	name     string
	typ      reflect.Type
	optional bool
	variadic bool
	kwargs   bool
}

// makeMethod synthesizes a [*Method] whose [Parameters] reflects specs. The method has no `do`
// reflect.Method — these tests only consult the parameter list, never invoke.
func makeMethod(specs ...paramSpec) *Method {

	params := make([]Parameter, len(specs))
	for i, s := range specs {
		params[i] = Parameter{
			Name:     s.name,
			Type:     s.typ,
			Optional: s.optional,
			Variadic: s.variadic,
			Kwargs:   s.kwargs,
		}
	}
	return &Method{parameters: params}
}

// makeNode builds a [*Node] bound to a synthesized [Action] whose method declares the given parameter
// specs and slot fills.
//
// Parameters:
//   - `id`: the node identifier.
//   - `name`: the action name; appears in validator error messages.
//   - `specs`: declared parameters on the synthesized method.
//   - `slots`: slot fills — a (name, [Binding]) pair for each entry. The function copies them into
//     the constructed node via [Node.setSlot]. The slot name does NOT have to match a parameter name
//     (matches behavior of the production planner — unmatched slot names are frame bindings).
func makeNode(id string, name string, specs []paramSpec, slots map[string]Binding) *Node {

	n, err := NewNode(NewNodeSpec().WithID(id).WithAction(&action{name: name, method: makeMethod(specs...)}))
	if err != nil {
		panic("makeNode: " + err.Error())
	}
	for k, v := range slots {
		n.setSlot(k, v)
	}
	return n
}

// makeBoundSubgraph builds a [*Subgraph] bound to a synthesized [Action] whose method declares the
// given parameter specs and slot fills.
func makeBoundSubgraph(id string, name string, specs []paramSpec, slots map[string]Binding) *Subgraph {

	spec := NewSubgraphSpec().WithID(id).WithAction(&action{name: name, method: makeMethod(specs...)})
	for k, v := range slots {
		spec.WithSlot(k, v)
	}

	sg, err := NewSubgraph(spec)
	if err != nil {
		panic("makeBoundSubgraph: " + err.Error())
	}
	return sg
}

// newTestGraph constructs a sealed [*Graph] for tests with `children` rooted at the graph's root subgraph.
//
// Convenience wrapper over [NewGraph] for the common test pattern of "make a graph containing these units."
// Origin / catalog / retry / errorAction / frameBindings / sopsClient are all zero or nil — tests that need
// any of those configured call [NewGraph] directly.
//
// Parameters:
//   - `t`: the test handle (for Helper marking and Fatalf on construction error).
//   - `children`: the variadic ExecutableUnit children to root.
//
// Returns:
//   - `*Graph`: the constructed graph; never nil on a non-fatal return.
func newTestGraph(t *testing.T, children ...ExecutableUnit) *Graph {

	t.Helper()
	g, err := NewGraph(NewGraphSpec().WithOrigin(OriginBase{}).WithUnits(children...))
	if err != nil {
		t.Fatalf("newTestGraph: %v", err)
	}
	return g
}

// promiseProducerFixture supplies real reflected methods for the [checkPromiseTypes] tests — [Method.ResultType]
// reads the reflected signature, so a synthesized parameter-only [*Method] (makeMethod) cannot stand in for a
// producer.
type promiseProducerFixture struct{}

func (promiseProducerFixture) ProduceChannel() (chan int, error) { return nil, nil }
func (promiseProducerFixture) ProduceInt() (int, error)          { return 0, nil }
func (promiseProducerFixture) ProduceString() (string, error)    { return "", nil }

// producerNode builds a [*Node] whose action's method is the named real method on promiseProducerFixture, so its
// declared result type participates in the promise type-check.
func producerNode(t *testing.T, id, methodName string) *Node {

	t.Helper()

	reflectedMethod, ok := reflect.TypeFor[promiseProducerFixture]().MethodByName(methodName)
	if !ok {
		t.Fatalf("promiseProducerFixture lacks method %q", methodName)
	}

	method, err := NewMethod(&reflectedMethod, nil, nil, nil, false)
	if err != nil {
		t.Fatalf("NewMethod(%s): %v", methodName, err)
	}

	node, err := NewNode(NewNodeSpec().WithID(id).WithAction(&action{name: "test.produce", method: method}))
	if err != nil {
		t.Fatalf("NewNode(%s): %v", id, err)
	}

	return node
}

// endregion

func TestValidateGraph_NilGraph_NoError(t *testing.T) {

	if err := ValidateGraph(nil); err != nil {
		t.Errorf("ValidateGraph(nil) = %v, want nil", err)
	}
}

func TestValidateGraph_EmptyGraph_NoError(t *testing.T) {

	g := newTestGraph(t)
	if err := ValidateGraph(g); err != nil {
		t.Errorf("ValidateGraph(empty) = %v, want nil", err)
	}
}

func TestValidateGraph_RequiredBound_NoError(t *testing.T) {

	g := newTestGraph(t, makeNode("n", "file.copy",
		[]paramSpec{{name: "source", typ: reflect.TypeFor[string]()}},
		map[string]Binding{"source": NewImmediateBinding("/tmp/x")},
	))

	if err := ValidateGraph(g); err != nil {
		t.Errorf("ValidateGraph = %v, want nil", err)
	}
}

func TestValidateGraph_RequiredMissing_ReturnsViolation(t *testing.T) {

	g := newTestGraph(t, makeNode("copy-1", "file.copy",
		[]paramSpec{{name: "source", typ: reflect.TypeFor[string]()}},
		nil,
	))

	err := ValidateGraph(g)
	if err == nil {
		t.Fatal("expected violation; got nil")
	}
	msg := err.Error()
	for _, want := range []string{"node", `"copy-1"`, `"file.copy"`, `required parameter "source" not bound`} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q missing %q", msg, want)
		}
	}
}

func TestValidateGraph_OptionalMissing_NoError(t *testing.T) {

	g := newTestGraph(t, makeNode("n", "file.copy",
		[]paramSpec{{name: "mode", typ: reflect.TypeFor[int](), optional: true}},
		nil,
	))

	if err := ValidateGraph(g); err != nil {
		t.Errorf("ValidateGraph = %v, want nil (Optional missing is fine)", err)
	}
}

func TestValidateGraph_VariadicMissing_NoError(t *testing.T) {

	g := newTestGraph(t, makeNode("n", "thing.do",
		[]paramSpec{{name: "args", typ: reflect.TypeFor[[]any](), variadic: true}},
		nil,
	))

	if err := ValidateGraph(g); err != nil {
		t.Errorf("ValidateGraph = %v, want nil (Variadic missing is fine)", err)
	}
}

func TestValidateGraph_KwargsMissing_NoError(t *testing.T) {

	g := newTestGraph(t, makeNode("n", "thing.do",
		[]paramSpec{{name: "kwargs", typ: reflect.TypeFor[map[string]any](), kwargs: true}},
		nil,
	))

	if err := ValidateGraph(g); err != nil {
		t.Errorf("ValidateGraph = %v, want nil (Kwargs missing is fine)", err)
	}
}

func TestValidateGraph_BoundSubgraph_MissingRequired_ReturnsViolation(t *testing.T) {

	sg := makeBoundSubgraph("iter-1", "flow.gather",
		[]paramSpec{{name: "items", typ: reflect.TypeFor[[]any]()}},
		nil,
	)
	g := newTestGraph(t, sg)

	err := ValidateGraph(g)
	if err == nil {
		t.Fatal("expected violation; got nil")
	}
	msg := err.Error()
	for _, want := range []string{"subgraph", `"iter-1"`, `"flow.gather"`, `required parameter "items" not bound`} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q missing %q", msg, want)
		}
	}
}

// TestValidateGraph_UnboundContainerSubgraph_NoError removed: NewSubgraph requires a bound action —
// a resolved Action or an action name. The graph root now binds "flow.subgraph" by name (seeded by
// NewGraphSpec), so it is no longer a special unbound case; TestValidateGraph_EmptyGraph_NoError
// covers the empty-root path.

func TestValidateGraph_TypeCollision_SurfacesAsViolation(t *testing.T) {

	g := newTestGraph(t,
		makeNode("n1", "stringly",
			[]paramSpec{{name: "a", typ: reflect.TypeFor[string]()}},
			map[string]Binding{"a": NewVariableBinding("x")},
		),
		makeNode("n2", "inty",
			[]paramSpec{{name: "b", typ: reflect.TypeFor[int]()}},
			map[string]Binding{"b": NewVariableBinding("x")},
		),
	)

	err := ValidateGraph(g)
	if err == nil {
		t.Fatal("expected type-collision violation; got nil")
	}
	msg := err.Error()
	for _, want := range []string{"incompatible types", `"x"`} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q missing %q", msg, want)
		}
	}
}

func TestValidateGraph_MultipleViolations_AllJoined(t *testing.T) {

	g := newTestGraph(t,
		makeNode("missing-a", "file.copy",
			[]paramSpec{{name: "source", typ: reflect.TypeFor[string]()}},
			nil,
		),
		makeNode("missing-b", "file.move",
			[]paramSpec{{name: "target", typ: reflect.TypeFor[string]()}},
			nil,
		),
		makeBoundSubgraph("iter-c", "flow.gather",
			[]paramSpec{{name: "items", typ: reflect.TypeFor[[]any]()}},
			nil,
		),
	)

	err := ValidateGraph(g)
	if err == nil {
		t.Fatal("expected joined violations; got nil")
	}

	joined, ok := err.(interface{ Unwrap() []error })
	if !ok {
		t.Fatalf("error does not unwrap to []error; got %T", err)
	}

	parts := joined.Unwrap()
	if len(parts) != 3 {
		t.Errorf("expected 3 violations; got %d: %v", len(parts), parts)
	}

	combined := err.Error()
	for _, want := range []string{`"missing-a"`, `"missing-b"`, `"iter-c"`} {
		if !strings.Contains(combined, want) {
			t.Errorf("error %q missing %q", combined, want)
		}
	}
}

func TestValidateGraph_PromiseType_Compatible_NoError(t *testing.T) {

	g := newTestGraph(t,
		producerNode(t, "producer", "ProduceString"),
		makeNode("consumer", "test.consume",
			[]paramSpec{{name: "input", typ: reflect.TypeFor[string]()}},
			map[string]Binding{"input": NewPromiseBinding("producer")},
		),
	)

	if err := ValidateGraph(g); err != nil {
		t.Errorf("ValidateGraph = %v, want nil (string output fills a string slot)", err)
	}
}

func TestValidateGraph_PromiseType_Incompatible_ReturnsViolation(t *testing.T) {

	g := newTestGraph(t,
		producerNode(t, "producer", "ProduceChannel"),
		makeNode("consumer", "test.consume",
			[]paramSpec{{name: "input", typ: reflect.TypeFor[string]()}},
			map[string]Binding{"input": NewPromiseBinding("producer")},
		),
	)

	err := ValidateGraph(g)
	if err == nil {
		t.Fatal("expected the promise type violation; got nil")
	}
	msg := err.Error()
	for _, want := range []string{`"consumer"`, `"input"`, "cannot bind", `"producer"`, "chan int", "string"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q missing %q", msg, want)
		}
	}
}

func TestValidateGraph_PromiseType_ReverseOnlyConvertible_Passes(t *testing.T) {

	// Pins the current D8 contract: the producer's int output has no conversion path INTO the
	// sourceConverter-typed slot; only the reverse direction exists (sourceConverter.CanConvertTo(int)).
	// [typesAreInterconvertible] is symmetric by its documented contract ("or vice versa" — convert.go), so the
	// binding passes the plan-time check. A directional check would reject this binding; whether D8 wants one is
	// tracked in the step-15/16 docs.
	g := newTestGraph(t,
		producerNode(t, "producer", "ProduceInt"),
		makeNode("consumer", "test.consume",
			[]paramSpec{{name: "input", typ: reflect.TypeFor[sourceConverter]()}},
			map[string]Binding{"input": NewPromiseBinding("producer")},
		),
	)

	if err := ValidateGraph(g); err != nil {
		t.Errorf("ValidateGraph = %v, want nil (the symmetric probe accepts a reverse-only conversion path)", err)
	}
}
