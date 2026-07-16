// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package decommission_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/decommission"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/deploy"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/readback"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/segment"

	// Blank-import the op inventory so provider registration runs for planning and graph loading.
	_ "github.com/NobleFactor/devlore-cli/pkg/op/inventory"
)

// deployFixture runs one real deploy: a plain link at the target root and a template nested under .config/app.
func deployFixture(t *testing.T) (sourceRoot, targetRoot string) {

	t.Helper()

	root := t.TempDir()
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))

	sourceRoot = filepath.Join(root, "src")
	targetRoot = filepath.Join(root, "home")

	if err := os.MkdirAll(filepath.Join(sourceRoot, "myproj", ".config", "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(targetRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceRoot, "myproj", ".zshrc"), []byte("plain zsh"), 0o644); err != nil {
		t.Fatal(err)
	}
	template := []byte("os={{ .Segments.OS }}")
	nested := filepath.Join(sourceRoot, "myproj", ".config", "app", "config.template")
	if err := os.WriteFile(nested, template, 0o644); err != nil {
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

// TestExecute_RemovesDeployedFiles decommissions the fixture project: the link and the rendered file are gone,
// the source is untouched, and the next fold shows an empty inventory (the removal folded).
func TestExecute_RemovesDeployedFiles(t *testing.T) {

	sourceRoot, targetRoot := deployFixture(t)

	err := decommission.Execute(context.Background(), &decommission.Config{Projects: []string{"myproj"}})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if _, err := os.Lstat(filepath.Join(targetRoot, ".zshrc")); !os.IsNotExist(err) {
		t.Errorf("link survives decommission (err %v)", err)
	}
	if _, err := os.Lstat(filepath.Join(targetRoot, ".config", "app", "config")); !os.IsNotExist(err) {
		t.Errorf("rendered file survives decommission (err %v)", err)
	}
	if _, err := os.Stat(filepath.Join(sourceRoot, "myproj", ".zshrc")); err != nil {
		t.Errorf("source disturbed: %v", err)
	}

	inventory, err := readback.Fold(context.Background())
	if err != nil {
		t.Fatalf("Fold after decommission: %v", err)
	}
	if len(inventory.Entries) != 0 {
		t.Errorf("inventory after decommission = %v, want empty", inventory.Entries)
	}
	if inventory.Runs != 2 {
		t.Errorf("Runs = %d, want 2 (deploy + decommission)", inventory.Runs)
	}
}

// TestExecute_PruneRemovesEmptyParents pins --prune: the emptied .config/app chain is removed up to the target
// root boundary.
func TestExecute_PruneRemovesEmptyParents(t *testing.T) {

	_, targetRoot := deployFixture(t)

	err := decommission.Execute(context.Background(),
		&decommission.Config{Projects: []string{"myproj"}, Prune: true})
	if err != nil {
		t.Fatalf("Execute --prune: %v", err)
	}

	if _, err := os.Lstat(filepath.Join(targetRoot, ".config")); !os.IsNotExist(err) {
		t.Errorf(".config survives --prune (err %v)", err)
	}
	if _, err := os.Stat(targetRoot); err != nil {
		t.Errorf("target root itself was pruned: %v", err)
	}
}

// TestExecute_RefusesReplacedLink pins the unlink safety property: a deployed link the user replaced with a
// real file is refused, not deleted — the scope fails and the file survives.
func TestExecute_RefusesReplacedLink(t *testing.T) {

	_, targetRoot := deployFixture(t)

	linkPath := filepath.Join(targetRoot, ".zshrc")
	if err := os.Remove(linkPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(linkPath, []byte("hand-written now"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := decommission.Execute(context.Background(), &decommission.Config{Projects: []string{"myproj"}})
	if err == nil || !strings.Contains(err.Error(), "scope") {
		t.Fatalf("Execute over a replaced link = %v, want a scope failure", err)
	}

	content, readErr := os.ReadFile(linkPath)
	if readErr != nil || string(content) != "hand-written now" {
		t.Errorf("replaced file disturbed: %q (err %v)", content, readErr)
	}
}

// TestExecute_RefusesUnknownProject pins the zero-knowledge refusal.
func TestExecute_RefusesUnknownProject(t *testing.T) {

	deployFixture(t)

	err := decommission.Execute(context.Background(), &decommission.Config{Projects: []string{"nosuch"}})
	if err == nil || !strings.Contains(err.Error(), "no deployed files") {
		t.Errorf("Execute for an unknown project = %v, want the no-deployed-files refusal", err)
	}
}
