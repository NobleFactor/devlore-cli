// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package cli

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"slices"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
	"github.com/NobleFactor/devlore-cli/pkg/iox"
	"github.com/NobleFactor/devlore-cli/pkg/process"
)

// `self upgrade` moves every devlore program in a prefix to one release (#947). It decides what to upgrade to, finds
// that release on GitHub or takes it from `--from`, and does nothing more when every program is already at it.
// Otherwise it fetches the release into a scratch directory, verifies it, unpacks it, and, for each program, places
// the new binary and hands the program to its own `self install`, which replaces the program's record. The scratch
// directory goes when the command does, interrupted or not.

var (
	// shippedPrograms are the programs a release archive carries, the Makefile's PRODUCTS, in the order an upgrade
	// installs them.
	//
	// The suite is decided before anything is downloaded, so this list, not the archive, says which programs a
	// prefix can hold; [carriesSuite] checks the archive against the suite once it is unpacked.
	shippedPrograms = []string{"lore", "star", "writ"}

	// upgradeEnvironmentFor returns what a `self upgrade` reaches beyond its own process. A package value, so that a
	// test can stand GitHub, the platform, the running executable and the children in for the real ones.
	upgradeEnvironmentFor = currentUpgradeEnvironment
)

// region SUPPORTING TYPES

// upgradeEnvironment is what a `self upgrade` reaches beyond its own process: GitHub, the platform it runs on, the
// executable it runs as, and the programs it starts.
type upgradeEnvironment struct {
	apiBase    string                                                             // the repository's API root
	client     *http.Client                                                       // every request goes through it
	token      string                                                             // GH_TOKEN; "" asks anonymously
	goos       string                                                             // the platform's operating system
	goarch     string                                                             // the platform's architecture
	executable string                                                             // the running program's path
	runChild   func(ctx context.Context, dir, binary string, args []string) error // runs one `self install`
}

// upgradeRequest is what one `self upgrade` was asked to do.
type upgradeRequest struct {
	program string        // the running program, which the suite holds whatever its manifest says
	prefix  string        // the installation prefix, as [upgradePrefix] found it from the running binary
	version string        // the running build's version, the result's `from`
	target  upgradeTarget // what decides the release
	from    string        // `--from`'s archive; read only when the target's mode is upgradeModeArchive
	shells  []string      // `--shell`, passed through to every program's `self install`
	dryRun  bool          // `--dry-run`: stop before placing a binary or starting a child
}

// upgradeReport is `self upgrade`'s result.
type upgradeReport struct {
	From       string   `json:"from"`       // the version of the build that ran the upgrade
	To         string   `json:"to"`         // the release the suite is upgraded to, or already at
	Channel    string   `json:"channel"`    // the channel that decided it; "" for a pin or an archive
	Prerelease bool     `json:"prerelease"` // whether that channel's pre-releases counted
	Prefix     string   `json:"prefix"`     // the installation prefix
	Programs   []string `json:"programs"`   // the suite: every shipped program the prefix holds
}

// endregion

// region HELPER FUNCTIONS

// Fallible actions

// currentUpgradeEnvironment returns what a `self upgrade` reaches in an ordinary run: GitHub, with GH_TOKEN when it
// is set; this platform; this executable; and each program's `self install`, run by [runUpgradeChild].
//
// Returns:
//   - `upgradeEnvironment`: the environment.
//   - `error`: the running executable cannot be found.
func currentUpgradeEnvironment() (upgradeEnvironment, error) {

	executable, err := os.Executable()
	if err != nil {
		return upgradeEnvironment{}, fmt.Errorf("cannot find the running executable: %w", err)
	}

	return upgradeEnvironment{
		apiBase:    githubAPIBase,
		client:     &http.Client{},
		token:      os.Getenv(tokenVariable),
		goos:       runtime.GOOS,
		goarch:     runtime.GOARCH,
		executable: executable,
		runChild:   runUpgradeChild,
	}, nil
}

