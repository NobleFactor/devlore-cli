// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package claimcheck_test

import (
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op/claimcheck"
)

// TestEveryClaimHolds is the gate: a `+devlore:claim=` directive that its own code contradicts fails the build.
//
// It runs over the announced providers rather than the whole module because that is where claims live, and a
// single load covers all of them — loading runs the compiler front end, so one pass is the point of doing this
// here instead of inside codegen.
func TestEveryClaimHolds(t *testing.T) {

	violations, err := claimcheck.Check("github.com/NobleFactor/devlore-cli/pkg/op/provider/...")
	if err != nil {
		t.Fatalf("claimcheck.Check: %v", err)
	}

	for _, violation := range violations {
		t.Error(violation)
	}
}
