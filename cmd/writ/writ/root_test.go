// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package writ

import (
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/cmd/internal/cli"
)

// TestRoot_KeepsTheOutputConvention pins the root registration through the shared checkers: every command
// inherits the common set from the root, none shadows an inherited flag or binds a reserved name, and the
// set on the root is the shared root's (10-command-line-interface.md §4, §14).
func TestRoot_KeepsTheOutputConvention(t *testing.T) {

	root := NewRootCmd()
	if len(root.Commands()) == 0 {
		t.Fatal("the root has no subcommands; nothing to check")
	}
	if v := cli.CheckNoOwnOutputFlag(root); len(v) > 0 {
		t.Errorf("%d violations:\n%s", len(v), strings.Join(v, "\n"))
	}
	if v := cli.CheckSharedSetOnRoot(root); len(v) > 0 {
		t.Errorf("the root's set is not the shared root's:\n%s", strings.Join(v, "\n"))
	}
}
