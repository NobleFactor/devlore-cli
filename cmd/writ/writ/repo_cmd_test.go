// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package writ

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/cmd/internal/cli"
	"github.com/NobleFactor/devlore-cli/cmd/internal/devlore"
)

// workingTree creates a temporary directory that passes the add-time git-working-tree validation.
func workingTree(t *testing.T) string {

	t.Helper()

	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o750); err != nil {
		t.Fatal(err)
	}
	return root
}

// sourceRepository creates a real single-commit git repository for offline file:// clone tests.
func sourceRepository(t *testing.T) string {

	t.Helper()

	return sourceRepositoryNamed(t, "")
}

// sourceRepositoryNamed is [sourceRepository] with the repository's directory named, so a test can say what
// `git clone` will name the clone (#793). Empty names the temporary directory itself.
func sourceRepositoryNamed(t *testing.T, name string) string {

	t.Helper()

	root := t.TempDir()
	if name != "" {
		root = filepath.Join(root, name)
		if err := os.Mkdir(root, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	env := isolatedGitEnv(t)

	for _, args := range [][]string{
		{"init", "--quiet", "--initial-branch=main"},
		{"-c", "user.name=t", "-c", "user.email=t@invalid", "commit", "--quiet", "--allow-empty", "-m", "seed"},
	} {
		command := exec.CommandContext(context.Background(), "git", append([]string{"-C", root}, args...)...)
		command.Env = env
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	return root
}

// isolatedGitEnv points git's global and system config at an empty file, so a test repository sees
// no configuration beyond what the test supplies.
//
// An empty regular file rather than os.DevNull. os.DevNull is "NUL" on Windows, and the git build
// on the windows-11-arm runner image refuses to open it as a config path:
//
//	fatal: unable to access 'NUL': Invalid argument
//
// The identical command succeeds on windows-latest, so this is a property of that image's git, not
// of the architecture — which is why it stayed hidden until windows/arm64 joined the matrix. An
// empty file is what git actually needs here: a config it can open and find nothing in. It carries
// none of the device-file semantics that vary by platform and by git build.
func isolatedGitEnv(t *testing.T) []string {

	t.Helper()

	empty := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatalf("write empty git config: %v", err)
	}

	return append(os.Environ(), "GIT_CONFIG_GLOBAL="+empty, "GIT_CONFIG_SYSTEM="+empty)
}

// runRepo executes the repo command family against a sandboxed layers directory and returns its output.
//
// The command runs through writ's real root: the registrations are a result, and [cli.Emit] renders through the
// common set the root registers, so a command built standalone has no set to render with.
//
// The sequence is the one `main` runs, validation included: [cli.ValidateCommandLine] refuses a verb a group
// does not have, where cobra alone would accept it, print help and exit 0 (#897). A harness that called only
// Execute would test a path no user takes.
//
// `SetArgs` precedes the validation, because with no arguments set cobra reads os.Args[1:] -- under `go test`,
// the test binary's own flags.
//
// Both streams come back joined, as they did before the pre-flight existed: help and narration go to stderr,
// and a caller asserting on either needs them. [runRepoResult] is for the assertions that decode, which must
// read stdout alone.
func runRepo(t *testing.T, args ...string) (string, error) {

	t.Helper()

	result, narration, err := runRepoStreams(t, args...)

	return result + narration, err
}

// runRepoResult is [runRepo] for a caller that decodes the result: stdout alone, with no narration in it.
//
// `git clone` writes progress to stderr through [cloneRepository], and a decoder handed both reads
// `Cloning into ...` as JSON.
func runRepoResult(t *testing.T, args ...string) (string, error) {

	t.Helper()

	result, _, err := runRepoStreams(t, args...)

	return result, err
}

// runRepoStreams runs the family and returns stdout and stderr separately.
func runRepoStreams(t *testing.T, args ...string) (result, narration string, err error) {

	t.Helper()

	root := NewRootCmd()

	var out, stderr strings.Builder
	root.SetOut(&out)
	root.SetErr(&stderr)

	argv := append([]string{"repo"}, args...)
	root.SetArgs(argv)

	if err := cli.ValidateCommandLine(root, argv); err != nil {
		return out.String(), stderr.String(), err
	}

	// Execute first, read the buffers after. A `return result.String(), narration.String(), root.Execute()`
	// evaluates left to right, so both builders are read before the command has written anything to them.
	err = root.Execute()

	return out.String(), stderr.String(), err
}

// registrations decodes what `repo list` returned.
func registrations(t *testing.T, out string) []RepoRegistration {

	t.Helper()

	var decoded []RepoRegistration
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("repo list is not json: %v\n%s", err, out)
	}
	return decoded
}

// stateOf returns the layer's state and root from a decoded listing.
func stateOf(t *testing.T, out, layer string) RepoRegistration {

	t.Helper()

	for _, registration := range registrations(t, out) {
		if registration.Layer == layer {
			return registration
		}
	}
	t.Fatalf("layer %q is absent from the listing:\n%s", layer, out)
	return RepoRegistration{}
}

func TestRepo_SetListUnset_RoundTrip(t *testing.T) {

	t.Setenv("XDG_DATA_HOME", t.TempDir())
	repo := workingTree(t)

	if _, err := runRepo(t, "set", "personal", repo); err != nil {
		t.Fatalf("set: %v", err)
	}

	listed, err := runRepoResult(t, "list")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if got := stateOf(t, listed, "personal"); got.State != repoStateRegistered || got.Root != repo {
		t.Fatalf("personal = %+v; want registered at %s", got, repo)
	}
	if got := stateOf(t, listed, "base"); got.State != repoStateUnregistered {
		t.Fatalf("base = %+v; want unregistered", got)
	}

	if _, err := runRepo(t, "unset", "personal"); err != nil {
		t.Fatalf("unset: %v", err)
	}
	listed, err = runRepoResult(t, "list")
	if err != nil {
		t.Fatal(err)
	}
	if got := stateOf(t, listed, "personal"); got.State != repoStateUnregistered {
		t.Fatalf("personal = %+v after unset; want unregistered", got)
	}
}

// TestRepo_Set_RePointsWithoutRefusing is the whole reason the verb is `set`.
//
// `add` refused a layer that was already registered and named `remove` in the refusal, so re-pointing a layer
// was two commands and the refusal arrived after the clone. A layer has exactly one registration; writing it
// again writes it again.
func TestRepo_Set_RePointsWithoutRefusing(t *testing.T) {

	t.Setenv("XDG_DATA_HOME", t.TempDir())
	first, second := workingTree(t), workingTree(t)

	if _, err := runRepo(t, "set", "personal", first); err != nil {
		t.Fatalf("first set: %v", err)
	}
	if _, err := runRepo(t, "set", "personal", second); err != nil {
		t.Fatalf("re-point: %v", err)
	}

	listed, err := runRepoResult(t, "list")
	if err != nil {
		t.Fatal(err)
	}
	if got := stateOf(t, listed, "personal"); got.Root != second {
		t.Fatalf("personal = %+v; want it re-pointed at %s", got, second)
	}
}

// TestRepo_Set_UnchangedIsNotAnError pins the third ruling of 2026-09-18.
//
// Setting a layer to the root it already has is neither a failure nor a silent success: it exits 0 and emits
// the record, so a script that re-asserts its configuration sees the same shape every time.
func TestRepo_Set_UnchangedIsNotAnError(t *testing.T) {

	t.Setenv("XDG_DATA_HOME", t.TempDir())
	repo := workingTree(t)

	if _, err := runRepo(t, "set", "team", repo); err != nil {
		t.Fatalf("set: %v", err)
	}

	out, err := runRepoResult(t, "set", "team", repo)
	if err != nil {
		t.Fatalf("setting an unchanged target: %v", err)
	}

	// `set` emits one record; `list` emits an array. The listing decoder does not read this.
	var record RepoRegistration
	if err := json.Unmarshal([]byte(out), &record); err != nil {
		t.Fatalf("set emitted no record: %v\n%s", err, out)
	}
	if record.Layer != "team" || record.State != repoStateRegistered || record.Root != repo {
		t.Fatalf("set emitted %+v; want team registered at %s", record, repo)
	}
}

// TestRepo_BareInvocation_PrintsHelp pins the group's contract: `repo` is a noun and takes no action, so it
// prints help and lists nothing (10-command-line-interface.md §3, invariant 7). Until this assertion, the test
// here matched layer names in that very help text and passed for the wrong reason.
func TestRepo_BareInvocation_PrintsHelp(t *testing.T) {

	t.Setenv("XDG_DATA_HOME", t.TempDir())

	out, err := runRepo(t)
	if err != nil {
		t.Fatalf("bare repo: %v", err)
	}

	if !strings.Contains(out, "Usage:") || !strings.Contains(out, "writ repo") {
		t.Errorf("bare repo did not print help:\n%s", out)
	}
	if strings.Contains(out, `"state"`) {
		t.Errorf("bare repo listed the registrations; a group takes no action:\n%s", out)
	}
}

// TestRepo_List_ReportsEveryLayer pins the listing itself: one record per layer, whatever its state.
func TestRepo_List_ReportsEveryLayer(t *testing.T) {

	t.Setenv("XDG_DATA_HOME", t.TempDir())

	listed, err := runRepoResult(t, "list")
	if err != nil {
		t.Fatalf("repo list: %v", err)
	}
	if got := len(registrations(t, listed)); got != len(LayerOrder) {
		t.Fatalf("listing has %d layers; want %d", got, len(LayerOrder))
	}
	for _, layer := range LayerOrder {
		stateOf(t, listed, layer)
	}
}

// TestRepo_Record_OwnerTellsWritsTreeFromYours is #792's ruling made machine-readable.
//
// A tree under the writ-owned home is one writ cloned, and so one writ may remove; a tree registered by path is
// the user's and is never touched. The record says which, so a script and a dry run can both read it.
func TestRepo_Record_OwnerTellsWritsTreeFromYours(t *testing.T) {

	dataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)

	cloned, err := runRepoResult(t, "set", "team", "file://"+sourceRepository(t))
	if err != nil {
		t.Fatalf("set by url: %v", err)
	}
	if got := decodeRecord(t, cloned); got.Owner != repoOwnerWrit {
		t.Errorf("a clone in the writ home has owner %q, want %q: %+v", got.Owner, repoOwnerWrit, got)
	}

	yours, err := runRepoResult(t, "set", "personal", workingTree(t))
	if err != nil {
		t.Fatalf("set by path: %v", err)
	}
	if got := decodeRecord(t, yours); got.Owner != repoOwnerUser {
		t.Errorf("a path you registered has owner %q, want %q: %+v", got.Owner, repoOwnerUser, got)
	}
}

