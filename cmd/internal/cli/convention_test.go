// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/result"
)

// region Tests

// TestAddOutputFlags_OutputCarriesTheShortForm is row 2 of the #740 test plan.
//
// `-o` is `--output`'s short form, as in `aws`, `az`, and `kubectl`. Binding the short form without the long
// one strands a user who spells out the flag they already know.
func TestAddOutputFlags_OutputCarriesTheShortForm(t *testing.T) {

	cmd := &cobra.Command{Use: "test"}
	var opts SinkOptions
	AddOutputFlags(cmd, &opts)

	flag := cmd.PersistentFlags().Lookup("output")
	if flag == nil {
		t.Fatal("--output is not bound")
	}
	if flag.Shorthand != "o" {
		t.Errorf("--output shorthand = %q, want %q", flag.Shorthand, "o")
	}
	if flag.DefValue != "json" {
		t.Errorf("--output default = %q, want json: a result is data first", flag.DefValue)
	}
}

// TestAddOutputFlags_FormatIsGone covers the other half of the rename.
//
// `--format` retires by deletion, not by aliasing. An alias would be the backward-compatibility shim the
// repository's governing principle forbids, and it doubles the surface every later change must consider.
func TestAddOutputFlags_FormatIsGone(t *testing.T) {

	cmd := &cobra.Command{Use: "test"}
	var opts SinkOptions
	AddOutputFlags(cmd, &opts)

	if cmd.PersistentFlags().Lookup("format") != nil || cmd.Flags().Lookup("format") != nil {
		t.Error("--format is still bound; it retires by deletion, never by alias")
	}
}

// TestAddOutputFlags_AreInheritedBySubcommands is the registration model.
//
// All four in-scope programs register the set once on their root, so every subcommand accepts every flag
// without opting in. That is how `aws` and `az` do it, and it is what makes the convention learnable.
func TestAddOutputFlags_AreInheritedBySubcommands(t *testing.T) {

	root := &cobra.Command{Use: "root"}
	var opts SinkOptions
	AddOutputFlags(root, &opts)

	child := &cobra.Command{Use: "child"}
	root.AddCommand(child)

	for _, name := range []string{"filter", "jq", "output", "store", "template"} {
		if child.InheritedFlags().Lookup(name) == nil {
			t.Errorf("subcommand does not inherit --%s; the set belongs on PersistentFlags", name)
		}
	}
}

// TestAddOutputFlags_SubcommandParsesTheShortForm proves the registration actually serves a user.
//
// The registration tests check where flags live; this one checks that `-o yaml` on a subcommand reaches the
// options struct, which is the only thing a user cares about.
func TestAddOutputFlags_SubcommandParsesTheShortForm(t *testing.T) {

	var opts SinkOptions
	root := &cobra.Command{Use: "root"}
	AddOutputFlags(root, &opts)

	ran := false
	root.AddCommand(&cobra.Command{Use: "child", RunE: func(*cobra.Command, []string) error {
		ran = true
		return nil
	}})

	root.SetArgs([]string{"child", "-o", "yaml", "--store", "/tmp/store"})
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !ran {
		t.Fatal("the subcommand did not run")
	}
	if opts.Format != "yaml" {
		t.Errorf("-o yaml gave Format %q, want yaml", opts.Format)
	}
	if opts.Store != "/tmp/store" {
		t.Errorf("--store gave Store %q, want /tmp/store", opts.Store)
	}
}

// TestFormatterByName_NoneWritesNothing is row 6a of the #740 test plan.
//
// `--output none` produces no result at all. This is not `> /dev/null`, which renders the result and then
// discards it: the rendering does not happen, and the value is reachable from a config file or an
// environment variable where no shell exists to redirect.
func TestFormatterByName_NoneWritesNothing(t *testing.T) {

	formatter, err := result.FormatterByName("none", "")
	if err != nil {
		t.Fatalf("FormatterByName(none): %v", err)
	}

	var buffer bytes.Buffer
	if err := formatter.Format(map[string]any{"loud": "value"}, &buffer); err != nil {
		t.Fatalf("Format: %v", err)
	}

	if buffer.Len() != 0 {
		t.Errorf("none wrote %d bytes (%q), want none", buffer.Len(), buffer.String())
	}
}

