// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

//go:build !windows

package file

import (
	"path/filepath"
	"testing"
)

// TestObserve_ReportsModeBits asserts Observe carries the file's permission bits through, and is unix-scoped.
//
// Windows reports 0666 from [os.FileMode] for every file regardless of its access-control list, so there is no
// mode signal there to observe and no true assertion to make. Whether a file excludes other accounts is a
// separate question with a separate instrument — see OwnerOnly — and it is not what this test is about.
func TestObserve_ReportsModeBits(t *testing.T) {

	dir := t.TempDir()
	p := testProvider(t, dir)

	path := filepath.Join(dir, "measured.txt")

	root := p.RuntimeEnvironment().Root()
	if err := root.WriteFile(root.NewPath(path), []byte("twelve bytes"), 0o640); err != nil {
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

	if observation.Mode.Perm() != 0o640 {
		t.Errorf("Observe: Mode.Perm() = %o, want %o", observation.Mode.Perm(), 0o640)
	}
}
