// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package writ

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/viper"

	"github.com/NobleFactor/devlore-cli/cmd/internal/cli"
	"github.com/NobleFactor/devlore-cli/pkg/xdg"
)

// --- helpers ---

// isolateDevloreHomes points every XDG base directory at a fresh temporary directory for one test, and resets viper
// when the test ends.
//
// Parameters:
//   - `t`: the test harness.
//
// Returns:
//   - `string`: the temporary directory holding the four homes.
func isolateDevloreHomes(t *testing.T) string {

	t.Helper()

	root := t.TempDir()
	for _, home := range []string{"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME"} {
		t.Setenv(home, filepath.Join(root, home))
	}
	t.Cleanup(viper.Reset)
	return root
}

// TestRoot_KeepsTheOutputConvention pins the root registration through the shared checkers: every command
// inherits the common set from the root, none shadows an inherited flag or binds a reserved name, and the
// set on the root is the shared root's (10-command-line-interface.md §4, §14).
//
// Parameters:
//   - `t`: the test harness.
func TestRoot_KeepsTheOutputConvention(t *testing.T) {

	root := NewRootCmd()
	if len(root.Commands()) == 0 {
		t.Fatal("the root has no subcommands; nothing to check")
	}
	if v := cli.CheckNoOwnOutputFlag(root); len(v) > 0 {
		t.Errorf("%d violations:\n%s", len(v), strings.Join(v, "\n"))
	}
	if v := cli.CheckSharedSetOnRoot(root); len(v) > 0 {
		t.Errorf("the root's set is not the shared root's:\n%s", strings.Join(v, "\n"))
	}
	if v := cli.CheckGroupsTakeNoAction(root); len(v) > 0 {
		t.Errorf("a group acts when invoked bare:\n%s", strings.Join(v, "\n"))
	}
}

