// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package readback_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/NobleFactor/devlore-cli/cmd/internal/cli"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/deploy"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/readback"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/segment"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/upgrade"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/file"

	// Blank-import the op inventory so provider registration runs for planning and graph loading.
	_ "github.com/NobleFactor/devlore-cli/pkg/op/inventory"
)

// deployFixture runs one real deploy (a plain link and a template) and returns the roots.
func deployFixture(t *testing.T) (sourceRoot, targetRoot string) {

	t.Helper()

	root := t.TempDir()
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))

	sourceRoot = filepath.Join(root, "src")
	targetRoot = filepath.Join(root, "home")

	if err := os.MkdirAll(filepath.Join(sourceRoot, "myproj"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(targetRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceRoot, "myproj", ".zshrc"), []byte("plain zsh"), 0o644); err != nil {
		t.Fatal(err)
	}
	template := []byte("os={{ .Segments.OS }}")
	if err := os.WriteFile(filepath.Join(sourceRoot, "myproj", ".gitconfig.template"), template, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &deploy.Config{
		SourceRoot: sourceRoot,
		TargetRoot: targetRoot,
		Projects:   []string{"myproj"},
		Segments:   segment.Segments{{Name: "OS", Value: "Darwin"}},
	}

	if _, err := deploy.Execute(context.Background(), cfg); err != nil {
		t.Fatalf("deploy fixture: %v", err)
	}

	return sourceRoot, targetRoot
}

// TestFold_AfterDeploy folds a real deploy: both targets appear with their actions, sources, and run identity.
func TestFold_AfterDeploy(t *testing.T) {

	sourceRoot, targetRoot := deployFixture(t)

	inventory, err := readback.Fold(context.Background())
	if err != nil {
		t.Fatalf("Fold: %v", err)
	}

	if inventory.Runs != 1 {
		t.Errorf("Runs = %d, want 1", inventory.Runs)
	}
	if len(inventory.Findings) != 0 {
		t.Errorf("Findings = %v, want none", inventory.Findings)
	}
	if len(inventory.Entries) != 2 {
		t.Fatalf("Entries = %d (%v), want 2", len(inventory.Entries), inventory.Entries)
	}

	link, ok := inventory.Entries[filepath.Join(targetRoot, ".zshrc")]
	if !ok {
		t.Fatalf("no entry for the linked target; entries: %v", inventory.Entries)
	}
	if link.Action != string(file.Link) {
		t.Errorf("link action = %q, want file.link", link.Action)
	}
	if want := filepath.Join(sourceRoot, "myproj", ".zshrc"); link.Source != want {
		t.Errorf("link source = %q, want %q", link.Source, want)
	}
	if link.Project != "myproj" {
		t.Errorf("link project = %q, want myproj", link.Project)
	}

	rendered, ok := inventory.Entries[filepath.Join(targetRoot, ".gitconfig")]
	if !ok {
		t.Fatalf("no entry for the rendered target; entries: %v", inventory.Entries)
	}
	if rendered.Action != string(file.WriteText) {
		t.Errorf("rendered action = %q, want file.write_text", rendered.Action)
	}
	if rendered.GraphChecksum == "" {
		t.Error("rendered entry carries no graph checksum")
	}
}

// TestFold_NukedTraceIsAFinding pins best-effort degradation: a deleted trace document becomes a finding, not
// a failure, and its run's entries vanish from the fold.
func TestFold_NukedTraceIsAFinding(t *testing.T) {

	deployFixture(t)

	traces, err := filepath.Glob(filepath.Join(cli.TracesDir(), "*", "2*.yaml"))
	if err != nil || len(traces) != 1 {
		t.Fatalf("traces = %v (err %v), want exactly one", traces, err)
	}
	if err := os.Remove(traces[0]); err != nil {
		t.Fatal(err)
	}

	inventory, err := readback.Fold(context.Background())
	if err != nil {
		t.Fatalf("Fold over a nuked trace: %v", err)
	}

	if len(inventory.Entries) != 0 {
		t.Errorf("Entries = %v, want none (the only run's trace is gone)", inventory.Entries)
	}
	if len(inventory.Findings) == 0 {
		t.Error("no finding for the nuked trace")
	}
}

// TestFold_NoLifetimeIsNotFound pins the never-deployed answer (#922, #756): a store with no current lifetime has
// no record, and the error is os.ErrNotExist.
func TestFold_NoLifetimeIsNotFound(t *testing.T) {

	t.Setenv("XDG_STATE_HOME", t.TempDir())

	_, err := readback.Fold(context.Background())
	if err == nil {
		t.Fatal("Fold with no lifetime = nil error, want not-found")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("error %v does not unwrap to os.ErrNotExist", err)
	}
}

// TestFold_DeployReplacesTheRecord pins #922: a file the previous lifetime deployed and this one does not is not in
// the record, and the previous lifetime is replaced, its runs kept.
func TestFold_DeployReplacesTheRecord(t *testing.T) {

	sourceRoot, targetRoot := deployFixture(t)

	first, err := cli.CurrentLifetime()
	if err != nil {
		t.Fatalf("CurrentLifetime after the first deploy: %v", err)
	}
	if len(first.Runs) != 1 || first.Runs[0].Operation != cli.RunOperationDeploy {
		t.Fatalf("first lifetime runs = %+v, want one deploy run", first.Runs)
	}

	if err := os.Remove(filepath.Join(sourceRoot, "myproj", ".zshrc")); err != nil {
		t.Fatal(err)
	}
	cfg := &deploy.Config{
		SourceRoot: sourceRoot,
		TargetRoot: targetRoot,
		Projects:   []string{"myproj"},
		Segments:   segment.Segments{{Name: "OS", Value: "Darwin"}},
	}
	if _, err := deploy.Execute(context.Background(), cfg); err != nil {
		t.Fatalf("second deploy: %v", err)
	}

	inventory, err := readback.Fold(context.Background())
	if err != nil {
		t.Fatalf("Fold: %v", err)
	}
	if _, stillThere := inventory.Entries[filepath.Join(targetRoot, ".zshrc")]; stillThere {
		t.Errorf("the record still holds .zshrc, which the current deploy did not place: %v", inventory.Entries)
	}
	if _, kept := inventory.Entries[filepath.Join(targetRoot, ".gitconfig")]; !kept {
		t.Errorf("the record lost .gitconfig, which the current deploy placed: %v", inventory.Entries)
	}
	if inventory.Runs != 1 {
		t.Errorf("Runs = %d, want 1: the current lifetime's deploy alone", inventory.Runs)
	}

	second, err := cli.CurrentLifetime()
	if err != nil {
		t.Fatalf("CurrentLifetime after the second deploy: %v", err)
	}
	if second.ID == first.ID {
		t.Fatal("the second deploy did not open a new lifetime")
	}
	replaced, err := cli.LoadLifetime(first.ID)
	if err != nil {
		t.Fatalf("LoadLifetime(first): %v", err)
	}
	if replaced.State != cli.LifetimeReplaced || len(replaced.Runs) != 1 {
		t.Errorf("first lifetime = {state %s, %d runs}, want replaced with its run kept", replaced.State, len(replaced.Runs))
	}
}

// TestFold_UpgradeStacksOnTheDeploy pins #922's stacking: an upgrade writes into the current lifetime, opening none,
// and the record still holds every entry the deploy placed.
func TestFold_UpgradeStacksOnTheDeploy(t *testing.T) {

	_, targetRoot := deployFixture(t)

	before, err := cli.CurrentLifetime()
	if err != nil {
		t.Fatalf("CurrentLifetime: %v", err)
	}

	cfg := &upgrade.Config{
		Projects: []string{"myproj"},
		Force:    true,
		Segments: segment.Segments{{Name: "OS", Value: "Linux"}},
	}
	if _, err := upgrade.Execute(context.Background(), cfg); err != nil {
		t.Fatalf("upgrade: %v", err)
	}

	after, err := cli.CurrentLifetime()
	if err != nil {
		t.Fatalf("CurrentLifetime after the upgrade: %v", err)
	}
	if after.ID != before.ID {
		t.Fatalf("the upgrade opened a new lifetime %s; want it to write into %s", after.ID, before.ID)
	}
	if len(after.Runs) != 2 || after.Runs[1].Operation != cli.RunOperationUpgrade {
		t.Errorf("runs = %+v, want the deploy then the upgrade", after.Runs)
	}

	inventory, err := readback.Fold(context.Background())
	if err != nil {
		t.Fatalf("Fold: %v", err)
	}
	if len(inventory.Entries) != 2 || inventory.Runs != 2 {
		t.Errorf("record = %d entries over %d runs, want 2 over 2", len(inventory.Entries), inventory.Runs)
	}
	if _, ok := inventory.Entries[filepath.Join(targetRoot, ".gitconfig")]; !ok {
		t.Errorf("the regenerated .gitconfig left the record: %v", inventory.Entries)
	}
}