// TestFormatterByName_UnknownNameListsTheSet keeps the error message honest as the set grows.
func TestFormatterByName_UnknownNameListsTheSet(t *testing.T) {

	_, err := result.FormatterByName("nope", "")
	if err == nil {
		t.Fatal("FormatterByName(nope) succeeded, want an error")
	}

	if !strings.Contains(err.Error(), "none") {
		t.Errorf("error does not list none among the formatters: %v", err)
	}
}

// TestStoreRoot_RelocatesGraphsAndTracesTogether is row 3 of the #740 test plan.
//
// `--store` names a store, not a directory to dump files in. Relocating it moves both subdirectories, so a
// trace still resolves to the definition it ran; moving one alone severs that tie.
func TestStoreRoot_RelocatesGraphsAndTracesTogether(t *testing.T) {

	root := t.TempDir()

	restore := SetStoreRoot(root)
	t.Cleanup(restore)

	graphs, traces := GraphsDir(), TracesDir()

	if !strings.HasPrefix(graphs, root) {
		t.Errorf("GraphsDir() = %q, want it under %q", graphs, root)
	}
	if !strings.HasPrefix(traces, root) {
		t.Errorf("TracesDir() = %q, want it under %q", traces, root)
	}
	if filepath.Dir(graphs) != filepath.Dir(traces) {
		t.Errorf("graphs and traces landed in different parents: %q vs %q",
			filepath.Dir(graphs), filepath.Dir(traces))
	}
}

// TestStoreRoot_DefaultsToTheStatePath covers the other half: an unset root changes nothing.
func TestStoreRoot_DefaultsToTheStatePath(t *testing.T) {

	if !strings.Contains(GraphsDir(), "graphs") {
		t.Errorf("GraphsDir() = %q, want the state path's graphs directory", GraphsDir())
	}
	if !strings.Contains(TracesDir(), "traces") {
		t.Errorf("TracesDir() = %q, want the state path's traces directory", TracesDir())
	}
}

// TestStoreRoot_RelocatesTheWholeStore is row 3a of the #740 test plan.
//
// `--store` names a store, not a directory to drop files in. A relocation that moves the trace but leaves
// the run index behind splits the store in two: the trace sits under the new root while the index recording
// it sits under the old one, and `writ status` reads the index.
//
// The state home and the store root are deliberately different directories, so anything that consults the
// state home instead of the store root lands in the wrong place and is caught rather than coinciding.
func TestStoreRoot_RelocatesTheWholeStore(t *testing.T) {

	stateHome, storeHome := t.TempDir(), t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateHome)

	restore := SetStoreRoot(storeHome)
	t.Cleanup(restore)

	const checksum = "sha256:5eaf00d5eaf00d5eaf00d5eaf00d5eaf00d5eaf00d5eaf00d5eaf00d5eaf00d5"

	path, err := WriteTrace(&op.Trace{GraphChecksum: checksum})
	if err != nil {
		t.Fatalf("WriteTrace: %v", err)
	}

	if !strings.HasPrefix(path, storeHome) {
		t.Errorf("trace written to %q, want it under the store root %q", path, storeHome)
	}

	// The index is part of the store, not of the state home. This is the assertion that fails when only the
	// directories are relocated.
	if !strings.HasPrefix(IndexPath(), storeHome) {
		t.Errorf("run index at %q, want it under the store root %q", IndexPath(), storeHome)
	}

	// The checksum tie survives: the trace is found by the definition it ran against.
	loaded, err := LoadLatestTrace(checksum)
	if err != nil {
		t.Fatalf("LoadLatestTrace: %v", err)
	}
	if loaded.GraphChecksum != checksum {
		t.Errorf("reloaded GraphChecksum = %q, want %q", loaded.GraphChecksum, checksum)
	}

	// And the index recorded it, in the relocated store.
	entries, err := ReadIndex()
	if err != nil {
		t.Fatalf("ReadIndex: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("index entries = %d, want 1", len(entries))
	}
	if entries[0].GraphChecksum != checksum {
		t.Errorf("index GraphChecksum = %q, want %q", entries[0].GraphChecksum, checksum)
	}
}

// endregion
