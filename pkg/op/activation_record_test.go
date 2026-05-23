// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"context"
	"strings"
	"testing"
)

// TestActivationRecord_DispatchChild_NotInstalled covers the error path on
// [ActivationRecord.DispatchChild] when the underlying closure has not been installed by the
// executor — i.e., a flow-method body somehow runs against an activation built outside the
// bound-subgraph dispatch path. Returning a clear error here is the contract; the alternative
// (panic on nil-dereference) would be hostile to providers.
func TestActivationRecord_DispatchChild_NotInstalled(t *testing.T) {

	activation := NewActivationRecord(nil, nil, &RuntimeEnvironment{})

	_, err := activation.DispatchChild(context.Background(), nil, NewRecoveryStack(), nil)
	if err == nil {
		t.Fatal("DispatchChild on a fresh activation: want error, got nil")
	}
	if !strings.Contains(err.Error(), "DispatchChild") {
		t.Errorf("error should mention DispatchChild: %v", err)
	}
}
