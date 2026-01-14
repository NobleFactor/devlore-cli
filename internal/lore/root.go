// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

// Package lore implements the lore CLI commands.
package lore

import (
	"github.com/spf13/cobra"

	"github.com/NobleFactor/devlore-cli/internal/clifactory"
	"github.com/NobleFactor/devlore-cli/schema"
)

// Version information, set at build time via ldflags.
var (
	version   = "dev"
	commit    = "none"
	buildDate = "unknown"
)

// NewRootCmd creates the root lore command with all subcommands.
func NewRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "lore",
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
	}

	// Global flags
	rootCmd.PersistentFlags().String("config", "", "Config file (default: ~/.config/lore/config.yaml)")
	rootCmd.PersistentFlags().String("registry", "", "Registry path")
	rootCmd.PersistentFlags().Bool("dry-run", false, "Show what would be done without making changes")
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "Verbose output")
	rootCmd.PersistentFlags().BoolP("quiet", "q", false, "Suppress non-error output")

	// Add subcommands
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

	// Shared metadata
	manHeader := clifactory.ManHeader{
		Title:   "LORE",
		Section: "1",
		Source:  "Lore " + version,
		Manual:  "Lore Manual",
	}
	configInfo := clifactory.ConfigInfo{
		Name:          "lore",
		Schema:        schema.LoreSchema,
		DefaultConfig: schema.LoreDefaultConfig,
	}

	// Add shared commands from clifactory
	rootCmd.AddCommand(clifactory.NewCompletionCmd(rootCmd))
	rootCmd.AddCommand(clifactory.NewVersionCmd(clifactory.VersionInfo{
		Version:   version,
		Commit:    commit,
		BuildDate: buildDate,
	}))
	rootCmd.AddCommand(clifactory.NewManCmd(rootCmd, manHeader))
	rootCmd.AddCommand(clifactory.NewConfigCmd(configInfo))
	rootCmd.AddCommand(clifactory.NewSelfInstallCmd(rootCmd, clifactory.SelfInstallInfo{
		Name:       "lore",
		ManHeader:  manHeader,
		ConfigInfo: configInfo,
	}))

	return rootCmd
}
