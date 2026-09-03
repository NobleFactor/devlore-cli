// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/NobleFactor/devlore-cli/cmd/internal/cli"
	starruntime "github.com/NobleFactor/devlore-cli/cmd/star/star"
)

// TestRoot_EverySubcommandAcceptsTheCommonSet pins the root registration: every command of star accepts
// every flag of the common set, because the set is on the root and inherited, and no command registers an
// output flag of its own on top of it.
//
// Before #743 star built a bare cobra.Command and bound no output flags at all, so `star lint go -o json`
// was an unknown flag on every command.
func TestRoot_EverySubcommandAcceptsTheCommonSet(t *testing.T) {

	root, runtime := newRootCmd()
	defer closeQuietly(t, runtime)
	if len(root.Commands()) == 0 {
		t.Fatal("the root has no subcommands; nothing to check")
	}

	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		for _, sub := range cmd.Commands() {
			for _, name := range []string{"output", "filter", "jq", "store"} {
				if sub.InheritedFlags().Lookup(name) == nil {
					t.Errorf("%s does not inherit --%s from the root", sub.CommandPath(), name)
				}
			}
			for _, name := range []string{"output", "format", "json"} {
				if sub.LocalFlags().Lookup(name) != nil {
					t.Errorf("%s registers its own --%s; the common set owns that name", sub.CommandPath(), name)
				}
			}
			walk(sub)
		}
	}
	walk(root)
}

// TestRoot_ConfigIsTheSharedSetPlusShowAndSync pins the 2026-09-02 ruling: one command named `config`,
// carrying the shared subcommands every program has, with star's two extension commands attached beneath
// it rather than under a second `config` of their own.
func TestRoot_ConfigIsTheSharedSetPlusShowAndSync(t *testing.T) {

	root, runtime := newRootCmd()
	defer closeQuietly(t, runtime)

	var configs []*cobra.Command
	for _, sub := range root.Commands() {
		if sub.Name() == "config" {
			configs = append(configs, sub)
		}
	}
	if len(configs) != 1 {
		t.Fatalf("expected exactly one command named config, found %d", len(configs))
	}

	var names []string
	for _, sub := range configs[0].Commands() {
		names = append(names, sub.Name())
	}
	for _, want := range []string{"edit", "get", "list", "path", "schema", "set", "unset", "validate", "show", "sync"} {
		if !slices.Contains(names, want) {
			t.Errorf("star config lacks %q; have %v", want, names)
		}
	}
}

// TestRoot_DocsHasStarlarkAloneAndManIsShared pins the 2026-09-02 ruling on man pages: the shared `man`
// is the one route, and star's `docs` group carries only what no other program has.
func TestRoot_DocsHasStarlarkAloneAndManIsShared(t *testing.T) {

	root, runtime := newRootCmd()
	defer closeQuietly(t, runtime)

	if !hasCommand(root, "man") {
		t.Error("star has no man command; the shared root registers one")
	}
	docs := findCommand(root, "docs")
	if docs == nil {
		t.Fatal("star has no docs command")
	}
	var names []string
	for _, sub := range docs.Commands() {
		names = append(names, sub.Name())
	}
	if !slices.Equal(names, []string{"starlark"}) {
		t.Errorf("star docs carries %v; only starlark is star's own", names)
	}
}

// TestRoot_SilencesUsage pins the 2026-09-02 ruling: no usage text on any error. The shared root sets it
// and cobra honors the root's setting for every command beneath.
func TestRoot_SilencesUsage(t *testing.T) {

	root, runtime := newRootCmd()
	defer closeQuietly(t, runtime)

	if !root.SilenceUsage {
		t.Error("the root does not silence usage; every error would print the usage block")
	}
}

// TestRoot_DryRunReachesTheRuntime pins the wrapper: the shared `--dry-run` is copied into
// starruntime.DryRun at dispatch, where star's scripts read it as `dry_run`.
func TestRoot_DryRunReachesTheRuntime(t *testing.T) {

	root, runtime := newRootCmd()
	defer closeQuietly(t, runtime)
	t.Cleanup(func() { starruntime.DryRun = false })

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"--dry-run", "version"})
	if err := root.Execute(); err != nil {
		t.Fatalf("star --dry-run version: %v\n%s", err, out.String())
	}
	if !starruntime.DryRun {
		t.Error("--dry-run was parsed but starruntime.DryRun is still false")
	}
}

