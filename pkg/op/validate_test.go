// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

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
func makeNode(id string, name ActionName, specs []paramSpec, slots map[string]Binding) *Node {

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
func makeBoundSubgraph(id string, name ActionName, specs []paramSpec) *Subgraph {

	spec := NewSubgraphSpec().WithID(id).WithAction(&action{name: name, method: makeMethod(specs...)})

	sg, err := NewSubgraph(spec)
	if err != nil {
		panic("makeBoundSubgraph: " + err.Error())
	}
	return sg
}

// newTestGraph constructs a sealed [*Graph] for tests with `children` rooted at the graph's root subgraph.
//
// Convenience wrapper over [NewGraph] for the common test pattern of "make a graph containing these units."
// Origin / catalog / retry / onError / frameBindings / sopsClient are all zero or nil — tests that need
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
func producerNode(t *testing.T, methodName string) *Node {

	t.Helper()

	reflectedMethod, ok := reflect.TypeFor[promiseProducerFixture]().MethodByName(methodName)
	if !ok {
		t.Fatalf("promiseProducerFixture lacks method %q", methodName)
	}

	method, err := NewMethod(&reflectedMethod, nil, nil, nil, false)
	if err != nil {
		t.Fatalf("NewMethod(%s): %v", methodName, err)
	}

	node, err := NewNode(NewNodeSpec().WithID("producer").WithAction(&action{name: "test.produce", method: method}))
	if err != nil {
		t.Fatalf("NewNode(%s): %v", methodName, err)
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
		producerNode(t, "ProduceString"),
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
		producerNode(t, "ProduceChannel"),
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
		producerNode(t, "ProduceInt"),
		makeNode("consumer", "test.consume",
			[]paramSpec{{name: "input", typ: reflect.TypeFor[sourceConverter]()}},
			map[string]Binding{"input": NewPromiseBinding("producer")},
		),
	)

	if err := ValidateGraph(g); err != nil {
		t.Errorf("ValidateGraph = %v, want nil (the symmetric probe accepts a reverse-only conversion path)", err)
	}
}

// TestValidateGraph_CheckPromiseTypes_MissingProducer pins the one lookup failure the pass reports rather than
// skips: a promise edge naming a unit absent from the graph is a structural violation in its own right.
func TestValidateGraph_CheckPromiseTypes_MissingProducer(t *testing.T) {

	g := newTestGraph(t,
		makeNode("consumer", "test.consume",
			[]paramSpec{{name: "input", typ: reflect.TypeFor[string]()}},
			map[string]Binding{"input": NewPromiseBinding("ghost")},
		),
	)

	err := ValidateGraph(g)
	if err == nil {
		t.Fatal("expected the missing-producer violation; got nil")
	}
	for _, want := range []string{`"consumer"`, `"input"`, `"ghost"`, "not found"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err.Error(), want)
		}
	}
}

// TestValidateGraph_CheckPromiseTypes_NoMethod pins the skip-silently contract of THIS pass, exercised directly:
// a producer whose action carries no method cannot declare a result type, so checkPromiseTypes adds nothing —
// the structural complaint ("action carries no method") belongs to the required-params pass, which is exactly
// why the type pass must stay quiet about it.
func TestValidateGraph_CheckPromiseTypes_NoMethod(t *testing.T) {

	producer, err := NewNode(NewNodeSpec().WithID("producer").WithAction(&action{name: "test.produce"}))
	if err != nil {
		t.Fatalf("NewNode: %v", err)
	}

	g := newTestGraph(t,
		producer,
		makeNode("consumer", "test.consume",
			[]paramSpec{{name: "input", typ: reflect.TypeFor[string]()}},
			map[string]Binding{"input": NewPromiseBinding("producer")},
		),
	)

	if violations := checkPromiseTypes(nil, g); len(violations) != 0 {
		t.Errorf("checkPromiseTypes = %v, want none (a method-less producer skips this pass silently)", violations)
	}
}

// TestValidateGraph_CheckPromiseTypes_NoParameter pins the other silent skip: a slot name with no matching
// declared parameter is a frame binding, not a type-check subject.
func TestValidateGraph_CheckPromiseTypes_NoParameter(t *testing.T) {

	g := newTestGraph(t,
		producerNode(t, "ProduceChannel"),
		makeNode("consumer", "test.consume",
			[]paramSpec{{name: "input", typ: reflect.TypeFor[string](), optional: true}},
			map[string]Binding{"frame_only": NewPromiseBinding("producer")},
		),
	)

	if err := ValidateGraph(g); err != nil {
		t.Errorf("ValidateGraph = %v, want nil (an unmatched slot name is a frame binding, not a parameter)", err)
	}
}

