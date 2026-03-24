// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package devloretest implements the devlore-test CLI commands.
package devloretest

import (
	"fmt"

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

// NewRootCmd creates the root devlore-test command with all subcommands.
func NewRootCmd() *cobra.Command {
	cli.SetProgramName("devlore-test")

	rootCmd := &cobra.Command{
		Use:   "devlore-test",
		Short: "Graph test harness for Starlark plan + execute + verify",
		Long: `devlore-test is the graph test harness for the devlore execution engine.

It executes a Starlark test script that builds an execution graph, runs the
graph through the engine, and verifies expectations against the results.

Output streams:
  graph    Output from the software under test (default: stdout)
  summary  Test result with pass/fail and expectation counts (default: stdout)
  receipt  Full execution graph transaction log (default: stdout)

Use --output to route streams to files or /dev/null:
  devlore-test run --output receipt=/tmp/receipt.yaml test.star
  devlore-test run --output graph=/dev/null --output receipt=/dev/null test.star`,
		CompletionOptions: cobra.CompletionOptions{
			HiddenDefaultCmd: true,
		},
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return initConfig(cmd)
		},
	}

	// Global flags
	rootCmd.PersistentFlags().String("config", "", "Config file (default: ~/.config/devlore/config.yaml)")
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "Verbose output")
	cli.AddSilentFlag(rootCmd)

	// Add subcommands
	rootCmd.AddCommand(newRunCmd())

	// Shared metadata
	manHeader := cli.ManHeader{
		Title:   "DEVLORE-TEST",
		Section: "1",
		Source:  "devlore-test " + version,
		Manual:  "devlore-test Manual",
	}
	configInfo := cli.ConfigInfo{
		Name:          "devlore-test",
		Schema:        schema.DevloreSchema,
		DefaultConfig: schema.TestDefaultConfig,
	}

	// Add shared commands from cli
	rootCmd.SetHelpCommand(cli.NewHelpCmd(rootCmd, manHeader))
	rootCmd.AddCommand(cli.NewVersionCmd(cli.VersionInfo{
		Version:   version,
		Commit:    commit,
		BuildDate: buildDate,
	}))
	rootCmd.AddCommand(cli.NewManCmd(rootCmd, manHeader))
	rootCmd.AddCommand(cli.NewConfigCmd(configInfo))
	rootCmd.AddCommand(cli.NewSelfInstallCmd(rootCmd, cli.SelfInstallInfo{
		Name:       "devlore-test",
		ManHeader:  manHeader,
		ConfigInfo: configInfo,
	}))

	return rootCmd
}

// initConfig initializes Viper configuration.
func initConfig(cmd *cobra.Command) error {
	if err := cli.InitViper(cli.ViperConfig{
		Name:            "devlore-test",
		EnvPrefix:       "DEVLORE_TEST",
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

	if err := cli.BindFlags(cmd.Root(), "devlore-test", true); err != nil {
		return err
	}

	if viper.GetBool("devlore-test.verbose") {
		cli.Note("Using config: %s", viper.ConfigFileUsed())
	}

	return nil
}
