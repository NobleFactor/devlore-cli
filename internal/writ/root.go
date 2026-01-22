// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

// Package writ implements the writ CLI commands.
package writ

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/NobleFactor/devlore-cli/internal/cli"
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
	cli.SetProgramName("writ")

	rootCmd := &cobra.Command{
		Use:   "writ",
		Short: "Environment manager with platform-aware symlinks",
		Long: `Writ orchestrates your portable environment—configuration, scripts, utilities,
templates, and software manifests. Lore is a component that writ delegates to
for software installation.

One command deploys your environment. Platform-aware projects adapt
automatically. Templates handle machine-specific values.

Writ exists because environment management shouldn't require manual symlink
creation, platform-specific scripts, or secret leakage into git.
Declare your environment once — writ deploys it everywhere you work.`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return initConfig(cmd)
		},
	}

	// Global flags
	rootCmd.PersistentFlags().String("config", "", "Config file (default: ~/.config/devlore/config.yaml)")
	rootCmd.PersistentFlags().String("target", "Home", "Target to operate on")
	rootCmd.PersistentFlags().Bool("dry-run", false, "Show what would be done without making changes")
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "Verbose output")
	rootCmd.PersistentFlags().BoolP("quiet", "q", false, "Suppress non-error output")

	// Deployment mode flags (mirrors lore ADR-033)
	rootCmd.PersistentFlags().Bool("interactive", false, "Force interactive mode (prompts for conflicts)")
	rootCmd.PersistentFlags().Bool("unattended", false, "Force unattended mode (no prompts, sensible defaults)")
	rootCmd.MarkFlagsMutuallyExclusive("interactive", "unattended")

	// Add subcommands
	rootCmd.AddCommand(newAddCmd())
	rootCmd.AddCommand(newRemoveCmd())
	rootCmd.AddCommand(newStatusCmd())
	rootCmd.AddCommand(newUpgradeCmd())
	rootCmd.AddCommand(newAdoptCmd())
	rootCmd.AddCommand(newListCmd())
	rootCmd.AddCommand(newRepoCmd())
	rootCmd.AddCommand(newInitCmd()) // Deprecated: use 'writ repo init'
	rootCmd.AddCommand(newConfigureCmd())
	rootCmd.AddCommand(newSecretsCmd())
	rootCmd.AddCommand(newReceiptCmd())

	// Shared metadata
	manHeader := cli.ManHeader{
		Title:   "WRIT",
		Section: "1",
		Source:  "Writ " + version,
		Manual:  "Writ Manual",
	}
	configInfo := cli.ConfigInfo{
		Name:          "writ",
		Schema:        schema.WritSchema,
		DefaultConfig: schema.WritDefaultConfig,
	}

	// Add shared commands from cli
	// Replace Cobra's built-in help with git-style help (prefers man pages)
	rootCmd.SetHelpCommand(cli.NewHelpCmd(rootCmd, manHeader))
	rootCmd.AddCommand(cli.NewCompletionCmd(rootCmd))
	rootCmd.AddCommand(cli.NewVersionCmd(cli.VersionInfo{
		Version:   version,
		Commit:    commit,
		BuildDate: buildDate,
	}))
	rootCmd.AddCommand(cli.NewManCmd(rootCmd, manHeader))
	rootCmd.AddCommand(cli.NewConfigCmd(configInfo))
	rootCmd.AddCommand(cli.NewSelfInstallCmd(rootCmd, cli.SelfInstallInfo{
		Name:       "writ",
		ManHeader:  manHeader,
		ConfigInfo: configInfo,
	}))

	return rootCmd
}

// initConfig initializes Viper configuration.
// Precedence (lowest to highest): config file → environment variables → flags
func initConfig(cmd *cobra.Command) error {
	// Initialize Viper with shared devlore config
	if err := cli.InitViper(cli.ViperConfig{
		Name:            "writ",
		EnvPrefix:       "WRIT",
		UseSharedConfig: true,
	}); err != nil {
		return err
	}

	// Check for custom config file via flag
	if cfgFile, _ := cmd.Flags().GetString("config"); cfgFile != "" {
		viper.SetConfigFile(cfgFile)
		if err := viper.ReadInConfig(); err != nil {
			return fmt.Errorf("failed to read config %s: %w", cfgFile, err)
		}
	}

	// Bind flags to viper (flag values override config/env)
	// Use cmd.Root() to bind persistent flags defined on the root command
	if err := cli.BindFlags(cmd.Root(), "writ", true); err != nil {
		return err
	}

	// Debug output if verbose
	if viper.GetBool("writ.verbose") {
		fmt.Fprintf(os.Stderr, "Using config: %s\n", viper.ConfigFileUsed())
	}

	return nil
}
