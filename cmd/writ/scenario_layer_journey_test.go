// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

// The layer-journey scenario (docs/plans/feature/855-layer-move-scenario.md, #855): the real writ binary,
// in a pristine sandbox, walks the whole journey a machine takes — `self install`; `repo set` for base,
// team and personal by path and by URL; a bare `deploy` that converges the implicit set; `deploy thenobles`
// on top and remembered; and then the move of 2026-09-07, when the base took over Declare-BashScript and
// the personal layer renamed its project and moved its consumers out of common.
//
// Every step is the ruled interface. Where today's binary lacks it, the step skips by the name of the issue
// that ships it (a capability probe, never a version), and the run prints its skip-list at the end. Where the
// ruling is about what deploy leaves behind, the step asserts today's behavior with the issue in its failure
// message, so the fix's landing is visible as this scenario failing at that step.
//
// Gated behind WRIT_SCENARIO_RUN=1 (the test-scenario make target), like the first scenario, whose sandbox,
// runner and assertions this file reuses and does not modify.
package main_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------------------------------------
// The journey sandbox
// ---------------------------------------------------------------------------------------------------------

// journeyLayer is one layer repository materialized in the sandbox: a committed working tree, named as the
// real repository is named so that #793's clone naming and #850's implicit projects see the real names, and
// a bare clone that stands in for "the URL".
type journeyLayer struct {
	Role    string // base, team, personal
	Name    string // noblefactor-ops, devlore-cli, personal — the repository name, which is the project name
	Path    string // the working tree
	Bare    string // the bare clone; file://<Bare> is the URL
	Fixture string // the checked-in tree it was materialized from
}

// journey is the whole sandbox: the first scenario's sandbox shape (so runWrit and the assertions apply
// unchanged), the three layers, the capability probes, and the skip-list the run prints at the end.
type journey struct {
	sandbox *scenarioSandbox
	layers  map[string]*journeyLayer // by role
	caps    capabilities
	skips   []string
}

// capabilities is what the binary under test can do today, probed rather than assumed.
type capabilities struct {
	repoSet         bool // #791: `writ repo set` / `unset`
	bareDeploy      bool // #843/#850: `writ deploy` with no project converges the implicit set
	implicitByName  bool // #850: a repository-named project deploys unnamed
	refresh         bool // #812: a verb that brings a writ-made clone forward
	decommissionAll bool // #851: `writ decommission --all`, re-converge, refusal by name
}

const (
	issueSelfInstallPlaceholders = 840
	issueRepoSetUnset            = 791
	issueRemoveLeavesClone       = 792
	issueCloneNaming             = 793
	issueRefreshClone            = 812
	issueBareDeploy              = 843
	issueImplicitProjects        = 850
	issueDecommission            = 851
	issueOrphans                 = 845
	issueDirtyAtRoot             = 852
	issueDryRunPreflight         = 853
	issueCollisionReport         = 470
)

// layerFixtures maps each role to the repository name and the checked-in tree that seeds it.
var layerFixtures = []journeyLayer{
	{Role: "base", Name: "noblefactor-ops", Fixture: "base"},
	{Role: "team", Name: "devlore-cli", Fixture: "team"},
	{Role: "personal", Name: "personal", Fixture: "personal-a"},
}

