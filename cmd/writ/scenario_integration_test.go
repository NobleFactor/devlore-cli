// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// The writ-deploy scenario harness (docs/plans/writ-deploy-scenario.md): a pristine sandbox user drives the
// real writ binary as a subprocess against a personal-layer repo. Gated behind WRIT_SCENARIO_RUN=1 (the
// test-scenario make target) so make test stays fast while the file stays compiled and linted.
package main_test

import (
	"archive/tar"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// scenarioSandbox is the pristine fake-user world: a fresh home, redirected XDG homes, the personal repo, and
// the environment every writ subprocess runs under.
type scenarioSandbox struct {
	Root string   // the sandbox root
	Home string   // the fake $HOME
	Repo string   // the personal-layer repo inside the sandbox
	Env  []string // the controlled subprocess environment
}

// newScenarioSandbox builds the sandbox: fresh HOME and XDG homes, the personal repo materialized (fixture by
// default, the real branch via WRIT_SCENARIO_REPO), and the personal layer registered through the settled
// packaging mechanism — the layers-dir symlink (config plays no part; the config-vs-layers separation).
func newScenarioSandbox(t *testing.T) *scenarioSandbox {

	t.Helper()

	if os.Getenv("WRIT_SCENARIO_RUN") == "" {
		t.Skip("scenario harness runs under make test-scenario (WRIT_SCENARIO_RUN=1)")
	}

	// Canonicalize the sandbox root: macOS puts temp dirs behind the /var -> /private/var symlink,
	// and writ's relative-link math is lexical — a symlinked prefix would skew the climb depth.
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(root, "home")
	dataHome := filepath.Join(root, "data")
	repo := materializePersonalRepo(t, root)

	for _, dir := range []string{home, filepath.Join(root, "config"), filepath.Join(root, "state"), dataHome} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	layers := filepath.Join(dataHome, "devlore", "writ", "layers")
	if err := os.MkdirAll(layers, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(repo, filepath.Join(layers, "personal")); err != nil {
		t.Fatal(err)
	}

	binDir := filepath.Dir(writBinary(t))

	return &scenarioSandbox{
		Root: root,
		Home: home,
		Repo: repo,
		Env: []string{
			"PATH=" + binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
			"HOME=" + home,
			"USERPROFILE=" + home,
			"XDG_CONFIG_HOME=" + filepath.Join(root, "config"),
			"XDG_STATE_HOME=" + filepath.Join(root, "state"),
			"XDG_DATA_HOME=" + dataHome,
			"XDG_CACHE_HOME=" + filepath.Join(root, "cache"),
			"TMPDIR=" + os.TempDir(),
		},
	}
}

// writBinary returns the built writ binary's path, failing with the build instruction when it is absent.
func writBinary(t *testing.T) string {

	t.Helper()

	binary := "writ"
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	path, err := filepath.Abs(filepath.Join("..", "..", "build", binary))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("writ binary missing at %s — run make test-scenario (it builds first): %v", path, err)
	}
	return path
}

// materializePersonalRepo produces the personal-layer repo inside the sandbox: the checked-in fixture by
// default; with WRIT_SCENARIO_REPO set, the named repo's scenario branch (WRIT_SCENARIO_BRANCH, default
// devlore-cli/writ-layer) extracted via git archive so the owner's checkout is never disturbed.
func materializePersonalRepo(t *testing.T, root string) string {

	t.Helper()

	dest := filepath.Join(root, "Workspace", "Personal")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}

	if source := os.Getenv("WRIT_SCENARIO_REPO"); source != "" {
		branch := os.Getenv("WRIT_SCENARIO_BRANCH")
		if branch == "" {
			branch = "devlore-cli/writ-layer"
		}
		extractGitArchive(t, source, branch, dest)
		initializeRepo(t, dest)
		return dest
	}

	copyFixture(t, filepath.Join("testdata", "personal-repo"), dest)
	initializeRepo(t, dest)
	return dest
}

