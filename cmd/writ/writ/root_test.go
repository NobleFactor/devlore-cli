// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package writ

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"

	"github.com/NobleFactor/devlore-cli/cmd/internal/cli"
)

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

	root := t.TempDir()
	for _, home := range []string{"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME"} {
		t.Setenv(home, filepath.Join(root, home))
	}
	t.Cleanup(viper.Reset)

	config := filepath.Join(root, "config.yaml")
	if err := os.WriteFile(config, []byte("writ:\n  targets:\n    home: /tmp/elsewhere\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	command := NewRootCmd()
	command.SetArgs([]string{"--config", config, "--silent", "reconcile", "-o", "none"})
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

	root := t.TempDir()
	for _, home := range []string{"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME"} {
		t.Setenv(home, filepath.Join(root, home))
	}
	t.Cleanup(viper.Reset)

	config := filepath.Join(root, "XDG_CONFIG_HOME", "devlore", "config.yaml")
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
