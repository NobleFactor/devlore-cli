// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package flow_test

import (
	"os"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"

	_ "github.com/NobleFactor/devlore-cli/pkg/op/inventory"
)

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

func TestFlowActionsRegistered(t *testing.T) {

	receiverRegistry := op.NewReceiverRegistry()
	runtimeEnvironment := &op.RuntimeEnvironment{ReceiverRegistry: receiverRegistry}

	want := []string{
		"flow.choose",
		"flow.gather",
		"flow.elevate",
		"flow.wait_until",
		"flow.complete",
		"flow.degraded",
		"flow.failed",
	}

	for _, name := range want {
		if _, err := runtimeEnvironment.ActionByName(name); err != nil {
			t.Errorf("action %q: %v", name, err)
		}
	}
}
