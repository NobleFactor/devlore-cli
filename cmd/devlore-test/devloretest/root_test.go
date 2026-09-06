// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package devloretest

import (
	"bytes"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/cmd/internal/cli"
)

// TestRoot_KeepsTheOutputConvention pins the root registration through the shared checkers: every command
// inherits the common set from the root, none shadows an inherited flag or binds a reserved name, and the
// set on the root is the shared root's (10-command-line-interface.md §4, §14).
//
// Before #757 devlore-test built its own cobra.Command, and `run` bound a --dry-run of its own that
// shadowed the root's.
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

// TestRoot_HelpWrapsLikeTheOthers pins what #757 was filed on: at COLUMNS=70 the longest flag line of
// devlore-test's help was 389 columns where writ's and lore's were 70, because the help wrapping added
// for #755 reached only the roots built by cli.NewRootCmd.
func TestRoot_HelpWrapsLikeTheOthers(t *testing.T) {

	t.Setenv("COLUMNS", "70")
	root := NewRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"run", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("devlore-test run --help: %v", err)
	}
	for _, line := range strings.Split(out.String(), "\n") {
		if strings.HasPrefix(strings.TrimLeft(line, " "), "-") && len(line) > 70 {
			t.Errorf("a flag line is %d columns at COLUMNS=70:\n%s", len(line), line)
		}
	}
}
