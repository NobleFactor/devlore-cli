// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// The writ-deploy scenario harness (docs/plans/writ-deploy-scenario.md): a pristine sandbox user drives the
// real writ binary as a subprocess against a personal-layer repo. Gated behind WRIT_SCENARIO_RUN=1 (the
// test-scenario make target) so make test stays fast while the file stays compiled and linted.
package main_test

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
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

	root := t.TempDir()
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
			"TMPDIR=" + os.TempDir(),
		},
	}
}

// writBinary returns the built writ binary's path, failing with the build instruction when it is absent.
func writBinary(t *testing.T) string {

	t.Helper()

	path, err := filepath.Abs(filepath.Join("..", "..", "build", "writ"))
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
		return dest
	}

	copyFixture(t, filepath.Join("testdata", "personal-repo"), dest)
	return dest
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
