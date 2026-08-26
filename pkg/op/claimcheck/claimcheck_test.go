// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package claimcheck_test

import (
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op/claimcheck"
)

// TestEveryClaimHolds is the gate: a `+devlore:claim=` directive that its own code contradicts fails the build.
//
// It loads the whole module, not just the providers: claims live on provider methods, but a claiming method may
// call an in-module helper, and the helper's body has to be readable for propagation to follow it. One load
// covers everything — loading runs the compiler front end, which is the point of doing this once here rather
// than once per provider inside codegen.
func TestEveryClaimHolds(t *testing.T) {

	violations, err := claimcheck.Check("github.com/NobleFactor/devlore-cli/...")
	if err != nil {
		t.Fatalf("claimcheck.Check: %v", err)
	}

	for _, violation := range violations {
		t.Error(violation)
	}
}
