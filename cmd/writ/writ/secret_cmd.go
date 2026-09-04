// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package writ

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/NobleFactor/devlore-cli/cmd/internal/cli"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/secret"
)

// newSecretCmd creates the `writ secret` parent command.
//
// Returns:
//   - `*cobra.Command`: the secret family parent with its implemented subcommands attached.
func newSecretCmd() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "secret",
		Short: "SOPS secrets for layer repositories",
		Long: `SOPS secrets for layer repositories.

The family (docs/plans/writ-secret-*.md): encrypt (authoring) is implemented;
init (the BYOK ceremony), decrypt, rekey, list, and recover are chartered.
Lifecycle commands operate only within registered layers; deploy-time
decryption is writ deploy's native behavior (<name>.sops deploys as <name>).`,
	}

	cmd.AddCommand(newSecretEncryptCmd())

	return cmd
}

// newSecretEncryptCmd creates the `writ secret encrypt` subcommand.
//
// Returns:
//   - `*cobra.Command`: the encrypt command.
func newSecretEncryptCmd() *cobra.Command {

	return &cobra.Command{
		Use:   "encrypt <file>...",
		Short: "Encrypt files to .sops siblings per the governing .sops.yaml",
		Long: `Encrypt files to .sops siblings per the governing .sops.yaml.

The <file>.sops sibling is written beside each input — deployed names are
unchanged (foo.env.sops deploys as foo.env). Recipients and document format
come from the .sops.yaml resolved within the containing layer; a file no
creation rule governs fails with the resolver's error. An existing sibling
refuses — encrypt never overwrites — and the plaintext source is never
deleted; removal belongs to the caller.

Every argument must lie inside a registered layer's working tree; register a
repository with 'writ repo add'. The run rides the standard pipeline: the
graph and trace persist to the execution store with receipts recorded.`,
		Example: `  writ secret encrypt Home/noblefactor/.config/service/credentials.yaml
  writ secret encrypt Home/common/.ssh/id_ed25519 Home/common/.ssh/id_rsa
  writ secret encrypt --dry-run Home/thenobles/.Personal-secrets/keys.json`,
		Args: cobra.MinimumNArgs(1),
		RunE: runSecretEncrypt,
	}
}

// runSecretEncrypt implements the encrypt command on the secret package.
func runSecretEncrypt(cmd *cobra.Command, args []string) error {

	graphs, err := secret.ExecuteEncrypt(cmd.Context(), &secret.EncryptConfig{
		Files:   args,
		DryRun:  viper.GetBool("writ.dry-run"),
		Verbose: viper.GetBool("writ.verbose"),
	})
	if err != nil {
		return err
	}

	// Under --dry-run the plan is the result, and the pipeline renders it like any other.
	if graphs != nil {
		return cli.Emit(cmd, graphs)
	}
	return nil
}
