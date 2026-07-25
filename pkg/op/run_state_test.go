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

func TestCondition_SerializesAsName(t *testing.T) {

	cases := []struct {
		condition Condition
		name      string
	}{
		{ConditionHealthy, "healthy"},
		{ConditionDegraded, "degraded"},
		{ConditionExecutionFailed, "execution_failed"},
		{ConditionCompensationFailed, "compensation_failed"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {

			jsonData, err := json.Marshal(testCase.condition)
			if err != nil {
				t.Fatalf("json.Marshal: %v", err)
			}
			if got, want := string(jsonData), `"`+testCase.name+`"`; got != want {
				t.Errorf("json form = %s, want %s", got, want)
			}

			var fromJSON Condition
			if err := json.Unmarshal(jsonData, &fromJSON); err != nil {
				t.Fatalf("json.Unmarshal: %v", err)
			}
			if fromJSON != testCase.condition {
				t.Errorf("json round-trip = %v, want %v", fromJSON, testCase.condition)
			}

			yamlData, err := yaml.Marshal(testCase.condition)
			if err != nil {
				t.Fatalf("yaml.Marshal: %v", err)
			}
			if got, want := string(yamlData), testCase.name+"\n"; got != want {
				t.Errorf("yaml form = %q, want %q", got, want)
			}

			var fromYAML Condition
			if err := yaml.Unmarshal(yamlData, &fromYAML); err != nil {
				t.Fatalf("yaml.Unmarshal: %v", err)
			}
			if fromYAML != testCase.condition {
				t.Errorf("yaml round-trip = %v, want %v", fromYAML, testCase.condition)
			}
		})
	}
}

func TestRunStatus_SerializesAsNestedQuartet(t *testing.T) {

	runStatus := RunStatus{
		Phase:     PhaseStopped,
		Condition: ConditionCompensationFailed,
		Reason:    ReasonCompensationFailed,
		Message:   "unwind failed: compensation error",
	}

	jsonData, err := json.Marshal(runStatus)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	want := `{"phase":"stopped","condition":"compensation_failed","reason":"compensation_failed","message":"unwind failed: compensation error"}`
	if got := string(jsonData); got != want {
		t.Errorf("json form = %s, want %s", got, want)
	}

	var fromJSON RunStatus
	if err := json.Unmarshal(jsonData, &fromJSON); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if fromJSON != runStatus {
		t.Errorf("json round-trip = %v, want %v", fromJSON, runStatus)
	}

	yamlData, err := yaml.Marshal(runStatus)
	if err != nil {
		t.Fatalf("yaml.Marshal: %v", err)
	}

	var fromYAML RunStatus
	if err := yaml.Unmarshal(yamlData, &fromYAML); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	if fromYAML != runStatus {
		t.Errorf("yaml round-trip = %v, want %v", fromYAML, runStatus)
	}
}

func TestRunStatus_OmitsEmptyReason(t *testing.T) {

	jsonData, err := json.Marshal(RunStatus{Phase: PhaseCompleted, Condition: ConditionHealthy})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if got, want := string(jsonData), `{"phase":"completed","condition":"healthy"}`; got != want {
		t.Errorf("json form = %s, want %s", got, want)
	}
}

func TestRunStatus_String(t *testing.T) {

	cases := []struct {
		status RunStatus
		want   string
	}{
		{RunStatus{Phase: PhaseRunning, Condition: ConditionHealthy}, "running/healthy"},
		{RunStatus{Phase: PhaseCompleted, Condition: ConditionDegraded}, "completed/degraded"},
		{
			RunStatus{Phase: PhaseStopped, Condition: ConditionCompensationFailed, Reason: ReasonCompensationFailed, Message: "unwind failed"},
			"stopped/compensation_failed: unwind failed",
		},
	}

	for _, testCase := range cases {
		if got := testCase.status.String(); got != testCase.want {
			t.Errorf("String() = %q, want %q", got, testCase.want)
		}
	}
}

func TestPhase_UnmarshalText_UnknownName(t *testing.T) {

	var phase Phase
	if err := phase.UnmarshalText([]byte("stranded")); err == nil {
		t.Error(`UnmarshalText("stranded") returned no error; want an unknown-name error`)
	}
}

func TestCondition_UnmarshalText_UnknownName(t *testing.T) {

	var condition Condition
	if err := condition.UnmarshalText([]byte("failed")); err == nil {
		t.Error(`UnmarshalText("failed") returned no error; the name was split into "execution_failed"/"compensation_failed"`)
	}
}

func TestPhase_MarshalText_OutOfRange(t *testing.T) {

	if _, err := Phase(99).MarshalText(); err == nil {
		t.Error("MarshalText on an out-of-range Phase returned no error")
	}
}

func TestCondition_MarshalText_OutOfRange(t *testing.T) {

	if _, err := Condition(99).MarshalText(); err == nil {
		t.Error("MarshalText on an out-of-range Condition returned no error")
	}
}
