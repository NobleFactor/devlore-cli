// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package lore implements the lore CLI commands.
package lore

import (
	"github.com/spf13/cobra"

	"github.com/NobleFactor/devlore-cli/internal/cli"
	"github.com/NobleFactor/devlore-cli/schema"
)

// Version information, set at build time via ldflags.
var (
	version   = "dev"
	commit    = "none"
	buildDate = "unknown"
)

// NewRootCmd creates the root lore command with all subcommands.
//
// Returns:
//   - *cobra.Command: configured lore command with registry flag and all subcommands
func NewRootCmd() *cobra.Command {
	rootCmd := cli.NewRootCmd(cli.RootConfig{
		Name:  "lore",
		Short: "The tribal knowledge package deployer",
		Long: `Lore is a cross-platform package deployment tool that captures tribal
knowledge about installing software.

It delegates to native package managers when possible and provides
custom deployment scripts when necessary.

Lore exists because "install this package" is rarely the whole story.
Docker requires adding vendor repositories, removing conflicting packages,
and installing five separate components. Pandoc needs a PDF engine, which
needs LaTeX, which needs tlmgr to install packages that aren't documented
anywhere.

Lore captures this knowledge once and shares it forever.
What took someone hours to figure out, you get in minutes.`,
		DefaultConfig: schema.LoreDefaultConfig,
		Version:       version,
		Commit:        commit,
		BuildDate:     buildDate,
	})

	rootCmd.PersistentFlags().String("registry", "", "Registry path")

	rootCmd.AddCommand(newDeployCmd())
	rootCmd.AddCommand(newUpgradeCmd())
	rootCmd.AddCommand(newDecommissionCmd())
	rootCmd.AddCommand(newReconcileCmd())
	rootCmd.AddCommand(newBundleCmd())
	rootCmd.AddCommand(newManifestCmd())
	rootCmd.AddCommand(newSearchCmd())
	rootCmd.AddCommand(newListCmd())
	rootCmd.AddCommand(newResolveCmd())
	rootCmd.AddCommand(newUpdateCmd())
	rootCmd.AddCommand(newOnboardCmd())
	rootCmd.AddCommand(newPublishCmd())
	rootCmd.AddCommand(newAuditCmd())
	rootCmd.AddCommand(newInspectCmd())

	return rootCmd
}
