// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/NobleFactor/devlore-cli/schema"
)

// RootConfig configures a root CLI command for lore or writ.
type RootConfig struct {
	Name          string // Command name ("lore" or "writ")
	Short         string // One-line description
	Long          string // Multi-line description
	DefaultConfig []byte // Schema default config (e.g., schema.LoreDefaultConfig)
	Version       string // Semantic version, set via ldflags
	Commit        string // Git commit hash, set via ldflags
	BuildDate     string // Build timestamp, set via ldflags
}

// NewRootCmd creates a root cobra command with all shared flags, metadata
// commands, and Viper configuration. The caller adds tool-specific flags
// and subcommands to the returned command.
//
// Parameters:
//   - cfg: root command configuration (name, descriptions, version info)
//
// Returns:
//   - *cobra.Command: configured root command with shared flags and metadata commands
func NewRootCmd(cfg RootConfig) *cobra.Command {
	SetProgramName(cfg.Name)

	rootCmd := &cobra.Command{
		Use:               cfg.Name,
		Short:             cfg.Short,
		Long:              cfg.Long,
		DisableAutoGenTag: true,
		CompletionOptions: cobra.CompletionOptions{
			HiddenDefaultCmd: true,
		},
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			return initRootConfig(cmd, cfg.Name)
		},
	}

	// Standard flags
	rootCmd.PersistentFlags().String("config", "", "Config file (default: ~/.config/devlore/config.yaml)")
	rootCmd.PersistentFlags().Bool("dry-run", false, "Show what would be done without making changes")
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "Verbose output")
	AddSilentFlag(rootCmd)

	// Deployment mode flags (ADR-033)
	rootCmd.PersistentFlags().Bool("interactive", false, "Force interactive mode (prompts, rich output)")
	rootCmd.PersistentFlags().Bool("unattended", false, "Force unattended mode (no prompts, sensible defaults)")
	rootCmd.MarkFlagsMutuallyExclusive("interactive", "unattended")

	// Model configuration flags
	// Resolution order: CLI flags → Environment → Config file → Keystore (api-key only)
	rootCmd.PersistentFlags().String("model", "", "Model name (e.g., claude-sonnet-4-20250514, gpt-4o)")
	rootCmd.PersistentFlags().String("model-api-key", "", "Model provider API key")
	rootCmd.PersistentFlags().String("model-endpoint", "", "Model provider endpoint URL")
	rootCmd.PersistentFlags().String("model-provider", "", "Model provider: anthropic, openai, azure-openai, ollama, github")

	// Shared metadata commands
	capitalized := strings.ToUpper(cfg.Name[:1]) + cfg.Name[1:]

	manHeader := ManHeader{
		Title:   strings.ToUpper(cfg.Name),
		Section: "1",
		Source:  capitalized + " " + cfg.Version,
		Manual:  capitalized + " Manual",
	}
	configInfo := ConfigInfo{
		Name:          cfg.Name,
		Schema:        schema.DevloreSchema,
		DefaultConfig: cfg.DefaultConfig,
	}

	rootCmd.SetHelpCommand(NewHelpCmd(rootCmd, manHeader))
	rootCmd.AddCommand(NewVersionCmd(VersionInfo{
		Version:   cfg.Version,
		Commit:    cfg.Commit,
		BuildDate: cfg.BuildDate,
	}))
	rootCmd.AddCommand(NewManCmd(rootCmd, manHeader))
	rootCmd.AddCommand(NewConfigCmd(configInfo))
	rootCmd.AddCommand(NewSelfInstallCmd(rootCmd, SelfInstallInfo{
		Name:       cfg.Name,
		ManHeader:  manHeader,
		ConfigInfo: configInfo,
	}))

	return rootCmd
}

// initRootConfig initializes Viper configuration for a root command.
// Precedence (lowest to highest): config file → environment variables → flags.
//
// Parameters:
//   - cmd: the cobra command triggering initialization
//   - name: tool name ("lore" or "writ"), used as Viper prefix and env prefix
//
// Returns:
//   - error: configuration, config file read, or flag binding failure
func initRootConfig(cmd *cobra.Command, name string) error {
	if err := InitViper(ViperConfig{
		Name:            name,
		EnvPrefix:       strings.ToUpper(name),
		UseSharedConfig: true,
	}); err != nil {
		return err
	}

	if cfgFile, _ := cmd.Flags().GetString("config"); cfgFile != "" { //nolint:errcheck // flag registered above
		viper.SetConfigFile(cfgFile)
		if err := viper.ReadInConfig(); err != nil {
			return fmt.Errorf("failed to read config %s: %w", cfgFile, err)
		}
	}

	if err := BindFlags(cmd.Root(), name, true); err != nil {
		return err
	}

	if viper.GetBool(name + ".verbose") {
		Note("Using config: %s", viper.ConfigFileUsed())
	}

	return nil
}
