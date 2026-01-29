// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

// Package lore implements the lore CLI commands.
package lore

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

// NewRootCmd creates the root lore command with all subcommands.
func NewRootCmd() *cobra.Command {
	cli.SetProgramName("lore")

	rootCmd := &cobra.Command{
		Use:               "lore",
		Short:             "The tribal knowledge package deployer",
		DisableAutoGenTag: true,
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
		CompletionOptions: cobra.CompletionOptions{
			HiddenDefaultCmd: true, // Hide from help, but still available (like Docker)
		},
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return initConfig(cmd)
		},
	}

	// Global flags
	rootCmd.PersistentFlags().String("config", "", "Config file (default: ~/.config/devlore/config.yaml)")
	rootCmd.PersistentFlags().String("registry", "", "Registry path")
	rootCmd.PersistentFlags().Bool("dry-run", false, "Show what would be done without making changes")
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "Verbose output")
	cli.AddSilentFlag(rootCmd)

	// Deployment mode flags (ADR-033)
	rootCmd.PersistentFlags().Bool("interactive", false, "Force interactive mode (prompts, rich output)")
	rootCmd.PersistentFlags().Bool("unattended", false, "Force unattended mode (no prompts, sensible defaults)")
	rootCmd.MarkFlagsMutuallyExclusive("interactive", "unattended")

	// Model configuration flags (override env/config)
	// Resolution order: CLI flags → Environment → Config file → Keystore (api-key only)
	// See internal/model/config.go for full documentation.
	rootCmd.PersistentFlags().String("model", "", "Model name (e.g., claude-sonnet-4-20250514, gpt-4o)")
	rootCmd.PersistentFlags().String("model-api-key", "", "Model provider API key")
	rootCmd.PersistentFlags().String("model-endpoint", "", "Model provider endpoint URL")
	rootCmd.PersistentFlags().String("model-provider", "", "Model provider: anthropic, openai, azure-openai, ollama, github")

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
	rootCmd.AddCommand(newPublishCmd())
	rootCmd.AddCommand(newAuditCmd())
	rootCmd.AddCommand(newInspectCmd())

	// Shared metadata
	manHeader := cli.ManHeader{
		Title:   "LORE",
		Section: "1",
		Source:  "Lore " + version,
		Manual:  "Lore Manual",
	}
	configInfo := cli.ConfigInfo{
		Name:          "lore",
		Schema:        schema.LoreSchema,
		DefaultConfig: schema.LoreDefaultConfig,
	}

	// Add shared commands from cli
	// Replace Cobra's built-in help with git-style help (prefers man pages)
	rootCmd.SetHelpCommand(cli.NewHelpCmd(rootCmd, manHeader))
	rootCmd.AddCommand(cli.NewVersionCmd(cli.VersionInfo{
		Version:   version,
		Commit:    commit,
		BuildDate: buildDate,
	}))
	rootCmd.AddCommand(cli.NewManCmd(rootCmd, manHeader))
	rootCmd.AddCommand(cli.NewConfigCmd(configInfo))
	rootCmd.AddCommand(cli.NewSelfInstallCmd(rootCmd, cli.SelfInstallInfo{
		Name:       "lore",
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
		Name:            "lore",
		EnvPrefix:       "LORE",
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
	if err := cli.BindFlags(cmd, "lore", true); err != nil {
		return err
	}

	// Debug output if verbose
	if viper.GetBool("lore.verbose") {
		fmt.Fprintf(os.Stderr, "Using config: %s\n", viper.ConfigFileUsed())
	}

	return nil
}