// TestRoot_RefusesWritTargets proves the retired key stops a lifecycle command (#925): a configuration that sets
// `writ.targets` fails with [cli.ExitConfig] and names `writ.scopes`, rather than being ignored.
//
// Parameters:
//   - `t`: the test harness.
func TestRoot_RefusesWritTargets(t *testing.T) {

	root := isolateDevloreHomes(t)

	config := filepath.Join(root, "config.yaml")
	if err := os.WriteFile(config, []byte("writ:\n  targets:\n    home: /tmp/elsewhere\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	command := NewRootCmd()
	command.SetArgs([]string{"--config", config, "--silent", "reconcile", "--output", "none"})
	err := command.Execute()
	if err == nil {
		t.Fatal("a configuration setting writ.targets was accepted")
	}
	if code := cli.ExitCode(err); code != cli.ExitConfig {
		t.Errorf("ExitCode = %d, want %d (EX_CONFIG)", code, cli.ExitConfig)
	}
	if !strings.Contains(err.Error(), "writ.scopes") {
		t.Errorf("the refusal does not name writ.scopes: %v", err)
	}
}

// TestRoot_ConfigRunsWithWritTargets proves the refusal stops only the lifecycle commands (#926): with `writ.targets`
// set, `writ config unset writ.targets` runs and removes the key, as it removes any other.
//
// Parameters:
//   - `t`: the test harness.
func TestRoot_ConfigRunsWithWritTargets(t *testing.T) {

	isolateDevloreHomes(t)

	config := xdg.ConfigPath("devlore", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(config), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config, []byte("writ:\n  targets:\n    home: /tmp/elsewhere\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	command := NewRootCmd()
	command.SetArgs([]string{"--silent", "config", "unset", "writ.targets"})
	if err := command.Execute(); err != nil {
		t.Fatalf("writ config unset writ.targets was refused: %v", err)
	}

	content, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), "targets") {
		t.Errorf("writ.targets is still in the configuration:\n%s", content)
	}
}

// TestRoot_ScopeOnLifecycleCommandsOnly proves `--scope` belongs to `deploy`, `upgrade`, `reconcile` and
// `decommission` and to no other command, the root least of all, where it would shadow the shared root's
// `workflow verify --scope` (#926, open question 3).
//
// Parameters:
//   - `t`: the test harness.
func TestRoot_ScopeOnLifecycleCommandsOnly(t *testing.T) {

	root := NewRootCmd()
	if root.PersistentFlags().Lookup("scope") != nil {
		t.Error("--scope is on the root; it belongs to the four lifecycle commands")
	}

	want := map[string]bool{"decommission": true, "deploy": true, "reconcile": true, "upgrade": true}
	for _, command := range root.Commands() {
		if registered := command.Flags().Lookup("scope") != nil; registered != want[command.Name()] {
			t.Errorf("writ %s: --scope registered = %t, want %t", command.Name(), registered, want[command.Name()])
		}
	}
}

// TestRoot_TargetIsGone proves `--target` is removed with no alias (#926): naming it is an unknown flag, which is a
// usage error.
//
// Parameters:
//   - `t`: the test harness.
func TestRoot_TargetIsGone(t *testing.T) {

	isolateDevloreHomes(t)

	command := NewRootCmd()
	command.SetArgs([]string{"--silent", "reconcile", "--output", "none", "--target", "Home"})
	err := command.Execute()
	if err == nil {
		t.Fatal("--target was accepted")
	}
	if code := cli.ExitCode(err); code != cli.ExitUsage {
		t.Errorf("ExitCode = %d, want %d (EX_USAGE): %v", code, cli.ExitUsage, err)
	}
}

// TestRoot_ScopeMatchesWithoutCase proves `--scope` names a defined scope without case, repeatably (#926): the run
// passes the check and reaches reconcile, which finds nothing deployed in the isolated homes.
//
// Parameters:
//   - `t`: the test harness.
func TestRoot_ScopeMatchesWithoutCase(t *testing.T) {

	isolateDevloreHomes(t)

	command := NewRootCmd()
	command.SetArgs([]string{"--silent", "reconcile", "--output", "none", "--scope", "home", "--scope", "SYSTEM"})
	err := command.Execute()
	if code := cli.ExitCode(err); code != cli.ExitNoInput {
		t.Errorf("ExitCode = %d, want %d (EX_NOINPUT, never deployed): %v", code, cli.ExitNoInput, err)
	}
}

// TestRoot_RefusesUndefinedScope proves `--scope` naming no scope defined here is a usage error that names it and the
// scopes that are (#926).
//
// Parameters:
//   - `t`: the test harness.
func TestRoot_RefusesUndefinedScope(t *testing.T) {

	isolateDevloreHomes(t)

	command := NewRootCmd()
	command.SetArgs([]string{"--silent", "reconcile", "--output", "none", "--scope", "Bogus"})
	err := command.Execute()
	if code := cli.ExitCode(err); code != cli.ExitUsage {
		t.Fatalf("ExitCode = %d, want %d (EX_USAGE): %v", code, cli.ExitUsage, err)
	}
	if !strings.Contains(err.Error(), "Bogus") || !strings.Contains(err.Error(), "Home") {
		t.Errorf("the refusal names neither the scope nor the scopes defined: %v", err)
	}
}

// TestRoot_RefusesUndefinedBuiltinScopeKey proves a `writ.scopes` key naming a builtin this platform does not define
// stops a lifecycle command with [cli.ExitConfig] (#926).
//
// Parameters:
//   - `t`: the test harness.
func TestRoot_RefusesUndefinedBuiltinScopeKey(t *testing.T) {

	if runtime.GOOS == "windows" {
		t.Skip("Windows defines ProgramFiles")
	}
	root := isolateDevloreHomes(t)

	config := filepath.Join(root, "config.yaml")
	if err := os.WriteFile(config, []byte("writ:\n  scopes:\n    ProgramFiles: /opt/programs\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	command := NewRootCmd()
	command.SetArgs([]string{"--config", config, "--silent", "reconcile", "--output", "none"})
	err := command.Execute()
	if code := cli.ExitCode(err); code != cli.ExitConfig {
		t.Fatalf("ExitCode = %d, want %d (EX_CONFIG): %v", code, cli.ExitConfig, err)
	}
	if !strings.Contains(err.Error(), "ProgramFiles") {
		t.Errorf("the refusal does not name ProgramFiles: %v", err)
	}
}
