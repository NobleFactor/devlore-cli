// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package writ

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/NobleFactor/devlore-cli/cmd/internal/cli"
	"github.com/NobleFactor/devlore-cli/cmd/internal/devlore"
)

// newRepoCmd builds the repo command family: layer-repository registration through the layers directory.
//
// Registration is packaging, not configuration (the settled config-vs-layers separation): a layer is a
// symlink under [devlore.WritLayersDir], never a config.yaml key. Bare `writ repo` lists, matching git-remote's
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
packaging, not configuration: registrations never appear in config.yaml.`,
		Example: `  writ repo add personal ~/Workspace/Personal
  writ repo list
  writ repo remove team`,
	}

	cmd.AddCommand(newRepoAddCmd())
	cmd.AddCommand(newRepoRemoveCmd())
	cmd.AddCommand(newRepoListCmd())

	return cmd
}

// newRepoAddCmd builds `repo add <layer> <working-tree-root>|<repository-url> [<working-tree-root>]`.
func newRepoAddCmd() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "add <layer> <working-tree-root>|<repository-url> [<working-tree-root>]",
		Short: "Register a repository as a layer",
		Long: `Register a repository as a layer.

The location is a local working-tree-root, or a repository URL — which triggers a
git clone (git-clone's own grammar: the optional trailing working-tree-root is the
clone destination, defaulting to the writ-owned home under
XDG_DATA_HOME/devlore/writ/repos). After placement the repository is entirely
yours: writ performs no hidden git operations, ever.`,
		Example: `  writ repo add personal ~/Workspace/Personal
  writ repo add team git@github.com:acme/team-env.git
  writ repo add personal git@github.com:me/personal.git ~/Workspace/Personal
  writ repo add personal git@github.com:me/personal.git --branch devlore-cli/writ-layer`,
		Args: cobra.RangeArgs(2, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			destination := ""
			if len(args) == 3 {
				destination = args[2]
			}
			branch, _ := cmd.Flags().GetString("branch") //nolint:errcheck // flag registered below
			return runRepoAdd(cmd, args[0], args[1], destination, branch)
		},
	}

	cmd.Flags().String("branch", "", "Branch to clone (repository-url form only)")

	return cmd
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

// RepoRegistration is a layer's registration as the `repo` commands report it: which layer, the working-tree
// root it points at, and what state the registration is in. A result like any other, so `--output json` feeds a
// script and `--output table` scans (10-command-line-interface.md §5).
type RepoRegistration struct {
	Layer string `json:"layer"`
	Root  string `json:"root,omitempty"`
	State string `json:"state"`
}

// The states a registration can be in, as [RepoRegistration.State] reports them.
const (
	repoStateRegistered   = "registered"
	repoStateUnregistered = "unregistered"
	repoStateBroken       = "broken"
	repoStateUnreadable   = "unreadable"
)

// runRepoAdd registers `layer` from `location` — a working-tree-root, or a repository URL cloned to
// `destination` (the writ-owned home when empty).
//
// Parameters:
//   - `cmd`: the invoking command; supplies the streams and context.
//   - `layer`: the layer name; must be one of [LayerOrder].
//   - `location`: a local working-tree-root (`~` expands; must be a git working tree) or a repository URL.
//   - `destination`: the clone destination for the URL form; empty selects the writ-owned home. Must be
//     empty for the working-tree-root form.
//   - `branch`: the branch to clone; URL form only.
//
// Returns:
//   - `error`: an unknown layer, a malformed combination, a failed clone, a non-working-tree root, an
//     existing registration, or a filesystem failure.
func runRepoAdd(cmd *cobra.Command, layer, location, destination, branch string) error {

	if !slices.Contains(LayerOrder, layer) {
		return fmt.Errorf("unknown layer %q (layers: base, team, personal)", layer)
	}

	root, err := resolveWorkingTreeRoot(cmd, layer, location, destination, branch)
	if err != nil {
		return err
	}

	layers := devlore.WritLayersDir()
	if err := os.MkdirAll(layers, 0o750); err != nil {
		return err
	}

	link := filepath.Join(layers, layer)
	if _, err := os.Lstat(link); err == nil {
		return fmt.Errorf("layer %s is already registered; run 'writ repo remove %s' first", layer, layer)
	}

	if err := os.Symlink(root, link); err != nil {
		return err
	}

	return cli.Emit(cmd, RepoRegistration{Layer: layer, Root: root, State: repoStateRegistered})
}

// resolveWorkingTreeRoot produces the layer's working-tree-root from the location operand: the validated
// local root, or the destination of a fresh clone for the URL form.
//
// Parameters:
//   - `cmd`: the invoking command; supplies the streams and context for the clone.
//   - `layer`: the layer name; names the writ-owned default clone destination.
//   - `location`: the polymorphic location operand.
//   - `destination`: the URL form's optional clone destination.
//   - `branch`: the URL form's optional branch.
//
// Returns:
//   - `string`: the absolute working-tree-root to register.
//   - `error`: a malformed combination, a failed clone, or a root that is not a git working tree.
func resolveWorkingTreeRoot(cmd *cobra.Command, layer, location, destination, branch string) (string, error) {

	if isRepositoryURL(location) {
		if destination == "" {
			destination = filepath.Join(devlore.WritReposDir(), layer)
		}
		return cloneRepository(cmd, location, expandPath(destination), branch)
	}

	if destination != "" {
		return "", fmt.Errorf("a working-tree-root takes no destination (got %q)", destination)
	}
	if branch != "" {
		return "", fmt.Errorf("--branch applies to the repository-url form only")
	}

	absolute, err := filepath.Abs(expandPath(location))
	if err != nil {
		return "", err
	}

	info, err := os.Stat(absolute)
	if err != nil {
		return "", fmt.Errorf("working-tree-root: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("working-tree-root %s is not a directory", absolute)
	}
	if _, err := os.Stat(filepath.Join(absolute, ".git")); err != nil {
		return "", fmt.Errorf(
			"%s is not a git working tree (deploy pins layers from git history; run 'git init' first)", absolute)
	}

	return absolute, nil
}

// isRepositoryURL reports whether `location` is a repository URL rather than a local path, by git-clone's
// own rules: any scheme (`://`), or the scp-like `[user@]host:path` — a colon before any slash with more
// than one character before it (a single character is a Windows drive letter).
//
// Parameters:
//   - `location`: the location operand to classify.
//
// Returns:
//   - `bool`: true for a repository URL.
func isRepositoryURL(location string) bool {

	if strings.Contains(location, "://") {
		return true
	}

	colon := strings.Index(location, ":")
	if colon <= 1 {
		return false
	}
	slash := strings.IndexAny(location, `/\`)
	return slash == -1 || colon < slash
}

// cloneRepository clones `url` to `destination` and returns the destination as an absolute path.
//
// The clone fully lands before anything registers; a failed clone into a destination this call created is
// removed best-effort, so nothing half-made survives. Clone output streams to the command's stderr — auth
// prompts and progress stay visible. After the clone, the repository is entirely the user's: writ performs
// no further git operations on it.
//
// Parameters:
//   - `cmd`: the invoking command; supplies the streams and context.
//   - `url`: the repository URL.
//   - `destination`: the clone destination; must not already exist.
//   - `branch`: the branch to clone, or "" for the remote's default.
//
// Returns:
//   - `string`: the absolute destination path.
//   - `error`: an existing destination or a clone failure.
func cloneRepository(cmd *cobra.Command, url, destination, branch string) (string, error) {

	absolute, err := filepath.Abs(destination)
	if err != nil {
		return "", err
	}

	if _, err := os.Lstat(absolute); err == nil {
		return "", fmt.Errorf("clone destination %s already exists", absolute)
	}

	arguments := []string{"clone"}
	if branch != "" {
		arguments = append(arguments, "--branch", branch)
	}
	arguments = append(arguments, url, absolute)

	//nolint:gosec // G204: git with the user's own url and destination — the command's purpose.
	clone := exec.CommandContext(cmd.Context(), "git", arguments...)
	clone.Stdout = cmd.ErrOrStderr()
	clone.Stderr = cmd.ErrOrStderr()

	if err := clone.Run(); err != nil {
		//nolint:errcheck // diagnose-ignored-error: best-effort cleanup of the half-made clone; see docs/architecture/2.8-eventing-infrastructure.md
		_ = os.RemoveAll(absolute)
		return "", fmt.Errorf("git clone %s: %w", url, err)
	}

	return absolute, nil
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

	link := filepath.Join(devlore.WritLayersDir(), layer)
	if _, err := os.Lstat(link); err != nil {
		return fmt.Errorf("layer %s is not registered", layer)
	}

	if err := os.Remove(link); err != nil {
		return err
	}

	return cli.Emit(cmd, RepoRegistration{Layer: layer, State: repoStateUnregistered})
}

// runRepoList prints every layer in order with its registration state.
//
// Parameters:
//   - `cmd`: the invoking command; supplies the output stream.
//
// Returns:
//   - `error`: a write failure on the output stream.
func runRepoList(cmd *cobra.Command) error {

	registrations := make([]RepoRegistration, 0, len(LayerOrder))
	for _, layer := range LayerOrder {
		registrations = append(registrations, repoRegistration(layer))
	}

	return cli.Emit(cmd, registrations)
}

// repoListLine renders one layer's registration line for the list report.
//
// Parameters:
//   - `layer`: the layer name to report on.
//
// Returns:
//   - `RepoRegistration`: the layer, the root it points at when there is one, and its state.
func repoRegistration(layer string) RepoRegistration {

	link := filepath.Join(devlore.WritLayersDir(), layer)

	info, err := os.Lstat(link)
	if err != nil {
		return RepoRegistration{Layer: layer, State: repoStateUnregistered}
	}

	target := link
	if info.Mode()&os.ModeSymlink != 0 {
		if target, err = os.Readlink(link); err != nil {
			return RepoRegistration{Layer: layer, State: repoStateUnreadable}
		}
	}

	if _, err := filepath.EvalSymlinks(link); err != nil {
		return RepoRegistration{Layer: layer, Root: target, State: repoStateBroken}
	}

	return RepoRegistration{Layer: layer, Root: target, State: repoStateRegistered}
}
