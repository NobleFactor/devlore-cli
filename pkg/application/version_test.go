// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package application

import "testing"

// --- IsPrerelease ---

// TestIsPrerelease_OnlyTrueIsTrue pins the parse. The linker sets strings, so the switch is stamped as a word and
// read here: `true` is a pre-release, and anything else is not -- `false`, the empty string of a local build, and a
// spelling the build never writes.
func TestIsPrerelease_OnlyTrueIsTrue(t *testing.T) {

	saved := Prerelease
	t.Cleanup(func() { Prerelease = saved })

	cases := []struct {
		stamp string
		want  bool
	}{
		{"true", true},
		{"false", false},
		{"", false},
		{"TRUE", false},
		{"1", false},
		{" true", false},
	}

	for _, testCase := range cases {
		Prerelease = testCase.stamp
		if got := IsPrerelease(); got != testCase.want {
			t.Errorf("Prerelease = %q: IsPrerelease() = %t, want %t", testCase.stamp, got, testCase.want)
		}
	}
}

// --- Channel ---

// TestChannel_UnstampedHasNone pins the default: a build the Makefile did not stamp -- `go test` among them -- has no
// channel, which is what a local build reports and what `self upgrade` refuses to guess at.
func TestChannel_UnstampedHasNone(t *testing.T) {

	if Channel != "" {
		t.Errorf("Channel = %q in an unstamped build; want none", Channel)
	}
	if Prerelease != "" {
		t.Errorf("Prerelease = %q in an unstamped build; want none", Prerelease)
	}
}
