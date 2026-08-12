// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package op

import (
	"math/rand/v2"
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

	// Jitter enables full jitter: the backoff curve becomes a ceiling, and [RetryPolicy.ComputeDelay] draws the
	// actual wait uniformly from [0, ceiling]. It spreads a correlated retry herd across the whole window instead
	// of releasing it as a synchronized spike (the anti-thundering-herd default for concurrent combinators such as
	// gather). Off by default; the graph-default policy (policies.retry) turns it on.
	Jitter bool `json:"jitter,omitempty" yaml:"jitter,omitempty"`
}

// region EXPORTED METHODS

// region Behaviors

// ComputeDelay returns the backoff delay before the given attempt.
//
// Combines [RetryPolicy.InitialDelay] with [RetryPolicy.Backoff] (none / linear / exponential) and caps the result at
// [RetryPolicy.MaxDelay] when MaxDelay is non-zero. When [RetryPolicy.Jitter] is set, the capped value is treated as a
// ceiling and the returned delay is drawn uniformly from [0, ceiling] (full jitter) — so this is non-deterministic
// only on the jitter path; the plain backoff paths stay deterministic. Returns 0 when InitialDelay is empty or
// unparseable.
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

	// Full jitter (anti-thundering-herd): the backoff curve above is the ceiling, and the actual wait is drawn
	// uniformly from [0, ceiling]. This spreads a correlated retry herd (e.g. a gather's concurrent bodies all
	// failing against one downstream resource) across the whole window instead of releasing it as a synchronized
	// spike. Applied after the MaxDelay cap, so the cap bounds the jitter window too.
	if r.Jitter && delay > 0 {
		//nolint:gosec // G404: jitter needs no cryptographic randomness; math/rand/v2 is the right tool.
		delay = time.Duration(rand.Int64N(int64(delay) + 1))
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