// TestSubgraph_MergeBubbled_Convertible pins the direct merge contract: interconvertible duplicate declarations
// coexist without error, and a resource-typed candidate does not displace an existing source-side type.
func TestSubgraph_MergeBubbled_Convertible(t *testing.T) {

	sg := makeBoundSubgraph("sg", "test.subgraph", nil)
	seen := map[string]Parameter{}

	if err := sg.mergeBubbled(seen, Parameter{Name: "path", Type: reflect.TypeFor[string]()}); err != nil {
		t.Fatalf("first merge: %v", err)
	}
	if err := sg.mergeBubbled(seen, Parameter{Name: "path", Type: reflect.TypeFor[*fakeResource]()}); err != nil {
		t.Fatalf("convertible merge: %v", err)
	}
	if seen["path"].Type != reflect.TypeFor[string]() {
		t.Errorf("merged type = %v, want string (the source-side type stands)", seen["path"].Type)
	}
}

// TestSubgraph_MergeBubbled_PreferSourceSide pins the selection rule: when the existing entry is the framework
// abstraction and the candidate is the source-side primitive, the primitive wins.
func TestSubgraph_MergeBubbled_PreferSourceSide(t *testing.T) {

	sg := makeBoundSubgraph("sg", "test.subgraph", nil)
	seen := map[string]Parameter{"path": {Name: "path", Type: reflect.TypeFor[*fakeResource]()}}

	if err := sg.mergeBubbled(seen, Parameter{Name: "path", Type: reflect.TypeFor[string]()}); err != nil {
		t.Fatalf("merge: %v", err)
	}
	if seen["path"].Type != reflect.TypeFor[string]() {
		t.Errorf("merged type = %v, want string (prefer the source side)", seen["path"].Type)
	}
}

// TestSubgraph_MergeBubbled_IrreconcilableTypes pins the error contract: types with no conversion bridge refuse
// to merge, naming the variable and both types, and the seen map is not mutated.
func TestSubgraph_MergeBubbled_IrreconcilableTypes(t *testing.T) {

	sg := makeBoundSubgraph("sg", "test.subgraph", nil)
	seen := map[string]Parameter{"path": {Name: "path", Type: reflect.TypeFor[chan int]()}}

	err := sg.mergeBubbled(seen, Parameter{Name: "path", Type: reflect.TypeFor[string]()})
	if err == nil {
		t.Fatal("expected the irreconcilable-types error; got nil")
	}
	for _, want := range []string{`"path"`, "chan int", "string", "incompatible"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err.Error(), want)
		}
	}
	if seen["path"].Type != reflect.TypeFor[chan int]() {
		t.Errorf("seen mutated on error: %v", seen["path"].Type)
	}
}

// resultTypeFixture supplies the four return shapes [Method.ResultType] classifies; the compensable shape is
// activation-first per the step-27 required floor.
type resultTypeFixture struct{}

func (resultTypeFixture) FirstReturn() string { return "" }
func (resultTypeFixture) ErrorOnly() error    { return nil }
func (resultTypeFixture) NoOutput()           {}
func (resultTypeFixture) Compensable(*ActivationRecord) (string, *ReceiptBase, error) {
	return "", nil, nil
}

// realMethod builds a [*Method] over the named real method of resultTypeFixture — the makeMethod extension the
// step-24 charter names: signature-derived behaviors are exercisable without receiver-registry plumbing.
func realMethod(t *testing.T, name string) *Method {

	t.Helper()

	reflectedMethod, ok := reflect.TypeFor[resultTypeFixture]().MethodByName(name)
	if !ok {
		t.Fatalf("resultTypeFixture lacks method %q", name)
	}

	method, err := NewMethod(&reflectedMethod, nil, nil, nil, false)
	if err != nil {
		t.Fatalf("NewMethod(%s): %v", name, err)
	}
	return method
}

// TestMethod_ResultType_FirstReturn pins the common case: the first return's type.
func TestMethod_ResultType_FirstReturn(t *testing.T) {

	if got := realMethod(t, "FirstReturn").ResultType(); got != reflect.TypeFor[string]() {
		t.Errorf("ResultType = %v, want string", got)
	}
}

// TestMethod_ResultType_ErrorOnly pins the error-only case: no result type.
func TestMethod_ResultType_ErrorOnly(t *testing.T) {

	if got := realMethod(t, "ErrorOnly").ResultType(); got != nil {
		t.Errorf("ResultType = %v, want nil", got)
	}
}

// TestMethod_ResultType_NoOutput pins the void case: no result type.
func TestMethod_ResultType_NoOutput(t *testing.T) {

	if got := realMethod(t, "NoOutput").ResultType(); got != nil {
		t.Errorf("ResultType = %v, want nil", got)
	}
}

// TestMethod_ResultType_Compensable pins the compensable shape: the product type, not the compensator or error.
func TestMethod_ResultType_Compensable(t *testing.T) {

	if got := realMethod(t, "Compensable").ResultType(); got != reflect.TypeFor[string]() {
		t.Errorf("ResultType = %v, want string (the product, not the compensator)", got)
	}
}
