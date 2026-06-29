// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package inventory

import (
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// TestCompensatorIndex_ResolvesRegisteredCompensators verifies the registry's compensator-name index resolves a
// provider's Compensate* method by its dotted name (provider name + snake method name), and that a forward action name
// does not resolve as a compensator.
//
// The index underpins the file-mutation compensation seam (phase-8 step 28): a receipt's compensatingAction names its
// own undo and resolves through [op.ReceiverRegistry].CompensatorByName. The test lives in the inventory package
// because it needs every provider gen package blank-imported so the registry's action list is populated — pkg/op itself
// cannot import the providers (they depend on op).
func TestCompensatorIndex_ResolvesRegisteredCompensators(t *testing.T) {

	registry := op.ReceiverRegistry()

	if _, ok := registry.CompensatorByName("file.compensate_file_mutation"); !ok {
		t.Error(`CompensatorByName("file.compensate_file_mutation") = false; want the file provider's CompensateFileMutation indexed`)
	}

	if _, ok := registry.CompensatorByName("file.compensate_walk_tree"); !ok {
		t.Error(`CompensatorByName("file.compensate_walk_tree") = false; want the file provider's CompensateWalkTree indexed`)
	}

	if _, ok := registry.CompensatorByName("file.write_text"); ok {
		t.Error(`CompensatorByName("file.write_text") = true; a forward action name must not resolve as a compensator`)
	}

	if _, ok := registry.CompensatorByName("file.compensate_nonexistent"); ok {
		t.Error(`CompensatorByName("file.compensate_nonexistent") = true; want false for an unregistered name`)
	}
}