// decideUpgrade decides what a `self upgrade` upgrades to, and says what the decision set aside.
//
// Parameters:
//   - `cmd`: the `self upgrade` command, its flags parsed.
//   - `info`: the program's install metadata, carrying the build's stamped channel.
//
// Returns:
//   - `upgradeTarget`: what the run upgrades to.
//   - `error`: [resolveUpgradeTarget]'s refusal.
func decideUpgrade(cmd *cobra.Command, info SelfInstallInfo) (upgradeTarget, error) {

	target, err := resolveUpgradeTarget(gatherUpgradeInputs(cmd, info))
	if err != nil {
		return upgradeTarget{}, err
	}

	if target.note != "" {
		Note("%s", target.note)
	}

	return target, nil
}

// fetchRelease fetches a release's archive and checksums file into scratch, verifies the archive, and unpacks it.
//
// Nothing is unpacked until the archive is verified (#947, Requirement 5), and nothing is returned until the unpacked
// archive is known to carry every program of the suite.
//
// Parameters:
//   - `ctx`: the run's context; canceling it stops a download.
//   - `env`: what the upgrade reaches.
//   - `scratch`: the run's scratch directory.
//   - `release`: the release.
//   - `suite`: the programs the prefix holds.
//
// Returns:
//   - `fsroot.Path`: the unpacked package, with every program of the suite in `pkg/bin`.
//   - `error`: [fetchFile]'s, [verifyArchive]'s, [unpackArchive]'s or [carriesSuite]'s.
func fetchRelease(
	ctx context.Context, env upgradeEnvironment, scratch fsroot.Dir, release upgradeRelease, suite []string,
) (fsroot.Path, error) {

	if _, err := fetchFile(ctx, env, scratch, release.checksums); err != nil {
		return fsroot.Path{}, err
	}

	sum, err := fetchFile(ctx, env, scratch, release.archive)
	if err != nil {
		return fsroot.Path{}, err
	}

	checksums, err := scratch.ReadFile(scratch.NewPath(release.checksums.name))
	if err != nil {
		return fsroot.Path{}, err
	}

	if err := verifyArchive(release.archive, sum, checksums, release.checksums.name); err != nil {
		return fsroot.Path{}, err
	}
	Note("Verified %s: its SHA-256 is %s", release.archive.name, sum)

	pkg, err := unpackArchive(scratch, release.archive.name)
	if err != nil {
		return fsroot.Path{}, err
	}

	if err := carriesSuite(scratch, pkg, release.archive.name, suite); err != nil {
		return fsroot.Path{}, err
	}

	return pkg, nil
}

// installSuite upgrades each program of the suite in turn: it places the program's new binary, then hands the
// program to its own `self install` (#947, Requirement 7).
//
// Placing the binary first is what lets any build install: the child then finds a target that is not running,
// whatever build it is. The child runs from `pkg/`, where it finds what it installs beside its binary. The first
// program that fails stops the run, by name; nothing is rolled back, and a rerun finishes the job. An interrupt stops
// it too, before the next program's binary is placed: nothing is placed under a canceled context, where no child
// could start to install it (#947, Requirement 6).
//
// Parameters:
//   - `ctx`: the run's context; canceling it stops a child, and the run before the next program.
//   - `env`: what the upgrade reaches; its runner starts the children.
//   - `prefixRoot`: the installation prefix.
//   - `pkg`: the unpacked package, with the programs in `pkg/bin`.
//   - `suite`: the programs, in order.
//   - `shells`: `--shell`, passed through to every child.
//   - `tag`: the release, for the narration and a failure.
//
// Returns:
//   - `error`: the first program's failure to be placed or to install itself, or the interrupt that stopped the run
//     before it, naming the program.
func installSuite(
	ctx context.Context, env upgradeEnvironment, prefixRoot fsroot.Dir, pkg fsroot.Path, suite, shells []string,
	tag string,
) error {

	for _, program := range suite {
		if ctx.Err() != nil {
			return stoppedAt(program, tag, context.Cause(ctx))
		}

		binary := filepath.Join(pkg.Abs(), "bin", executableName(program))
		Note("Upgrading %s to %s", program, tag)

		if _, err := replaceBinary(prefixRoot, binary, program); err != nil {
			return stoppedAt(program, tag, err)
		}

		if err := env.runChild(ctx, pkg.Abs(), binary, installArgs(prefixRoot.Name(), shells)); err != nil {
			return stoppedAt(program, tag, fmt.Errorf("its self install failed: %w", err))
		}
	}

	return nil
}

