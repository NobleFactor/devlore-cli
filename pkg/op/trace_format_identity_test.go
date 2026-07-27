// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"encoding/json"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// formatIdentityTrace builds a trace whose variables exercise the any-typed regions where JSON and YAML
// decode differently (ints, bools, strings, lists, nested maps) — the risk surface for cross-format
// checksum identity — and stamps its checksum the way WriteTrace does at persist.
func formatIdentityTrace(t *testing.T) *Trace {

	t.Helper()

	trace := &Trace{
		GraphChecksum: "sha256:0123",
		Transitions: []RunStatusTransition{
			{
				Phase:     PhaseRunning,
				Condition: ConditionHealthy,
				At:        time.Date(2026, 7, 27, 1, 2, 3, 456789000, time.UTC),
				UnitID:    "leaf",
			},
		},
		Variables: map[string]Variable{
			"order":    {Name: "order", Value: 4, Source: VariableSource{Kind: VariableSourceKindDefault, Name: "order"}},
			"prune":    {Name: "prune", Value: true},
			"run_root": {Name: "run_root", Value: "/tmp/format-identity"},
			"projects": {Name: "projects", Value: []any{"alpha", "beta"}},
			"files":    {Name: "files", Value: map[string]any{"unit-1": map[string]any{"target": "/tmp/t", "layer": ""}}},
		},
	}

	if err := trace.StampChecksum(); err != nil {
		t.Fatalf("StampChecksum: %v", err)
	}

	return trace
}

func TestTraceChecksum_IdenticalAcrossJSONAndYAMLDocuments(t *testing.T) {

	trace := formatIdentityTrace(t)

	yamlDoc, err := yaml.Marshal(trace)
	if err != nil {
		t.Fatalf("yaml marshal: %v", err)
	}

	jsonDoc, err := json.Marshal(trace)
	if err != nil {
		t.Fatalf("json marshal: %v", err)
	}

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
	if yamlChecksum != trace.Checksum {
		t.Errorf("stored checksum %q != live trace checksum %q", yamlChecksum, trace.Checksum)
	}
}

func TestLoadTrace_YAMLDocumentVerifies_RichTrace(t *testing.T) {

	trace := formatIdentityTrace(t)

	data, err := yaml.Marshal(trace)
	if err != nil {
		t.Fatalf("yaml marshal: %v", err)
	}

	loaded, err := LoadTrace(data)
	if err != nil {
		t.Fatalf("LoadTrace(yaml): %v", err)
	}
	if loaded.Checksum != trace.Checksum {
		t.Errorf("loaded checksum %q != original %q", loaded.Checksum, trace.Checksum)
	}
}

func TestLoadTrace_JSONDocumentVerifies(t *testing.T) {

	trace := formatIdentityTrace(t)

	data, err := json.Marshal(trace)
	if err != nil {
		t.Fatalf("json marshal: %v", err)
	}

	loaded, err := LoadTrace(data)
	if err != nil {
		t.Fatalf("LoadTrace(json bytes): %v", err)
	}
	if loaded.Checksum != trace.Checksum {
		t.Errorf("loaded checksum %q != original %q", loaded.Checksum, trace.Checksum)
	}
}

func TestLoadTrace_CrossFormatIdentity(t *testing.T) {

	trace := formatIdentityTrace(t)

	yamlDoc, err := yaml.Marshal(trace)
	if err != nil {
		t.Fatalf("yaml marshal: %v", err)
	}

	jsonDoc, err := json.Marshal(trace)
	if err != nil {
		t.Fatalf("json marshal: %v", err)
	}

	fromYAML, err := LoadTrace(yamlDoc)
	if err != nil {
		t.Fatalf("LoadTrace(yaml): %v", err)
	}

	fromJSON, err := LoadTrace(jsonDoc)
	if err != nil {
		t.Fatalf("LoadTrace(json bytes): %v", err)
	}

	if fromYAML.Checksum != fromJSON.Checksum {
		t.Errorf("cross-format checksums differ: yaml-loaded %q, json-loaded %q",
			fromYAML.Checksum, fromJSON.Checksum)
	}
	if fromYAML.Checksum != trace.Checksum {
		t.Errorf("yaml-loaded checksum %q != original %q", fromYAML.Checksum, trace.Checksum)
	}
}
