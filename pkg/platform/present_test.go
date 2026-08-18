// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package platform

import (
	"runtime"
	"testing"
)

// alwaysPresentBinary names an executable every supported host is guaranteed to carry.
//
// Returns:
//   - `string`: "cmd" on Windows, "sh" elsewhere.
func alwaysPresentBinary() string {
	if runtime.GOOS == "windows" {
		return "cmd"
	}
	return "sh"
}

// TestDriverPresentFindsAnInstalledExecutable verifies Present resolves a binary that is certainly on the PATH.
func TestDriverPresentFindsAnInstalledExecutable(t *testing.T) {

	d := newDriver(&fakeRawDriver{binary: alwaysPresentBinary()})

	if !d.Present() {
		t.Errorf("Present() = false for %q, want true", alwaysPresentBinary())
	}
}

// TestDriverPresentRejectsAMissingExecutable verifies Present reports false when nothing resolves.
func TestDriverPresentRejectsAMissingExecutable(t *testing.T) {

	d := newDriver(&fakeRawDriver{binary: "devlore-no-such-package-manager"})

	if d.Present() {
		t.Error("Present() = true for a binary that does not exist, want false")
	}
}

// TestCompositePresentReportsAnyLeaf verifies the router answers for the machine, not for one manager.
//
// A host carrying flatpak but not apt can still install natively, so one present leaf is enough.
func TestCompositePresentReportsAnyLeaf(t *testing.T) {

	tests := []struct {
		name   string
		leaves []leaf
		want   bool
	}{
		{
			name:   "no leaves at all",
			leaves: nil,
			want:   false,
		},
		{
			name:   "every leaf absent",
			leaves: []leaf{&fakeLeaf{typ: "deb"}, &fakeLeaf{typ: "rpm"}},
			want:   false,
		},
		{
			name:   "one leaf of several present",
			leaves: []leaf{&fakeLeaf{typ: "deb"}, &fakeLeaf{typ: "rpm", present: true}},
			want:   true,
		},
		{
			name:   "every leaf present",
			leaves: []leaf{&fakeLeaf{typ: "deb", present: true}, &fakeLeaf{typ: "rpm", present: true}},
			want:   true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			router := newComposite(test.leaves, nil)

			if got := router.Present(); got != test.want {
				t.Errorf("Present() = %v, want %v", got, test.want)
			}
		})
	}
}