// resolveRelease finds the release a run installs: the archive `--from` names, or the release on GitHub the target
// names, and says which it is.
//
// Parameters:
//   - `ctx`: the run's context.
//   - `env`: what the upgrade reaches.
//   - `request`: the run's request.
//
// Returns:
//   - `upgradeRelease`: the release's tag and its two files.
//   - `error`: [localRelease]'s, [findRelease]'s or [releaseFiles]'s.
func resolveRelease(ctx context.Context, env upgradeEnvironment, request upgradeRequest) (upgradeRelease, error) {

	if request.target.mode == upgradeModeArchive {
		release, err := localRelease(request.from, env.goos, env.goarch)
		if err != nil {
			return upgradeRelease{}, err
		}
		Note("%s is release %s", request.from, release.tag)
		return release, nil
	}

	found, err := findRelease(ctx, env, request.target)
	if err != nil {
		return upgradeRelease{}, err
	}

	switch {
	case request.target.mode == upgradeModePin:
		Note("%s pins release %s", pinVariable, found.TagName)
	case request.target.channel == channelRelease && request.target.prerelease:
		Note("The newest release or pre-release on the release channel is %s", found.TagName)
	default:
		Note("The newest release on the %s channel is %s", request.target.channel, found.TagName)
	}

	return releaseFiles(found, env.goos, env.goarch)
}

// runSelfUpgrade runs `self upgrade`: it finds the prefix, decides what to upgrade to, upgrades the suite, and emits
// the result.
//
// The running program is refused before anything is decided (#947, Requirement 7): a decision's advice, or its note
// of a source it set aside, would mislead a run that cannot go ahead whatever is decided. The run is interruptible:
// an interrupt or a SIGTERM cancels the download and any child, and the command returns through the scratch
// directory's removal rather than dying with it on disk (#947, Requirement 6).
//
// Parameters:
//   - `cmd`: the `self upgrade` command, its flags parsed.
//   - `info`: the program's install metadata: its name, version and stamped channel.
//   - `from`: `--from`'s archive; "" when it was not given.
//   - `shells`: `--shell`'s shells.
//
// Returns:
//   - `error`: [upgradePrefix]'s refusal of the running program, the decision's refusal, the upgrade's failure, or the
//     result's rendering.
func runSelfUpgrade(cmd *cobra.Command, info SelfInstallInfo, from string, shells []string) error {

	env, err := upgradeEnvironmentFor()
	if err != nil {
		return err
	}

	prefix, err := upgradePrefix(env.executable, info.Name)
	if err != nil {
		return err
	}

	target, err := decideUpgrade(cmd, info)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	report, err := upgrade(ctx, env, upgradeRequest{
		program: info.Name,
		prefix:  prefix,
		version: info.Version,
		target:  target,
		from:    from,
		shells:  shells,
		dryRun:  viper.GetBool(info.Name + ".dry-run"),
	})
	if err != nil {
		return err
	}

	return Emit(cmd, report)
}

