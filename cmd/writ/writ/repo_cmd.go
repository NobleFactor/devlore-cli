// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package writ

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/spf13/cobra"

	"github.com/NobleFactor/devlore-cli/internal/cli"
)

// newRepoCmd builds the repo command family: layer-repository registration through the layers directory.
//
// Registration is packaging, not configuration (the settled config-vs-layers separation): a layer is a
// symlink under [cli.WritLayersDir], never a config.yaml key. Bare `writ repo` lists, matching git-remote's
// idiom; `rm` and `ls` alias `remove` and `list`, matching docker's.
//
// Returns:
//   - `*cobra.Command`: the assembled repo command.
func newRepoCmd() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "repo [command]",
		Short: "Manage layer repository registrations",
		Long: `Manage layer repository registrations.

A layer (base, team, or personal) is registered by a symlink in the writ layers
directory (XDG_DATA_HOME/devlore/writ/layers) pointing at the repository. This is
packaging, not configuration: registrations never appear in config.yaml.

With no subcommand, repo lists the registrations.`,
		Example: `  writ repo add personal ~/Workspace/Personal
  writ repo                       # list registrations
  writ repo remove team`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return runRepoList(cmd) },
	}

	cmd.AddCommand(newRepoAddCmd())
	cmd.AddCommand(newRepoRemoveCmd())
	cmd.AddCommand(newRepoListCmd())

	return cmd
}

// newRepoAddCmd builds `repo add <layer> <path>`.
func newRepoAddCmd() *cobra.Command {

	return &cobra.Command{
		Use:   "add <layer> <path>",
		Short: "Register a repository as a layer",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRepoAdd(cmd, args[0], args[1])
		},
	}
}

// newRepoRemoveCmd builds `repo remove <layer>` (alias `rm`).
func newRepoRemoveCmd() *cobra.Command {

	return &cobra.Command{
		Use:     "remove <layer>",
		Aliases: []string{"rm"},
		Short:   "Unregister a layer (does not delete files)",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRepoRemove(cmd, args[0])
		},
	}
}

// newRepoListCmd builds `repo list` (alias `ls`).
func newRepoListCmd() *cobra.Command {

	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List layer registrations",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runRepoList(cmd)
		},
	}
}

// runRepoAdd registers `layer` as a symlink to `path` in the layers directory.
//
// Parameters:
//   - `cmd`: the invoking command; supplies the output stream.
//   - `layer`: the layer name; must be one of [LayerOrder].
//   - `path`: the repository path; `~` expands, the path must exist and be a directory.
//
// Returns:
//   - `error`: an unknown layer, a missing or non-directory path, an existing registration, or a
//     filesystem failure.
func runRepoAdd(cmd *cobra.Command, layer, path string) error {

	if !slices.Contains(LayerOrder, layer) {
		return fmt.Errorf("unknown layer %q (layers: base, team, personal)", layer)
	}

	absolute, err := filepath.Abs(expandPath(path))
	if err != nil {
		return err
	}

	info, err := os.Stat(absolute)
	if err != nil {
		return fmt.Errorf("repository path: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("repository path %s is not a directory", absolute)
	}

	layers := cli.WritLayersDir()
	if err := os.MkdirAll(layers, 0o750); err != nil {
		return err
	}

	link := filepath.Join(layers, layer)
	if _, err := os.Lstat(link); err == nil {
		return fmt.Errorf("layer %s is already registered; run 'writ repo remove %s' first", layer, layer)
	}

	if err := os.Symlink(absolute, link); err != nil {
		return err
	}

	_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s -> %s\n", layer, absolute)
	return err
}

// runRepoRemove unregisters `layer` by removing its layers-directory symlink; files are untouched.
//
// Parameters:
//   - `cmd`: the invoking command; supplies the output stream.
//   - `layer`: the layer name; must be one of [LayerOrder] and currently registered.
//
// Returns:
//   - `error`: an unknown layer, an unregistered layer, or a filesystem failure.
func runRepoRemove(cmd *cobra.Command, layer string) error {

	if !slices.Contains(LayerOrder, layer) {
		return fmt.Errorf("unknown layer %q (layers: base, team, personal)", layer)
	}

	link := filepath.Join(cli.WritLayersDir(), layer)
	if _, err := os.Lstat(link); err != nil {
		return fmt.Errorf("layer %s is not registered", layer)
	}

	if err := os.Remove(link); err != nil {
		return err
	}

	_, err := fmt.Fprintf(cmd.OutOrStdout(), "%s unregistered\n", layer)
	return err
}

// runRepoList prints every layer in order with its registration state.
//
// Parameters:
//   - `cmd`: the invoking command; supplies the output stream.
//
// Returns:
//   - `error`: a write failure on the output stream.
func runRepoList(cmd *cobra.Command) error {

	out := cmd.OutOrStdout()

	for _, layer := range LayerOrder {

		line := repoListLine(layer)
		if _, err := fmt.Fprint(out, line); err != nil {
			return err
		}
	}

	return nil
}

// repoListLine renders one layer's registration line for the list report.
//
// Parameters:
//   - `layer`: the layer name to render.
//
// Returns:
//   - `string`: the newline-terminated report line.
func repoListLine(layer string) string {

	link := filepath.Join(cli.WritLayersDir(), layer)

	info, err := os.Lstat(link)
	if err != nil {
		return fmt.Sprintf("%-8s (not registered)\n", layer)
	}

	target := link
	if info.Mode()&os.ModeSymlink != 0 {
		if target, err = os.Readlink(link); err != nil {
			return fmt.Sprintf("%-8s (unreadable link)\n", layer)
		}
	}

	if _, err := filepath.EvalSymlinks(link); err != nil {
		return fmt.Sprintf("%-8s -> %s (broken)\n", layer, target)
	}

	return fmt.Sprintf("%-8s -> %s\n", layer, target)
}