// newJourney builds the sandbox with every layer materialized and none registered. Registration is a step of
// the journey, not of the harness.
func newJourney(t *testing.T) *journey {

	t.Helper()

	if os.Getenv("WRIT_SCENARIO_RUN") == "" {
		t.Skip("scenario harness runs under make test-scenario (WRIT_SCENARIO_RUN=1)")
	}

	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(root, "home")
	for _, dir := range []string{home, filepath.Join(root, "config"), filepath.Join(root, "state"), filepath.Join(root, "data"), filepath.Join(root, "cache"), filepath.Join(root, "remotes")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	binDir := filepath.Dir(writBinary(t))
	sandbox := &scenarioSandbox{
		Root: root,
		Home: home,
		Env: []string{
			"PATH=" + binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
			"HOME=" + home,
			"USERPROFILE=" + home,
			"XDG_CONFIG_HOME=" + filepath.Join(root, "config"),
			"XDG_STATE_HOME=" + filepath.Join(root, "state"),
			"XDG_DATA_HOME=" + filepath.Join(root, "data"),
			"XDG_CACHE_HOME=" + filepath.Join(root, "cache"),
			"TMPDIR=" + os.TempDir(),
		},
	}
	writeTargetConfig(t, filepath.Join(root, "config"), home)

	j := &journey{sandbox: sandbox, layers: map[string]*journeyLayer{}}
	for _, fixture := range layerFixtures {
		layer := fixture
		layer.Path = filepath.Join(root, "Workspace", layer.Name)
		layer.Bare = filepath.Join(root, "remotes", layer.Name+".git")
		materializeLayer(t, &layer)
		j.layers[layer.Role] = &layer
	}
	sandbox.Repo = j.layers["personal"].Path

	t.Cleanup(func() {
		if len(j.skips) == 0 {
			t.Logf("layer-journey skip-list: empty — every ruled step ran")
			return
		}
		sort.Strings(j.skips)
		t.Logf("layer-journey skip-list (%d), the rulings still outstanding on %s/%s:\n  %s",
			len(j.skips), runtime.GOOS, runtime.GOARCH, strings.Join(j.skips, "\n  "))
	})

	return j
}

// materializeLayer copies a fixture tree into the sandbox, restores what a checked-in tree cannot carry
// (executable bits, the bridge symlink), commits it, and makes the bare clone that stands in for its URL.
func materializeLayer(t *testing.T, layer *journeyLayer) {

	t.Helper()

	if err := os.MkdirAll(layer.Path, 0o755); err != nil {
		t.Fatal(err)
	}
	copyFixture(t, filepath.Join("testdata", "layer-journey", layer.Fixture), layer.Path)
	markScriptsExecutable(t, layer.Path)

	if layer.Fixture == "personal-a" {
		// The bridge the 2026-08 migration left: local/bin/Declare-BashScript -> ../../.local/bin/Declare-BashScript.
		// Created here rather than checked in, because a Windows checkout would flatten a symlink.
		bridgeDir := filepath.Join(layer.Path, "Home", "common", "local", "bin")
		if err := os.MkdirAll(bridgeDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join("..", "..", ".local", "bin", "Declare-BashScript"), filepath.Join(bridgeDir, "Declare-BashScript")); err != nil {
			t.Fatalf("bridge symlink: %v", err)
		}
	}

	initializeRepo(t, layer.Path)
	gitIn(t, layer.Path, "clone", "--bare", "--quiet", layer.Path, layer.Bare)
	gitIn(t, layer.Path, "remote", "add", "origin", layer.Bare)
}

// markScriptsExecutable restores the executable bit copyFixture drops on every file under a bin/ directory.
func markScriptsExecutable(t *testing.T, root string) {

	t.Helper()

	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		if filepath.Base(filepath.Dir(path)) == "bin" {
			return os.Chmod(path, 0o755)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// gitIn runs git in `dir` under the isolated configuration, failing the test on a non-zero exit.
func gitIn(t *testing.T, dir string, args ...string) {

	t.Helper()

	cmd := exec.CommandContext(context.Background(), "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(isolatedGitEnv(t), "GIT_AUTHOR_NAME=scenario", "GIT_AUTHOR_EMAIL=scenario@invalid", "GIT_COMMITTER_NAME=scenario", "GIT_COMMITTER_EMAIL=scenario@invalid")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, output)
	}
}

// ---------------------------------------------------------------------------------------------------------
// Registration: the mechanism, and the ruled verbs
// ---------------------------------------------------------------------------------------------------------

// layersDir is where a registration lives: XDG_DATA_HOME/devlore/writ/layers/<role> -> working tree. The
// symlink is the registration (the config-vs-layers separation); the verbs create and remove it.
func (j *journey) layersDir() string {
	return filepath.Join(j.sandbox.Root, "data", "devlore", "writ", "layers")
}

// reposDir is writ's own home for the clones it makes from a URL registration.
func (j *journey) reposDir() string {
	return filepath.Join(j.sandbox.Root, "data", "devlore", "writ", "repos")
}

// registerByMechanism registers a layer the way the settled mechanism does — the symlink — so the parts of
// the journey that are not about the verbs run whether or not #791 has shipped. By URL, the harness makes the
// clone writ would have made, named as `git clone` names it (#793), and points the symlink at it.
func (j *journey) registerByMechanism(t *testing.T, role string, byURL bool) {

	t.Helper()

	layer := j.layers[role]
	target := layer.Path
	if byURL {
		target = filepath.Join(j.reposDir(), layer.Name)
		if _, err := os.Stat(target); os.IsNotExist(err) {
			if err := os.MkdirAll(j.reposDir(), 0o755); err != nil {
				t.Fatal(err)
			}
			gitIn(t, j.sandbox.Root, "clone", "--quiet", "file://"+filepath.ToSlash(layer.Bare), target)
		}
	}
	if err := os.MkdirAll(j.layersDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(j.layersDir(), role)
	_ = os.Remove(link) //nolint:errcheck // a stale registration from an earlier step is replaced
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("register %s -> %s: %v", role, target, err)
	}
}

// registeredRoles lists the layers registered right now, in precedence order.
func (j *journey) registeredRoles() []string {

	var roles []string
	for _, role := range []string{"base", "team", "personal"} {
		if _, err := os.Lstat(filepath.Join(j.layersDir(), role)); err == nil {
			roles = append(roles, role)
		}
	}
	return roles
}

// ---------------------------------------------------------------------------------------------------------
// Probes and skips
// ---------------------------------------------------------------------------------------------------------

// probe asks the binary what it can do. A probe is a help text or a harmless invocation, never a version.
func (j *journey) probe(t *testing.T) {

	t.Helper()

	// cobra answers `repo set --help` with the parent's help and exit 0 when `set` is unknown, so the
	// verbs are read from the command list, not from an exit code.
	repoHelp, _, _ := runWrit(t, j.sandbox, "repo", "--help")
	j.caps.repoSet = strings.Contains(repoHelp, "\n  set ") && strings.Contains(repoHelp, "\n  unset ")

	for _, verb := range []string{"refresh", "update", "pull", "fetch", "sync"} {
		if strings.Contains(repoHelp, "\n  "+verb) {
			j.caps.refresh = true
		}
	}

	decommissionHelp, _, _ := runWrit(t, j.sandbox, "decommission", "--help")
	j.caps.decommissionAll = strings.Contains(decommissionHelp, "--all")

	// The bare deploy is probed with a dry run against whatever is registered; the zero-argument refusal
	// is a parse error, so it is the same with no layers as with three.
	_, stderr, err := runWrit(t, j.sandbox, "deploy", "--dry-run", "-o", "none")
	j.caps.bareDeploy = err == nil || !strings.Contains(stderr, "requires at least 1 arg")
	// Implicit repository-named projects arrive with the same change as the bare form (#850).
	j.caps.implicitByName = j.caps.bareDeploy

	t.Logf("capabilities: repo set=%v bare deploy=%v implicit by name=%v refresh=%v decommission --all=%v",
		j.caps.repoSet, j.caps.bareDeploy, j.caps.implicitByName, j.caps.refresh, j.caps.decommissionAll)
}

// skip records the outstanding ruling and skips the current step.
func (j *journey) skip(t *testing.T, issue int, what string) {

	t.Helper()

	entry := fmt.Sprintf("devlore-cli#%d: %s", issue, what)
	j.skips = append(j.skips, entry)
	t.Skipf("needs %s", entry)
}

// ---------------------------------------------------------------------------------------------------------
// Deploy, and reading what it did
// ---------------------------------------------------------------------------------------------------------

// deploy runs the ruled form — a bare `writ deploy` plus whatever is named — when the binary has it. Until
// #843 and #850 ship it names the implicit set itself: `common`, and each registered repository's own-named
// project where a registered layer carries one. This is the one shim in the scenario, and it is logged.
func (j *journey) deploy(t *testing.T, flags []string, named ...string) (stdout, stderr string, err error) {

	t.Helper()

	args := append([]string{"deploy"}, flags...)
	if j.caps.bareDeploy {
		args = append(args, named...)
	} else {
		implicit := j.implicitToday()
		t.Logf("shim for #%d/#%d: naming the implicit set %v until the bare form ships", issueBareDeploy, issueImplicitProjects, implicit)
		args = append(args, implicit...)
		args = append(args, named...)
	}
	return runWrit(t, j.sandbox, args...)
}

// implicitToday is the implicit set #850 rules, computed by the harness: `common`, plus each registered
// repository's name when any registered layer's Home carries a project by that name.
func (j *journey) implicitToday() []string {

	set := []string{"common"}
	roles := j.registeredRoles()
	for _, role := range roles {
		name := j.layers[role].Name
		for _, other := range roles {
			entries, err := os.ReadDir(filepath.Join(j.registeredPath(other), "Home"))
			if err != nil {
				continue
			}
			for _, entry := range entries {
				project := strings.SplitN(entry.Name(), ".", 2)[0]
				if entry.IsDir() && project == name && !contains(set, name) {
					set = append(set, name)
				}
			}
		}
	}
	return set
}

// registeredPath resolves a registration to the working tree it points at.
func (j *journey) registeredPath(role string) string {

	resolved, err := filepath.EvalSymlinks(filepath.Join(j.layersDir(), role))
	if err != nil {
		return ""
	}
	return resolved
}

func contains(list []string, item string) bool {
	for _, candidate := range list {
		if candidate == item {
			return true
		}
	}
	return false
}

var collisionPattern = regexp.MustCompile(`(\d+) source collision\(s\)`)

// collisionsIn reads the collision count writ narrates on stderr; zero when it narrates none.
func collisionsIn(stderr string) int {

	match := collisionPattern.FindStringSubmatch(stderr)
	if match == nil {
		return 0
	}
	n, _ := strconv.Atoi(match[1]) //nolint:errcheck // the pattern guarantees digits
	return n
}

// reconcileEntry is the part of `writ reconcile -o json` the journey reads.
type reconcileEntry struct {
	State  string `json:"state"`
	Target string `json:"target"`
}

// reconcile returns reconcile's entries grouped by state.
func (j *journey) reconcile(t *testing.T) map[string][]string {

	t.Helper()

	out, stderr, err := runWrit(t, j.sandbox, "reconcile", "-o", "json")
	if err != nil {
		t.Fatalf("writ reconcile failed: %v\nstderr: %s", err, stderr)
	}
	var report struct {
		Entries []reconcileEntry `json:"entries"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("reconcile -o json is not the expected shape: %v\n%s", err, out)
	}
	byState := map[string][]string{}
	for _, entry := range report.Entries {
		byState[entry.State] = append(byState[entry.State], entry.Target)
	}
	return byState
}

// deployedLinks walks the sandbox home and returns every symlink with the source it points at (resolved
// lexically against the link's directory), and whether that source exists.
func (j *journey) deployedLinks(t *testing.T) map[string]linkState {

	t.Helper()

	links := map[string]linkState{}
	err := filepath.WalkDir(j.sandbox.Home, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink == 0 {
			return nil
		}
		destination, err := os.Readlink(path)
		if err != nil {
			return err
		}
		if !filepath.IsAbs(destination) {
			destination = filepath.Join(filepath.Dir(path), destination)
		}
		_, statErr := os.Stat(path)
		links[path] = linkState{Source: filepath.Clean(destination), Resolves: statErr == nil}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return links
}

type linkState struct {
	Source   string
	Resolves bool
}

// dangling returns the links that do not resolve, sorted.
func dangling(links map[string]linkState) []string {

	var out []string
	for path, state := range links {
		if !state.Resolves {
			out = append(out, path)
		}
	}
	sort.Strings(out)
	return out
}

// ---------------------------------------------------------------------------------------------------------
// Running what was deployed
// ---------------------------------------------------------------------------------------------------------

// runScript runs a deployed bash script with --help under bash — the platform's own on Unix, Git for
// Windows' on Windows, which is the whole reason a git-supporting script is `common`.
func (j *journey) runScript(t *testing.T, path string, args ...string) (string, error) {

	t.Helper()

	cmd := exec.CommandContext(context.Background(), "bash", append([]string{filepath.ToSlash(path)}, args...)...)
	cmd.Dir = j.sandbox.Home
	cmd.Env = j.sandbox.Env
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// assertHelp asserts a deployed consumer answers --help through the shared parsing: exit 0 and its synopsis.
func (j *journey) assertHelp(t *testing.T, path string) {

	t.Helper()

	out, err := j.runScript(t, path, "--help")
	if err != nil {
		t.Fatalf("%s --help failed: %v\n%s", path, err, out)
	}
	name := filepath.Base(path)
	if !strings.Contains(out, name) {
		t.Fatalf("%s --help did not print its synopsis or manual:\n%s", path, out)
	}
}

// ---------------------------------------------------------------------------------------------------------
// What each platform deploys
// ---------------------------------------------------------------------------------------------------------

// selectorsHere lists the selector suffixes this platform matches, most general first.
func selectorsHere(t *testing.T) []string {

	t.Helper()

	switch runtime.GOOS {
	case "darwin":
		return []string{"Unix", "Darwin"}
	case "linux":
		if isDebianFamily() {
			return []string{"Unix", "Linux", "Debian"}
		}
		return []string{"Unix", "Linux"}
	case "windows":
		return []string{"Windows"}
	default:
		t.Fatalf("no selector table for %s", runtime.GOOS)
		return nil
	}
}

// isDebianFamily reads /etc/os-release the way writ's detector does: Debian, or ID_LIKE naming it.
func isDebianFamily() bool {

	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return false
	}
	text := strings.ToLower(string(data))
	return strings.Contains(text, "id=debian") || strings.Contains(text, "id_like=debian") || strings.Contains(text, "id_like=\"debian") || strings.Contains(text, "debian")
}

// consumersAtA lists the personal-a consumers this platform deploys, with their deployed path at commit A.
func (j *journey) consumersAtA(t *testing.T) map[string]string {

	t.Helper()

	home := j.sandbox.Home
	consumers := map[string]string{}
	for _, selector := range selectorsHere(t) {
		if selector == "Windows" {
			continue
		}
		for _, verb := range []string{"Get", "Test"} {
			name := verb + "-" + selector + "Scenario"
			consumers[name] = filepath.Join(home, "local", "bin", name)
		}
	}
	return consumers
}

// ---------------------------------------------------------------------------------------------------------
// The move: personal-a becomes personal-b, in place, as a second commit
// ---------------------------------------------------------------------------------------------------------

// applyMove turns the personal working tree from commit A into commit B: the rename to noblefactor-ops,
// every consumer out of common.<selector>/local into noblefactor-ops.<selector>/.local, the helper and its
// bridge deleted, the two context scripts made self-contained. It commits and pushes to the bare remote, and
// returns the repository-relative paths that moved or disappeared.
func (j *journey) applyMove(t *testing.T) []string {

	t.Helper()

	repo := j.layers["personal"].Path
	var gone []string

	move := func(from, to string) {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(repo, to)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(filepath.Join(repo, from), filepath.Join(repo, to)); err != nil {
			t.Fatalf("move %s -> %s: %v", from, to, err)
		}
		gone = append(gone, from)
	}
	remove := func(relative string) {
		if err := os.Remove(filepath.Join(repo, relative)); err != nil {
			t.Fatalf("remove %s: %v", relative, err)
		}
		gone = append(gone, relative)
	}

	// The rename: noblefactor{,.Unix} -> noblefactor-ops{,.Unix}. Every file under them moves.
	for _, project := range []string{"noblefactor", "noblefactor.Unix"} {
		from := filepath.Join("Home", project)
		to := filepath.Join("Home", strings.Replace(project, "noblefactor", "noblefactor-ops", 1))
		files := filesUnder(t, filepath.Join(repo, from))
		if err := os.Rename(filepath.Join(repo, from), filepath.Join(repo, to)); err != nil {
			t.Fatal(err)
		}
		for _, file := range files {
			gone = append(gone, filepath.Join(from, file))
		}
	}

	// The consumers: common.<selector>/local/{bin,share} -> noblefactor-ops.<selector>/.local/{bin,share}.
	for _, selector := range []string{"Darwin", "Linux", "Debian", "Unix"} {
		from := filepath.Join("Home", "common."+selector, "local")
		to := filepath.Join("Home", "noblefactor-ops."+selector, ".local")
		for _, file := range filesUnder(t, filepath.Join(repo, from)) {
			move(filepath.Join(from, file), filepath.Join(to, file))
		}
		_ = os.RemoveAll(filepath.Join(repo, from)) //nolint:errcheck // the emptied tree
	}

	// The helper leaves personal: the file, the bridge, the three assets.
	remove(filepath.Join("Home", "common", "local", "bin", "Declare-BashScript"))
	remove(filepath.Join("Home", "common", ".local", "bin", "Declare-BashScript"))
	remove(filepath.Join("Home", "common", ".local", "share", "man", "man1", "Declare-BashScript.1"))
	remove(filepath.Join("Home", "common", ".local", "share", "bash-completion", "completions", "Declare-BashScript"))
	remove(filepath.Join("Home", "common", ".local", "share", "zsh", "site-functions", "_Declare-BashScript"))

	// The context scripts stop sourcing it (the state personal#174 left them in; the ruling that every bash
	// script sources it is held on noblefactor-ops#147 and does not change what the move did).
	for _, relative := range []string{filepath.Join("Home", "thenobles.Darwin", "local", "bin", "tn"), filepath.Join("Home", "microsoft.Unix", "local", "bin", "ms")} {
		makeSelfContained(t, filepath.Join(repo, relative))
	}

	gitIn(t, repo, "add", "-A")
	gitIn(t, repo, "commit", "--quiet", "-m", "the move: noblefactor-ops takes the foundation; consumers say so by project")
	gitIn(t, repo, "push", "--quiet", "origin", "HEAD:main")

	sort.Strings(gone)
	return gone
}

// filesUnder lists the regular files and symlinks under dir, relative to it.
func filesUnder(t *testing.T, dir string) []string {

	t.Helper()

	var files []string
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		files = append(files, relative)
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return files
}

// makeSelfContained rewrites a fixture consumer so it sources nothing: the two helper lines become a local
// preamble with what the script uses. Same shape as personal#174 gave the six real ones.
func makeSelfContained(t *testing.T, path string) {

	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	preamble := `set -o errexit -o nounset -o pipefail

script_name="$(basename "$0")" && readonly script_name
readonly EX_USAGE=64 EX_CONFIG=78

function error {
    local rc=$1
    shift 1
    printf '[%s] [\033[31m✘\033[0m] %s\n' "$script_name" "$*" >&2
    ((rc == 0)) || exit "$rc"
}

function note {
    printf '[%s] [+] %s\n' "$script_name" "$*" >&2
}

function usage {
    printf '%s\n' "$1"
    exit 0
}

function require_darwin {
    [[ "$(uname -s)" == "Darwin" ]] || error $EX_CONFIG "This script requires macOS (Darwin)."
}

function require_nix {
    case "$(uname -s)" in
        Darwin | Linux) ;;
        *) error $EX_CONFIG "This script requires Linux or macOS (Darwin)." ;;
    esac
}

if ! script_arguments=$(getopt -n "${script_name}" -o "h" --long "help" -- "$@" 2>&1); then
    error $EX_USAGE "${script_arguments}"
fi
readonly script_arguments
`
	pattern := regexp.MustCompile(`# shellcheck source=Declare-BashScript\nsource "\$\(dirname "\$0"\)/Declare-BashScript" "\$0" "help" "h" "\$@"\n`)
	if !pattern.MatchString(text) {
		t.Fatalf("%s does not carry the standard source lines", path)
	}
	if err := os.WriteFile(path, []byte(pattern.ReplaceAllLiteralString(text, preamble)), 0o755); err != nil {
		t.Fatal(err)
	}
}

// ---------------------------------------------------------------------------------------------------------
// The scenario
// ---------------------------------------------------------------------------------------------------------

// TestWritLayerJourneyScenario_Harness is Phase 1's deliverable: three layers materialized and named, each
// with a working tree and a bare remote, and the binary answering inside the sandbox.
func TestWritLayerJourneyScenario_Harness(t *testing.T) {

	j := newJourney(t)
	j.probe(t)

	for _, role := range []string{"base", "team", "personal"} {
		layer := j.layers[role]
		if _, err := os.Stat(filepath.Join(layer.Path, ".git")); err != nil {
			t.Fatalf("%s (%s) is not a repository: %v", role, layer.Name, err)
		}
		if _, err := os.Stat(filepath.Join(layer.Bare, "HEAD")); err != nil {
			t.Fatalf("%s bare remote missing: %v", role, err)
		}
	}
	helper := filepath.Join(j.layers["base"].Path, "Home", "common", ".local", "bin", "Declare-BashScript")
	if _, err := os.Stat(helper); err != nil {
		t.Fatalf("the base fixture does not carry Declare-BashScript: %v", err)
	}
	bridge := filepath.Join(j.layers["personal"].Path, "Home", "common", "local", "bin", "Declare-BashScript")
	if info, err := os.Lstat(bridge); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("personal-a's bridge symlink is missing or not a symlink: %v", err)
	}

	stdout, stderr, err := runWrit(t, j.sandbox, "--help")
	if err != nil || !strings.Contains(stdout, "writ") {
		t.Fatalf("writ --help in the journey sandbox: %v\n%s", err, stderr)
	}
}

// TestWritLayerJourneyScenario_Part0_SelfInstall: the binary installs itself into the sandbox home, and a
// fresh install registers nothing.
func TestWritLayerJourneyScenario_Part0_SelfInstall(t *testing.T) {

	j := newJourney(t)
	j.probe(t)
	prefix := filepath.Join(j.sandbox.Home, ".local")

	t.Run("0.1 self install", func(t *testing.T) {
		if stdout, stderr, err := runWrit(t, j.sandbox, "self", "install", prefix, "--shell", "bash"); err != nil {
			t.Fatalf("writ self install failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
		}
		installed := filepath.Join(prefix, "bin", "writ")
		if runtime.GOOS == "windows" {
			installed += ".exe"
		}
		if _, err := os.Stat(installed); err != nil {
			t.Fatalf("self install did not place the binary at %s: %v", installed, err)
		}
	})

	t.Run("0.2 a fresh install registers nothing", func(t *testing.T) {
		// Today self install leaves empty placeholders under layers/ that repo list reads as registrations (#840).
		entries, _ := os.ReadDir(j.layersDir()) //nolint:errcheck // absent is the correct answer
		var placeholders []string
		for _, entry := range entries {
			placeholders = append(placeholders, entry.Name())
		}
		if len(placeholders) > 0 {
			j.skip(t, issueSelfInstallPlaceholders, fmt.Sprintf("self install left placeholder registrations %v", placeholders))
		}
		stdout, stderr, err := runWrit(t, j.sandbox, "repo", "list", "-o", "json")
		if err != nil {
			t.Fatalf("writ repo list failed: %v\nstderr: %s", err, stderr)
		}
		if strings.Contains(stdout, "\"base\"") || strings.Contains(stdout, "\"team\"") {
			j.skip(t, issueSelfInstallPlaceholders, "repo list reads self install's placeholders as registrations")
		}
	})
}

// TestWritLayerJourneyScenario_Part1_RepoSet: the ruled verbs, for every layer, by path and by URL, in turn.
func TestWritLayerJourneyScenario_Part1_RepoSet(t *testing.T) {

	j := newJourney(t)
	j.probe(t)

	if !j.caps.repoSet {
		j.skip(t, issueRepoSetUnset, "`writ repo set` and `unset` replace `add` and `remove`")
	}

	for _, role := range []string{"base", "team", "personal"} {
		layer := j.layers[role]
		url := "file://" + filepath.ToSlash(layer.Bare)

		// 1.1 by path on an empty slot
		if _, stderr, err := runWrit(t, j.sandbox, "repo", "set", role, layer.Path); err != nil {
			t.Fatalf("repo set %s <path>: %v\n%s", role, err, stderr)
		}
		// 1.2 by URL over it: replaced, narrated, cloned under writ's home named as git clone names it (#793)
		if _, stderr, err := runWrit(t, j.sandbox, "repo", "set", role, url); err != nil {
			t.Fatalf("repo set %s <url>: %v\n%s", role, err, stderr)
		} else if !strings.Contains(stderr, "was") || !strings.Contains(stderr, "now") {
			t.Fatalf("repo set %s <url> did not narrate the replacement:\n%s", role, stderr)
		}
		if _, err := os.Stat(filepath.Join(j.reposDir(), layer.Name)); err != nil {
			t.Fatalf("the clone is not named as git clone names it (#%d): %v", issueCloneNaming, err)
		}
		// 1.3 by path over the clone: the writ-owned clone it displaces is removed (#792)
		if _, stderr, err := runWrit(t, j.sandbox, "repo", "set", role, layer.Path); err != nil {
			t.Fatalf("repo set %s <path> over the clone: %v\n%s", role, err, stderr)
		}
		if _, err := os.Stat(filepath.Join(j.reposDir(), layer.Name)); err == nil {
			t.Fatalf("replacing a URL registration left writ's own clone behind (#%d)", issueRemoveLeavesClone)
		}
		// 1.4 unset
		if _, stderr, err := runWrit(t, j.sandbox, "repo", "unset", role); err != nil {
			t.Fatalf("repo unset %s: %v\n%s", role, err, stderr)
		}
		if _, err := os.Lstat(filepath.Join(j.layersDir(), role)); err == nil {
			t.Fatalf("repo unset %s left the registration", role)
		}
	}
	for _, old := range []string{"add", "remove", "rm", "ls"} {
		if _, _, err := runWrit(t, j.sandbox, "repo", old, "--help"); err == nil {
			t.Fatalf("`writ repo %s` is still a command; #%d retires it", old, issueRepoSetUnset)
		}
	}
	// 1.5 a path that is not a repository is refused, naming it
	stray := filepath.Join(j.sandbox.Root, "not-a-repository")
	if err := os.MkdirAll(stray, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, stderr, err := runWrit(t, j.sandbox, "repo", "set", "personal", stray); err == nil || !strings.Contains(stderr, "not-a-repository") {
		t.Fatalf("repo set accepted a non-repository or did not name it:\n%s", stderr)
	}

	// 1.6 a URL-registered clone is brought forward when its remote advances (#812)
	if !j.caps.refresh {
		j.skip(t, issueRefreshClone, "a verb that refreshes a clone writ made")
	}
}

// TestWritLayerJourneyScenario_Part1_Subsets: every non-empty subset of the three layers, registered by
// path or by URL in turn, and a bare deploy that converges exactly what the registered layers contribute.
func TestWritLayerJourneyScenario_Part1_Subsets(t *testing.T) {

	subsets := [][]string{{"base"}, {"team"}, {"personal"}, {"base", "team"}, {"base", "personal"}, {"team", "personal"}, {"base", "team", "personal"}}
	for i, subset := range subsets {
		subset := subset
		t.Run(strings.Join(subset, "+"), func(t *testing.T) {

			j := newJourney(t)
			j.probe(t)
			for k, role := range subset {
				j.registerByMechanism(t, role, (i+k)%2 == 1)
			}

			if _, stderr, err := j.deploy(t, nil); err != nil {
				t.Fatalf("deploy with %v registered failed: %v\n%s", subset, err, stderr)
			}
			home := j.sandbox.Home
			has := func(role string) bool { return contains(subset, role) }

			// common* from each registered layer, and nothing from an unregistered one
			assertPresence(t, has("base"), filepath.Join(home, ".local", "bin", "git-scenario"))
			assertPresence(t, has("team"), filepath.Join(home, ".config", "scenario", "team.conf"))
			assertPresence(t, has("base") || has("personal"), filepath.Join(home, ".local", "bin", "Declare-BashScript"))
			assertPresence(t, has("personal"), filepath.Join(home, ".config", "scenario", "personal.conf"))
			// the repository-named project: personal's Home/devlore-cli deploys only when devlore-cli is a layer
			wantOverrides := has("personal") && has("team")
			if wantOverrides && !j.caps.implicitByName {
				// the shim names it; the ruling makes it implicit — the same outcome, so no skip here
				t.Logf("Home/devlore-cli deployed by the shim's naming; #%d makes it implicit", issueImplicitProjects)
			}
			assertPresence(t, wantOverrides, filepath.Join(home, ".config", "scenario", "devlore-cli.conf"))
			// a project named for nothing configured never deploys unnamed
			assertAbsent(t, filepath.Join(home, ".config", "scenario", "tn.conf"))
			if len(subset) == 1 && subset[0] == "base" {
				t.Logf("base alone from an empty sandbox deployed: #477's proof that deploying base needs no base")
			}
		})
	}
}

// assertPresence asserts a path exists when want is true and is absent when it is false.
func assertPresence(t *testing.T, want bool, path string) {

	t.Helper()

	_, err := os.Lstat(path)
	switch {
	case want && err != nil:
		t.Fatalf("expected %s to be deployed: %v", path, err)
	case !want && err == nil:
		t.Fatalf("expected nothing at %s", path)
	}
}

// TestWritLayerJourneyScenario_Part2_Deploy: all three layers, personal at commit A; the deploy, the named
// project, persistence, decommission. One sandbox, the steps in order, each reporting on its own.
func TestWritLayerJourneyScenario_Part2_Deploy(t *testing.T) {

	j := newJourney(t)
	j.probe(t)
	for _, role := range []string{"base", "team", "personal"} {
		j.registerByMechanism(t, role, false)
	}
	home := j.sandbox.Home

	t.Run("2.1-2.2 deploy, and the four collisions", func(t *testing.T) {
		// At A personal still carries the helper, so it and its three assets collide with the base's — personal
		// wins, and writ says so (#470). The bare form itself is #843/#850; the shim names the set until then.
		if !j.caps.bareDeploy {
			t.Logf("2.1 bare `writ deploy` needs #%d/#%d; the shim names the set", issueBareDeploy, issueImplicitProjects)
		}
		_, stderr, err := j.deploy(t, nil, "noblefactor")
		if err != nil {
			t.Fatalf("deploy failed: %v\n%s", err, stderr)
		}
		if got := collisionsIn(stderr); got != 4 {
			t.Fatalf("expected 4 source collisions narrated (#%d), got %d:\n%s", issueCollisionReport, got, stderr)
		}
		helper := filepath.Join(home, ".local", "bin", "Declare-BashScript")
		resolved, err := filepath.EvalSymlinks(helper)
		if err != nil || !strings.HasPrefix(resolved, j.layers["personal"].Path) {
			t.Fatalf("at A the helper must resolve to personal's copy (precedence); got %s (%v)", resolved, err)
		}
	})

	t.Run("2.3 selectors", func(t *testing.T) {
		for name, path := range j.consumersAtA(t) {
			if _, err := os.Lstat(path); err != nil {
				t.Fatalf("%s should be deployed on %s: %v", name, runtime.GOOS, err)
			}
		}
		for _, selector := range []string{"Darwin", "Linux", "Debian", "Unix"} {
			if contains(selectorsHere(t), selector) {
				continue
			}
			assertAbsent(t, filepath.Join(home, "local", "bin", "Get-"+selector+"Scenario"))
		}
		assertPresence(t, runtime.GOOS == "windows", filepath.Join(home, "local", "bin", "w.ps1"))
		assertLinked(t, filepath.Join(home, ".local", "bin", "git-scenario"), "git-scenario")
		assertLinked(t, filepath.Join(home, ".local", "bin", "git-a"), "git-a")
		assertPresence(t, runtime.GOOS != "windows", filepath.Join(home, ".local", "bin", "nf-unix"))
	})

	t.Run("2.4 every consumer answers --help", func(t *testing.T) {
		j.assertHelp(t, filepath.Join(home, ".local", "bin", "git-scenario"))
		j.assertHelp(t, filepath.Join(home, ".local", "bin", "git-a"))
		for _, path := range j.consumersAtA(t) {
			j.assertHelp(t, path)
		}
	})

	t.Run("2.5 deploy thenobles", func(t *testing.T) {
		if _, stderr, err := j.deploy(t, nil, "noblefactor", "thenobles"); err != nil {
			t.Fatalf("deploy thenobles failed: %v\n%s", err, stderr)
		}
		assertLinked(t, filepath.Join(home, ".config", "scenario", "tn.conf"), "thenobles")
		assertPresence(t, runtime.GOOS == "darwin", filepath.Join(home, "local", "bin", "tn"))
	})

	t.Run("2.9 reconcile: everything linked", func(t *testing.T) {
		states := j.reconcile(t)
		for state := range states {
			if state != "linked" && state != "copied" {
				t.Fatalf("reconcile reports %d entries in state %q after a clean deploy: %v", len(states[state]), state, states[state])
			}
		}
	})

	t.Run("2.6 a bare deploy keeps thenobles", func(t *testing.T) {
		if !j.caps.bareDeploy {
			j.skip(t, issueImplicitProjects, "deploy adds a named project to the machine's selection and a later bare deploy keeps it")
		}
		if _, stderr, err := j.deploy(t, nil); err != nil {
			t.Fatalf("bare redeploy failed: %v\n%s", err, stderr)
		}
		assertLinked(t, filepath.Join(home, ".config", "scenario", "tn.conf"), "thenobles")
	})

	t.Run("2.7-2.8 decommission", func(t *testing.T) {
		if !j.caps.decommissionAll {
			j.skip(t, issueDecommission, "decommission re-converges after a named removal and refuses implicit projects by name")
		}
		if _, stderr, err := runWrit(t, j.sandbox, "decommission", "thenobles"); err != nil {
			t.Fatalf("decommission thenobles: %v\n%s", err, stderr)
		}
		assertAbsent(t, filepath.Join(home, ".config", "scenario", "tn.conf"))
		for _, name := range []string{"common", "noblefactor-ops"} {
			if _, stderr, err := runWrit(t, j.sandbox, "decommission", name); err == nil || !strings.Contains(stderr, "repo unset") {
				t.Fatalf("decommission %s must be refused, pointing at writ repo unset:\n%s", name, stderr)
			}
		}
	})
}

// TestWritLayerJourneyScenario_Part3_Move: personal advances from A to B under the deployed links, by path
// and by URL.
func TestWritLayerJourneyScenario_Part3_Move(t *testing.T) {

	for _, byURL := range []bool{false, true} {
		byURL := byURL
		name := "by-path"
		if byURL {
			name = "by-url"
		}
		t.Run(name, func(t *testing.T) {

			j := newJourney(t)
			j.probe(t)
			j.registerByMechanism(t, "base", false)
			j.registerByMechanism(t, "team", false)
			j.registerByMechanism(t, "personal", byURL)
			home := j.sandbox.Home

			if _, stderr, err := j.deploy(t, nil, "noblefactor"); err != nil {
				t.Fatalf("deploy at A failed: %v\n%s", err, stderr)
			}
			before := j.deployedLinks(t)
			if len(dangling(before)) != 0 {
				t.Fatalf("links dangle before the move: %v", dangling(before))
			}

			// The move lands in the working tree, or in the remote.
			gone := j.applyMove(t)
			if byURL {
				if !j.caps.refresh {
					j.skip(t, issueRefreshClone, "bringing the URL-registered clone forward to commit B")
				}
			}

			// 3.1 the links whose sources moved or vanished dangle, and only those
			after := j.deployedLinks(t)
			var expected []string
			for path, state := range before {
				if _, err := os.Stat(state.Source); err != nil {
					expected = append(expected, path)
				}
			}
			sort.Strings(expected)
			got := dangling(after)
			if strings.Join(got, "\n") != strings.Join(expected, "\n") {
				t.Fatalf("dangling after the move:\n  got  %d: %v\n  want %d: %v\n(moved or deleted in the repository: %d paths)", len(got), got, len(expected), expected, len(gone))
			}
			if len(got) == 0 {
				t.Fatal("the move dangled nothing; the fixture no longer carries the dependency it is meant to move")
			}
			helper := filepath.Join(home, ".local", "bin", "Declare-BashScript")
			if !contains(got, helper) {
				t.Fatalf("the helper's link should dangle after the move; it resolves")
			}
			states := j.reconcile(t)
			delete(states, "linked")
			delete(states, "copied")
			t.Logf("3.1 reconcile between the commits reports: %v", summarize(states))

			// 3.2 the dry run says nothing about the occupied targets it is about to relink (#853)
			if _, stderr, err := j.deploy(t, []string{"--dry-run", "-o", "none"}); err != nil {
				t.Fatalf("dry run failed: %v\n%s", err, stderr)
			} else if strings.Contains(stderr, "occupied") || strings.Contains(stderr, "refusing") {
				t.Fatalf("the dry run now reports the pre-flight — #%d has landed; update this step:\n%s", issueDryRunPreflight, stderr)
			}

			// 3.3 the redeploy: everything relinked to the new sources, the helper now the base's, no collision
			_, stderr, err := j.deploy(t, []string{"--conflict=replace"})
			if err != nil {
				t.Fatalf("redeploy failed: %v\n%s", err, stderr)
			}
			if got := collisionsIn(stderr); got != 0 {
				t.Fatalf("after the move nothing should collide; %d narrated:\n%s", got, stderr)
			}
			resolved, err := filepath.EvalSymlinks(helper)
			if err != nil || !strings.HasPrefix(resolved, j.layers["base"].Path) {
				t.Fatalf("after the move the helper must resolve to the base's copy; got %s (%v)", resolved, err)
			}
			for name := range j.consumersAtA(t) {
				j.assertHelp(t, filepath.Join(home, ".local", "bin", name))
			}
			j.assertHelp(t, filepath.Join(home, ".local", "bin", "git-a"))

			// the old links under ~/local remain as orphans (#845)
			orphans := dangling(j.deployedLinks(t))
			for _, orphan := range orphans {
				if !strings.HasPrefix(orphan, filepath.Join(home, "local")+string(os.PathSeparator)) {
					t.Fatalf("an orphan outside ~/local after the redeploy: %s", orphan)
				}
			}
			var oldTree int
			for _, path := range expected {
				if strings.HasPrefix(path, filepath.Join(home, "local")+string(os.PathSeparator)) {
					oldTree++
				}
			}
			if len(orphans) != oldTree {
				t.Fatalf("expected the %d old ~/local links to remain as orphans (#%d); found %d: %v", oldTree, issueOrphans, len(orphans), orphans)
			}

			// 3.4 / 3.5 the store remembers the old targets: after the hand cleanup they are `missing`
			for _, orphan := range orphans {
				if err := os.Remove(orphan); err != nil {
					t.Fatal(err)
				}
			}
			states = j.reconcile(t)
			if got := len(states["missing"]); got != oldTree {
				t.Fatalf("reconcile should report the %d removed old targets as missing (#%d); got %d: %v", oldTree, issueOrphans, got, summarize(states))
			}

			// 3.6 the default policy accepts writ's own links
			if _, stderr, err := j.deploy(t, nil); err != nil {
				t.Fatalf("a redeploy under the default policy must accept writ's own links: %v\n%s", err, stderr)
			}

			// 3.7 a file outside Home/ forces --allow-dirty (#852)
			repo := j.registeredPath("personal")
			if err := os.WriteFile(filepath.Join(repo, "Inventory", "x"), []byte("dirty outside Home\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, stderr, err := j.deploy(t, nil); err == nil {
				t.Fatalf("deploy accepted a dirty repository without --allow-dirty — #%d has landed; update this step", issueDirtyAtRoot)
			} else if !strings.Contains(stderr, "uncommitted changes") {
				t.Fatalf("expected the dirty refusal, got: %v\n%s", err, stderr)
			}
			if _, stderr, err := j.deploy(t, []string{"--allow-dirty"}); err != nil {
				t.Fatalf("deploy --allow-dirty failed: %v\n%s", err, stderr)
			}
		})
	}
}

// summarize renders reconcile states as counts.
func summarize(states map[string][]string) string {

	var parts []string
	for state, targets := range states {
		parts = append(parts, fmt.Sprintf("%s=%d", state, len(targets)))
	}
	sort.Strings(parts)
	return strings.Join(parts, " ")
}