// TestRepo_Record_PreviousIsWhatWasDisplaced is why the record exists at all.
//
// Narration is stderr, so `--output json` never sees "was X, now Y"; whatever a script needs about the change
// has to be in the record. Re-pointing carries the displaced target; unsetting carries the removed one; a
// fresh registration carries none.
func TestRepo_Record_PreviousIsWhatWasDisplaced(t *testing.T) {

	t.Setenv("XDG_DATA_HOME", t.TempDir())
	first, second := workingTree(t), workingTree(t)

	fresh, err := runRepoResult(t, "set", "personal", first)
	if err != nil {
		t.Fatal(err)
	}
	if got := decodeRecord(t, fresh); got.Previous != nil {
		t.Errorf("a fresh registration carries a previous: %+v", got.Previous)
	}

	repointed, err := runRepoResult(t, "set", "personal", second)
	if err != nil {
		t.Fatal(err)
	}
	got := decodeRecord(t, repointed)
	if got.Root != second || got.Previous == nil || got.Previous.Root != first || got.Previous.Owner != repoOwnerUser {
		t.Errorf("re-pointing did not carry the displaced target: %+v previous %+v", got, got.Previous)
	}

	removed, err := runRepoResult(t, "unset", "personal")
	if err != nil {
		t.Fatal(err)
	}
	got = decodeRecord(t, removed)
	if got.State != repoStateUnregistered || got.Previous == nil || got.Previous.Root != second {
		t.Errorf("unset did not carry the removed target: %+v previous %+v", got, got.Previous)
	}
}

