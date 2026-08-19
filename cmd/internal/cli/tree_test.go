// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// TestOpenTree_CreatesAChainAndOpensIt verifies the compound operation: a tree several levels below anything
// that exists is created, and the returned root is usable for writes.
func TestOpenTree_CreatesAChainAndOpensIt(t *testing.T) {

	deep := filepath.Join(t.TempDir(), "a", "b", "c")

	root, err := OpenTree(deep)
	if err != nil {
		t.Fatalf("OpenTree(%s): %v", deep, err)
	}

	defer func() {
		if closeErr := root.Close(); closeErr != nil {
			t.Errorf("Close: %v", closeErr)
		}
	}()

	info, err := os.Stat(deep)
	if err != nil {
		t.Fatalf("stat after OpenTree: %v", err)
	}

	if !info.IsDir() {
		t.Errorf("OpenTree(%s) produced a non-directory", deep)
	}

	if err := root.WriteFile(root.NewPath("f.txt"), []byte("x"), 0o600); err != nil {
		t.Errorf("write through the opened root: %v", err)
	}
}

// TestOpenTree_OpensATreeThatAlreadyExists verifies creation is idempotent — the common case, since a tree is
// absent only on first use.
func TestOpenTree_OpensATreeThatAlreadyExists(t *testing.T) {

	existing := t.TempDir()

	root, err := OpenTree(existing)
	if err != nil {
		t.Fatalf("OpenTree(%s): %v", existing, err)
	}

	if closeErr := root.Close(); closeErr != nil {
		t.Errorf("Close: %v", closeErr)
	}
}

// TestOpenTree_RefusesAPathThatCannotAnchorARoot verifies a non-absolute path is refused rather than guessed at.
//
// On Windows this is the case that matters: a leading separator with no volume is drive-relative and resolves
// against whatever drive the process is standing on. Guessing a volume here would answer a question #392 owns.
func TestOpenTree_RefusesAPathThatCannotAnchorARoot(t *testing.T) {

	for _, path := range []string{"relative/path", ""} {
		if _, err := OpenTree(path); err == nil {
			t.Errorf("OpenTree(%q) = nil error, want a refusal", path)
		}
	}
}
