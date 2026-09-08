// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package op

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/NobleFactor/devlore-cli/pkg/application"
)

// formatIdentityGraph builds a graph whose origin annotations exercise the any-typed regions where JSON and
// YAML decode differently (ints, bools, strings, lists, nested maps) — the risk surface for cross-format
// checksum identity.
func formatIdentityGraph(t *testing.T, conceivedAt time.Time) *Graph {

	t.Helper()

	registry := ReceiverRegistry()

	completeAction, err := registry.BuildAction("flow.complete")
	if err != nil {
		t.Fatalf("BuildAction(flow.complete): %v", err)
	}

	leaf, err := NewNode(NewNodeSpec().WithID("leaf").WithAction(completeAction))
	if err != nil {
		t.Fatalf("NewNode(leaf): %v", err)
	}

	origin := NewOriginBase("writ", "home", NewAnnotationMap(map[string]any{
		"run_root": "/tmp/format-identity",
		"order":    4,
		"prune":    true,
		"projects": []any{"alpha", "beta"},
		"files": map[string]any{
			"unit-1": map[string]any{"target": "/tmp/t", "layer": ""},
		},
	}))

	graph, err := NewGraph(NewGraphSpec().WithOrigin(origin).WithUnits(leaf).WithTimestamp(conceivedAt))
	if err != nil {
		t.Fatalf("NewGraph: %v", err)
	}

	return graph
}

// serializeGraph saves the graph through [Graph.Serialize] with the given encoder factory and returns the
// document bytes — the same save surface WriteGraph and dry-run output use.
func serializeGraph(t *testing.T, graph *Graph, format string) []byte {

	t.Helper()

	var buf bytes.Buffer

	var encoder Encoder
	switch format {
	case "json":
		encoder = json.NewEncoder(&buf)
	case "yaml":
		encoder = yaml.NewEncoder(&buf)
	default:
		t.Fatalf("unsupported format %q", format)
	}

	if err := graph.Serialize(encoder); err != nil {
		t.Fatalf("Serialize(%s): %v", format, err)
	}

	return buf.Bytes()
}

// formatIdentityEnvironment builds the loading environment LoadGraph requires.
func formatIdentityEnvironment(t *testing.T) *RuntimeEnvironment {

	t.Helper()

	runtimeEnvironment, err := NewRuntimeEnvironment(context.Background(),
		NewRuntimeEnvironmentSpec("test").WithApplication(&application.Application{Name: "test"}))
	if err != nil {
		t.Fatalf("NewRuntimeEnvironment: %v", err)
	}

	return runtimeEnvironment
}

func TestGraphChecksum_IdenticalAcrossJSONAndYAMLDocuments(t *testing.T) {

	graph := formatIdentityGraph(t, time.Unix(1_700_000_000, 0).UTC())

	yamlDoc := serializeGraph(t, graph, "yaml")
	jsonDoc := serializeGraph(t, graph, "json")

	var fromYAML map[string]any
	if err := yaml.Unmarshal(yamlDoc, &fromYAML); err != nil {
		t.Fatalf("yaml decode: %v", err)
	}

	var fromJSON map[string]any
	if err := json.Unmarshal(jsonDoc, &fromJSON); err != nil {
		t.Fatalf("json decode: %v", err)
	}

	yamlChecksum, _ := fromYAML["checksum"].(string)
	jsonChecksum, _ := fromJSON["checksum"].(string)

	if yamlChecksum == "" || jsonChecksum == "" {
		t.Fatalf("stored checksums: yaml %q, json %q — want both non-empty", yamlChecksum, jsonChecksum)
	}
	if yamlChecksum != jsonChecksum {
		t.Errorf("stored checksums differ: yaml %q, json %q", yamlChecksum, jsonChecksum)
	}
	if yamlChecksum != graph.Checksum() {
		t.Errorf("stored checksum %q != live graph checksum %q", yamlChecksum, graph.Checksum())
	}
}

func TestLoadGraph_YAMLDocumentVerifies(t *testing.T) {

	graph := formatIdentityGraph(t, time.Unix(1_700_000_000, 0).UTC())

	loaded, err := LoadGraph(formatIdentityEnvironment(t), serializeGraph(t, graph, "yaml"), "yaml")
	if err != nil {
		t.Fatalf("LoadGraph(yaml): %v", err)
	}

	if loaded.Checksum() != graph.Checksum() {
		t.Errorf("loaded checksum %q != original %q", loaded.Checksum(), graph.Checksum())
	}
}

func TestLoadGraph_JSONDocumentVerifies(t *testing.T) {

	graph := formatIdentityGraph(t, time.Unix(1_700_000_000, 0).UTC())

	loaded, err := LoadGraph(formatIdentityEnvironment(t), serializeGraph(t, graph, "json"), "json")
	if err != nil {
		t.Fatalf("LoadGraph(json): %v", err)
	}

	if loaded.Checksum() != graph.Checksum() {
		t.Errorf("loaded checksum %q != original %q", loaded.Checksum(), graph.Checksum())
	}
}

