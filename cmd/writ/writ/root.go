// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

// Package writ implements the writ CLI commands.
package writ

import (
	"github.com/spf13/cobra"

	"github.com/NobleFactor/devlore-cli/cmd/internal/cli"
	"github.com/NobleFactor/devlore-cli/schema"
)

// Version information, set at build time via ldflags.
var (
	version   = "dev"
	commit    = "none"
	buildDate = "unknown"
)

// NewRootCmd creates the root writ command with all subcommands.
//
// Returns:
//   - *cobra.Command: configured writ command with target flag and all subcommands
func NewRootCmd() *cobra.Command {
	rootCmd := cli.NewRootCmd(cli.RootConfig{
		Name:  "writ",
		Short: "Environment manager with platform-aware symlinks",
		Long: `Writ orchestrates your portable environment—configuration, scripts, utilities,
templates, and software manifests. Lore is a component that writ delegates to
for software installation.

One command deploys your environment. Platform-aware projects adapt
automatically. Templates handle machine-specific values.

Writ exists because environment management shouldn't require manual symlink
creation, platform-specific scripts, or secret leakage into git.
Declare your environment once — writ deploys it everywhere you work.`,
		DefaultConfig: schema.WritDefaultConfig,
		Version:       version,
		Commit:        commit,
		BuildDate:     buildDate,
	})

	rootCmd.PersistentFlags().String("target", "Home", "Target to operate on")

	rootCmd.AddCommand(newDeployCmd())
	rootCmd.AddCommand(newDecommissionCmd())
	rootCmd.AddCommand(newStatusCmd())
	rootCmd.AddCommand(newVerifyCmd())
	rootCmd.AddCommand(newUpgradeCmd())
	rootCmd.AddCommand(newAdoptCmd())
	rootCmd.AddCommand(newMigrateCmd())
	rootCmd.AddCommand(newRepoCmd())
	rootCmd.AddCommand(newSecretCmd())

	return rootCmd
}