// initializeRepo turns the materialized tree into a committed git repository — deploy pins layer sources to
// git-worktree snapshots and refuses layers that are not clean repos.
func initializeRepo(t *testing.T, dest string) {

	t.Helper()

	for _, args := range [][]string{
		{"init", "--quiet", "--initial-branch=main"},
		{"add", "-A"},
		{"-c", "user.name=scenario", "-c", "user.email=scenario@invalid", "commit", "--quiet", "-m", "scenario baseline"},
	} {
		cmd := exec.CommandContext(context.Background(), "git", append([]string{"-C", dest}, args...)...)
		// The real user's global git config must not reach the sandbox repo (branch-protection
		// hooks would reject the baseline commit on main).
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
}

// copyFixture copies the checked-in fixture tree into the sandbox destination.
func copyFixture(t *testing.T, source, dest string) {

	t.Helper()

	err := filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dest, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// extractGitArchive materializes `branch` of the repo at `source` into `dest` by streaming git archive
// through an in-process tar reader — read-only against the repo, no checkout disturbance.
func extractGitArchive(t *testing.T, source, branch, dest string) {

	t.Helper()

	cmd := exec.CommandContext(context.Background(), "git", "-C", source, "archive", branch)
	stream, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	reader := tar.NewReader(stream)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		target, err := containedPath(dest, header.Name)
		if err != nil {
			t.Fatal(err)
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				t.Fatal(err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				t.Fatal(err)
			}
			data, err := io.ReadAll(reader)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(target, data, os.FileMode(header.Mode)&0o777); err != nil {
				t.Fatal(err)
			}
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(header.Linkname, target); err != nil {
				t.Fatal(err)
			}
		}
	}

	if err := cmd.Wait(); err != nil {
		t.Fatalf("git archive %s %s: %v", source, branch, err)
	}
}

// containedPath joins an archive entry name onto dest, refusing names that would escape it.
func containedPath(dest, name string) (string, error) {

	target := filepath.Join(dest, filepath.FromSlash(name))
	if !strings.HasPrefix(target, filepath.Clean(dest)+string(os.PathSeparator)) {
		return "", fmt.Errorf("archive entry %q escapes the sandbox", name)
	}
	return target, nil
}

// runWrit runs the built writ binary inside the sandbox and returns its combined outcome.
func runWrit(t *testing.T, sandbox *scenarioSandbox, args ...string) (stdout, stderr string, err error) {

	t.Helper()

	cmd := exec.CommandContext(context.Background(), writBinary(t), args...)
	cmd.Dir = sandbox.Home
	cmd.Env = sandbox.Env

	var outBuffer, errBuffer strings.Builder
	cmd.Stdout = &outBuffer
	cmd.Stderr = &errBuffer
	err = cmd.Run()
	return outBuffer.String(), errBuffer.String(), err
}

// TestWritDeployScenario_Harness is the phase-1 deliverable: the sandbox stands up — pristine homes, the
// personal repo materialized, the layer registered — and the real writ binary runs green inside it.
func TestWritDeployScenario_Harness(t *testing.T) {

	sandbox := newScenarioSandbox(t)

	stdout, stderr, err := runWrit(t, sandbox, "--help")
	if err != nil {
		t.Fatalf("writ --help failed: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stdout, "writ") {
		t.Fatalf("writ --help output does not mention writ:\n%s", stdout)
	}

	resolved, err := filepath.EvalSymlinks(filepath.Join(sandbox.Root, "data", "devlore", "writ", "layers", "personal"))
	if err != nil {
		t.Fatalf("personal layer symlink does not resolve: %v", err)
	}
	expected, err := filepath.EvalSymlinks(sandbox.Repo)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != expected {
		t.Fatalf("layer symlink resolves to %s, want %s", resolved, expected)
	}
}

// assertLinked asserts `path` is a symlink that resolves to readable content containing `want`.
func assertLinked(t *testing.T, path, want string) {

	t.Helper()

	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("expected symlink at %s: %v", path, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected %s to be a symlink, mode %v", path, info.Mode())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		destination, _ := os.Readlink(path) //nolint:errcheck // diagnostic best effort
		t.Fatalf("symlink %s (-> %s) does not resolve: %v", path, destination, err)
	}
	if !strings.Contains(string(data), want) {
		t.Fatalf("%s content %q does not contain %q", path, data, want)
	}
}

// assertRendered asserts `path` is a regular file (a rendered copy, not a link) containing every want.
func assertRendered(t *testing.T, path string, wants ...string) {

	t.Helper()

	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("expected rendered file at %s: %v", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("expected %s to be a rendered copy, found a symlink", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range wants {
		if !strings.Contains(string(data), want) {
			t.Fatalf("%s content %q does not contain %q", path, data, want)
		}
	}
}

// assertAbsent asserts nothing exists at `path`.
func assertAbsent(t *testing.T, path string) {

	t.Helper()

	if _, err := os.Lstat(path); err == nil {
		t.Fatalf("expected nothing at %s", path)
	}
}

// segmentOS returns the capitalized OS segment value for the running platform.
func segmentOS() string {

	switch runtime.GOOS {
	case "darwin":
		return "Darwin"
	case "linux":
		return "Linux"
	default:
		return runtime.GOOS
	}
}

// TestWritDeployScenario_Deploy is the phase-2 leg: deploy noblefactor and thenobles into the sandbox, then
// assert the deployed filesystem, the status report, the execution store, and a clean second deploy.
func TestWritDeployScenario_Deploy(t *testing.T) {

	sandbox := newScenarioSandbox(t)

	stdout, stderr, err := runWrit(t, sandbox, "deploy", "noblefactor", "thenobles")
	if err != nil {
		t.Fatalf("writ deploy failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	// The per-file inventory assertions are fixture-specific; a real repo (WRIT_SCENARIO_REPO) carries
	// the owner's content, so that mode asserts the generic invariants only (status, store, re-deploy).
	fixtureMode := os.Getenv("WRIT_SCENARIO_REPO") == ""

	// The deployed inventory: base dot-content on every platform, segment variants by matching, the
	// template rendered (suffix stripped, copied not linked), undeployed projects absent.
	scenario := filepath.Join(sandbox.Home, ".config", "scenario")
	if fixtureMode {
		assertLinked(t, filepath.Join(scenario, "base.conf"), "noblefactor (base dot-content)")
		assertLinked(t, filepath.Join(scenario, "shared.conf"), "thenobles (base dot-content)")

		if runtime.GOOS == "darwin" || runtime.GOOS == "linux" {
			assertLinked(t, filepath.Join(sandbox.Home, "local", "share", "scenario", "nf-unix.conf"), "noblefactor.Unix")
			assertRendered(t, filepath.Join(scenario, "writ.conf"), "os = "+segmentOS(), "arch = "+runtime.GOARCH)
		}
		tnDarwin := filepath.Join(sandbox.Home, "local", "share", "scenario", "tn-darwin.conf")
		if runtime.GOOS == "darwin" {
			assertLinked(t, tnDarwin, "thenobles.Darwin")
		} else {
			assertAbsent(t, tnDarwin)
		}
		assertAbsent(t, filepath.Join(sandbox.Home, "local", "share", "scenario", "all.conf"))
		assertAbsent(t, filepath.Join(sandbox.Home, "scenario-note.md"))
	}

	// The status report, machine-readable: every classified entry is healthy.
	statusOut, statusErr, err := runWrit(t, sandbox, "status", "--json")
	if err != nil {
		t.Fatalf("writ status failed: %v\nstderr: %s", err, statusErr)
	}
	var report struct {
		Entries []struct {
			State   string `json:"state"`
			Target  string `json:"target"`
			Project string `json:"project"`
		} `json:"entries"`
	}
	if err := json.Unmarshal([]byte(statusOut), &report); err != nil {
		t.Fatalf("status --json is not parseable: %v\n%s", err, statusOut)
	}
	if len(report.Entries) < 3 {
		t.Fatalf("status reports %d entries, expected at least 3:\n%s", len(report.Entries), statusOut)
	}
	for _, entry := range report.Entries {
		if entry.State != "linked" && entry.State != "copied" {
			t.Fatalf("entry %s (%s) has state %q, expected linked or copied", entry.Target, entry.Project, entry.State)
		}
	}

	// The execution store: at least one persisted graph, one timestamped trace, and a non-empty run index.
	stateHome := filepath.Join(sandbox.Root, "state", "devlore")
	graphs, err := filepath.Glob(filepath.Join(stateHome, "graphs", "*.yaml"))
	if err != nil || len(graphs) == 0 {
		t.Fatalf("no persisted graphs under %s (err %v)", stateHome, err)
	}
	traces, err := filepath.Glob(filepath.Join(stateHome, "traces", "*", "2*.yaml"))
	if err != nil || len(traces) == 0 {
		t.Fatalf("no persisted traces under %s (err %v)", stateHome, err)
	}
	index, err := os.ReadFile(filepath.Join(stateHome, "index.ndjson"))
	if err != nil || len(index) == 0 {
		t.Fatalf("run index missing or empty (err %v)", err)
	}

	// A second deploy against the already-deployed home succeeds (the commit-keyed snapshot makes the
	// existing links already correct) and appends traces rather than conflicting.
	_, stderr2, err := runWrit(t, sandbox, "deploy", "noblefactor", "thenobles")
	if err != nil {
		t.Fatalf("second writ deploy failed: %v\nstderr: %s", err, stderr2)
	}
	tracesAfter, err := filepath.Glob(filepath.Join(stateHome, "traces", "*", "2*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(tracesAfter) <= len(traces) {
		t.Fatalf("second deploy added no trace: %d before, %d after", len(traces), len(tracesAfter))
	}
}
