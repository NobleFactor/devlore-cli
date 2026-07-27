// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/NobleFactor/devlore-cli/pkg/application"
)

// formatIdentityGraph builds a graph whose origin annotations exercise the any-typed regions where JSON and
// YAML decode differently (ints, bools, strings, lists, nested maps) — the risk surface for cross-format
// checksum identity.
func formatIdentityGraph(t *testing.T) *Graph {

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

	graph, err := NewGraph(NewGraphSpec().WithOrigin(origin).WithUnits(leaf))
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

	return NewRuntimeEnvironment(context.Background(),
		NewRuntimeEnvironmentSpec("test").WithApplication(&application.Application{Name: "test"}))
}

func TestGraphChecksum_IdenticalAcrossJSONAndYAMLDocuments(t *testing.T) {

	graph := formatIdentityGraph(t)

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

	graph := formatIdentityGraph(t)

	loaded, err := LoadGraph(formatIdentityEnvironment(t), serializeGraph(t, graph, "yaml"), "yaml")
	if err != nil {
		t.Fatalf("LoadGraph(yaml): %v", err)
	}

	if loaded.Checksum() != graph.Checksum() {
		t.Errorf("loaded checksum %q != original %q", loaded.Checksum(), graph.Checksum())
	}
}

func TestLoadGraph_JSONDocumentVerifies(t *testing.T) {

	graph := formatIdentityGraph(t)

	loaded, err := LoadGraph(formatIdentityEnvironment(t), serializeGraph(t, graph, "json"), "json")
	if err != nil {
		t.Fatalf("LoadGraph(json): %v", err)
	}

	if loaded.Checksum() != graph.Checksum() {
		t.Errorf("loaded checksum %q != original %q", loaded.Checksum(), graph.Checksum())
	}
}

func TestLoadGraph_CrossFormatIdentity(t *testing.T) {

	graph := formatIdentityGraph(t)
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