// TestRepo_Record_SourceIsTheFetchURL pins the rule for `source`: the upstream's remote, else origin, else empty.
//
// Empty is an answer -- a repository made by `git init` and never pushed has no remote, and writ's own refusal
// tells the user to make exactly that. A tree whose `.git` is an empty directory is not a repository git can
// read, and that too answers empty rather than failing the command.
func TestRepo_Record_SourceIsTheFetchURL(t *testing.T) {

	t.Setenv("XDG_DATA_HOME", t.TempDir())

	noRemote, err := runRepoResult(t, "set", "team", sourceRepository(t))
	if err != nil {
		t.Fatal(err)
	}
	if got := decodeRecord(t, noRemote); got.Source != "" {
		t.Errorf("a repository with no remote has source %q, want empty", got.Source)
	}

	unreadable, err := runRepoResult(t, "set", "base", workingTree(t))
	if err != nil {
		t.Fatal(err)
	}
	if got := decodeRecord(t, unreadable); got.Source != "" {
		t.Errorf("a tree git cannot read has source %q, want empty", got.Source)
	}

	withOrigin := sourceRepository(t)
	command := exec.CommandContext(context.Background(), "git", "-C", withOrigin, "remote", "add", "origin", "git@example.invalid:me/env.git")
	command.Env = isolatedGitEnv(t)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git remote add: %v\n%s", err, output)
	}
	registered, err := runRepoResult(t, "set", "personal", withOrigin)
	if err != nil {
		t.Fatal(err)
	}
	if got := decodeRecord(t, registered); got.Source != "git@example.invalid:me/env.git" {
		t.Errorf("source = %q, want the origin URL", got.Source)
	}
}

// TestRepo_Record_BranchIsPresentOnlyWhenGiven keeps the flag's ruling visible in the record.
func TestRepo_Record_BranchIsPresentOnlyWhenGiven(t *testing.T) {

	t.Setenv("XDG_DATA_HOME", t.TempDir())
	source := sourceRepository(t)

	plain, err := runRepoResult(t, "set", "team", "file://"+source)
	if err != nil {
		t.Fatal(err)
	}
	if got := decodeRecord(t, plain); got.Branch != "" {
		t.Errorf("no --branch was given, yet the record carries %q", got.Branch)
	}

	branched, err := runRepoResult(t, "set", "personal", "file://"+source, filepath.Join(t.TempDir(), "p"), "--branch", "main")
	if err != nil {
		t.Fatal(err)
	}
	if got := decodeRecord(t, branched); got.Branch != "main" {
		t.Errorf("--branch main was given, yet the record carries %q", got.Branch)
	}
}

// decodeRecord reads the one record `set` and `unset` emit.
func decodeRecord(t *testing.T, out string) RepoRegistration {

	t.Helper()

	var record RepoRegistration
	if err := json.Unmarshal([]byte(out), &record); err != nil {
		t.Fatalf("no record was emitted: %v\n%s", err, out)
	}
	return record
}

// TestRepo_Unset_TakesTheCloneWritMade is #792.
//
// `repo add <layer> <url>` cloned into the writ-owned home and `repo remove` left the clone behind, so the next
// registration by URL found the directory occupied and the operator was cleaning up writ's own state by hand --
// which is what blocked every layer on the Linux virtual machine on 2026-09-18. A clone writ made is writ's to
// remove.
func TestRepo_Unset_TakesTheCloneWritMade(t *testing.T) {

	dataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)

	source := sourceRepository(t)
	if _, err := runRepoResult(t, "set", "team", "file://"+source); err != nil {
		t.Fatalf("set by url: %v", err)
	}
	clone := filepath.Join(dataHome, "devlore", "writ", "repos", filepath.Base(source))
	if _, err := os.Stat(filepath.Join(clone, ".git")); err != nil {
		t.Fatalf("no clone at %s: %v", clone, err)
	}

	out, err := runRepoResult(t, "unset", "team")
	if err != nil {
		t.Fatalf("unset: %v", err)
	}
	if got := decodeRecord(t, out); got.Previous == nil || got.Previous.Owner != repoOwnerWrit {
		t.Errorf("the record does not say the clone was writ's: %+v", got)
	}
	if _, err := os.Stat(clone); !os.IsNotExist(err) {
		t.Errorf("the clone writ made survived its unset: %v", err)
	}

	// And it can be registered by URL again, which is the whole point.
	if _, err := runRepoResult(t, "set", "team", "file://"+sourceRepository(t)); err != nil {
		t.Fatalf("set by url after unset: %v", err)
	}
}

// TestRepo_Unset_LeavesYourTree is the other half of #792's ruling.
func TestRepo_Unset_LeavesYourTree(t *testing.T) {

	t.Setenv("XDG_DATA_HOME", t.TempDir())
	yours := workingTree(t)

	if _, err := runRepoResult(t, "set", "personal", yours); err != nil {
		t.Fatal(err)
	}
	out, err := runRepoResult(t, "unset", "personal")
	if err != nil {
		t.Fatal(err)
	}
	if got := decodeRecord(t, out); got.Previous == nil || got.Previous.Owner != repoOwnerUser {
		t.Errorf("the record does not say the tree was yours: %+v", got)
	}
	if _, err := os.Stat(yours); err != nil {
		t.Fatalf("a tree registered by path was removed: %v", err)
	}
}

