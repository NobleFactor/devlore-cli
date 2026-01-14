// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

package writ

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add [flags] <package>...",
		Short: "Deploy packages by creating symlinks in the target location",
		Long: `Deploy packages by creating symlinks in the target location.

Files inside each package directory are symlinked to the target (default: ~).
Platform-specific variants (e.g., package-darwin) are selected automatically.`,
		Example: `  writ add noblefactor
  writ add all noblefactor thenobles
  writ add --overwrite noblefactor`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("add %v: not yet implemented\n", args)
			return nil
		},
	}

	cmd.Flags().Bool("overwrite", false, "Replace existing files (backs up to .writ-backup)")

	return cmd
}

func newRemoveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <package>...",
		Short: "Remove symlinks for the specified packages",
		Long: `Remove symlinks for the specified packages.

Only removes symlinks that point to the package. Leaves real files untouched.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("remove %v: not yet implemented\n", args)
			return nil
		},
	}
	return cmd
}

func newStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status [<package>...]",
		Short: "Show symlink status for packages",
		Long: `Show symlink status for packages.

Status indicators:
  ✓ Linked   — Symlink exists and points to package
  ⚠ Conflict — File exists but isn't our symlink
  ✗ Missing  — Package file has no corresponding symlink
  ? Orphan   — Symlink points to nonexistent file`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("status: not yet implemented")
			return nil
		},
	}
	return cmd
}

func newAdoptCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "adopt <package> <file>",
		Short: "Move a file from target location into a package and create symlink",
		Long: `Move a file from target location into a package and create symlink.

Use this to bring existing configuration files under version control.`,
		Example: `  writ adopt noblefactor .zshrc
  writ adopt noblefactor .config/nvim/init.lua`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("adopt %s %s: not yet implemented\n", args[0], args[1])
			return nil
		},
	}
	return cmd
}

func newListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available packages for the current target",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("list: not yet implemented")
			return nil
		},
	}
	return cmd
}

func newInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a new dotfiles repository in the current directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("init: not yet implemented")
			return nil
		},
	}

	cmd.Flags().Bool("force", false, "Overwrite existing structure")

	return cmd
}

func newConfigureCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "configure",
		Short: "Discover system info and configure template variables",
		Long: `Discover system info and configure template variables.

Auto-detects user name, email, editor, and platform. Prompts for values
that cannot be determined automatically.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("configure: not yet implemented")
			return nil
		},
	}

	cmd.Flags().Bool("non-interactive", false, "Auto-detect only, fail on missing values")

	return cmd
}
