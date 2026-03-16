// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package flow

import (
	"fmt"
	"time"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// PredicateFunc is a re-evaluable condition for polling actions.
type PredicateFunc func(any) (bool, error)

// WaitUntil is an event-driven sensor — a synchronization primitive that
// pauses execution until a condition is satisfied or a timeout expires.
//
// Slots:
//   - target: any — the value to evaluate the predicate against (typically a promise)
//   - predicate: PredicateFunc — condition to evaluate
//   - timeout: string — maximum wait time (Go duration, e.g. "5m")
//   - interval: string — poll interval (Go duration, default "5s")
//
// Result: the target value when the predicate returns true.
type WaitUntil struct{}

// Name returns the dotted action name.
func (a *WaitUntil) Name() string { return "flow.wait_until" }

// Params returns nil — WaitUntil uses untyped slots.
func (a *WaitUntil) Params() []op.ParamInfo { return nil }

// Do polls the predicate at the configured interval until it returns true
// or the timeout expires.
func (a *WaitUntil) Do(ctx *op.Context, slots map[string]any) (op.Result, op.Complement, error) {
	target := slots["target"]

	pred, ok := slots["predicate"].(PredicateFunc)
	if !ok {
		return nil, nil, fmt.Errorf("wait_until: missing or invalid 'predicate' slot")
	}

	timeout, err := parseDurationSlot(slots, "timeout", 0)
	if err != nil {
		return nil, nil, fmt.Errorf("wait_until: %w", err)
	}
	if timeout == 0 {
		return nil, nil, fmt.Errorf("wait_until: 'timeout' slot is required")
	}

	interval, err := parseDurationSlot(slots, "interval", 5*time.Second)
	if err != nil {
		return nil, nil, fmt.Errorf("wait_until: %w", err)
	}

	// Evaluate immediately before entering the poll loop.
	matched, err := pred(target)
	if err != nil {
		return nil, nil, fmt.Errorf("wait_until: predicate error: %w", err)
	}
	if matched {
		return target, nil, nil
	}

	// Poll loop.
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		case <-deadline.C:
			return nil, nil, fmt.Errorf("wait_until: timeout after %s", timeout)
		case <-ticker.C:
			matched, err := pred(target)
			if err != nil {
				return nil, nil, fmt.Errorf("wait_until: predicate error: %w", err)
			}
			if matched {
				return target, nil, nil
			}
		}
	}
}

// parseDurationSlot extracts a duration from slots by name.
// Accepts string (Go duration) or time.Duration. Returns defaultVal if the
// slot is absent or nil.
func parseDurationSlot(slots map[string]any, name string, defaultVal time.Duration) (time.Duration, error) {
	v, ok := slots[name]
	if !ok || v == nil {
		return defaultVal, nil
	}
	switch val := v.(type) {
	case string:
		d, err := time.ParseDuration(val)
		if err != nil {
			return 0, fmt.Errorf("invalid %s duration %q: %w", name, val, err)
		}
		return d, nil
	case time.Duration:
		return val, nil
	default:
		return 0, fmt.Errorf("invalid %s type %T", name, v)
	}
}
