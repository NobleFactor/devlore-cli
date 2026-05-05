// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package status

import "errors"

// Discard is a silent [Sink] implementation. Every emission method discards its argument; [Discard.Fail]
// still returns a non-nil error wrapping the message so callers can propagate the failure.
//
// Discard is the default [Sink] for unconfigured runtime environment specs and for tests that don't
// assert on output. It carries no state and is trivially safe to share across goroutines.
type Discard struct{}

// Compile-time interface guard.
var _ Sink = Discard{}

// region EXPORTED METHODS

// region Behaviors

// Note discards msg.
func (Discard) Note(_ string) {}

// Warn discards msg.
func (Discard) Warn(_ string) {}

// Error discards msg.
func (Discard) Error(_ string) {}

// Fail discards the emission but still returns a non-nil error wrapping msg.
func (Discard) Fail(msg string) error { return errors.New(msg) }

// Print discards the msg.
func (Discard) Print(_ string) {}

// Succeed discards msg.
func (Discard) Succeed(_ string) {}

// endregion

// endregion
