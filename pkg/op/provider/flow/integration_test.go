// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package flow_test

import (
	"os"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/flow"

	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/flow/gen"
)

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

func TestFlowActionsRegistered(t *testing.T) {

	runtimeEnvironment := &op.RuntimeEnvironment{}

	want := []op.ActionName{
		flow.Choose,
		flow.Gather,
		flow.WaitUntil,
		flow.Complete,
		flow.Degraded,
		flow.Failed,
	}

	for _, name := range want {
		if _, err := runtimeEnvironment.ActionByName(name); err != nil {
			t.Errorf("action %q: %v", name, err)
		}
	}
}