// TestRepo_Set_TakesTheCloneItDisplaces is #792 on the `set` side: re-pointing a layer away from writ's clone
// removes the clone, and away from the user's tree removes nothing.
func TestRepo_Set_TakesTheCloneItDisplaces(t *testing.T) {

	dataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	yours := workingTree(t)

	source := sourceRepository(t)
	if _, err := runRepoResult(t, "set", "team", "file://"+source); err != nil {
		t.Fatal(err)
	}
	clone := filepath.Join(dataHome, "devlore", "writ", "repos", filepath.Base(source))

	out, err := runRepoResult(t, "set", "team", yours)
	if err != nil {
		t.Fatalf("re-point away from the clone: %v", err)
	}
	if got := decodeRecord(t, out); got.Previous == nil || got.Previous.Root != clone || got.Previous.Owner != repoOwnerWrit {
		t.Errorf("the record does not name the displaced clone: %+v previous %+v", got, got.Previous)
	}
	if _, err := os.Stat(clone); !os.IsNotExist(err) {
		t.Errorf("the displaced clone survived: %v", err)
	}

	other := workingTree(t)
	if _, err := runRepoResult(t, "set", "team", other); err != nil {
		t.Fatalf("re-point away from your tree: %v", err)
	}
	if _, err := os.Stat(yours); err != nil {
		t.Fatalf("a displaced tree of yours was removed: %v", err)
	}
}

