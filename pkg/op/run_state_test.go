// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"encoding/json"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestRunState_SerializesAsName(t *testing.T) {

	cases := []struct {
		state RunState
		name  string
	}{
		{RunStatePending, "pending"},
		{RunStateRunning, "running"},
		{RunStatePaused, "paused"},
		{RunStateDegraded, "degraded"},
		{RunStateCompleted, "completed"},
		{RunStateFailed, "failed"},
		{RunStateFailedCompensation, "failed_compensation"},
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

			var fromJSON RunState
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

			var fromYAML RunState
			if err := yaml.Unmarshal(yamlData, &fromYAML); err != nil {
				t.Fatalf("yaml.Unmarshal: %v", err)
			}
			if fromYAML != testCase.state {
				t.Errorf("yaml round-trip = %v, want %v", fromYAML, testCase.state)
			}
		})
	}
}

func TestRunState_UnmarshalText_UnknownName(t *testing.T) {

	var state RunState
	if err := state.UnmarshalText([]byte("stranded")); err == nil {
		t.Error(`UnmarshalText("stranded") returned no error; the name was renamed to "failed_compensation"`)
	}
}

func TestRunState_MarshalText_OutOfRange(t *testing.T) {

	if _, err := RunState(99).MarshalText(); err == nil {
		t.Error("MarshalText on an out-of-range value returned no error")
	}
}
