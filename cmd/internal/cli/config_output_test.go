// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// probeConfig lays out a devlore config tree in a temporary XDG home and returns a root whose `config`
// subcommands read it.
func probeConfig(t *testing.T, contents string) (root string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	dir := filepath.Join(home, "devlore")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if contents != "" {
		if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// runProbe runs the shared root with the arguments and returns what reached stdout.
func runProbe(t *testing.T, args ...string) string {
	t.Helper()
	root := NewRootCmd(RootConfig{Name: "probe", Short: "a probe"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(new(bytes.Buffer))
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		t.Fatalf("probe %s: %v", strings.Join(args, " "), err)
	}
	return out.String()
}

// TestConfigCommands_RenderThroughTheCommonSet pins Requirement 5 of 776-output-enforcement.md: the shared
// config subcommands are results, so `-o` renders them, where they printed plain text before.
func TestConfigCommands_RenderThroughTheCommonSet(t *testing.T) {

	dir := probeConfig(t, "probe:\n  name: x\n  depth: 2\n")

	t.Run("get emits the keys and their values", func(t *testing.T) {
		var got map[string]any
		if err := json.Unmarshal([]byte(runProbe(t, "config", "get", "probe.name", "-o", "json")), &got); err != nil {
			t.Fatal(err)
		}
		if got["probe.name"] != "x" {
			t.Errorf("got %v", got)
		}
	})

	t.Run("list emits every setting by dotted key", func(t *testing.T) {
		var got map[string]any
		if err := json.Unmarshal([]byte(runProbe(t, "config", "list", "-o", "json")), &got); err != nil {
			t.Fatal(err)
		}
		if got["probe.name"] != "x" || got["probe.depth"] != float64(2) {
			t.Errorf("got %v", got)
		}
	})

	t.Run("path emits the path as a value", func(t *testing.T) {
		got := strings.TrimSpace(runProbe(t, "config", "path", "-o", "value"))
		if got != filepath.Join(dir, "config.yaml") {
			t.Errorf("got %q", got)
		}
	})

	t.Run("schema emits the schema as json", func(t *testing.T) {
		var got map[string]any
		if err := json.Unmarshal([]byte(runProbe(t, "config", "schema")), &got); err != nil {
			t.Fatal(err)
		}
		if _, ok := got["properties"]; !ok {
			t.Errorf("the schema has no properties: %v", got)
		}
	})

	t.Run("validate emits a report", func(t *testing.T) {
		var got configReport
		if err := json.Unmarshal([]byte(runProbe(t, "config", "validate", "-o", "json")), &got); err != nil {
			t.Fatal(err)
		}
		if !got.Present || !got.Valid {
			t.Errorf("got %+v", got)
		}
	})
}
