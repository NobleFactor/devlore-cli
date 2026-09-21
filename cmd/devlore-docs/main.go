// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

// Command devlore-docs generates the CLI reference: one markdown page per command, for every program in the
// suite -- writ, lore, star and devlore-test (#787). Each program's tree is built in-process from its
// `NewRootCmd`, so the pages say what the binaries say.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/NobleFactor/devlore-cli/cmd/devlore-test/devloretest"
	"github.com/NobleFactor/devlore-cli/cmd/lore/lore"
	"github.com/NobleFactor/devlore-cli/cmd/star/star"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ"
	"github.com/NobleFactor/devlore-cli/pkg/iox"
)

func main() {
	outputDir := flag.String("output-dir", "docs/cli", "Output directory for generated docs")
	version := flag.String("version", "dev", "Version string for generated docs")
	flag.Parse()

	if err := run(*outputDir, *version); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "docgen: %v\n", err)
		os.Exit(1)
	}
}

func run(outputDir, version string) (err error) {
	fmt.Println("Generating CLI reference documentation...")

	// star's root comes with the session its extension commands run in; the walk needs only the tree.
	starCmd, starApplication := star.NewRootCmd()
	defer iox.Close(&err, starApplication)

	for _, program := range []struct {
		name string
		root *cobra.Command
	}{
		{"writ", writ.NewRootCmd()},
		{"lore", lore.NewRootCmd()},
		{"star", starCmd},
		{"devlore-test", devloretest.NewRootCmd()},
	} {
		fmt.Printf("\n%s (%d commands):\n", program.name, countCommands(program.root))
		if err := GenerateTree(program.root, outputDir, program.name, version); err != nil {
			return fmt.Errorf("generating %s docs: %w", program.name, err)
		}
	}

	fmt.Println("\nDone.")
	return nil
}

// programs are the suite's four, in the order the reference lists them; the test of #787 reads this list.
var programs = []string{"writ", "lore", "star", "devlore-test"}

func countCommands(cmd *cobra.Command) int {
	count := 0
	for _, c := range cmd.Commands() {
		if !c.Hidden && c.Name() != "help" && c.Name() != "man" && c.Name() != "version" {
			count++
			count += countCommands(c)
		}
	}
	return count
}
