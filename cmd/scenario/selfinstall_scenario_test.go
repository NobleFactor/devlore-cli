// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package scenario

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// selfInstallTools are the tools that install themselves. Every one of them must lay out the same tree.
var selfInstallTools = []string{"lore", "star", "writ"}

// TestSelfInstallScenario drives `self install` and `self uninstall` through the real binaries, per tool, on
// whatever platform the runner is.
//
// This is the install path's only end-to-end coverage. The unit tests in cmd/internal/cli call runSelfInstall
// in process, which proves the layout but not that a shipped binary can install itself — the difference that
// matters on Windows, where the binary carries an extension and the runner's filesystem semantics differ.
//
// Modes are deliberately not asserted: `Mode().Perm()` reports 0666 on Windows whatever the DACL says (#405
// ruling 5). Enforcement is proved by the DACL read-back tests, not here.
func TestSelfInstallScenario(t *testing.T) {

	if os.Getenv("DEVLORE_SCENARIO_RUN") == "" {
		t.Skip("scenario runs under make test-scenario (DEVLORE_SCENARIO_RUN=1)")
	}

	for _, tool := range selfInstallTools {
		t.Run(tool, func(t *testing.T) {

			binary := toolBinary(t, tool)
			sandbox := t.TempDir()
			prefix := filepath.Join(sandbox, "prefix")
			environment := sandboxEnvironment(sandbox)

			// Install, naming the shell so the completion assertion holds on a runner with no shells.
			if out, err := run(t, binary, environment, "self", "install", prefix, "--shell", "bash"); err != nil {
				t.Fatalf("%s self install failed: %v\n%s", tool, err, out)
			}

			for _, relative := range []string{
				filepath.Join("bin", tool+exeSuffix()),
				filepath.Join("share", "bash-completion", "completions", tool),
				filepath.Join("share", tool, "manifest.json"),
			} {
				if _, err := os.Stat(filepath.Join(prefix, relative)); err != nil {
					t.Errorf("%s install did not produce %s: %v", tool, relative, err)
				}
			}

			recorded := manifestFiles(t, prefix, tool)
			if len(recorded) == 0 {
				t.Fatalf("%s manifest records no files", tool)
			}
			for _, relative := range recorded {
				if _, err := os.Stat(filepath.Join(prefix, relative)); err != nil {
					t.Errorf("%s manifest names %s, which does not exist: %v", tool, relative, err)
				}
			}

			// Uninstall takes away exactly what the manifest recorded.
			if out, err := run(t, binary, environment, "self", "uninstall", prefix, "--force"); err != nil {
				t.Fatalf("%s self uninstall failed: %v\n%s", tool, err, out)
			}

			for _, relative := range recorded {
				if _, err := os.Stat(filepath.Join(prefix, relative)); !os.IsNotExist(err) {
					t.Errorf("%s left %s behind after uninstall (err = %v)", tool, relative, err)
				}
			}
		})
	}
}

// toolBinary returns the built binary's path, failing with the build instruction when it is absent.
//
// Parameters:
//   - `t`: the test harness.
//   - `tool`: the tool name.
//
// Returns:
//   - `string`: the absolute path to the built binary.
func toolBinary(t *testing.T, tool string) string {

	t.Helper()

	// Products live under build/<goos>-<goarch>/ so one host can build every platform; only this
	// machine's own directory holds a binary it can execute.
	path, err := filepath.Abs(
		filepath.Join("..", "..", "build", runtime.GOOS+"-"+runtime.GOARCH, tool+exeSuffix()),
	)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("%s not built — run `make build` first: %v", path, err)
	}

	return path
}

// sandboxEnvironment returns a controlled environment whose every devlore location sits inside `sandbox`.
//
// `self install` initializes config and cache alongside the prefix, so an unredirected run would write into
// the developer's own trees. PATH is carried through because the install shells out to detect `man`.
//
// Parameters:
//   - `sandbox`: the temporary directory every location is rooted at.
//
// Returns:
//   - `[]string`: the subprocess environment.
func sandboxEnvironment(sandbox string) []string {
	return []string{
		"PATH=" + os.Getenv("PATH"),
		"XDG_CONFIG_HOME=" + filepath.Join(sandbox, "config"),
		"XDG_CACHE_HOME=" + filepath.Join(sandbox, "cache"),
		"XDG_DATA_HOME=" + filepath.Join(sandbox, "data"),
		"XDG_STATE_HOME=" + filepath.Join(sandbox, "state"),
		"TMPDIR=" + os.TempDir(),
	}
}

// manifestFiles reads the installed manifest and returns the paths it records, relative to the prefix.
//
// Parameters:
//   - `t`: the test harness.
//   - `prefix`: the installation prefix.
//   - `tool`: the installed tool.
//
// Returns:
//   - `[]string`: the recorded paths, relative to `prefix`.
func manifestFiles(t *testing.T, prefix, tool string) []string {

	t.Helper()

	data, err := os.ReadFile(filepath.Join(prefix, "share", tool, "manifest.json"))
	if err != nil {
		t.Fatalf("read %s manifest: %v", tool, err)
	}

	var m struct {
		Files []struct {
			Path string `json:"path"`
		} `json:"files"`
	}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("parse %s manifest: %v", tool, err)
	}

	paths := make([]string, 0, len(m.Files))
	for _, f := range m.Files {
		paths = append(paths, filepath.FromSlash(f.Path))
	}

	return paths
}

// run executes the binary with the controlled environment and returns its combined output.
//
// Parameters:
//   - `t`: the test harness.
//   - `binary`: the executable to run.
//   - `environment`: the subprocess environment.
//   - `args`: the command arguments.
//
// Returns:
//   - `string`: combined stdout and stderr.
//   - `error`: non-nil when the command exits non-zero.
func run(t *testing.T, binary string, environment []string, args ...string) (string, error) {

	t.Helper()

	cmd := exec.CommandContext(t.Context(), binary, args...) //nolint:gosec // G204: the binary is one we built and the args are literals
	cmd.Env = environment

	out, err := cmd.CombinedOutput()

	return strings.TrimSpace(string(out)), err
}

// exeSuffix returns the platform's executable suffix.
//
// Returns:
//   - `string`: ".exe" on Windows, empty elsewhere.
func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}
