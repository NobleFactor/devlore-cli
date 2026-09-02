// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package lore

import (
	"testing"

	"github.com/spf13/cobra"
)

// TestRoot_EverySubcommandAcceptsTheCommonSet pins the root registration: every command of lore accepts
// every flag of the common set, because the set is on the root and inherited, and no command registers an
// output flag of its own on top of it.
//
// Before #775 the set lived on `inspect` alone, so `lore search -o json` was an unknown flag while
// `lore inspect -o json` was not -- the convention was learnable on one command out of fourteen.
func TestRoot_EverySubcommandAcceptsTheCommonSet(t *testing.T) {

	root := NewRootCmd()
	if len(root.Commands()) == 0 {
		t.Fatal("the root has no subcommands; nothing to check")
	}

	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		for _, sub := range cmd.Commands() {
			for _, name := range []string{"output", "filter", "jq", "store"} {
				if sub.InheritedFlags().Lookup(name) == nil {
					t.Errorf("%s does not inherit --%s from the root", sub.CommandPath(), name)
				}
			}
			for _, name := range []string{"output", "format", "json"} {
				if sub.LocalFlags().Lookup(name) != nil {
					t.Errorf("%s registers its own --%s; the common set owns that name", sub.CommandPath(), name)
				}
			}
			walk(sub)
		}
	}
	walk(root)
}
