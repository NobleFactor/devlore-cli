// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

// Package devloretest implements the devlore-test CLI commands.
package devloretest

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/NobleFactor/devlore-cli/cmd/internal/cli"
	"github.com/NobleFactor/devlore-cli/pkg/application"
	"github.com/NobleFactor/devlore-cli/pkg/assert"
	"github.com/NobleFactor/devlore-cli/pkg/sink"
	"github.com/NobleFactor/devlore-cli/pkg/status"
	"github.com/NobleFactor/devlore-cli/schema"
)

// Version information, stamped once for every command in [application].
var (
	version   = application.Version
	commit    = application.Commit
	buildDate = application.BuildDate
)

// NewRootCmd creates the root devlore-test command with all subcommands.
func NewRootCmd() *cobra.Command {

	var opts cli.SinkOptions

	rootCmd := &cobra.Command{
		Use:   "devlore-test",
		Short: "Graph test harness for Starlark plan + execute + verify",
		Long: `devlore-test is the graph test harness for the devlore execution engine.

It executes a Starlark test script that builds an execution graph, runs the
graph through the engine, and verifies expectations against the results.

The result goes to stdout as JSON; narration goes to stderr; the definition
and its traces go to the execution store.

  devlore-test run test.star
  devlore-test run -o yaml test.star
  devlore-test run --store ./run test.star
  devlore-test run -o none test.star      # exit code only`,
		SilenceUsage: true,
		CompletionOptions: cobra.CompletionOptions{
			HiddenDefaultCmd: true,
		},
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {

			// Construct the package-global status.UI from parsed flags. The same instance flows
			// into RuntimeEnvironmentSpec.Status so --silent applies uniformly across all
			// emission points. The choice between Console and Discard is at the construction
			// site — Console always emits; Discard always drops.
			silent := assert.Must(cmd.Flags().GetBool("silent"))
			var s sink.Sink
			if silent {
				s = sink.Discard()
			} else {
				s = sink.Stderr()
			}
			cli.SetUI(status.NewNarrator("devlore-test", s))

			return initConfig(cmd)
		},
	}

	// Global flags
	rootCmd.PersistentFlags().String("config", "", "Config file (default: ~/.config/devlore/config.yaml)")
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "Verbose output")
	cli.AddSilentFlag(rootCmd)

	// Add subcommands
	// The common set binds once, on the root: every subcommand accepts every flag.
	cli.AddOutputFlags(rootCmd, &opts)

	rootCmd.AddCommand(newRunCmd(&opts))

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
	versionInfo := cli.VersionInfo{
		Version:   version,
		Commit:    commit,
		BuildDate: buildDate,
	}

	rootCmd.SetHelpCommand(cli.NewHelpCmd(rootCmd, manHeader))
	cli.AddVersionFlag(rootCmd, versionInfo)
	rootCmd.AddCommand(cli.NewVersionCmd(versionInfo))
	rootCmd.AddCommand(cli.NewManCmd(rootCmd, manHeader))
	rootCmd.AddCommand(cli.NewConfigCmd(configInfo))
	rootCmd.AddCommand(cli.NewSelfCmd(rootCmd, cli.SelfInstallInfo{
		Name:       "devlore-test",
		Version:    version,
		ManHeader:  manHeader,
		ConfigInfo: &configInfo,
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
