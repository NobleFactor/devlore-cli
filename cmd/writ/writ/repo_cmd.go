// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package writ

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/NobleFactor/devlore-cli/cmd/internal/cli"
	"github.com/NobleFactor/devlore-cli/cmd/internal/devlore"
)

// newRepoCmd builds the repo command family: layer-repository registration through the layers directory.
//
// Registration is packaging, not configuration (the settled config-vs-layers separation): a layer is a
// symlink under [devlore.WritLayersDir], never a config.yaml key. The verbs are `set`, `unset` and `list`, with
// no aliases (#791); bare `writ repo` prints the group's help, as every group does.
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
		Example: `  writ repo set personal ~/Workspace/Personal
  writ repo list
  writ repo unset team`,
	}

	cmd.AddCommand(newRepoSetCmd())
	cmd.AddCommand(newRepoUnsetCmd())
	cmd.AddCommand(newRepoListCmd())

	return cmd
}

// newRepoSetCmd builds `repo set <layer> <working-tree-root>|<repository-url> [<working-tree-root>]`.
//
// `set`, not `add`, because a layer has exactly one registration: registering a layer that is already
// registered re-points it rather than failing, which is the vocabulary `config set` and `config unset`
// already use on this root for a keyed value (#791, ruled 2026-09-04).
func newRepoSetCmd() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "set <layer> <working-tree-root>|<repository-url> [<working-tree-root>]",
		Short: "Register a layer, or re-point one that is registered",
		Long: `Register a layer, or re-point one that is registered.

The location is a local working-tree-root, or a repository URL — which triggers a
git clone (git-clone's own grammar: the optional trailing working-tree-root is the
clone destination). Without one, the clone lands in the writ-owned home under
XDG_DATA_HOME/devlore/writ/repos, named as git clone names it: acme/team-env.git
clones to repos/team-env. Two layers whose repositories share a name are refused.
After placement the repository is entirely yours: writ performs no hidden git
operations, ever.`,
		Example: `  writ repo set personal ~/Workspace/Personal
  writ repo set team git@github.com:acme/team-env.git
  writ repo set personal git@github.com:me/personal.git ~/Workspace/Personal
  writ repo set personal git@github.com:me/personal.git --branch devlore-cli/writ-layer`,
		Args: cobra.RangeArgs(2, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			destination := ""
			if len(args) == 3 {
				destination = args[2]
			}
			branch, _ := cmd.Flags().GetString("branch") //nolint:errcheck // flag registered below
			return runRepoSet(cmd, args[0], args[1], destination, branch)
		},
	}

	cmd.Flags().String("branch", "", "Branch to clone (repository-url form only)")

	return cmd
}

// newRepoUnsetCmd builds `repo unset <layer>`.
//
// No alias. `rm` and `ls` went with the rename: this is a greenfield product, and a second spelling of a verb
// is a second thing to learn and to document.
func newRepoUnsetCmd() *cobra.Command {

	return &cobra.Command{
		Use:   "unset <layer>",
		Short: "Unregister a layer",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRepoUnset(cmd, args[0])
		},
	}
}

