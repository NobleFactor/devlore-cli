// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package file

import (
	"path/filepath"
	"testing"
)

// TestObserve_ReportsFileFields asserts the platform-independent measurement fields of an Observation, beyond
// the single existing observe unit which asserts only Exists (step-52 backfill).
//
// Mode is covered separately in observe_fields_unix_test.go: Windows reports 0666 for every file whatever its
// access-control list, so an assertion on the mode bits there decides on nothing.
func TestObserve_ReportsFileFields(t *testing.T) {
	dir := t.TempDir()
	p := testProvider(t, dir)

	path := filepath.Join(dir, "measured.txt")
	content := []byte("twelve bytes")

	root := p.RuntimeEnvironment().Root()
	if err := root.WriteFile(root.NewPath(path), content, 0o640); err != nil {
		t.Fatal(err)
	}

	resource, err := DiscoverRegular(p.RuntimeEnvironment(), path)
	if err != nil {
		t.Fatalf("DiscoverRegular: %v", err)
	}

	observation, err := p.Observe(resource)
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}

	if !observation.Exists {
		t.Fatal("Observe: Exists = false for a real file")
	}
	if observation.Size != int64(len(content)) {
		t.Errorf("Observe: Size = %d, want %d", observation.Size, len(content))
	}
	if observation.ModTime.IsZero() {
		t.Error("Observe: ModTime is zero for a real file")
	}
}
