// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package op

import (
	"encoding/json"
	"io/fs"
	"reflect"
	"testing"
	"time"
)

// region Fixture

// numberFidelityFixture carries a method whose parameter has a DECLARED numeric type, which is what #711 is
// about — the field says how to read the number, so the document does not have to.
//
// fs.FileMode is the shape that exposed this: file.mkdir(mode=0o644) reaches a uint32, and a json round trip
// was handing it a float64 that op.Convert silently truncated back. An `any` parameter would prove something
// else entirely (#712), because there no declared type exists at either end.
type numberFidelityFixture struct{ ProviderBase }

// Chmod takes a file mode, the declared-uint32 parameter this test is built around.
func (p *numberFidelityFixture) Chmod(mode fs.FileMode) error { return nil }

func init() {

	AnnounceProvider(reflect.TypeFor[numberFidelityFixture](), RoleAction,
		func(runtimeEnvironment *RuntimeEnvironment) (any, error) {
			return &numberFidelityFixture{ProviderBase: NewProviderBase(runtimeEnvironment)}, nil
		},
		map[string]MethodMetadata{
			"Chmod": {ParameterNames: []string{"mode"}},
		})
}

// numberFidelityGraph builds a one-node graph whose single argument is the integer 0o644.
func numberFidelityGraph(t *testing.T) *Graph {

	t.Helper()

	action, err := ReceiverRegistry().BuildAction("numberFidelityFixture.chmod")
	if err != nil {
		t.Fatalf("BuildAction: %v", err)
	}

	node, err := NewNode(NewNodeSpec().WithID("chmod").WithAction(action).
		WithSlot("mode", NewImmediateBinding(int64(0o644))))
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

// reloadedMode returns the value bound to the graph's single node's "mode" slot.
func reloadedMode(t *testing.T, graph *Graph) any {

	t.Helper()

	nodes := graph.Nodes()
	if len(nodes) != 1 {
		t.Fatalf("graph has %d nodes, want 1", len(nodes))
	}

	slots := nodes[0].ResolveSlots(nil, nil)

	mode, bound := slots["mode"]
	if !bound {
		t.Fatalf("node has no \"mode\" slot; slots are %v", slots)
	}

	return mode
}

// TestConvert_SerializedNumberTooLargeForItsFieldIsRefused is row 4 of the #711 test plan.
//
// Reading a number against its field means the field's RANGE is part of the answer. A value the parameter
// cannot hold has to be refused: narrowing it silently produces a plausible number nobody wrote, which is the
// same defect as the float64 this issue removes, wearing different clothes.
//
// Tested against [Convert] rather than through a document, deliberately. Reaching this path from a graph
// requires a document whose mode literal exceeds uint32, and editing a saved document invalidates its
// checksum -- so [LoadGraph] refuses it for tampering long before any parameter is read. That ordering is
// correct and worth stating: integrity is a stronger gate than range, and it runs first. It also means a
// graph-level test of this behavior would pass without ever reaching the code it claims to cover.
func TestConvert_SerializedNumberTooLargeForItsFieldIsRefused(t *testing.T) {

	// 2^33: comfortably inside int64, comfortably outside the uint32 an fs.FileMode is.
	const oversized = json.Number("8589934592")

	got, err := Convert(nil, oversized, reflect.TypeFor[fs.FileMode]())
	if err == nil {
		t.Errorf("Convert(%s -> fs.FileMode) = %#v, want an error: the field cannot hold it", oversized, got)
	}
}

// TestConvert_SerializedNumberReadsAgainstTheField covers the accepting half.
//
// A rule that refuses everything is not a rule. Each of these is a number a document legitimately carries,
// read against a field that can hold it.
func TestConvert_SerializedNumberReadsAgainstTheField(t *testing.T) {

	for _, testCase := range []struct {
		name   string
		number json.Number
		target reflect.Type
		want   any
	}{
		{"file mode", json.Number("420"), reflect.TypeFor[fs.FileMode](), fs.FileMode(0o644)},
		{"count", json.Number("7"), reflect.TypeFor[int](), 7},
		{"negative", json.Number("-3"), reflect.TypeFor[int64](), int64(-3)},
		{"ratio", json.Number("1.5"), reflect.TypeFor[float64](), 1.5},
		{"integral float stays a float", json.Number("2"), reflect.TypeFor[float64](), 2.0},
	} {
		t.Run(testCase.name, func(t *testing.T) {

			got, err := Convert(nil, testCase.number, testCase.target)
			if err != nil {
				t.Fatalf("Convert(%s -> %s): %v", testCase.number, testCase.target, err)
			}
			if !reflect.DeepEqual(got, testCase.want) {
				t.Errorf("Convert(%s -> %s) = %#v, want %#v", testCase.number, testCase.target, got, testCase.want)
			}
		})
	}
}

// endregion

// region Tests

// TestLoadGraph_JSONIntegerReloadsAsAnInteger is row 1 of the #711 test plan.
//
// json has ONE number type. Decoding into an `any` slot with plain json.Unmarshal yields float64 for every
// number, so an authored 0o644 comes back as a float and reaches an fs.FileMode parameter as one. Nothing has
// noticed because op.Convert truncated it back — which is the compensation this issue removes.
func TestLoadGraph_JSONIntegerReloadsAsAnInteger(t *testing.T) {

	document := serializeGraph(t, numberFidelityGraph(t), "json")

	loaded, err := LoadGraph(formatIdentityEnvironment(t), document, "json")
	if err != nil {
		t.Fatalf("LoadGraph(json): %v", err)
	}

	mode := reloadedMode(t, loaded)

	if _, isFloat := mode.(float64); isFloat {
		t.Errorf("mode reloaded as float64(%v); an authored integer must not become a float", mode)
	}
}

// TestLoadGraph_JSONAndYAMLReloadIdentically is row 2 of the #711 test plan.
//
// [TestGraphChecksum_IdenticalAcrossJSONAndYAMLDocuments] proves one graph produces two documents carrying
// the same STORED checksum — which it must, since both are written from the same in-memory graph. The reverse
// direction is what has never been tested: two documents, reloaded, recomputing the same checksum.
//
// That is where the codecs part company. yaml.v3 decodes an integer as an int; json yields float64. The same
// graph therefore reloads to different in-memory values depending on which codec wrote it, and the canonical
// form is computed from those values.
func TestLoadGraph_JSONAndYAMLReloadIdentically(t *testing.T) {

	graph := numberFidelityGraph(t)

	fromJSON, err := LoadGraph(formatIdentityEnvironment(t), serializeGraph(t, graph, "json"), "json")
	if err != nil {
		t.Fatalf("LoadGraph(json): %v", err)
	}

	fromYAML, err := LoadGraph(formatIdentityEnvironment(t), serializeGraph(t, graph, "yaml"), "yaml")
	if err != nil {
		t.Fatalf("LoadGraph(yaml): %v", err)
	}

	if fromJSON.Checksum() != fromYAML.Checksum() {
		t.Errorf("reloaded checksums differ by codec: json %q, yaml %q",
			fromJSON.Checksum(), fromYAML.Checksum())
	}

	jsonMode := reloadedMode(t, fromJSON)
	yamlMode := reloadedMode(t, fromYAML)

	if reflect.TypeOf(jsonMode) != reflect.TypeOf(yamlMode) {
		t.Errorf("mode reloaded as %T from json and %T from yaml; the codecs must agree",
			jsonMode, yamlMode)
	}
}

// endregion