// TestRoot_SilentWiresOneNarrator pins the other half of the wrapper: the runtime environment's status
// is the narrator the shared pre-run built, so one --silent gate covers cli.Note and ui.note() alike.
func TestRoot_SilentWiresOneNarrator(t *testing.T) {

	root, runtime := newRootCmd()
	defer closeQuietly(t, runtime)

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"--silent", "version"})
	if err := root.Execute(); err != nil {
		t.Fatalf("star --silent version: %v\n%s", err, out.String())
	}
	if runtime.Environment().Status != cli.UI() {
		t.Error("the runtime environment's status is not the narrator the shared pre-run built")
	}
}

// TestRoot_SelfInstallInstallsTheExtensions pins the hook forwarding: star's extensions ride
// RootConfig.PostInstallHooks into the shared `self install`, so the move onto the shared root did not
// drop them. The whole install runs into a temporary prefix, with the XDG homes pointed away from the
// developer's own.
func TestRoot_SelfInstallInstallsTheExtensions(t *testing.T) {

	for _, name := range []string{"XDG_CONFIG_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME"} {
		t.Setenv(name, t.TempDir())
	}
	t.Chdir(filepath.Join("..", "..")) // the extensions hook finds star/extensions at the repository root

	root, runtime := newRootCmd()
	defer closeQuietly(t, runtime)

	prefix := t.TempDir()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"self", "install", prefix})
	if err := root.Execute(); err != nil {
		t.Fatalf("star self install: %v\n%s", err, out.String())
	}

	installed := filepath.Join(prefix, "share", "star", "extensions", "com.noblefactor.devlore.Actions", "extension.yaml")
	if _, err := os.Stat(installed); err != nil {
		t.Errorf("the extensions were not installed: %v", err)
	}
}

// closeQuietly closes the session and reports, rather than fails on, a close error: the tests above are
// about the command tree, and a close error is a different finding.
func closeQuietly(t *testing.T, runtime *starruntime.Application) {
	t.Helper()
	if err := runtime.Close(); err != nil {
		t.Logf("closing the runtime: %v", err)
	}
}

// findCommand returns the root's direct child of that name, or nil.
func findCommand(root *cobra.Command, name string) *cobra.Command {
	for _, sub := range root.Commands() {
		if sub.Name() == name {
			return sub
		}
	}
	return nil
}

// hasCommand reports whether the root has a direct child of that name.
func hasCommand(root *cobra.Command, name string) bool {
	return findCommand(root, name) != nil
}

// TestRegisterStarlarkCommand_RefusesReservedFlagNames pins the loader's half of the convention: an
// extension command may not bind a name the common set owns. Cobra would let the leaf shadow the
// inherited flag silently, which is how `generate -o json` once meant a directory.
func TestRegisterStarlarkCommand_RefusesReservedFlagNames(t *testing.T) {

	for _, name := range []string{"filter", "format", "jq", "json", "output", "store"} {
		root := &cobra.Command{Use: "star"}
		cmd := &starruntime.Command{
			Name:      "probe.leaf",
			Extension: &starruntime.Extension{Name: "probe"},
			Flags:     []starruntime.Flag{{Name: name, Type: "string"}},
		}
		err := registerStarlarkCommand(root, cmd)
		if err == nil {
			t.Errorf("--%s was accepted on an extension command; the common set owns that name", name)
			continue
		}
		if !strings.Contains(err.Error(), "--"+name) {
			t.Errorf("the refusal does not name the flag: %v", err)
		}
	}

	root := &cobra.Command{Use: "star"}
	cmd := &starruntime.Command{
		Name:      "probe.leaf",
		Extension: &starruntime.Extension{Name: "probe"},
		Flags:     []starruntime.Flag{{Name: "source", Type: "string"}},
	}
	if err := registerStarlarkCommand(root, cmd); err != nil {
		t.Errorf("--source is not reserved, yet it was refused: %v", err)
	}
}

