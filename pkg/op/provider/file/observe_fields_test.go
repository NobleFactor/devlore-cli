// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package file

import (
	"os"
	"path/filepath"
	"testing"
)

// TestObserve_ReportsFileFields asserts the measurement fields of an Observation (Size / Mode / ModTime), beyond the
// single existing observe unit which asserts only Exists (step-52 backfill).
func TestObserve_ReportsFileFields(t *testing.T) {
	dir := t.TempDir()
	p := testProvider(t, dir)

	path := filepath.Join(dir, "measured.txt")
	content := []byte("twelve bytes")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o640); err != nil {
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
	if observation.Mode.Perm() != 0o640 {
		t.Errorf("Observe: Mode.Perm() = %o, want %o", observation.Mode.Perm(), os.FileMode(0o640))
	}
	if observation.ModTime.IsZero() {
		t.Error("Observe: ModTime is zero for a real file")
	}
}