// runUpgradeChild runs one program's `self install`, its narration relayed as notes.
//
// The child is captured and never sees the terminal (10-command-line-interface.md §10, ruled 2026-09-03). It narrates
// on stderr, as every program on the shared root does, so its stderr is relayed as notes, not warnings; its exit
// status says whether it failed. Canceling `ctx` kills it.
//
// Parameters:
//   - `ctx`: the run's context.
//   - `dir`: the directory it runs in.
//   - `binary`: the program to run.
//   - `args`: its arguments.
//
// Returns:
//   - `error`: [process.Runner.RunNarrating]'s, naming the program and its exit status when it fails.
func runUpgradeChild(ctx context.Context, dir, binary string, args []string) error {

	// The program is the release's own, verified and unpacked into this run's scratch directory.
	child := exec.CommandContext(ctx, binary, args...) //nolint:gosec // G204: the release's verified program
	child.Dir = dir

	return process.NewRunner(ctx, false, nil, UI()).RunNarrating(child)
}

// stoppedAt says that the run stopped at `program`, and what finishes the job.
//
// Parameters:
//   - `program`: the program the run stopped at.
//   - `tag`: the release it was being upgraded to.
//   - `err`: why.
//
// Returns:
//   - `error`: the failure, naming the program.
func stoppedAt(program, tag string, err error) error {
	return fmt.Errorf("upgrading %s to %s: %w (nothing is rolled back; running self upgrade again finishes the job)",
		program, tag, err)
}

// upgrade moves every shipped program the prefix holds to one release (#947, Requirements 3 to 8, 10 and 11).
//
// When every program is already at the release, it says so, downloads nothing and changes nothing. Under `--dry-run`
// it fetches and verifies the release into scratch, says what it would do, and stops before placing a binary or
// starting a child, because `self install` itself writes whatever it is told. An interrupt that lands once the
// release is fetched, while it is verified or unpacked, stops the run there, dry or not. The scratch directory is
// removed on every return.
//
// Parameters:
//   - `ctx`: the run's context; canceling it stops a download, a child, or the run before it places a binary.
//   - `env`: what the upgrade reaches.
//   - `request`: what the run was asked to do.
//
// Returns:
//   - `upgradeReport`: the result.
//   - `error`: the first failure to find, fetch, verify, unpack or install the release, or to remove the scratch
//     directory; or the interrupt that stopped the run.
func upgrade(ctx context.Context, env upgradeEnvironment, request upgradeRequest) (report upgradeReport, err error) {

	prefix := request.prefix

	prefixRoot, err := fsroot.OpenExisting(prefix)
	if err != nil {
		return upgradeReport{}, err
	}
	defer iox.Close(&err, prefixRoot)

	suite := heldPrograms(prefixRoot, request.program)

	release, err := resolveRelease(ctx, env, request)
	if err != nil {
		return upgradeReport{}, err
	}

	report = upgradeReport{From: request.version, To: release.tag, Channel: request.target.channel,
		Prerelease: request.target.prerelease, Prefix: prefix, Programs: suite}

	if allCurrent(prefixRoot, suite, release.tag) {
		Note("Nothing to upgrade: %s in %s already at %s", joinPrograms(suite), prefix, release.tag)
		return report, nil
	}

	scratch, err := fsroot.OpenScratch("devlore-upgrade-*")
	if err != nil {
		return upgradeReport{}, err
	}
	defer iox.Close(&err, scratch)

	pkg, err := fetchRelease(ctx, env, scratch, release, suite)
	if err != nil {
		return upgradeReport{}, err
	}

	// Verifying and unpacking do not watch the context, so an interrupt that landed while they ran is honored here.
	if ctx.Err() != nil {
		return upgradeReport{}, context.Cause(ctx)
	}

	if request.dryRun {
		Note("Dry run: would upgrade %s in %s to %s; nothing was changed", joinPrograms(suite), prefix, release.tag)
		return report, nil
	}

	if err := installSuite(ctx, env, prefixRoot, pkg, suite, request.shells, release.tag); err != nil {
		return upgradeReport{}, err
	}

	Success("Upgraded %s in %s to %s", joinPrograms(suite), prefix, release.tag)

	return report, nil
}

