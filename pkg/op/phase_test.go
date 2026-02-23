// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"testing"
	"time"
)

func TestComputeDelay_BackoffNone(t *testing.T) {
	rp := &RetryPolicy{
		Backoff:      BackoffNone,
		InitialDelay: "1s",
	}
	tests := []struct {
		name    string
		attempt int
		want    time.Duration
	}{
		{"attempt_0", 0, 1 * time.Second},
		{"attempt_1", 1, 1 * time.Second},
		{"attempt_5", 5, 1 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rp.ComputeDelay(tt.attempt)
			if got != tt.want {
				t.Errorf("ComputeDelay(%d) = %v, want %v", tt.attempt, got, tt.want)
			}
		})
	}
}

func TestComputeDelay_BackoffLinear(t *testing.T) {
	rp := &RetryPolicy{
		Backoff:      BackoffLinear,
		InitialDelay: "100ms",
	}
	tests := []struct {
		name    string
		attempt int
		want    time.Duration
	}{
		{"attempt_0", 0, 100 * time.Millisecond},
		{"attempt_1", 1, 200 * time.Millisecond},
		{"attempt_2", 2, 300 * time.Millisecond},
		{"attempt_4", 4, 500 * time.Millisecond},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rp.ComputeDelay(tt.attempt)
			if got != tt.want {
				t.Errorf("ComputeDelay(%d) = %v, want %v", tt.attempt, got, tt.want)
			}
		})
	}
}

func TestComputeDelay_BackoffExponential(t *testing.T) {
	rp := &RetryPolicy{
		Backoff:      BackoffExponential,
		InitialDelay: "100ms",
	}
	tests := []struct {
		name    string
		attempt int
		want    time.Duration
	}{
		{"attempt_0", 0, 100 * time.Millisecond},  // 100ms * 2^0 = 100ms
		{"attempt_1", 1, 200 * time.Millisecond},  // 100ms * 2^1 = 200ms
		{"attempt_2", 2, 400 * time.Millisecond},  // 100ms * 2^2 = 400ms
		{"attempt_3", 3, 800 * time.Millisecond},  // 100ms * 2^3 = 800ms
		{"attempt_4", 4, 1600 * time.Millisecond}, // 100ms * 2^4 = 1600ms
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rp.ComputeDelay(tt.attempt)
			if got != tt.want {
				t.Errorf("ComputeDelay(%d) = %v, want %v", tt.attempt, got, tt.want)
			}
		})
	}
}

func TestComputeDelay_MaxDelayCap(t *testing.T) {
	rp := &RetryPolicy{
		Backoff:      BackoffExponential,
		InitialDelay: "1s",
		MaxDelay:     "5s",
	}
	tests := []struct {
		name    string
		attempt int
		want    time.Duration
	}{
		{"attempt_0_under_cap", 0, 1 * time.Second},
		{"attempt_1_under_cap", 1, 2 * time.Second},
		{"attempt_2_under_cap", 2, 4 * time.Second},
		{"attempt_3_capped", 3, 5 * time.Second},   // 8s capped to 5s
		{"attempt_4_capped", 4, 5 * time.Second},   // 16s capped to 5s
		{"attempt_10_capped", 10, 5 * time.Second}, // way over, capped
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rp.ComputeDelay(tt.attempt)
			if got != tt.want {
				t.Errorf("ComputeDelay(%d) = %v, want %v", tt.attempt, got, tt.want)
			}
		})
	}
}

func TestComputeDelay_ZeroInitialDelay(t *testing.T) {
	tests := []struct {
		name    string
		backoff BackoffStrategy
	}{
		{"none", BackoffNone},
		{"linear", BackoffLinear},
		{"exponential", BackoffExponential},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rp := &RetryPolicy{
				Backoff:      tt.backoff,
				InitialDelay: "",
			}
			got := rp.ComputeDelay(3)
			if got != 0 {
				t.Errorf("ComputeDelay with zero initial = %v, want 0", got)
			}
		})
	}
}

func TestComputeDelay_DefaultBackoff(t *testing.T) {
	rp := &RetryPolicy{
		Backoff:      "unknown_strategy",
		InitialDelay: "500ms",
	}
	// The default case in the switch returns initial delay unchanged.
	got := rp.ComputeDelay(3)
	if got != 500*time.Millisecond {
		t.Errorf("ComputeDelay with unknown backoff = %v, want 500ms", got)
	}
}

func TestParseInitialDelay(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  time.Duration
	}{
		{"valid_seconds", "5s", 5 * time.Second},
		{"valid_millis", "200ms", 200 * time.Millisecond},
		{"valid_minutes", "2m", 2 * time.Minute},
		{"empty_string", "", 0},
		{"invalid_string", "not-a-duration", 0},
		{"invalid_number_only", "42", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rp := &RetryPolicy{InitialDelay: tt.input}
			got := rp.ParseInitialDelay()
			if got != tt.want {
				t.Errorf("ParseInitialDelay(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseMaxDelay(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  time.Duration
	}{
		{"valid_seconds", "30s", 30 * time.Second},
		{"valid_millis", "500ms", 500 * time.Millisecond},
		{"empty_string", "", 0},
		{"invalid_string", "bad", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rp := &RetryPolicy{MaxDelay: tt.input}
			got := rp.ParseMaxDelay()
			if got != tt.want {
				t.Errorf("ParseMaxDelay(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
