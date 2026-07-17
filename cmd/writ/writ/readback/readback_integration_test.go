// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package readback_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/deploy"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/readback"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/segment"
	"github.com/NobleFactor/devlore-cli/internal/cli"

	// Blank-import the op inventory so provider registration runs for planning and graph loading.
	_ "github.com/NobleFactor/devlore-cli/pkg/op/inventory"
)

// deployFixture runs one real deploy (a plain link and a template) and returns the roots.
func deployFixture(t *testing.T) (sourceRoot, targetRoot string) {

	t.Helper()

	root := t.TempDir()
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("HOME", root)

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

	if err := deploy.Execute(context.Background(), cfg); err != nil {
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
	if link.Action != "file.link" {
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
	if rendered.Action != "file.write_text" {
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

	traces, err := filepath.Glob(filepath.Join(cli.ReceiptsDir(), "*", "2*.yaml"))
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

// TestFold_MissingIndexIsAnError pins the settled ruling: no index, no report.
func TestFold_MissingIndexIsAnError(t *testing.T) {

	deployFixture(t)

	if err := os.Remove(cli.IndexPath()); err != nil {
		t.Fatal(err)
	}

	_, err := readback.Fold(context.Background())
	if err == nil {
		t.Fatal("Fold over a missing index = nil error, want the hard error")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("error %v does not unwrap to os.ErrNotExist", err)
	}
}
