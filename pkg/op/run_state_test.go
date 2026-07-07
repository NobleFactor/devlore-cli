// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"encoding/json"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestPhase_SerializesAsName(t *testing.T) {

	cases := []struct {
		phase Phase
		name  string
	}{
		{PhasePreparing, "preparing"},
		{PhaseRunning, "running"},
		{PhasePausing, "pausing"},
		{PhasePaused, "paused"},
		{PhaseStopping, "stopping"},
		{PhaseStopped, "stopped"},
		{PhaseCompleted, "completed"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {

			jsonData, err := json.Marshal(testCase.phase)
			if err != nil {
				t.Fatalf("json.Marshal: %v", err)
			}
			if got, want := string(jsonData), `"`+testCase.name+`"`; got != want {
				t.Errorf("json form = %s, want %s", got, want)
			}

			var fromJSON Phase
			if err := json.Unmarshal(jsonData, &fromJSON); err != nil {
				t.Fatalf("json.Unmarshal: %v", err)
			}
			if fromJSON != testCase.phase {
				t.Errorf("json round-trip = %v, want %v", fromJSON, testCase.phase)
			}

			yamlData, err := yaml.Marshal(testCase.phase)
			if err != nil {
				t.Fatalf("yaml.Marshal: %v", err)
			}
			if got, want := string(yamlData), testCase.name+"\n"; got != want {
				t.Errorf("yaml form = %q, want %q", got, want)
			}

			var fromYAML Phase
			if err := yaml.Unmarshal(yamlData, &fromYAML); err != nil {
				t.Fatalf("yaml.Unmarshal: %v", err)
			}
			if fromYAML != testCase.phase {
				t.Errorf("yaml round-trip = %v, want %v", fromYAML, testCase.phase)
			}
		})
	}
}

func TestState_SerializesAsName(t *testing.T) {

	cases := []struct {
		state State
		name  string
	}{
		{StateHealthy, "healthy"},
		{StateDegraded, "degraded"},
		{StateFailedExecution, "failed_execution"},
		{StateFailedCompensation, "failed_compensation"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {

			jsonData, err := json.Marshal(testCase.state)
			if err != nil {
				t.Fatalf("json.Marshal: %v", err)
			}
			if got, want := string(jsonData), `"`+testCase.name+`"`; got != want {
				t.Errorf("json form = %s, want %s", got, want)
			}

			var fromJSON State
			if err := json.Unmarshal(jsonData, &fromJSON); err != nil {
				t.Fatalf("json.Unmarshal: %v", err)
			}
			if fromJSON != testCase.state {
				t.Errorf("json round-trip = %v, want %v", fromJSON, testCase.state)
			}

			yamlData, err := yaml.Marshal(testCase.state)
			if err != nil {
				t.Fatalf("yaml.Marshal: %v", err)
			}
			if got, want := string(yamlData), testCase.name+"\n"; got != want {
				t.Errorf("yaml form = %q, want %q", got, want)
			}

			var fromYAML State
			if err := yaml.Unmarshal(yamlData, &fromYAML); err != nil {
				t.Fatalf("yaml.Unmarshal: %v", err)
			}
			if fromYAML != testCase.state {
				t.Errorf("yaml round-trip = %v, want %v", fromYAML, testCase.state)
			}
		})
	}
}

func TestRunState_SerializesAsNestedPair(t *testing.T) {

	runState := RunState{Phase: PhaseStopped, State: StateFailedCompensation}

	jsonData, err := json.Marshal(runState)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if got, want := string(jsonData), `{"phase":"stopped","state":"failed_compensation"}`; got != want {
		t.Errorf("json form = %s, want %s", got, want)
	}

	var fromJSON RunState
	if err := json.Unmarshal(jsonData, &fromJSON); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if fromJSON != runState {
		t.Errorf("json round-trip = %v, want %v", fromJSON, runState)
	}

	yamlData, err := yaml.Marshal(runState)
	if err != nil {
		t.Fatalf("yaml.Marshal: %v", err)
	}

	var fromYAML RunState
	if err := yaml.Unmarshal(yamlData, &fromYAML); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	if fromYAML != runState {
		t.Errorf("yaml round-trip = %v, want %v", fromYAML, runState)
	}
}

func TestRunState_StringRendersBothDimensions(t *testing.T) {

	runState := RunState{Phase: PhaseCompleted, State: StateDegraded}
	if got, want := runState.String(), "completed/degraded"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestPhase_UnmarshalText_UnknownName(t *testing.T) {

	var phase Phase
	if err := phase.UnmarshalText([]byte("stranded")); err == nil {
		t.Error(`UnmarshalText("stranded") returned no error; want an unknown-name error`)
	}
}

func TestState_UnmarshalText_UnknownName(t *testing.T) {

	var state State
	if err := state.UnmarshalText([]byte("failed")); err == nil {
		t.Error(`UnmarshalText("failed") returned no error; the name was split into "failed_execution"/"failed_compensation"`)
	}
}

func TestPhase_MarshalText_OutOfRange(t *testing.T) {

	if _, err := Phase(99).MarshalText(); err == nil {
		t.Error("MarshalText on an out-of-range Phase returned no error")
	}
}

func TestState_MarshalText_OutOfRange(t *testing.T) {

	if _, err := State(99).MarshalText(); err == nil {
		t.Error("MarshalText on an out-of-range State returned no error")
	}
}
