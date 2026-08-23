// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

//go:build unix

package file

import (
	"path/filepath"
	"syscall"
	"testing"
)

// TestAny_Exists_RefusesAKindTheTaxonomyLacks pins the bounded half of the permissive predicate: an
// unasserted claim admits any taxonomy kind and ONLY a taxonomy kind. A FIFO exists on disk, but no
// variant could ever resolve to it, so a claim over one must not activate — an activation that cannot
// name what it activated to would be a false claim by construction.
//
// Unix-only: the fixture needs mkfifo, and Windows has no equivalent entry in this taxonomy.
func TestAny_Exists_RefusesAKindTheTaxonomyLacks(t *testing.T) {

	dir := t.TempDir()
	environment := testEnvironment(t, dir)

	if err := syscall.Mkfifo(filepath.Join(dir, "a-fifo"), 0o600); err != nil {
		t.Skipf("mkfifo unavailable here: %v", err)
	}

	if anyAt(t, environment, "a-fifo").Exists() {
		t.Error("Exists() = true over a FIFO; the taxonomy has no variant to resolve to")
	}
}
