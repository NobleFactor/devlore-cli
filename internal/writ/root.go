// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

// Package writ implements the writ CLI commands.
package writ

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

// NewRootCmd creates the root writ command with all subcommands.
func NewRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "writ",
		Short: "Dotfiles manager with platform-aware symlinks",
		Long: `Writ manages symlinks between a dotfiles repository and target directories.

One command deploys your configuration. Platform-aware packages adapt
automatically. Templates handle machine-specific values.

Writ exists because dotfiles management shouldn't require manual symlink
creation, platform-specific scripts, or secret leakage into git.
Declare your configuration once — writ deploys it everywhere you work.

Writ integrates with lore for complete machine setup:
lore installs tools, writ deploys configuration.`,
	}

	// Global flags
	rootCmd.PersistentFlags().String("config", "", "Config file (default: ~/.config/writ/config.yaml)")
	rootCmd.PersistentFlags().String("target", "Home", "Target to operate on")
	rootCmd.PersistentFlags().Bool("dry-run", false, "Show what would be done without making changes")
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "Verbose output")
	rootCmd.PersistentFlags().BoolP("quiet", "q", false, "Suppress non-error output")

	// Add subcommands
	rootCmd.AddCommand(newAddCmd())
	rootCmd.AddCommand(newRemoveCmd())
	rootCmd.AddCommand(newStatusCmd())
	rootCmd.AddCommand(newAdoptCmd())
	rootCmd.AddCommand(newListCmd())
	rootCmd.AddCommand(newInitCmd())
	rootCmd.AddCommand(newConfigureCmd())

	// Shared metadata
	manHeader := clifactory.ManHeader{
		Title:   "WRIT",
		Section: "1",
		Source:  "Writ " + version,
		Manual:  "Writ Manual",
	}
	configInfo := clifactory.ConfigInfo{
		Name:          "writ",
		Schema:        schema.WritSchema,
		DefaultConfig: schema.WritDefaultConfig,
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
		Name:       "writ",
		ManHeader:  manHeader,
		ConfigInfo: configInfo,
	}))

	return rootCmd
}
