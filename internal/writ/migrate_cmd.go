// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

package writ

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/NobleFactor/devlore-cli/internal/writ/migrate"
)

func newMigrateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate [flags] <source-root>",
		Short: "Analyze and migrate an existing environment repository to writ conventions",
		Long: `Analyze an existing environment repository and produce a migration plan.

Writ auto-detects the source system (Tuckr, Stow, chezmoi, yadm, bare git,
or script-based setups using <project>-<Platform> directories) and generates
a plan showing what would change.

By default, only the plan is shown. Use --execute to perform the migration
(directory renames from dash to dot convention).

Supported output formats: text (default), yaml, json.`,
		Example: `  writ migrate ~/my-env/Configs
  writ migrate ~/my-env/Configs --format yaml
  writ migrate ~/my-env/Configs --execute
  writ migrate ~/my-env/Configs --execute --verbose`,
		Args: cobra.ExactArgs(1),
		RunE: runMigrate,
	}

	cmd.Flags().Bool("execute", false, "Perform the migration (rename directories)")
	cmd.Flags().String("format", "text", "Output format: text, yaml, json")
	cmd.Flags().String("system", "", "Override auto-detection with a specific source system")

	return cmd
}

func runMigrate(cmd *cobra.Command, args []string) error {
	sourceRoot, err := filepath.Abs(args[0])
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}

	info, err := os.Stat(sourceRoot)
	if err != nil {
		return fmt.Errorf("source root: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("source root %s is not a directory", sourceRoot)
	}

	execute, _ := cmd.Flags().GetBool("execute")
	format, _ := cmd.Flags().GetString("format")
	verbose, _ := cmd.Root().Flags().GetBool("verbose")

	opts := migrate.Options{
		SourceRoot: sourceRoot,
		Execute:    execute,
		Verbose:    verbose,
		Format:     format,
	}

	plan, err := migrate.BuildPlan(opts)
	if err != nil {
		return err
	}

	if execute {
		return migrate.Execute(os.Stderr, plan)
	}

	return migrate.FormatPlan(os.Stdout, plan, format)
}
