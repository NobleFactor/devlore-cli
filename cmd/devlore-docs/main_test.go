// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRun_EveryProgramHasATree is #787's guard: the reference covers the four programs of the suite, by name,
// so a fifth program -- or a dropped one -- fails here rather than on the published site. star's tree must
// carry an extension's command, since the extensions are most of what star is.
func TestRun_EveryProgramHasATree(t *testing.T) {

	out := t.TempDir()
	if err := run(out, "test"); err != nil {
		t.Fatalf("run: %v", err)
	}

	// A program's root page is `<program>.md`; its commands are pages under `<program>/`.
	for _, program := range programs {
		if _, err := os.Stat(filepath.Join(out, program+".md")); err != nil {
			t.Errorf("%s: no root page: %v", program, err)
		}
		if entries, err := os.ReadDir(filepath.Join(out, program)); err != nil || len(entries) == 0 {
			t.Errorf("%s: no command pages under %s: %v", program, filepath.Join(out, program), err)
		}
	}

	if _, err := os.Stat(filepath.Join(out, "star", "gh", "issues", "report.md")); err != nil {
		t.Errorf("star's tree has no page for an extension command: %v", err)
	}
}
