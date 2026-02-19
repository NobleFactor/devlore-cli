// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlark

import (
	"fmt"

	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/internal/execution"
)

// PhaseContext provides phase metadata to lifecycle scripts.
// Passed as the second call argument: def install(package, phase):
//
// Starlark API:
//
//	phase.name         # Phase name (e.g., "install", "provision")
//	phase.action       # Lifecycle action (e.g., "deploy", "remove")
//	phase.retry(max_attempts=3, backoff="exponential")
type PhaseContext struct {
	// PhaseName is the lifecycle phase (e.g., "install", "provision").
	PhaseName string

	// Action is the lifecycle action (e.g., "deploy", "remove").
	Action string

	// Retry holds the retry policy configured by the script.
	Retry *execution.RetryPolicy
}

// ToStarlark returns a Starlark value exposing phase.name, phase.action, phase.retry().
func (c *PhaseContext) ToStarlark() starlark.Value {
	return &starlarkPhaseContext{ctx: c}
}

// starlarkPhaseContext wraps PhaseContext for Starlark exposure.
type starlarkPhaseContext struct {
	ctx *PhaseContext
}

func (s *starlarkPhaseContext) String() string        { return "phase" }
func (s *starlarkPhaseContext) Type() string          { return "phase" }
func (s *starlarkPhaseContext) Freeze()               {}
func (s *starlarkPhaseContext) Truth() starlark.Bool  { return true }
func (s *starlarkPhaseContext) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable: phase") }

func (s *starlarkPhaseContext) Attr(name string) (starlark.Value, error) {
	switch name {
	case "name":
		return starlark.String(s.ctx.PhaseName), nil
	case "action":
		return starlark.String(s.ctx.Action), nil
	case "retry":
		return starlark.NewBuiltin("phase.retry", s.retry), nil
	default:
		return nil, starlark.NoSuchAttrError(fmt.Sprintf("phase has no attribute %q", name))
	}
}

func (s *starlarkPhaseContext) AttrNames() []string {
	return []string{"action", "name", "retry"}
}

// retry configures the retry policy for the phase.
//
// Usage:
//
//	phase.retry(max_attempts=3, backoff="exponential", initial_delay="1s", max_delay="30s")
func (s *starlarkPhaseContext) retry(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
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

	s.ctx.Retry = policy
	return starlark.None, nil
}
