// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package git

import (
	"os"
	"os/exec"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

var _ op.Provider = (*Provider)(nil) // Interface Guard: ensures *Provider implements op.Provider.

// Provider provides git actions.
//
// +devlore:access=planned
type Provider struct {
	op.ProviderBase

	// cloneFn is a test hook that, when non-nil, receives the full argv for `git clone ...` and returns the
	// error the real `git` would have returned. Nil means exec the real git binary.
	cloneFn func(args []string) error
}

// NewProvider constructs a Provider bound to ctx.
//
// Parameters:
//   - ctx: execution context.
//
// Returns:
//   - *Provider: the initialized provider.
func NewProvider(ctx *op.RuntimeEnvironment) *Provider {
	return &Provider{ProviderBase: op.NewProviderBase(ctx)}
}

// region EXPORTED METHODS

// region Behaviors

// Compensable actions

// Clone clones a repository into a newly-created directory.
//
// Identity for the cloned repository is constructed by [NewResource] from directory; operational metadata (Remotes,
// Bare, Dirty, HEAD) is populated by [Resource.Resolve] after the clone completes. When directory is empty, the
// directory name is derived from repository via [guessDirName] — the same algorithm git itself uses for `git clone
// <repository>` with no explicit directory.
//
// The nine named options correspond one-to-one with `git clone` flags under the kwarg-to-flag rule (strip leading `--`,
// convert `-` to `_`, always expect a value — `--no-tags` becomes `no_tags=<bool>`). Any additional options a caller
// needs pass through kwargs and are translated using the same rule in reverse; see [buildCloneArgs].
//
// Parameters:
//   - repository:        remote git URL (HTTPS, SSH, git protocol, or local path) to clone from.
//   - directory:         local filesystem path where the repository will be cloned; empty defers to git's
//     own naming algorithm via [guessDirName].
//   - bare:              when true, emits `--bare` — bare repository (no working tree).
//   - branch:            when non-empty, emits `--branch <branch>` — branch, tag, or ref to check out.
//   - depth:             when > 0, emits `--depth <depth>` — shallow clone with truncated history.
//   - filter:            when non-empty, emits `--filter=<filter>` — partial-clone filter specification.
//   - noCheckout:        when true, emits `--no-checkout` — populate `.git/` but leave the working tree empty.
//   - noTags:            when true, emits `--no-tags` — do not fetch tags.
//   - origin:            when non-empty, emits `--origin <origin>` — name to use for the upstream remote in
//     place of "origin".
//   - recurseSubmodules: when true, emits `--recurse-submodules` — initialize and clone submodules recursively.
//   - singleBranch:      when true, emits `--single-branch` — fetch only the specified branch's history.
//   - kwargs:            catch-all for any `git clone` option not in the named set; each entry becomes an
//     additional flag per the kwarg-to-flag rule.
//
// Returns:
//   - *Resource: the cloned git.Resource with populated metadata.
//   - *Resource: the compensation handle — the same [*Resource] as the first return, passed to
//     [Provider.CompensateClone] to reverse the clone. Git's Clone creates a directory rather than
//     displacing one, so per the Tombstone rule (a tombstone exists for any object moved to a
//     RecoverySite) there is no git tombstone; the compensation handle is the created Resource itself.
//     Nil on error from `git clone` or resource construction; non-nil when the directory exists on disk
//     even if [Resource.Resolve] failed afterward.
//   - error:     any error from directory derivation, resource construction, or the underlying `git clone`.
//
// +devlore:defaults directory="",bare=false,branch="",depth=0,filter="",noCheckout=false,noTags=false,origin="",recurseSubmodules=false,singleBranch=false
func (p *Provider) Clone(
	activationRecord *op.ActivationRecord,
	repository string,
	directory string,
	bare bool,
	branch string,
	depth int,
	filter string,
	noCheckout bool,
	noTags bool,
	origin string,
	recurseSubmodules bool,
	singleBranch bool,
	kwargs map[string]any,
) (*Resource, *Receipt, error) {

	if directory == "" {
		guessed, err := guessDirName(repository)
		if err != nil {
			return nil, nil, err
		}
		directory = guessed
	}

	destination, err := NewResource(activationRecord, directory)
	if err != nil {
		return nil, nil, err
	}

	args := buildCloneArgs(
		repository,
		destination.SourcePath.Abs(),
		bare,
		branch,
		depth,
		filter,
		noCheckout,
		noTags,
		origin,
		recurseSubmodules,
		singleBranch,
		kwargs,
	)

	if err := p.doClone(args); err != nil {
		return nil, nil, err
	}

	if err := destination.Resolve(); err != nil {
		return destination, nil, err
	}

	return destination, NewReceipt(destination), nil
}

// CompensateClone removes the cloned directory.
//
// Clone is a Bucket-B action: the cloned tree is creation, not displacement, so there is nothing to restore from
// [op.RecoverySite] — compensation just removes the directory. A nil receipt is a no-op (Clone never produced a
// resource to reverse).
//
// Parameters:
//   - receipt: the [*Receipt] returned by [Provider.Clone]; may be nil.
//
// Returns:
//   - error: any error from [os.RemoveAll] on the cloned directory; nil when receipt or its resource is nil.
func (p *Provider) CompensateClone(receipt *Receipt) error {

	if receipt == nil {
		return nil
	}

	resource, ok := receipt.Resource().(*Resource)
	if !ok || resource == nil {
		return nil
	}

	return os.RemoveAll(resource.SourcePath.Abs())
}

// Fallible actions

// Checkout checks out a ref in the given repository directory.
//
// After `git checkout` succeeds, [Resource.Resolve] is called to re-read on-disk state — per the "Resolve
// resolves all metadata, no exceptions" rule, Ref/HEAD/Dirty/Remotes come from `.git/`, not from direct
// assignment in this method's body.
//
// Parameters:
//   - repo: git resource identifying the local repository.
//   - ref:  branch, tag, or commit to check out.
//
// Returns:
//   - *Resource: the repository resource with metadata refreshed by Resolve.
//   - error:     any error from `git checkout` or from the subsequent Resolve.
func (p *Provider) Checkout(repo *Resource, ref string) (*Resource, error) {

	cmd := exec.Command("git", "-C", repo.SourcePath.Abs(), "checkout", ref)

	if err := p.RuntimeEnvironment().Run(cmd); err != nil {
		return nil, err
	}

	if err := repo.Resolve(); err != nil {
		return repo, err
	}

	return repo, nil
}

// Pull pulls the latest changes in the given repository directory.
//
// After `git pull` succeeds, [Resource.Resolve] is called to re-read on-disk state — HEAD and Dirty may
// have changed.
//
// Parameters:
//   - repo: git resource identifying the local repository.
//
// Returns:
//   - *Resource: the repository resource with metadata refreshed by Resolve.
//   - error:     any error from `git pull` or from the subsequent Resolve.
func (p *Provider) Pull(repo *Resource) (*Resource, error) {

	cmd := exec.Command("git", "-C", repo.SourcePath.Abs(), "pull")

	if err := p.RuntimeEnvironment().Run(cmd); err != nil {
		return nil, err
	}

	if err := repo.Resolve(); err != nil {
		return repo, err
	}

	return repo, nil
}

// endregion

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// Fallible actions

// doClone runs `git args...` via the real git binary, or via the cloneFn test hook when one is installed.
//
// Parameters:
//   - args: the complete argv (starting with "clone"), as produced by [buildCloneArgs].
//
// Returns:
//   - error: any error from running the clone.
func (p *Provider) doClone(args []string) error {

	if p.cloneFn != nil {
		return p.cloneFn(args)
	}

	cmd := exec.Command("git", args...)
	return p.RuntimeEnvironment().Run(cmd)
}

// endregion

// endregion
