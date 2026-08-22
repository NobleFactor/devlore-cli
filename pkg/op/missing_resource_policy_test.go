// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package op

import (
	"encoding/json"
	"testing"
)

// The policy's document form is its canonical lowercase name in both directions, and the zero value is
// Stop — fail-safe: an unset policy can never accidentally tolerate (§3, ruled 2026-08-22).

func TestMissingResourcePolicy_RoundTripAndZeroValue(t *testing.T) {

	var unset MissingResourcePolicy
	if unset != MissingResourcePolicyStop {
		t.Fatalf("zero value = %v, want Stop — the fail-safe default", unset)
	}

	for _, tc := range []struct {
		policy MissingResourcePolicy
		name   string
	}{
		{MissingResourcePolicyStop, "stop"},
		{MissingResourcePolicyIgnore, "ignore"},
	} {
		if tc.policy.String() != tc.name {
			t.Errorf("String(%d) = %q, want %q", int(tc.policy), tc.policy.String(), tc.name)
		}

		data, err := json.Marshal(tc.policy)
		if err != nil {
			t.Fatalf("marshal %s: %v", tc.name, err)
		}
		if string(data) != `"`+tc.name+`"` {
			t.Errorf("document form = %s, want %q — never a bare ordinal", data, tc.name)
		}

		var back MissingResourcePolicy
		if err := json.Unmarshal(data, &back); err != nil {
			t.Fatalf("unmarshal %s: %v", data, err)
		}
		if back != tc.policy {
			t.Errorf("round trip %s = %v, want %v", tc.name, back, tc.policy)
		}
	}

	var invalid MissingResourcePolicy
	if err := invalid.UnmarshalText([]byte("shrug")); err == nil {
		t.Error("unknown policy name accepted — must refuse")
	}

	// Skip was considered and dropped (ruled 2026-08-22); the name refuses like any other stranger.
	if err := invalid.UnmarshalText([]byte("skip")); err == nil {
		t.Error(`"skip" accepted — the variant was dropped and must refuse`)
	}
}