// TestRepo_DryRun_EmitsAndDoesNothing pins the last box of #792: a dry run names what would go, and goes
// nowhere near it.
func TestRepo_DryRun_EmitsAndDoesNothing(t *testing.T) {

	dataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	yours, other := workingTree(t), workingTree(t)

	source := sourceRepository(t)
	if _, err := runRepoResult(t, "set", "team", "file://"+source); err != nil {
		t.Fatal(err)
	}
	clone := filepath.Join(dataHome, "devlore", "writ", "repos", filepath.Base(source))
	if _, err := runRepoResult(t, "set", "personal", yours); err != nil {
		t.Fatal(err)
	}

	// unset, dry: the record names the clone, the clone and the registration both stay.
	out, err := runRepoResult(t, "unset", "team", "--dry-run")
	if err != nil {
		t.Fatalf("dry unset: %v", err)
	}
	if got := decodeRecord(t, out); got.Previous == nil || got.Previous.Root != clone {
		t.Errorf("a dry unset did not name the clone that would go: %+v", got)
	}
	if _, err := os.Stat(filepath.Join(clone, ".git")); err != nil {
		t.Errorf("a dry unset removed the clone: %v", err)
	}
	listed, err := runRepoResult(t, "list")
	if err != nil {
		t.Fatal(err)
	}
	if got := stateOf(t, listed, "team"); got.State != repoStateRegistered {
		t.Errorf("a dry unset unregistered team: %+v", got)
	}

	// set, dry: the record describes the re-point, and nothing moved.
	out, err = runRepoResult(t, "set", "personal", other, "--dry-run")
	if err != nil {
		t.Fatalf("dry set: %v", err)
	}
	if got := decodeRecord(t, out); got.Root != other || got.Previous == nil || got.Previous.Root != yours {
		t.Errorf("a dry set did not describe the re-point: %+v previous %+v", got, got.Previous)
	}
	listed, err = runRepoResult(t, "list")
	if err != nil {
		t.Fatal(err)
	}
	if got := stateOf(t, listed, "personal"); got.Root != yours {
		t.Errorf("a dry set re-pointed personal: %+v", got)
	}

	// set by url, dry: nothing is cloned.
	another := sourceRepository(t)
	if _, err := runRepoResult(t, "set", "base", "file://"+another, "--dry-run"); err != nil {
		t.Fatalf("dry set by url: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dataHome, "devlore", "writ", "repos", filepath.Base(another))); !os.IsNotExist(err) {
		t.Errorf("a dry set cloned: %v", err)
	}
}

// TestRemoveWritClone_RefusesWhatIsNotWrits is the guard on the only deletion in the file.
//
// The callers decide by the record's Owner; this decides again from the path, because a field is a claim and a
// path is a fact. A tree outside the writ home, and the home itself, must both be refused with nothing removed.
func TestRemoveWritClone_RefusesWhatIsNotWrits(t *testing.T) {

	dataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)

	yours := workingTree(t)
	if err := removeWritClone(yours); err == nil {
		t.Fatal("a tree outside the writ home was accepted for removal")
	}
	if _, err := os.Stat(yours); err != nil {
		t.Fatalf("a refused removal still removed the tree: %v", err)
	}

	home := filepath.Join(dataHome, "devlore", "writ", "repos")
	if err := os.MkdirAll(home, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := removeWritClone(home); err == nil {
		t.Fatal("the writ home itself was accepted for removal")
	}
	if _, err := os.Stat(home); err != nil {
		t.Fatalf("a refused removal still removed the home: %v", err)
	}

	clone := filepath.Join(home, "team")
	if err := os.MkdirAll(clone, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := removeWritClone(clone); err != nil {
		t.Fatalf("a clone in the writ home was refused: %v", err)
	}
	if _, err := os.Stat(clone); !os.IsNotExist(err) {
		t.Errorf("the clone survived: %v", err)
	}
}

// TestRepo_RetiredSpellings_AreUnknown pins the rename.
//
// `add`, `remove`, `rm` and `ls` are gone, not aliased: this is a greenfield product, and a second spelling of
// a verb is a second thing to learn, to document, and to keep working.
//
// This test failed when it was written, and the failure was the point: every retired spelling exited 0 and
// printed the group's help, because cobra checks unknown commands at the root alone. [cli.RefuseUnknownVerbs]
// closed that (#897, lane 8), and a user who types the old verb is now told it is gone rather than shown help
// and a success code.
func TestRepo_RetiredSpellings_AreUnknown(t *testing.T) {

	t.Setenv("XDG_DATA_HOME", t.TempDir())
	repo := workingTree(t)

	for _, spelling := range [][]string{
		{"add", "team", repo},
		{"remove", "team"},
		{"rm", "team"},
		{"ls"},
	} {
		t.Run(spelling[0], func(t *testing.T) {
			_, err := runRepo(t, spelling...)
			if err == nil {
				t.Fatalf("%q is still accepted", spelling[0])
			}
			if !strings.Contains(err.Error(), "unknown command") {
				t.Fatalf("%q failed for the wrong reason: %v", spelling[0], err)
			}
		})
	}
}

func TestRepo_Set_Errors(t *testing.T) {

	t.Setenv("XDG_DATA_HOME", t.TempDir())
	repo := workingTree(t)

	if _, err := runRepo(t, "set", "sideways", repo); err == nil || !strings.Contains(err.Error(), "unknown layer") {
		t.Fatalf("expected unknown-layer error, got %v", err)
	}
	if _, err := runRepo(t, "set", "personal", filepath.Join(repo, "absent")); err == nil {
		t.Fatal("expected missing-path error")
	}
	if _, err := runRepo(t, "set", "personal", t.TempDir()); err == nil || !strings.Contains(err.Error(), "not a git working tree") {
		t.Fatalf("expected working-tree validation error, got %v", err)
	}
	if _, err := runRepo(t, "set", "personal", repo, "elsewhere"); err == nil || !strings.Contains(err.Error(), "takes no destination") {
		t.Fatalf("expected no-destination error, got %v", err)
	}
	// --branch applies to a path too (#983); a branch the tree does not have, and no origin to fetch it from, is
	// refused, naming it.
	if _, err := runRepo(t, "set", "personal", "--branch", "main", repo); err == nil || !strings.Contains(err.Error(), "branch main") {
		t.Fatalf("expected a refusal naming the missing branch, got %v", err)
	}

	// Registering twice is a replacement, not a refusal. The case `add` failed on is the case `set` exists for.
	if _, err := runRepo(t, "set", "personal", repo); err != nil {
		t.Fatal(err)
	}
	if _, err := runRepo(t, "set", "personal", repo); err != nil {
		t.Fatalf("setting an already-registered layer must not fail: %v", err)
	}
}

// TestRepo_Set_RefusesBeforeCloning is the defect the old ordering carried.
//
// `add` resolved the location first -- cloning, for the URL form -- and only then discovered a reason to
// refuse, so a repository was copied to disk and left there. Every refusal now precedes the clone, which is
// observable: the destination must not exist afterwards.
func TestRepo_Set_RefusesBeforeCloning(t *testing.T) {

	t.Setenv("XDG_DATA_HOME", t.TempDir())
	source := sourceRepository(t)
	destination := filepath.Join(t.TempDir(), "clone-me-not")

	_, err := runRepo(t, "set", "sideways", "file://"+source, destination)
	if err == nil || !strings.Contains(err.Error(), "unknown layer") {
		t.Fatalf("expected unknown-layer error, got %v", err)
	}
	if _, err := os.Stat(destination); err == nil {
		t.Fatalf("%s was cloned before the refusal", destination)
	}
}

// TestRepo_Unset_Unregistered_Succeeds inverts what `remove` asserted.
//
// Ruled 2026-09-18: unsetting a layer that is not registered is the state the caller asked for, not a failure.
// Twice in a row must also succeed, which is what makes it usable from a script.
func TestRepo_Unset_Unregistered_Succeeds(t *testing.T) {

	t.Setenv("XDG_DATA_HOME", t.TempDir())

	out, err := runRepoResult(t, "unset", "team")
	if err != nil {
		t.Fatalf("unsetting an unregistered layer: %v", err)
	}
	var record RepoRegistration
	if err := json.Unmarshal([]byte(out), &record); err != nil {
		t.Fatalf("unset emitted no record: %v\n%s", err, out)
	}
	if record.Layer != "team" || record.State != repoStateUnregistered {
		t.Fatalf("unset emitted %+v; want team unregistered", record)
	}

	if _, err := runRepo(t, "unset", "team"); err != nil {
		t.Fatalf("unsetting twice: %v", err)
	}
}

func TestRepo_List_MarksBrokenLink(t *testing.T) {

	dataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	repo := workingTree(t)

	if _, err := runRepo(t, "set", "personal", repo); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(repo); err != nil {
		t.Fatal(err)
	}

	listed, err := runRepoResult(t, "list")
	if err != nil {
		t.Fatal(err)
	}
	if got := stateOf(t, listed, "personal"); got.State != repoStateBroken {
		t.Fatalf("personal = %+v after its target was removed; want broken", got)
	}
}

// TestRepo_List_EmptyLayerDirectoryIsUnregistered is #1030: the empty directories an earlier `self install` left
// where the layers go are no layer. A layer is a git working tree, as `repo set` requires.
func TestRepo_List_EmptyLayerDirectoryIsUnregistered(t *testing.T) {

	t.Setenv("XDG_DATA_HOME", t.TempDir())
	for _, layer := range LayerOrder {
		if err := os.MkdirAll(filepath.Join(devlore.WritLayersDir(), layer), 0o750); err != nil {
			t.Fatal(err)
		}
	}

	listed, err := runRepoResult(t, "list")
	if err != nil {
		t.Fatal(err)
	}
	for _, layer := range LayerOrder {
		if got := stateOf(t, listed, layer); got.State != repoStateUnregistered || got.Root != "" {
			t.Errorf("%s = %+v over an empty directory; want unregistered, with no root", layer, got)
		}
	}
}

// TestRepo_List_LinkToATreeThatIsNotARepositoryIsBroken is #1030: a link to a directory with no `.git` names no
// layer writ can deploy from, so it is reported for `repo set` to repair, not as registered.
func TestRepo_List_LinkToATreeThatIsNotARepositoryIsBroken(t *testing.T) {

	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if err := os.MkdirAll(devlore.WritLayersDir(), 0o750); err != nil {
		t.Fatal(err)
	}
	tree := t.TempDir()
	if err := os.Symlink(tree, filepath.Join(devlore.WritLayersDir(), "team")); err != nil {
		t.Fatal(err)
	}

	listed, err := runRepoResult(t, "list")
	if err != nil {
		t.Fatal(err)
	}
	if got := stateOf(t, listed, "team"); got.State != repoStateBroken {
		t.Fatalf("team = %+v, a link to a tree with no .git; want broken", got)
	}
}

// TestRepo_Set_ReplacesAnEmptyLayerDirectory is #1030: registering a layer where an earlier `self install` left an
// empty directory puts the registration in its place.
func TestRepo_Set_ReplacesAnEmptyLayerDirectory(t *testing.T) {

	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if err := os.MkdirAll(filepath.Join(devlore.WritLayersDir(), "personal"), 0o750); err != nil {
		t.Fatal(err)
	}
	repo := workingTree(t)

	if _, err := runRepo(t, "set", "personal", repo); err != nil {
		t.Fatalf("repo set over an empty layer directory: %v", err)
	}

	listed, err := runRepoResult(t, "list")
	if err != nil {
		t.Fatal(err)
	}
	if got := stateOf(t, listed, "personal"); got.State != repoStateRegistered || got.Root != repo {
		t.Fatalf("personal = %+v after repo set; want registered at %s", got, repo)
	}
}

func TestIsRepositoryURL_Table(t *testing.T) {

	cases := []struct {
		location string
		want     bool
	}{
		{"https://github.com/me/x.git", true},
		{"ssh://git@host/x.git", true},
		{"file:///abs/src.git", true},
		{"git@github.com:me/x.git", true},
		{"host:path/to/repo", true},
		{"/abs/path", false},
		{"relative/path", false},
		{"./x:y", false},
		{`D:\a\b`, false},
		{"C:/x", false},
		{"~/Workspace/Personal", false},
	}
	for _, c := range cases {
		if got := isRepositoryURL(c.location); got != c.want {
			t.Errorf("isRepositoryURL(%q) = %v, want %v", c.location, got, c.want)
		}
	}
}

// TestHumanishName_Table pins #793's rule on the URL forms writ admits: a `.git` suffix, a trailing `/`, the
// bare form, the scp-like form with no slash, and a URL that yields no name.
func TestHumanishName_Table(t *testing.T) {

	cases := []struct {
		url  string
		name string
	}{
		{"git@github.com:NobleFactor/noblefactor-ops.git", "noblefactor-ops"},
		{"https://github.com/David-Noble-at-work/personal/", "personal"},
		{"file:///tmp/env", "env"},
		{"ssh://git@host/x/y.git/", "y"},
		{"host:env.git", "env"},
	}
	for _, c := range cases {
		got, err := humanishName(c.url)
		if err != nil {
			t.Errorf("humanishName(%q): %v", c.url, err)
			continue
		}
		if got != c.name {
			t.Errorf("humanishName(%q) = %q, want %q", c.url, got, c.name)
		}
	}

	for _, url := range []string{"https://", "file:///", "host:"} {
		if got, err := humanishName(url); err == nil {
			t.Errorf("humanishName(%q) = %q; want a refusal, as git gives", url, got)
		}
	}
}

// TestRepo_Set_ClonesURL_DefaultHome is #793 through the command: a URL without a destination clones into the
// writ-owned home under the name git gives it -- `team-env`, from `team-env.git` -- and not under its layer.
func TestRepo_Set_ClonesURL_DefaultHome(t *testing.T) {

	dataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	source := sourceRepositoryNamed(t, "team-env.git")

	out, err := runRepoResult(t, "set", "team", "file://"+source)
	if err != nil {
		t.Fatalf("set url: %v", err)
	}

	expected := filepath.Join(dataHome, "devlore", "writ", "repos", "team-env")

	var record RepoRegistration
	if err := json.Unmarshal([]byte(out), &record); err != nil {
		t.Fatalf("set emitted no record: %v\n%s", err, out)
	}
	if record.Root != expected || record.State != repoStateRegistered {
		t.Fatalf("set emitted %+v; want it registered at the writ-owned home %s", record, expected)
	}
	if _, err := os.Stat(filepath.Join(expected, ".git")); err != nil {
		t.Fatalf("clone missing at default home: %v", err)
	}
}

// TestRepo_Set_RefusesTwoLayersOneName is #793's other half: two repositories with one name would clone to one
// directory, so the second layer is refused, naming both, before anything is cloned.
func TestRepo_Set_RefusesTwoLayersOneName(t *testing.T) {

	dataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	first, second := sourceRepositoryNamed(t, "env.git"), sourceRepositoryNamed(t, "env.git")

	if _, err := runRepoResult(t, "set", "base", "file://"+first); err != nil {
		t.Fatal(err)
	}
	clone := filepath.Join(dataHome, "devlore", "writ", "repos", "env")

	_, narration, err := runRepoStreams(t, "set", "team", "file://"+second)
	if err == nil {
		t.Fatal("a second layer resolving to the same clone name was accepted")
	}
	for _, name := range []string{"base", "team"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("the refusal does not name %s: %v", name, err)
		}
	}
	if strings.Contains(narration, "Cloning into") {
		t.Errorf("the refusal came after a clone was attempted:\n%s", narration)
	}

	listed, err := runRepoResult(t, "list")
	if err != nil {
		t.Fatal(err)
	}
	if got := stateOf(t, listed, "team"); got.State != repoStateUnregistered {
		t.Errorf("team was registered despite the refusal: %+v", got)
	}
	if got := stateOf(t, listed, "base"); got.Root != clone {
		t.Errorf("base's clone moved: %+v", got)
	}
}

func TestRepo_Set_ClonesURL_PositionalDestination(t *testing.T) {

	t.Setenv("XDG_DATA_HOME", t.TempDir())
	source := sourceRepository(t)
	destination := filepath.Join(t.TempDir(), "Personal")

	if _, err := runRepo(t, "set", "personal", "file://"+source, destination); err != nil {
		t.Fatalf("set url with destination: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destination, ".git")); err != nil {
		t.Fatalf("clone missing at positional destination: %v", err)
	}

	if _, err := runRepo(t, "unset", "personal"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(destination); err != nil {
		t.Fatal("unset must not delete a clone the user named a destination for")
	}
}

// --- #983: what an earlier run left, and the branch asked for ---

// runRepoNarrated is [runRepoStreams] with writ's narration captured: set says what it found and did through the
// narrator, which the root binds to os.Stderr when the command starts, so os.Stderr is a file for the run. These
// tests set the environment, so none runs in parallel with another.
func runRepoNarrated(t *testing.T, args ...string) (string, error) {

	t.Helper()

	captured, err := os.CreateTemp(t.TempDir(), "narration")
	if err != nil {
		t.Fatal(err)
	}
	previous := os.Stderr
	os.Stderr = captured
	_, streams, err := runRepoStreams(t, args...)
	os.Stderr = previous

	narration, readErr := os.ReadFile(captured.Name())
	if readErr != nil {
		t.Fatal(readErr)
	}
	_ = captured.Close()

	return string(narration) + streams, err
}

// gitIn runs git in `root` with the test's isolated configuration and an identity to commit with, failing the test
// when git does.
func gitIn(t *testing.T, root string, args ...string) string {

	t.Helper()

	full := append([]string{"-C", root, "-c", "user.name=t", "-c", "user.email=t@invalid"}, args...)
	command := exec.CommandContext(context.Background(), "git", full...)
	command.Env = isolatedGitEnv(t)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}

	return strings.TrimSpace(string(output))
}

// commitFile writes `content` to `file` in `root` and commits it.
func commitFile(t *testing.T, root, file, content string) {

	t.Helper()

	if err := os.WriteFile(filepath.Join(root, file), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	gitIn(t, root, "add", file)
	gitIn(t, root, "commit", "--quiet", "-m", "set "+file+" to "+content)
}

// cloneOf clones `source` to a temporary working tree and returns it, its origin `source`.
func cloneOf(t *testing.T, source string) string {

	t.Helper()

	tree := filepath.Join(t.TempDir(), "tree")
	gitIn(t, filepath.Dir(tree), "clone", "--quiet", source, tree)

	return tree
}

// TestRepo_Set_UsesTheCloneAnEarlierRunLeft is #983: a clone that landed, its registration never written, is at the
// destination when the same command runs again, which registers it rather than refusing or cloning again.
func TestRepo_Set_UsesTheCloneAnEarlierRunLeft(t *testing.T) {

	dataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	source := sourceRepositoryNamed(t, "env.git")
	url := "file://" + source
	clone := filepath.Join(dataHome, "devlore", "writ", "repos", "env")
	if err := os.MkdirAll(filepath.Dir(clone), 0o750); err != nil {
		t.Fatal(err)
	}
	gitIn(t, filepath.Dir(clone), "clone", "--quiet", url, clone)

	narration, err := runRepoNarrated(t, "set", "team", url)
	if err != nil {
		t.Fatalf("set over the clone an earlier run left: %v\n%s", err, narration)
	}
	if strings.Contains(narration, "Cloning into") {
		t.Errorf("the clone an earlier run left was cloned again:\n%s", narration)
	}
	if !strings.Contains(narration, "found the clone of "+url+" at "+clone+", on main") {
		t.Errorf("set did not say it found the clone:\n%s", narration)
	}

	listed, err := runRepoResult(t, "list")
	if err != nil {
		t.Fatal(err)
	}
	if got := stateOf(t, listed, "team"); got.State != repoStateRegistered || got.Root != clone {
		t.Errorf("team = %+v; want registered at %s", got, clone)
	}
}

// TestRepo_Set_RefusesACloneOfAnotherRemote is #983's other half: a destination holding a clone of another
// repository is refused, naming it, and nothing is registered.
func TestRepo_Set_RefusesACloneOfAnotherRemote(t *testing.T) {

	dataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	wanted, other := sourceRepositoryNamed(t, "env.git"), sourceRepositoryNamed(t, "env.git")
	clone := filepath.Join(dataHome, "devlore", "writ", "repos", "env")
	if err := os.MkdirAll(filepath.Dir(clone), 0o750); err != nil {
		t.Fatal(err)
	}
	gitIn(t, filepath.Dir(clone), "clone", "--quiet", "file://"+other, clone)

	_, _, err := runRepoStreams(t, "set", "team", "file://"+wanted)
	if err == nil || !strings.Contains(err.Error(), "file://"+other) {
		t.Fatalf("set over a clone of another remote = %v; want a refusal naming file://%s", err, other)
	}
}

// TestRepo_Set_RefusesADestinationThatIsNotATree is #983: a destination that is not a git working tree is refused.
func TestRepo_Set_RefusesADestinationThatIsNotATree(t *testing.T) {

	dataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	source := sourceRepositoryNamed(t, "env.git")
	clone := filepath.Join(dataHome, "devlore", "writ", "repos", "env")
	if err := os.MkdirAll(clone, 0o750); err != nil {
		t.Fatal(err)
	}

	_, _, err := runRepoStreams(t, "set", "team", "file://"+source)
	if err == nil || !strings.Contains(err.Error(), "not a git working tree") {
		t.Fatalf("set over a plain directory = %v; want a refusal saying it is not a git working tree", err)
	}
}

// TestRepo_Set_WithoutABranchLeavesTheTree is #983's ruling: with no --branch, the tree stays on what it has
// checked out, and set says which.
func TestRepo_Set_WithoutABranchLeavesTheTree(t *testing.T) {

	t.Setenv("XDG_DATA_HOME", t.TempDir())
	tree := cloneOf(t, sourceRepository(t))
	gitIn(t, tree, "switch", "--quiet", "--create", "work")

	narration, err := runRepoNarrated(t, "set", "personal", tree)
	if err != nil {
		t.Fatalf("set: %v\n%s", err, narration)
	}
	if !strings.Contains(narration, "found a repository at "+tree+", on work") {
		t.Errorf("set did not say the tree's branch:\n%s", narration)
	}
	if got := gitIn(t, tree, "branch", "--show-current"); got != "work" {
		t.Errorf("the tree moved to %s; want it left on work", got)
	}
}

// TestRepo_Set_SwitchesToALocalBranch is #983: --branch with a path switches the tree, and says from what.
func TestRepo_Set_SwitchesToALocalBranch(t *testing.T) {

	t.Setenv("XDG_DATA_HOME", t.TempDir())
	tree := cloneOf(t, sourceRepository(t))
	gitIn(t, tree, "branch", "work")

	narration, err := runRepoNarrated(t, "set", "personal", tree, "--branch", "work")
	if err != nil {
		t.Fatalf("set --branch work: %v\n%s", err, narration)
	}
	if got := gitIn(t, tree, "branch", "--show-current"); got != "work" {
		t.Errorf("the tree is on %s; want work", got)
	}
	if !strings.Contains(narration, "on work, switched from main to work") {
		t.Errorf("set did not say it switched:\n%s", narration)
	}
}

// TestRepo_Set_FetchesABranchPushedSince is #983's D2: a branch origin gained after the tree's last fetch is
// fetched, and switched to, the local branch tracking origin's.
func TestRepo_Set_FetchesABranchPushedSince(t *testing.T) {

	t.Setenv("XDG_DATA_HOME", t.TempDir())
	source := sourceRepository(t)
	tree := cloneOf(t, source)
	gitIn(t, source, "branch", "later")

	narration, err := runRepoNarrated(t, "set", "personal", tree, "--branch", "later")
	if err != nil {
		t.Fatalf("set --branch later: %v\n%s", err, narration)
	}
	if !strings.Contains(narration, "fetched origin") {
		t.Errorf("set did not say it fetched:\n%s", narration)
	}
	if got := gitIn(t, tree, "rev-parse", "--abbrev-ref", "later@{upstream}"); got != "origin/later" {
		t.Errorf("later tracks %q; want origin/later", got)
	}
}

// TestRepo_Set_RefusesABranchThatIsNowhere is #983: a branch in neither the tree nor origin is refused, and the
// layer is not registered.
func TestRepo_Set_RefusesABranchThatIsNowhere(t *testing.T) {

	t.Setenv("XDG_DATA_HOME", t.TempDir())
	tree := cloneOf(t, sourceRepository(t))

	_, _, err := runRepoStreams(t, "set", "personal", tree, "--branch", "nowhere")
	if err == nil || !strings.Contains(err.Error(), "nowhere") {
		t.Fatalf("set --branch nowhere = %v; want a refusal naming the branch", err)
	}

	listed, err := runRepoResult(t, "list")
	if err != nil {
		t.Fatal(err)
	}
	if got := stateOf(t, listed, "personal"); got.State != repoStateUnregistered {
		t.Errorf("personal = %+v; want unregistered after the refusal", got)
	}
}

// TestRepo_Set_RefusesASwitchUncommittedChangesBlock is #983: git switch refuses to overwrite uncommitted changes,
// and so does set, registering nothing and leaving the change where it was.
func TestRepo_Set_RefusesASwitchUncommittedChangesBlock(t *testing.T) {

	t.Setenv("XDG_DATA_HOME", t.TempDir())
	source := sourceRepository(t)
	commitFile(t, source, "f", "main")
	gitIn(t, source, "switch", "--quiet", "--create", "other")
	commitFile(t, source, "f", "other")
	gitIn(t, source, "switch", "--quiet", "main")
	tree := cloneOf(t, source)
	if err := os.WriteFile(filepath.Join(tree, "f"), []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, _, err := runRepoStreams(t, "set", "personal", tree, "--branch", "other")
	if err == nil || !strings.Contains(err.Error(), "git switch other") {
		t.Fatalf("set --branch other over an uncommitted change = %v; want git switch's refusal", err)
	}

	if content, _ := os.ReadFile(filepath.Join(tree, "f")); string(content) != "mine" {
		t.Errorf("the uncommitted change became %q; want it left as mine", content)
	}
	listed, err := runRepoResult(t, "list")
	if err != nil {
		t.Fatal(err)
	}
	if got := stateOf(t, listed, "personal"); got.State != repoStateUnregistered {
		t.Errorf("personal = %+v; want unregistered after the refusal", got)
	}
}

// TestRepo_Set_FastForwardsABranchBehind is #983's D2: a branch behind origin's is brought up to date.
func TestRepo_Set_FastForwardsABranchBehind(t *testing.T) {

	t.Setenv("XDG_DATA_HOME", t.TempDir())
	source := sourceRepository(t)
	tree := cloneOf(t, source)
	commitFile(t, source, "f", "newer")

	narration, err := runRepoNarrated(t, "set", "personal", tree, "--branch", "main")
	if err != nil {
		t.Fatalf("set --branch main: %v\n%s", err, narration)
	}
	if gitIn(t, tree, "rev-parse", "HEAD") != gitIn(t, source, "rev-parse", "HEAD") {
		t.Errorf("main was not brought up to origin's:\n%s", narration)
	}
	if !strings.Contains(narration, "fast-forwarded main to origin/main") {
		t.Errorf("set did not say it fast-forwarded:\n%s", narration)
	}
}

// TestRepo_Set_LeavesADivergedBranch is #983's D2: a branch with commits origin lacks is the user's, left as it is
// and said so.
func TestRepo_Set_LeavesADivergedBranch(t *testing.T) {

	t.Setenv("XDG_DATA_HOME", t.TempDir())
	source := sourceRepository(t)
	tree := cloneOf(t, source)
	commitFile(t, source, "f", "theirs")
	commitFile(t, tree, "g", "mine")
	mine := gitIn(t, tree, "rev-parse", "HEAD")

	narration, err := runRepoNarrated(t, "set", "personal", tree, "--branch", "main")
	if err != nil {
		t.Fatalf("set --branch main: %v\n%s", err, narration)
	}
	if got := gitIn(t, tree, "rev-parse", "HEAD"); got != mine {
		t.Errorf("a diverged main moved from %s to %s", mine, got)
	}
	if !strings.Contains(narration, "main has commits origin lacks; left as it is") {
		t.Errorf("set did not say it left main:\n%s", narration)
	}
}
