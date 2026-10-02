// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

// Package writ implements the writ CLI commands.
package writ

import (
	"errors"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/NobleFactor/devlore-cli/cmd/internal/cli"
	"github.com/NobleFactor/devlore-cli/pkg/application"
	"github.com/NobleFactor/devlore-cli/schema"
)

// Version information, stamped once for every command in [application].
var (
	version    = application.Version
	commit     = application.Commit
	buildDate  = application.BuildDate
	channel    = application.Channel
	prerelease = application.IsPrerelease()
)

// lifecycleCommands are the commands a retired configuration key stops: the ones that deploy from the layers or read
// the record (#926). Every other command runs -- `config` above all, since it is how the key is removed.
var lifecycleCommands = map[string]bool{
	"adopt":        true,
	"decommission": true,
	"deploy":       true,
	"reconcile":    true,
	"upgrade":      true,
}

// NewRootCmd creates the root writ command with all subcommands.
//
// Returns:
//   - `*cobra.Command`: the writ command, with its persistent flags and every subcommand.
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
		Channel:       channel,
		Prerelease:    prerelease,
	})

	rootCmd.PersistentFlags().String("target", "Home", "Target to operate on")

	// The configuration is loaded by the shared root's pre-run. writ then refuses a key it has retired rather than
	// ignoring it (#925), for the lifecycle commands only (#926): `writ config unset` must still run, or the key could
	// not be removed with writ, as `git config --unset` removes any key.
	loadConfiguration := rootCmd.PersistentPreRunE
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {

		if err := loadConfiguration(cmd, args); err != nil {
			return err
		}
		if cmd.Parent() != cmd.Root() || !lifecycleCommands[cmd.Name()] {
			return nil
		}
		return refuseRetiredConfiguration()
	}

	rootCmd.AddCommand(newDeployCmd())
	rootCmd.AddCommand(newDecommissionCmd())
	rootCmd.AddCommand(newReconcileCmd())
	rootCmd.AddCommand(newUpgradeCmd())
	rootCmd.AddCommand(newAdoptCmd())
	rootCmd.AddCommand(newMigrateCmd())
	rootCmd.AddCommand(newRepoCmd())
	rootCmd.AddCommand(newSecretCmd())

	return rootCmd
}

// region HELPER FUNCTIONS

// refuseRetiredConfiguration refuses a configuration that still sets `writ.targets`.
//
// Scope roots are named under `writ.scopes` (#925): the word is scope, not target. A configuration that names them
// the old way would otherwise be ignored without a word, and the deployment would land in the default root.
//
// Returns:
//   - `error`: an [cli.ExitConfig]-coded refusal naming `writ.scopes` when `writ.targets` is set; nil otherwise.
func refuseRetiredConfiguration() error {

	if viper.IsSet("writ.targets") {
		return cli.ExitWith(cli.ExitConfig,
			errors.New("writ.targets is retired: name scope roots under writ.scopes, for example writ.scopes.Home"))
	}
	return nil
}

// endregion
