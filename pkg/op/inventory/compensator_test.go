// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package inventory

import (
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/file"
)

// TestCompensatingActionIndex_ResolvesRegisteredActions verifies the registry's compensating-action-name index resolves
// a provider's Compensate* method by its dotted name (provider name + snake method name), and that a forward action
// name does not resolve as a compensating action.
//
// The index underpins the file-mutation compensation seam (phase-8 step 31): a receipt's compensatingAction names its
// own undo and resolves through [op.ReceiverRegistry].CompensatingActionByName. The test lives in the inventory package
// because it needs every provider gen package blank-imported so the registry's action list is populated — pkg/op itself
// cannot import the providers (they depend on op).
func TestCompensatingActionIndex_ResolvesRegisteredActions(t *testing.T) {

	registry := op.ReceiverRegistry()

	if _, ok := registry.CompensatingActionByName("file.compensate_file_mutation"); !ok {
		t.Error(`CompensatingActionByName("file.compensate_file_mutation") = false; want CompensateFileMutation indexed`)
	}

	if _, ok := registry.CompensatingActionByName("file.compensate_walk_tree"); !ok {
		t.Error(`CompensatingActionByName("file.compensate_walk_tree") = false; want CompensateWalkTree indexed`)
	}

	if _, ok := registry.CompensatingActionByName(string(file.WriteText)); ok {
		t.Error(`CompensatingActionByName("file.write_text") = true; a forward action is not a compensating action`)
	}

	if _, ok := registry.CompensatingActionByName("file.compensate_nonexistent"); ok {
		t.Error(`CompensatingActionByName("file.compensate_nonexistent") = true; want false for an unregistered name`)
	}
}