// upgradePrefix finds the installation prefix from the running binary, refusing a run the upgrade cannot serve.
//
// The binary is `<prefix>/bin/<program>`, its links resolved, as [installedPrefixOf] finds it for `self uninstall`
// too. A program no release carries is refused: devlore-test is a tool, built, not shipped. So is a binary renamed
// away from its program's name, which the upgrade, placing each program under its own, would leave as it is while
// reporting success.
//
// Parameters:
//   - `executable`: the running binary's path.
//   - `program`: the running program's name.
//
// Returns:
//   - `string`: the installation prefix.
//   - `error`: an [ExitUsage] error for a program no release carries; an [ExitConfig] error for a renamed binary; or
//     [installedPrefixOf]'s.
func upgradePrefix(executable, program string) (string, error) {

	if !slices.Contains(shippedPrograms, program) {
		return "", ExitWith(ExitUsage, fmt.Errorf("%s is not in a release: a release carries %s, and self upgrade "+
			"upgrades only those; %s is upgraded by building it", program, joinPrograms(shippedPrograms), program))
	}

	prefix, binary, err := installedPrefixOf(executable)
	if err != nil {
		return "", err
	}

	if filepath.Base(binary) != executableName(program) {
		return "", ExitWith(ExitConfig, fmt.Errorf("the running binary is %s, not %s: an upgrade places each "+
			"program under its own name, so this one would be left as it is; run %s from <prefix>/bin/%s",
			binary, executableName(program), program, executableName(program)))
	}

	return prefix, nil
}

// Actions

// allCurrent reports whether every program of the suite is at the release: its binary is in `bin/`, and its manifest
// names the release (#947, Requirement 8).
//
// A program's version is the one its manifest records, which its `self install` writes from its stamp. A manifest
// that is missing or cannot be read is not current, and neither is a program missing its binary, which a run
// interrupted between placing binaries leaves.
//
// Parameters:
//   - `prefixRoot`: the installation prefix.
//   - `suite`: the programs.
//   - `tag`: the release.
//
// Returns:
//   - `bool`: true when there is nothing to upgrade.
func allCurrent(prefixRoot fsroot.Dir, suite []string, tag string) bool {

	for _, program := range suite {
		if _, err := prefixRoot.Lstat(prefixRoot.NewPath("bin", executableName(program))); err != nil {
			return false
		}

		recorded, err := readManifest(prefixRoot.Name(), program)
		if err != nil || recorded.Version != tag {
			return false
		}
	}

	return true
}

// heldPrograms returns the suite: every shipped program the prefix owns, by its manifest, and the one running.
//
// The manifest is the record of what a program owns (#933), so a binary of a shipped program's name with no manifest
// is someone else's, `star` the tar archiver for one, and is left alone. One with a manifest and no binary, which an
// interrupted run leaves, is held, and the upgrade puts its binary back. The running program is held whatever its
// manifest says: it is the binary the upgrade runs from.
//
// Parameters:
//   - `prefixRoot`: the installation prefix.
//   - `running`: the program running the upgrade.
//
// Returns:
//   - `[]string`: the programs, in [shippedPrograms]' order.
func heldPrograms(prefixRoot fsroot.Dir, running string) []string {

	var held []string
	for _, program := range shippedPrograms {
		_, manifestErr := prefixRoot.Lstat(prefixRoot.NewPath(relativeManifestPath(program)))
		if manifestErr == nil || program == running {
			held = append(held, program)
		}
	}

	return held
}

// installArgs returns the arguments of a program's `self install` into `prefix`, `--shell` passed through.
//
// Parameters:
//   - `prefix`: the installation prefix.
//   - `shells`: `--shell`'s shells; none lets each program detect them, as every install does (#947, D8).
//
// Returns:
//   - `[]string`: the arguments.
func installArgs(prefix string, shells []string) []string {

	args := []string{"self", "install", prefix}
	for _, shell := range shells {
		args = append(args, "--shell", shell)
	}

	return args
}

// endregion
