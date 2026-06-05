// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"time"
)

// RetryPolicy configures retry behavior for an executable unit.
type RetryPolicy struct {

	// MaxAttempts is the maximum number of retries (0 = no retry, fail immediately).
	MaxAttempts int `json:"max_attempts" yaml:"max_attempts"`

	// Backoff is the delay strategy: none, linear, exponential.
	Backoff BackoffStrategy `json:"backoff" yaml:"backoff"`

	// InitialDelay is the delay before the first retry (Go duration string, e.g. "1s").
	InitialDelay string `json:"initial_delay,omitempty" yaml:"initial_delay,omitempty"`

	// MaxDelay caps the delay between retries (Go duration string, e.g. "30s").
	MaxDelay string `json:"max_delay,omitempty" yaml:"max_delay,omitempty"`
}

// region EXPORTED METHODS

// region Behaviors

// ComputeDelay returns the backoff delay before the given attempt.
//
// Combines [RetryPolicy.InitialDelay] with [RetryPolicy.Backoff] (none / linear / exponential) and caps the result at
// [RetryPolicy.MaxDelay] when MaxDelay is non-zero. Returns 0 when InitialDelay is empty or unparseable.
//
// Parameters:
//   - `attempt`: the 0-based attempt number for which the delay applies.
//
// Returns:
//   - `time.Duration`: the computed delay; 0 when no delay should be applied.
func (r RetryPolicy) ComputeDelay(attempt int) time.Duration {

	initial := r.ParseInitialDelay()

	if initial == 0 {
		return 0
	}

	var delay time.Duration

	switch r.Backoff {
	case BackoffNone:
		delay = initial
	case BackoffLinear:
		delay = initial * time.Duration(attempt+1)
	case BackoffExponential:
		delay = initial
		for i := 0; i < attempt; i++ {
			delay *= 2
		}
	default:
		delay = initial
	}

	if maxDelay := r.ParseMaxDelay(); maxDelay > 0 && delay > maxDelay {
		delay = maxDelay
	}

	return delay
}

// ParseInitialDelay parses [RetryPolicy.InitialDelay] into a [time.Duration].
//
// Returns:
//   - `time.Duration`: the parsed duration, or 0 when InitialDelay is empty or unparseable.
func (r RetryPolicy) ParseInitialDelay() time.Duration {

	if r.InitialDelay == "" {
		return 0
	}

	d, err := time.ParseDuration(r.InitialDelay)
	if err != nil {
		return 0
	}

	return d
}

// ParseMaxDelay parses [RetryPolicy.MaxDelay] into a [time.Duration].
//
// Returns:
//   - `time.Duration`: the parsed duration, or 0 when MaxDelay is empty or unparseable.
func (r RetryPolicy) ParseMaxDelay() time.Duration {

	if r.MaxDelay == "" {
		return 0
	}

	d, err := time.ParseDuration(r.MaxDelay)
	if err != nil {
		return 0
	}

	return d
}

// endregion

// endregion

// BackoffStrategy defines how delays increase between retries.
type BackoffStrategy string

// BackoffStrategy constants define the available retry backoff strategies.
const (
	// BackoffNone applies no delay between retries.
	BackoffNone BackoffStrategy = "none"
	// BackoffLinear increases delay linearly between retries.
	BackoffLinear BackoffStrategy = "linear"
	// BackoffExponential doubles the delay between each retry.
	BackoffExponential BackoffStrategy = "exponential"
)