// newRepoListCmd builds `repo list`.
func newRepoListCmd() *cobra.Command {

	return &cobra.Command{
		Use:   "list",
		Short: "List layer registrations",
		Args:  cobra.NoArgs,
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
	State string `json:"state"`

	// The tree the registration points at. Promoted, so the record reads layer, state, source, root, owner.
	RepoTarget

	// Branch is the branch a URL form cloned, present only when `--branch` was given.
	Branch string `json:"branch,omitempty"`

	// Previous is the registration this one displaced, or the one `unset` removed. Absent when nothing was
	// there. The same shape as the target above, so one reader parses both; when its Owner is `writ`, its
	// Root is the clone that goes with it (#792).
	Previous *RepoTarget `json:"previous,omitempty"`
}

// RepoTarget is where a layer's working tree is, which repository it is, and whose tree it is.
//
// `Source` and `Root` answer different questions -- today's single root could not say which repository a
// layer was, nor whether its tree was writ's or the user's, and #792's ruling turns on exactly that. `Owner`
// is that ruling made machine-readable: a tree under the writ-owned home is writ's to remove; any other is
// never touched.
type RepoTarget struct {

	// Source is the URL writ would fetch from: the checked-out branch's upstream remote, else `origin`, else
	// empty. Empty is a real answer -- a repository made by `git init` and never pushed has no remote, and
	// writ's own refusal text tells the user to make exactly that.
	Source string `json:"source,omitempty"`

	// Root is the working tree writ reads.
	Root string `json:"root,omitempty"`

	// Owner is `writ` when Root lies under the writ-owned home and `user` otherwise.
	Owner string `json:"owner,omitempty"`
}

// Who owns a layer's working tree, as [RepoTarget.Owner] reports it.
const (
	repoOwnerWrit = "writ"
	repoOwnerUser = "user"
)

// The states a registration can be in, as [RepoRegistration.State] reports them.
const (
	repoStateRegistered   = "registered"
	repoStateUnregistered = "unregistered"
	repoStateBroken       = "broken"
	repoStateUnreadable   = "unreadable"
)

// runRepoSet registers `layer` at `location`, or re-points it there.
//
// A layer has exactly one registration, so registering one that is already registered is a replacement rather
// than a refusal (#791, ruled 2026-09-04). Setting a layer to the root it already has is neither: it narrates
// `unchanged` and emits the record.
//
// **Every refusal precedes the clone.** The old `add` resolved the location first -- cloning, for the URL form
// -- and only then discovered that the layer was registered, so a refusal arrived after a repository had been
// copied to disk and left there. [plannedRoot] answers where the tree will be without touching the network, so
// the decision to clone is taken after every reason not to.
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
//   - `error`: an unknown layer, a malformed combination, a clone name another layer already holds, a failed
//     clone, a non-working-tree root, or a filesystem failure.
func runRepoSet(cmd *cobra.Command, layer, location, destination, branch string) error {

	if !slices.Contains(LayerOrder, layer) {
		return fmt.Errorf("unknown layer %q (layers: base, team, personal)", layer)
	}

	root, clone, err := settledRoot(cmd.Context(), layer, location, destination, branch)
	if err != nil {
		return err
	}

	previous := repoRegistration(cmd.Context(), layer)

	if previous.State == repoStateRegistered && previous.Root == root {
		cli.Note("%s: unchanged, %s", layer, root)
		return cli.Emit(cmd, RepoRegistration{
			Layer:      layer,
			State:      repoStateRegistered,
			RepoTarget: repoTarget(cmd.Context(), root),
			Branch:     branch,
		})
	}

	record := RepoRegistration{Layer: layer, State: repoStateRegistered, Branch: branch}
	if previous.State != repoStateUnregistered {
		displaced := previous.RepoTarget
		record.Previous = &displaced
	}

	// A dry run emits what the real run would and does nothing: no clone, no symlink, no removal. The
	// target is described from its path alone, since a tree that would be cloned is not there to ask.
	if viper.GetBool("writ.dry-run") {
		record.RepoTarget = RepoTarget{Root: root, Owner: ownerOf(root)}
		narrateSet(layer, root, previous, true)
		return cli.Emit(cmd, record)
	}

	if clone {
		if _, err := cloneRepository(cmd, location, root, branch); err != nil {
			return err
		}
	} else if err := validateWorkingTree(root); err != nil {
		return err
	}

	if err := writeRegistration(layer, root); err != nil {
		return err
	}

	narrateSet(layer, root, previous, false)

	// The displaced tree goes with its registration when it was writ's own (#792). A tree the user registered
	// by path is theirs and stays.
	if record.Previous != nil && record.Previous.Owner == repoOwnerWrit {
		if err := removeWritClone(record.Previous.Root); err != nil {
			return err
		}
	}

	record.RepoTarget = repoTarget(cmd.Context(), root)

	return cli.Emit(cmd, record)
}

// writeRegistration points `layer` at `root`, replacing whatever registration was there.
//
// A registration is a symlink under the layers directory and nothing else (`repo_cmd.go`'s own design note):
// replacing one is removing the old link and writing the new, with no intermediate state a reader could
// observe as "registered nowhere".
//
// Parameters:
//   - `layer`: the layer name.
//   - `root`: the absolute working-tree-root.
//
// Returns:
//   - `error`: a filesystem failure.
func writeRegistration(layer, root string) error {

	layers := devlore.WritLayersDir()
	if err := os.MkdirAll(layers, 0o750); err != nil {
		return err
	}

	link := filepath.Join(layers, layer)
	if _, err := os.Lstat(link); err == nil {
		if err := os.Remove(link); err != nil {
			return err
		}
	}

	return os.Symlink(root, link)
}

// narrateSet says what `set` did, or under a dry run what it would do.
//
// Parameters:
//   - `layer`: the layer.
//   - `root`: where it now points, or would.
//   - `previous`: what was there before.
//   - `dryRun`: whether nothing happened.
func narrateSet(layer, root string, previous RepoRegistration, dryRun bool) {

	would := ""
	if dryRun {
		would = "would be "
	}

	switch {
	case previous.State == repoStateUnregistered:
		cli.Note("%s: %snow %s", layer, would, root)
	case previous.Owner == repoOwnerWrit:
		cli.Note("%s: %swas %s, now %s; the clone writ made at %s %sgoes with it",
			layer, would, previous.Root, root, previous.Root, would)
	default:
		cli.Note("%s: %swas %s, now %s", layer, would, previous.Root, root)
	}
}

// settledRoot is the root `set` will use, settled before anything is cloned: [plannedRoot]'s answer, refused
// when another layer's registration already holds it (#793).
//
// Parameters:
//   - `ctx`: for the git invocations that resolve the other registrations.
//   - `layer`: the layer being set.
//   - `location`: the polymorphic location operand.
//   - `destination`: the URL form's optional clone destination.
//   - `branch`: the URL form's optional branch.
//
// Returns:
//   - `string`: the absolute working-tree-root the registration will point at.
//   - `bool`: whether that root has to be cloned into being.
//   - `error`: [plannedRoot]'s refusals, or a clone destination another layer holds.
func settledRoot(ctx context.Context, layer, location, destination, branch string) (root string, clone bool, err error) {

	root, clone, err = plannedRoot(location, destination, branch)
	if err == nil && clone {
		err = sharedCloneRefusal(ctx, root, layer)
	}

	return root, clone, err
}

// plannedRoot answers where the layer's working tree will be, and whether getting it there needs a clone.
//
// Nothing here touches the network or the filesystem beyond resolving a path: it exists so that every refusal
// this command can make happens before a clone rather than after one. A URL without a destination clones into
// the writ-owned home under the name `git clone` would give it (#793): `repos/noblefactor-ops`, never
// `repos/base`, so the directory says which repository it holds.
//
// Parameters:
//   - `location`: the polymorphic location operand.
//   - `destination`: the URL form's optional clone destination.
//   - `branch`: the URL form's optional branch.
//
// Returns:
//   - `string`: the absolute working-tree-root the registration will point at.
//   - `bool`: whether that root has to be cloned into being.
//   - `error`: a malformed combination of operands, or a URL that yields no directory name.
func plannedRoot(location, destination, branch string) (root string, clone bool, err error) {

	if isRepositoryURL(location) {
		if destination == "" {
			name, err := humanishName(location)
			if err != nil {
				return "", false, err
			}
			destination = filepath.Join(devlore.WritReposDir(), name)
		}
		absolute, err := filepath.Abs(expandPath(destination))
		if err != nil {
			return "", false, err
		}
		return absolute, true, nil
	}

	if destination != "" {
		return "", false, fmt.Errorf("a working-tree-root takes no destination (got %q)", destination)
	}
	if branch != "" {
		return "", false, fmt.Errorf("--branch applies to the repository-url form only")
	}

	absolute, err := filepath.Abs(expandPath(location))
	if err != nil {
		return "", false, err
	}

	return absolute, false, nil
}

// validateWorkingTree refuses a root that is not a git working tree.
//
// Deploy plans against pinned git history, so a layer that is not a repository has nothing to pin.
//
// Parameters:
//   - `root`: the absolute working-tree-root.
//
// Returns:
//   - `error`: the root is missing, is not a directory, or holds no `.git`.
func validateWorkingTree(root string) error {

	info, err := os.Stat(root)
	if err != nil {
		return fmt.Errorf("working-tree-root: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("working-tree-root %s is not a directory", root)
	}
	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		return fmt.Errorf(
			"%s is not a git working tree (deploy pins layers from git history; run 'git init' first)", root)
	}

	return nil
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

// humanishName is the directory `git clone <url>` would create: the URL's last path component, with a trailing
// `/` and a `.git` suffix stripped -- git's own `guess_dir_name`, reduced to the URL forms [isRepositoryURL]
// admits (#793, ruled 2026-09-04).
//
// `git@github.com:NobleFactor/noblefactor-ops.git` names `noblefactor-ops`; `https://host/x/personal/` names
// `personal`; the scp-like `host:env`, with no slash, names `env`.
//
// Parameters:
//   - `url`: the repository URL.
//
// Returns:
//   - `string`: the directory name.
//   - `error`: the URL yields no name, which git refuses the same way.
func humanishName(url string) (string, error) {

	// Both separators, as [isRepositoryURL] reads them: git's `is_dir_sep` admits `\` on Windows, and the
	// tests there clone `file://C:\...`.
	name := strings.TrimRight(url, `/\`)
	name = strings.TrimSuffix(name, ".git")
	name = strings.TrimRight(name, `/\`)

	if separator := strings.LastIndexAny(name, `/\`); separator >= 0 {
		name = name[separator+1:]
	} else if colon := strings.LastIndex(name, ":"); colon >= 0 {
		name = name[colon+1:]
	}

	if name == "" || name == "." || name == ".." {
		return "", fmt.Errorf("no directory name can be derived from %q; name the clone destination", url)
	}

	return name, nil
}

// sharedCloneRefusal refuses a clone destination another layer's registration already points at.
//
// Two layers whose repositories share a name would clone to one directory. git refuses to clone into an
// existing one; writ refuses first, before the clone, and names both layers (#793). The layer being set is
// exempt: re-pointing it to the clone it already holds is `unchanged`, decided by the caller.
//
// Parameters:
//   - `ctx`: for the git invocations that resolve each registration.
//   - `root`: the clone destination about to be used.
//   - `layer`: the layer being set.
//
// Returns:
//   - `error`: another layer holds `root`.
func sharedCloneRefusal(ctx context.Context, root, layer string) error {

	for _, other := range LayerOrder {
		if other == layer {
			continue
		}
		registration := repoRegistration(ctx, other)
		if registration.State != repoStateUnregistered && registration.Root == root {
			return fmt.Errorf("%s and %s resolve to the same clone, %s: two layers cannot share one repository name",
				layer, other, root)
		}
	}

	return nil
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

// runRepoUnset unregisters `layer`, and says nothing if it was not registered.
//
// Idempotent by ruling (2026-09-18): a command that is safe to run twice is a command a script can use, and
// "it is not registered" is the state the caller asked for rather than a failure. Files are untouched here;
// removing the clone writ made for a layer is #792's work.
//
// Parameters:
//   - `cmd`: the invoking command; supplies the output stream.
//   - `layer`: the layer name; must be one of [LayerOrder].
//
// Returns:
//   - `error`: an unknown layer, or a filesystem failure.
func runRepoUnset(cmd *cobra.Command, layer string) error {

	if !slices.Contains(LayerOrder, layer) {
		return fmt.Errorf("unknown layer %q (layers: base, team, personal)", layer)
	}

	previous := repoRegistration(cmd.Context(), layer)
	if previous.State == repoStateUnregistered {
		return cli.Emit(cmd, RepoRegistration{Layer: layer, State: repoStateUnregistered})
	}

	displaced := previous.RepoTarget
	record := RepoRegistration{Layer: layer, State: repoStateUnregistered, Previous: &displaced}

	would := ""
	if viper.GetBool("writ.dry-run") {
		would = "would be "
	}

	if previous.Owner == repoOwnerWrit {
		cli.Note("%s: %sunregistered; the clone writ made at %s %sgoes with it", layer, would, previous.Root, would)
	} else {
		cli.Note("%s: %sunregistered, was %s; the tree is yours and stays", layer, would, previous.Root)
	}

	if would != "" {
		return cli.Emit(cmd, record)
	}

	link := filepath.Join(devlore.WritLayersDir(), layer)
	if err := os.Remove(link); err != nil {
		return err
	}

	if previous.Owner == repoOwnerWrit {
		if err := removeWritClone(previous.Root); err != nil {
			return err
		}
	}

	return cli.Emit(cmd, record)
}

// removeWritClone deletes a clone writ made in its own home, and refuses anything else.
//
// The ruling (#792): a clone writ made is writ's to remove; a working tree the user registered by path is never
// touched. The caller has already decided by [RepoTarget.Owner]; this decides again from the path, because the
// only function in this file that deletes a tree should not trust a field to say which tree.
//
// Parameters:
//   - `root`: the clone's root.
//
// Returns:
//   - `error`: the root is not a strict child of the writ-owned home, or the removal failed.
func removeWritClone(root string) error {

	if ownerOf(root) != repoOwnerWrit {
		return fmt.Errorf("refusing to remove %s: not a clone writ made in %s", root, devlore.WritReposDir())
	}

	return os.RemoveAll(root)
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
		registrations = append(registrations, repoRegistration(cmd.Context(), layer))
	}

	return cli.Emit(cmd, registrations)
}

// repoRegistration reports one layer's registration: its state, and when it points somewhere, the tree it
// points at with that tree's repository and owner.
//
// Parameters:
//   - `ctx`: for the git invocation that resolves the tree's remote.
//   - `layer`: the layer name to report on.
//
// Returns:
//   - `RepoRegistration`: the layer, its state, and its target when there is one.
func repoRegistration(ctx context.Context, layer string) RepoRegistration {

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
		return RepoRegistration{Layer: layer, State: repoStateBroken, RepoTarget: repoTarget(ctx, target)}
	}

	return RepoRegistration{Layer: layer, State: repoStateRegistered, RepoTarget: repoTarget(ctx, target)}
}

// repoTarget describes the tree at `root`: where it is, which repository it is, and whose it is.
//
// Parameters:
//   - `ctx`: for the git invocation.
//   - `root`: the working-tree-root.
//
// Returns:
//   - `RepoTarget`: root, source and owner. Source is empty when the tree has no remote or git cannot read it.
func repoTarget(ctx context.Context, root string) RepoTarget {

	return RepoTarget{Source: sourceOf(ctx, root), Root: root, Owner: ownerOf(root)}
}

// ownerOf reports whose tree `root` is.
//
// A tree under the writ-owned home is one writ cloned, and so one writ may remove (#792). Decided with
// [filepath.Rel] rather than a prefix test, so a sibling whose name merely begins the same way is not misread
// as writ's -- and the home itself is not writ's either: only a strict child is, because the answer decides
// what `unset` may delete, and the home is every layer's clone at once.
//
// Parameters:
//   - `root`: the working-tree-root.
//
// Returns:
//   - `string`: [repoOwnerWrit] or [repoOwnerUser].
func ownerOf(root string) string {

	relative, err := filepath.Rel(devlore.WritReposDir(), root)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return repoOwnerUser
	}

	return repoOwnerWrit
}

// sourceOf reports the URL writ would fetch from for the tree at `root`.
//
// The checked-out branch's upstream remote first, then `origin`, then nothing. Empty is an answer, not a
// failure: a repository made by `git init` and never pushed has no remote, and a tree git cannot read has no
// answer to give -- neither is a reason to fail the command that asked.
//
// Parameters:
//   - `ctx`: for the git invocations.
//   - `root`: the working-tree-root.
//
// Returns:
//   - `string`: the remote's URL, or empty.
func sourceOf(ctx context.Context, root string) string {

	remote := "origin"
	if upstream, err := gitOutput(ctx, root, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}"); err == nil {
		if name, _, found := strings.Cut(upstream, "/"); found && name != "" {
			remote = name
		}
	}

	url, err := gitOutput(ctx, root, "remote", "get-url", remote)
	if err != nil {
		return ""
	}

	return url
}

// gitOutput runs git in `root` and returns its trimmed standard output.
//
// Parameters:
//   - `ctx`: the invocation's context.
//   - `root`: the working tree to run in.
//   - `arguments`: git's arguments.
//
// Returns:
//   - `string`: standard output, trimmed.
//   - `error`: git failed, or is not installed.
func gitOutput(ctx context.Context, root string, arguments ...string) (string, error) {

	//nolint:gosec // G204: git in the user's own working tree, reading its configuration -- the command's purpose.
	command := exec.CommandContext(ctx, "git", append([]string{"-C", root}, arguments...)...)
	command.Stderr = nil

	output, err := command.Output()
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(output)), nil
}
