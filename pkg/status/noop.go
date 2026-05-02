// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package status

import "errors"

// NoOp is a silent [UI] implementation. Every emission method discards its argument; [NoOp.Fail]
// still returns a non-nil error wrapping the message so callers can propagate the failure.
//
// NoOp is the default [UI] for unconfigured runtime environment specs and for tests that don't
// assert on output. It carries no state and is trivially safe to share across goroutines.
type NoOp struct{}

// Compile-time interface guard.
var _ UI = NoOp{}

// region EXPORTED METHODS

// region Behaviors

// Note discards msg.
func (NoOp) Note(_ string) {}

// Warn discards msg.
func (NoOp) Warn(_ string) {}

// Error discards msg.
func (NoOp) Error(_ string) {}

// Success discards msg.
func (NoOp) Success(_ string) {}

// Fail discards the emission but still returns a non-nil error wrapping msg.
func (NoOp) Fail(msg string) error { return errors.New(msg) }

// Print discards msg.
func (NoOp) Print(_ string) {}

// endregion

// endregion
