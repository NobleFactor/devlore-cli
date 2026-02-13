// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlark

import (
	"fmt"

	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/internal/execution"
)

// PhaseConfig collects phase configuration from Starlark configure() hooks.
// Passed to configure(phase) as the single argument.
//
// Starlark API:
//
//	def configure(phase):
//	    phase.retry(max_attempts=3, backoff="exponential", initial_delay="1s")
type PhaseConfig struct {
	// Retry holds the retry policy configured by the script.
	Retry *execution.RetryPolicy
}

// NewPhaseConfig creates a new PhaseConfig for collecting script configuration.
func NewPhaseConfig() *PhaseConfig {
	return &PhaseConfig{}
}

// ToStarlark returns a Starlark value exposing phase.retry().
func (c *PhaseConfig) ToStarlark() starlark.Value {
	return &starlarkPhaseConfig{config: c}
}

// starlarkPhaseConfig wraps PhaseConfig for Starlark exposure.
type starlarkPhaseConfig struct {
	config *PhaseConfig
}

func (s *starlarkPhaseConfig) String() string        { return "phase" }
func (s *starlarkPhaseConfig) Type() string          { return "phase" }
func (s *starlarkPhaseConfig) Freeze()               {}
func (s *starlarkPhaseConfig) Truth() starlark.Bool  { return true }
func (s *starlarkPhaseConfig) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable: phase") }

func (s *starlarkPhaseConfig) Attr(name string) (starlark.Value, error) {
	switch name {
	case "retry":
		return starlark.NewBuiltin("phase.retry", s.retry), nil
	default:
		return nil, starlark.NoSuchAttrError(fmt.Sprintf("phase has no attribute %q", name))
	}
}

func (s *starlarkPhaseConfig) AttrNames() []string {
	return []string{"retry"}
}

// retry configures the retry policy for the phase.
//
// Usage:
//
//	phase.retry(max_attempts=3, backoff="exponential", initial_delay="1s", max_delay="30s")
func (s *starlarkPhaseConfig) retry(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var maxAttempts int
	var backoff, initialDelay, maxDelay string

	if err := starlark.UnpackArgs("retry", args, kwargs,
		"max_attempts", &maxAttempts,
		"backoff?", &backoff,
		"initial_delay?", &initialDelay,
		"max_delay?", &maxDelay,
	); err != nil {
		return nil, err
	}

	if maxAttempts < 0 {
		return nil, fmt.Errorf("retry: max_attempts must be non-negative, got %d", maxAttempts)
	}

	policy := &execution.RetryPolicy{
		MaxAttempts: maxAttempts,
	}

	if backoff != "" {
		switch backoff {
		case "none":
			policy.Backoff = execution.BackoffNone
		case "linear":
			policy.Backoff = execution.BackoffLinear
		case "exponential":
			policy.Backoff = execution.BackoffExponential
		default:
			return nil, fmt.Errorf("retry: unknown backoff %q (use none, linear, exponential)", backoff)
		}
	}

	if initialDelay != "" {
		policy.InitialDelay = initialDelay
	}
	if maxDelay != "" {
		policy.MaxDelay = maxDelay
	}

	s.config.Retry = policy
	return starlark.None, nil
}
