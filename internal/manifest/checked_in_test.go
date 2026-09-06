// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package manifest

import (
	"path/filepath"
	"testing"
)

// TestCheckedInManifests_AreValid validates every packages-manifest.yaml this repository ships under Home/
// against the validator writ runs at deploy time. The five layer manifests listed packages as bare strings
// while the schema and the validator required objects with a name field, and nothing here noticed until
// `writ deploy noblefactor` refused the team layer on a fresh machine (2026-09-04). A manifest in the
// repository that defines the format is either valid or this test says so.
func TestCheckedInManifests_AreValid(t *testing.T) {

	manifests, err := filepath.Glob(filepath.Join("..", "..", "Home", "*", "packages-manifest.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(manifests) == 0 {
		t.Fatal("no checked-in manifests found under Home/; the glob or the layout moved")
	}

	for _, path := range manifests {
		if err := Validate(path); err != nil {
			t.Errorf("%s: %v", path, err)
		}
	}
}
