// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package op

import (
	"bytes"
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// region Fixture

// anySlotFixture carries a method whose parameter is declared `any`, which is what #712 is about.
//
// [numberFidelityFixture] proves the complementary case for #711: a declared type tells the reader how to read
// a serialized number, so the document does not have to record it. Here no declaration exists at either end,
// so whatever the document fails to record is gone for good.
type anySlotFixture struct{ ProviderBase }

// Keep accepts a value of any type, the declared-`any` parameter these tests are built around.
//
// Parameters:
//   - `value`: the value under test; the method does nothing with it.
//
// Returns:
//   - `error`: always nil.
func (p *anySlotFixture) Keep(value any) error { return nil }

func init() {

	AnnounceProvider(reflect.TypeFor[anySlotFixture](), NewProviderFlags(SurfaceWorkflow, PlacementQualified),
		func(runtimeEnvironment *RuntimeEnvironment) (any, error) {
			return &anySlotFixture{ProviderBase: NewProviderBase(runtimeEnvironment)}, nil
		},
		map[string]MethodMetadata{
			"Keep": {ParameterNames: []string{"value"}},
		})
}

// anySlotGraph builds a one-node graph whose single `any` slot holds `value`.
//
// Parameters:
//   - `t`: the test that fails if the graph cannot be built.
//   - `value`: the value to bind into the `any` slot.
//
// Returns:
//   - `*Graph`: the one-node graph.
func anySlotGraph(t *testing.T, value any) *Graph {

	t.Helper()

	action, err := ReceiverRegistry().BuildAction("anySlotFixture.keep")
	if err != nil {
		t.Fatalf("BuildAction: %v", err)
	}

	node, err := NewNode(NewNodeSpec().WithID("keep").WithAction(action).
		WithSlot("value", NewImmediateBinding(value)))
	if err != nil {
		t.Fatalf("NewNode: %v", err)
	}

	graph, err := NewGraph(NewGraphSpec().
		WithOrigin(NewOriginBase("test", "home", NewAnnotationMap(nil))).
		WithUnits(node).
		WithTimestamp(time.Unix(1_700_000_000, 0).UTC()))
	if err != nil {
		t.Fatalf("NewGraph: %v", err)
	}

	return graph
}

// reloadedAnyValue returns the value bound to `graph`'s single node's "value" slot.
//
// Parameters:
//   - `t`: the test that fails if the graph does not have the expected shape.
//   - `graph`: the reloaded graph.
//
// Returns:
//   - `any`: the bound value.
func reloadedAnyValue(t *testing.T, graph *Graph) any {

	t.Helper()

	nodes := graph.Nodes()
	if len(nodes) != 1 {
		t.Fatalf("graph has %d nodes, want 1", len(nodes))
	}

	slots := nodes[0].ResolveSlots(nil, nil)

	value, bound := slots["value"]
	if !bound {
		t.Fatalf("node has no \"value\" slot; slots are %v", slots)
	}

	return value
}

// roundTripAnySlot saves `value` into an `any` slot and returns what reloading the document yields.
//
// Parameters:
//   - `t`: the test that fails if the round trip cannot be completed.
//   - `value`: the value to bind into the `any` slot.
//   - `format`: the document format, "json" or "yaml".
//
// Returns:
//   - `any`: the value the reloaded graph holds.
func roundTripAnySlot(t *testing.T, value any, format string) any {

	t.Helper()

	document := serializeGraph(t, anySlotGraph(t, value), format)

	loaded, err := LoadGraph(formatIdentityEnvironment(t), document, format)
	if err != nil {
		t.Fatalf("LoadGraph(%s): %v", format, err)
	}

	return reloadedAnyValue(t, loaded)
}

// endregion

// region Tests

// TestLoadGraph_AGraphWithAValueInAnAnySlotLoads establishes the precondition every value assertion needs.
//
// The canonical form a checksum is computed over is built from the RELOADED values, so a value that reloads as
// a different type changes the recomputed checksum and [LoadGraph] rejects the document for tampering. That
// makes an `any` slot's type loss a hard failure rather than a silent one, and it runs before any assertion
// about what the slot holds -- so a value test that skips this precondition is only ever proving this.
func TestLoadGraph_AGraphWithAValueInAnAnySlotLoads(t *testing.T) {

	for _, testCase := range []struct {
		name  string
		value any
	}{
		{"string", "hello"},
		{"bool", true},
		{"integer", int64(42)},
		{"float", float64(42)},
	} {
		t.Run(testCase.name, func(t *testing.T) {

			document := serializeGraph(t, anySlotGraph(t, testCase.value), "json")

			if _, err := LoadGraph(formatIdentityEnvironment(t), document, "json"); err != nil {
				t.Errorf("LoadGraph(json) with %T in an `any` slot: %v", testCase.value, err)
			}
		})
	}
}

// TestLoadGraph_FloatInAnAnySlotReloadsAsAFloat is row 1 of the #712 test plan.
//
// json.Marshal writes float64(42) as `42` -- the shortest form that round-trips a float64 -- so the document
// says nothing about the value having been a float, and no declared type exists to say it either. Finding 1.
func TestLoadGraph_FloatInAnAnySlotReloadsAsAFloat(t *testing.T) {

	got := roundTripAnySlot(t, float64(42), "json")

	if _, isFloat := got.(float64); !isFloat {
		t.Errorf("float64(42) in an `any` slot reloaded as %T(%v), want a float64", got, got)
	}
}

// TestLoadGraph_AnIntegerInAnAnySlotReloadsAsAnInteger is row 2 of the #712 test plan.
//
// The accepting half of row 1. A rule that turns every number into a float is not a fix, so the integer case
// has to be pinned down alongside it.
func TestLoadGraph_AnIntegerInAnAnySlotReloadsAsAnInteger(t *testing.T) {

	got := roundTripAnySlot(t, int64(42), "json")

	if _, isInteger := got.(int64); !isInteger {
		t.Errorf("int64(42) in an `any` slot reloaded as %T(%v), want an int64", got, got)
	}
}

// TestLoadGraph_BytesInAnAnySlotReloadAsBytes is row 3 of the #712 test plan.
//
// json.Marshal writes a `[]byte` as a base64 string, so in an `any` slot the VALUE changes and not merely its
// type: "hi" comes back "aGk=". Syntax cannot express the difference between a string and a base64 string,
// which is one of the reasons the envelope won over letting the document's shape carry the type. Finding 2.
func TestLoadGraph_BytesInAnAnySlotReloadAsBytes(t *testing.T) {

	want := []byte("hi")

	got := roundTripAnySlot(t, want, "json")

	gotBytes, isBytes := got.([]byte)
	if !isBytes || !bytes.Equal(gotBytes, want) {
		t.Errorf("[]byte(%q) in an `any` slot reloaded as %T(%v), want []byte(%q)", want, got, got, want)
	}
}

// TestLoadGraph_AResourceInAnAnySlotReloadsAsAResource is row 14 of the #712 test plan.
//
// [ResourceBase.MarshalJSON] is json.Marshal of the URI, so a Resource serializes to a bare string that an
// `any` slot cannot tell from a string the author typed. Finding 3.
func TestLoadGraph_AResourceInAnAnySlotReloadsAsAResource(t *testing.T) {

	resource, err := newConvertResource(formatIdentityEnvironment(t), "any-slot")
	if err != nil {
		t.Fatalf("newConvertResource: %v", err)
	}

	got := roundTripAnySlot(t, resource, "json")

	if _, isResource := got.(Resource); !isResource {
		t.Errorf("a Resource in an `any` slot reloaded as %T(%v), want a Resource", got, got)
	}
}

// TestSaveGraph_ANonFiniteFloatInAnAnySlotSaves is row 15 of the #712 test plan.
//
// encoding/json refuses a non-finite float outright (encode.go:572 raises UnsupportedValueError), while
// yaml.v3 writes .inf and succeeds, so one graph saves in one format and fails in the other. Finding 4.
//
// Asserted against [Graph.Serialize] rather than [serializeGraph], which fails the test on an encode error and
// so could never observe one. The failure may instead surface while building the graph, if the checksum path
// marshals first; that is the same defect reported one step earlier.
func TestSaveGraph_ANonFiniteFloatInAnAnySlotSaves(t *testing.T) {

	graph := anySlotGraph(t, math.Inf(1))

	var buffer bytes.Buffer
	if err := graph.Serialize(json.NewEncoder(&buffer)); err != nil {
		t.Errorf("Serialize(json) with +Inf in an `any` slot: %v; a string payload carries what a bare number cannot", err)
	}
}

// TestLoadGraph_AnAnySlotNeverHoldsAJSONNumber is row 4 of the #712 test plan.
//
// #713 gave the json branch decoder.UseNumber() and reads the literal against the declared type of the
// parameter it fills. An `any` parameter has no declared type, so tryParseSerializedNumber matches no case and
// convertDirect returns the json.Number untouched -- a decoder artifact loose in the runtime. Finding 5.
func TestLoadGraph_AnAnySlotNeverHoldsAJSONNumber(t *testing.T) {

	got := roundTripAnySlot(t, float64(42), "json")

	if number, isNumber := got.(json.Number); isNumber {
		t.Errorf("an `any` slot reloaded holding json.Number(%q); a decoder type must not escape the codec", number)
	}
}

// TestIsTruthy_AJSONNumberZeroIsFalsy is row 5 of the #712 test plan.
//
// json.Number is a NAMED type whose underlying type is string, so [scalarTruthy]'s `case string:` does not
// match it and the helper reports "not a scalar". Control reaches the reflect switch, whose Kind is String,
// which no case lists, so it lands on `default: return true`. A round-tripped zero is therefore truthy where
// float64(0) is falsy, and a resumed decision node takes the wrong branch.
func TestIsTruthy_AJSONNumberZeroIsFalsy(t *testing.T) {

	if IsTruthy(json.Number("0")) {
		t.Error("IsTruthy(json.Number(\"0\")) = true, want false: a zero is falsy however it was decoded")
	}
}

// TestRecoveryStack_ALargeIntegerSurvivesAResume is row 20 of the #712 test plan.
//
// recovery_stack.go decodes with plain json.Unmarshal into `Result any`. Without UseNumber every json number
// reaching an `any` becomes a float64, which represents integers exactly only to 2^53, so the low digits of a
// large int64 are lost on the path whose whole job is restoring what already ran. Finding 6.
//
// Compared as documents rather than as values: the stack's result is not reachable through an accessor, and
// re-serializing a faithfully reloaded stack has to reproduce the bytes it came from.
func TestRecoveryStack_ALargeIntegerSurvivesAResume(t *testing.T) {

	// 2^53 + 1, the smallest positive integer a float64 cannot represent.
	const large = int64(9007199254740993)

	receipt := &ReceiptBase{}
	if err := receipt.Commit(nil, large, nil, nil); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	stack := NewRecoveryStack()
	stack.Push(receipt)

	before, err := json.Marshal(stack)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	reloaded := NewRecoveryStack()
	if err := json.Unmarshal(before, reloaded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	after, err := json.Marshal(reloaded)
	if err != nil {
		t.Fatalf("Marshal(reloaded): %v", err)
	}

	if got, want := recordedResult(t, after), recordedResult(t, before); got != want {
		t.Errorf("a result of %d reloaded as %s, want %s: a float64 cannot hold it", large, got, want)
	}
}

// recordedResult returns the literal text of the single entry's result in an encoded recovery stack.
//
// Read with UseNumber so the probe reports the digits the document actually carries rather than re-running the
// float64 conversion this test exists to detect.
//
// Parameters:
//   - `t`: the test that fails if the document does not have the expected shape.
//   - `document`: an encoded [RecoveryStack].
//
// Returns:
//   - `string`: the result's literal text.
func recordedResult(t *testing.T, document []byte) string {

	t.Helper()

	var probe struct {
		Entries []struct {
			Result json.Number `json:"result"`
		} `json:"entries"`
	}

	decoder := json.NewDecoder(bytes.NewReader(document))
	decoder.UseNumber()

	if err := decoder.Decode(&probe); err != nil {
		t.Fatalf("decode probe: %v", err)
	}

	if len(probe.Entries) != 1 {
		t.Fatalf("stack has %d entries, want 1", len(probe.Entries))
	}

	return probe.Entries[0].Result.String()
}

// TestYAMLMarshal_AnIntegralFloatEmitsNoDecimalPoint confirms a claim the plan makes, so it passes today.
//
// The plan states that yaml.Marshal(float64(42)) emits `42`, which is what makes finding 1 codec-independent
// and decides whether the write side needs fixing in both codecs or only one. An unverified claim in a design
// document is a guess, and this one is load-bearing.
func TestYAMLMarshal_AnIntegralFloatEmitsNoDecimalPoint(t *testing.T) {

	data, err := yaml.Marshal(float64(42))
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	if got := strings.TrimSpace(string(data)); got != "42" {
		t.Errorf("yaml.Marshal(float64(42)) = %q, want %q: finding 1 assumes both codecs lose the float", got, "42")
	}
}

// endregion
