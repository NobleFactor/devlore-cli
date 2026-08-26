// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package main

import "testing"

// TestGateProof_DeliberateFailure fails on purpose. It exists only to prove that ruleset 21539972
// refuses a merge when a required platform check is red, and it is deleted with the throwaway
// branch it lives on. It must never reach develop.
func TestGateProof_DeliberateFailure(t *testing.T) {
	t.Fatal("deliberate failure: proving the required platform checks block a merge")
}
