// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Command docgen generates Docker-style CLI reference documentation
// for the writ and lore commands.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/NobleFactor/devlore-cli/cmd/lore/lore"
	"github.com/NobleFactor/devlore-cli/internal/tools/docgen"
	"github.com/NobleFactor/devlore-cli/internal/writ"
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

func run(outputDir, version string) error {
	fmt.Println("Generating CLI reference documentation...")

	writCmd := writ.NewRootCmd()
	fmt.Printf("\nwrit (%d commands):\n", countCommands(writCmd))
	if err := docgen.GenerateTree(writCmd, outputDir, "writ", version); err != nil {
		return fmt.Errorf("generating writ docs: %w", err)
	}

	loreCmd := lore.NewRootCmd()
	fmt.Printf("\nlore (%d commands):\n", countCommands(loreCmd))
	if err := docgen.GenerateTree(loreCmd, outputDir, "lore", version); err != nil {
		return fmt.Errorf("generating lore docs: %w", err)
	}

	fmt.Println("\nDone.")
	return nil
}

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