// TestRoot_GenerateTakesThePackageAlone pins the 2026-09-02 ruling on the generator's line:
// `star devlore actions generate [--dry-run] <source>`. The operand is the provider's package and the files
// land inside it, so there is no destination to name and no flag for the source; `--gen` and `--write`, two
// booleans the script required to be true, are gone with them.
func TestRoot_GenerateTakesThePackageAlone(t *testing.T) {

	t.Chdir(filepath.Join("..", "..")) // the devlore.* extensions load from the repository root
	root, runtime := newRootCmd()
	defer closeQuietly(t, runtime)

	generate := findPath(root, "devlore", "actions", "generate")
	if generate == nil {
		t.Fatal("star has no devlore actions generate")
	}
	if generate.Use != "generate <source>" {
		t.Errorf("Use = %q, want %q", generate.Use, "generate <source>")
	}
	for _, name := range []string{"source", "gen", "write", "output"} {
		if generate.LocalFlags().Lookup(name) != nil {
			t.Errorf("generate still binds --%s", name)
		}
	}
}

// TestRoot_GenerateDryRunWritesNothing pins the preview: under the global --dry-run the generator narrates
// what it would write and touches nothing. The json provider is the probe; its generated files are compared
// byte for byte before and after.
func TestRoot_GenerateDryRunWritesNothing(t *testing.T) {

	t.Chdir(filepath.Join("..", ".."))
	root, runtime := newRootCmd()
	defer closeQuietly(t, runtime)
	t.Cleanup(func() { starruntime.DryRun = false })

	pkg := filepath.Join("pkg", "op", "provider", "json")
	before := snapshot(t, pkg)

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"--dry-run", "devlore", "actions", "generate", pkg})
	if err := root.Execute(); err != nil {
		t.Fatalf("star --dry-run devlore actions generate %s: %v\n%s", pkg, err, out.String())
	}

	after := snapshot(t, pkg)
	for name, content := range before {
		if after[name] != content {
			t.Errorf("--dry-run rewrote %s", name)
		}
	}
}

// findPath walks the tree by command names.
func findPath(root *cobra.Command, names ...string) *cobra.Command {
	cmd := root
	for _, name := range names {
		cmd = findCommand(cmd, name)
		if cmd == nil {
			return nil
		}
	}
	return cmd
}

// snapshot reads every generated file of a provider package, keyed by relative path.
func snapshot(t *testing.T, pkg string) map[string]string {
	t.Helper()
	files := map[string]string{}
	matches, err := filepath.Glob(filepath.Join(pkg, "gen", "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	matches = append(matches, filepath.Join(pkg, "action_names.gen.go"))
	for _, name := range matches {
		content, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		files[name] = string(content)
	}
	if len(files) < 2 {
		t.Fatalf("expected generated files under %s, found %d", pkg, len(files))
	}
	return files
}

// TestRoot_DocsStarlarkIsAResult pins the last stdout site: the authoring guide is a result and reaches
// stdout through the pipeline, so `-o value` prints the prose and the json default quotes it.
func TestRoot_DocsStarlarkIsAResult(t *testing.T) {

	root, runtime := newRootCmd()
	defer closeQuietly(t, runtime)

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"docs", "starlark", "-o", "value"})
	if err := root.Execute(); err != nil {
		t.Fatalf("star docs starlark -o value: %v", err)
	}
	if !strings.Contains(out.String(), "WRITING STARLARK OPERATIONS") {
		t.Errorf("the guide did not reach the result stream:\n%.200s", out.String())
	}
}

// TestRoot_UnimplementedKeyCommandsFail pins that an unimplemented command fails and says so, rather than
// printing a note to stdout and exiting 0.
func TestRoot_UnimplementedKeyCommandsFail(t *testing.T) {

	for _, leaf := range []string{"generate", "list", "rotate"} {
		root, runtime := newRootCmd()
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		root.SetArgs([]string{"key", leaf})
		err := root.Execute()
		closeQuietly(t, runtime)
		if err == nil {
			t.Errorf("star key %s reported success while unimplemented", leaf)
			continue
		}
		if !strings.Contains(err.Error(), "not implemented") {
			t.Errorf("star key %s: %v; expected it to say it is not implemented", leaf, err)
		}
	}
}
