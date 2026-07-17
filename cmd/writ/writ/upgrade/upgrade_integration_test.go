// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package upgrade_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/deploy"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/segment"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/upgrade"

	// Blank-import the op inventory so provider registration runs for planning and graph loading.
	_ "github.com/NobleFactor/devlore-cli/pkg/op/inventory"
)

// fixtureSegments is the segment set shared by the deploy fixture and the upgrade configs.
var fixtureSegments = segment.Segments{{Name: "OS", Value: "Darwin"}}

// deployFixture runs one real deploy: a plain link and a template, returning the roots and the template source.
func deployFixture(t *testing.T) (sourceRoot, targetRoot, templateSource string) {

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
	templateSource = filepath.Join(sourceRoot, "myproj", ".gitconfig.template")
	if err := os.WriteFile(templateSource, []byte("os={{ .Segments.OS }}"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &deploy.Config{
		SourceRoot: sourceRoot,
		TargetRoot: targetRoot,
		Projects:   []string{"myproj"},
		Segments:   fixtureSegments,
	}

	if err := deploy.Execute(context.Background(), cfg); err != nil {
		t.Fatalf("deploy fixture: %v", err)
	}

	return sourceRoot, targetRoot, templateSource
}

// upgradeConfig returns the baseline upgrade configuration for the fixture.
func upgradeConfig() *upgrade.Config {
	return &upgrade.Config{Segments: fixtureSegments}
}

// TestExecute_UpToDateDoesNothing pins the clean case: an unchanged deployment upgrades to a no-op.
func TestExecute_UpToDateDoesNothing(t *testing.T) {

	_, targetRoot, _ := deployFixture(t)
	rendered := filepath.Join(targetRoot, ".gitconfig")

	before, err := os.Stat(rendered)
	if err != nil {
		t.Fatal(err)
	}

	if err := upgrade.Execute(context.Background(), upgradeConfig()); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	after, err := os.Stat(rendered)
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Error("up-to-date target was rewritten")
	}
}

// TestExecute_MissingTargetRegeneratesFreely pins the missing case: a deleted copied target regenerates
// without --force.
func TestExecute_MissingTargetRegeneratesFreely(t *testing.T) {

	_, targetRoot, _ := deployFixture(t)
	rendered := filepath.Join(targetRoot, ".gitconfig")

	if err := os.Remove(rendered); err != nil {
		t.Fatal(err)
	}

	if err := upgrade.Execute(context.Background(), upgradeConfig()); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	content, err := os.ReadFile(rendered)
	if err != nil {
		t.Fatalf("read regenerated target: %v", err)
	}
	if got := string(content); got != "os=Darwin" {
		t.Errorf("regenerated content = %q, want %q", got, "os=Darwin")
	}
}

// TestExecute_StaleSourceRegeneratesFreely pins the step-48 attribution: a source change with an untouched
// target (its digest matches the run's recorded as-deployed identity) regenerates WITHOUT --force.
func TestExecute_StaleSourceRegeneratesFreely(t *testing.T) {

	_, targetRoot, templateSource := deployFixture(t)
	rendered := filepath.Join(targetRoot, ".gitconfig")

	if err := os.WriteFile(templateSource, []byte("os={{ .Segments.OS }} v2"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := upgrade.Execute(context.Background(), upgradeConfig()); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	content, err := os.ReadFile(rendered)
	if err != nil || string(content) != "os=Darwin v2" {
		t.Errorf("stale target = %q (err %v), want the fresh render %q", content, err, "os=Darwin v2")
	}
}

// TestExecute_ModifiedTargetIsForceGated pins the other attribution arm: a locally-edited target (digest
// differs from the recorded identity) skips without --force and regenerates with it — even when the source
// also changed.
func TestExecute_ModifiedTargetIsForceGated(t *testing.T) {

	_, targetRoot, templateSource := deployFixture(t)
	rendered := filepath.Join(targetRoot, ".gitconfig")

	if err := os.WriteFile(rendered, []byte("my local edits"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(templateSource, []byte("os={{ .Segments.OS }} v2"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Without --force: the local edit survives.
	if err := upgrade.Execute(context.Background(), upgradeConfig()); err != nil {
		t.Fatalf("Execute (no force): %v", err)
	}
	content, err := os.ReadFile(rendered)
	if err != nil || string(content) != "my local edits" {
		t.Fatalf("locally-modified target changed without --force: %q (err %v)", content, err)
	}

	// With --force: regenerated from the current source.
	cfg := upgradeConfig()
	cfg.Force = true
	if err := upgrade.Execute(context.Background(), cfg); err != nil {
		t.Fatalf("Execute --force: %v", err)
	}
	content, err = os.ReadFile(rendered)
	if err != nil || string(content) != "os=Darwin v2" {
		t.Errorf("forced regeneration = %q (err %v), want %q", content, err, "os=Darwin v2")
	}
}

// TestExecute_SymlinksAreNeverTouched pins the link exclusion: upgrade only considers copied entries.
func TestExecute_SymlinksAreNeverTouched(t *testing.T) {

	_, targetRoot, _ := deployFixture(t)
	link := filepath.Join(targetRoot, ".zshrc")

	before, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}

	cfg := upgradeConfig()
	cfg.Force = true
	if err := upgrade.Execute(context.Background(), cfg); err != nil {
		t.Fatalf("Execute --force: %v", err)
	}

	after, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if after.Mode()&os.ModeSymlink == 0 || !after.ModTime().Equal(before.ModTime()) {
		t.Error("symlink was touched by upgrade")
	}
}