func TestLoadGraph_CrossFormatIdentity(t *testing.T) {

	graph := formatIdentityGraph(t, time.Unix(1_700_000_000, 0).UTC())
	environment := formatIdentityEnvironment(t)

	fromYAML, err := LoadGraph(environment, serializeGraph(t, graph, "yaml"), "yaml")
	if err != nil {
		t.Fatalf("LoadGraph(yaml): %v", err)
	}

	fromJSON, err := LoadGraph(environment, serializeGraph(t, graph, "json"), "json")
	if err != nil {
		t.Fatalf("LoadGraph(json): %v", err)
	}

	if fromYAML.Checksum() != fromJSON.Checksum() {
		t.Errorf("cross-format checksums differ: yaml-loaded %q, json-loaded %q",
			fromYAML.Checksum(), fromJSON.Checksum())
	}
	if fromYAML.Checksum() != graph.Checksum() {
		t.Errorf("yaml-loaded checksum %q != original %q", fromYAML.Checksum(), graph.Checksum())
	}
}

// TestGraphChecksum_IdenticalAcrossConstructionTimes pins the temporal half of graph identity.
//
// Its sibling above pins the format half: the same graph saved as JSON or YAML checksums the same. This pins the
// other axis — when a graph was built does not change what it is. Both serve the guarantee 2.4 states, that
// identical inputs produce an identical graph on any machine every time, and the timestamp broke it:
// canonicalGraph carried the build time, so no two runs of an identical plan ever agreed (#690).
//
// The two timestamps are supplied and deliberately far apart, so the test varies exactly the thing it is about
// rather than hoping two constructions land in different seconds.
func TestGraphChecksum_IdenticalAcrossConstructionTimes(t *testing.T) {

	first := formatIdentityGraph(t, time.Unix(1_700_000_000, 0).UTC())
	second := formatIdentityGraph(t, time.Unix(1_900_000_000, 0).UTC())

	if first.Timestamp().Equal(second.Timestamp()) {
		t.Fatalf("both graphs report %s; the test cannot distinguish identity from coincidence", first.Timestamp())
	}

	if first.Checksum() != second.Checksum() {
		t.Errorf("checksums differ across construction times:\n  first  %s at %s\n  second %s at %s\n"+
			"a graph's identity is its content, and when it was built is not content",
			first.Checksum(), first.Timestamp().Format(time.RFC3339),
			second.Checksum(), second.Timestamp().Format(time.RFC3339))
	}
}

// TestLoadGraph_OriginAnnotationsReloadTyped pins #712 phase 3, item 3, at the origin seam: every annotation value
// carries its type in the document, so a reload from either codec hands back the value the plan authored -- an
// integer as an int64, never a float64, and containers as the document's shapes.
func TestLoadGraph_OriginAnnotationsReloadTyped(t *testing.T) {

	graph := formatIdentityGraph(t, time.Unix(1_700_000_000, 0).UTC())
	for _, format := range []string{"json", "yaml"} {
		t.Run(format, func(t *testing.T) {
			loaded, err := LoadGraph(formatIdentityEnvironment(t), serializeGraph(t, graph, format), format)
			if err != nil {
				t.Fatalf("LoadGraph(%s): %v", format, err)
			}
			annotations := loaded.Origin().Annotations()
			for name, want := range map[string]any{
				"run_root": "/tmp/format-identity",
				"order":    int64(4),
				"prune":    true,
				"projects": []any{"alpha", "beta"},
				"files":    map[string]any{"unit-1": map[string]any{"target": "/tmp/t", "layer": ""}},
			} {
				got, ok := annotations.Get(name)
				if !ok {
					t.Fatalf("annotation %q is missing after reload", name)
				}
				if !reflect.DeepEqual(got, want) {
					t.Errorf("annotation %q = %#v (%T); want %#v (%T)", name, got, got, want, want)
				}
			}
		})
	}
}

// TestLoadGraph_ABareAnnotationIsRefused pins the strictness: an annotation has no declared type, so a value that
// does not carry its own is a load error naming the key -- raised at decode, before the checksum is even consulted.
func TestLoadGraph_ABareAnnotationIsRefused(t *testing.T) {

	document := serializeGraph(t, formatIdentityGraph(t, time.Unix(1_700_000_000, 0).UTC()), "json")
	enveloped := []byte(`{"$string":"/tmp/format-identity"}`)
	if !bytes.Contains(document, enveloped) {
		t.Fatalf("the document does not carry the enveloped run_root:\n%s", document)
	}
	doctored := bytes.Replace(document, enveloped, []byte(`"/tmp/format-identity"`), 1)

	_, err := LoadGraph(formatIdentityEnvironment(t), doctored, "json")
	if err == nil || !strings.Contains(err.Error(), `annotation "run_root"`) {
		t.Fatalf("LoadGraph(bare annotation) error = %v; want a refusal naming run_root", err)
	}
}
