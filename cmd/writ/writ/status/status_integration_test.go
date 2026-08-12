// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package status_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/deploy"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/segment"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/status"
	"github.com/NobleFactor/devlore-cli/internal/cli"

	// Blank-import the op inventory so provider registration runs for planning and graph loading.
	_ "github.com/NobleFactor/devlore-cli/pkg/op/inventory"
)

// fixtureSegments is the segment set shared by the deploy fixture and the status configs.
var fixtureSegments = segment.Segments{{Name: "OS", Value: "Darwin"}}

// deployFixture runs one real deploy (a plain link and a template) and returns the roots and template source.
func deployFixture(t *testing.T) (sourceRoot, targetRoot, templateSource string) {

	t.Helper()

	root := t.TempDir()
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("HOME", root)
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))

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

// statusConfig returns the baseline status configuration for the fixture.
func statusConfig() *status.Config {
	return &status.Config{Segments: fixtureSegments}
}

// entryFor finds the report entry for a target.
func entryFor(t *testing.T, report *status.Report, target string) status.Entry {

	t.Helper()
	for i := range report.Entries {
		if report.Entries[i].Target == target {
			return report.Entries[i]
		}
	}
	t.Fatalf("no entry for %s; entries: %+v", target, report.Entries)
	return status.Entry{}
}

// TestBuildReport_CleanDeployment pins the healthy shape: linked + copied entries, three absent layers, one
// folded run, no findings.
func TestBuildReport_CleanDeployment(t *testing.T) {

	_, targetRoot, _ := deployFixture(t)

	report, err := status.BuildReport(context.Background(), statusConfig())
	if err != nil {
		t.Fatalf("BuildReport: %v", err)
	}

	link := entryFor(t, report, filepath.Join(targetRoot, ".zshrc"))
	if link.State != status.StateLinked || link.Repair != "" {
		t.Errorf("link entry = %+v, want linked with no repair", link)
	}

	rendered := entryFor(t, report, filepath.Join(targetRoot, ".gitconfig"))
	if rendered.State != status.StateCopied || rendered.Repair != "" {
		t.Errorf("rendered entry = %+v, want copied with no repair", rendered)
	}

	if len(report.Layers) != 3 {
		t.Fatalf("layers = %d, want 3", len(report.Layers))
	}
	for _, layer := range report.Layers {
		if layer.State != "absent" {
			t.Errorf("layer %s = %s, want absent (no layers registered in the fixture)", layer.Name, layer.State)
		}
	}

	if report.Health.Runs != 1 || len(report.Health.Findings) != 0 {
		t.Errorf("health = %+v, want 1 run, no findings", report.Health)
	}
}

// TestBuildReport_Classifications pins the finding classes and their repair pointers.
func TestBuildReport_Classifications(t *testing.T) {

	sourceRoot, targetRoot, templateSource := deployFixture(t)

	// Missing: delete the rendered file.
	if err := os.Remove(filepath.Join(targetRoot, ".gitconfig")); err != nil {
		t.Fatal(err)
	}

	// Conflict: replace the link with a real file.
	linkPath := filepath.Join(targetRoot, ".zshrc")
	if err := os.Remove(linkPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(linkPath, []byte("hand-written"), 0o644); err != nil {
		t.Fatal(err)
	}

	report, err := status.BuildReport(context.Background(), statusConfig())
	if err != nil {
		t.Fatalf("BuildReport: %v", err)
	}

	missing := entryFor(t, report, filepath.Join(targetRoot, ".gitconfig"))
	if missing.State != status.StateMissing || missing.Repair != "writ deploy" {
		t.Errorf("missing entry = %+v, want missing with repair 'writ deploy'", missing)
	}

	conflict := entryFor(t, report, linkPath)
	if conflict.State != status.StateConflict {
		t.Errorf("conflict entry = %+v, want conflict", conflict)
	}

	// Modified-or-stale: restore the deployment, then change the source.
	if err := os.Remove(linkPath); err != nil {
		t.Fatal(err)
	}
	redeploy := &deploy.Config{
		SourceRoot: sourceRoot,
		TargetRoot: targetRoot,
		Projects:   []string{"myproj"},
		Segments:   fixtureSegments,
	}
	if err := deploy.Execute(context.Background(), redeploy); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(templateSource, []byte("os={{ .Segments.OS }} v2"), 0o644); err != nil {
		t.Fatal(err)
	}

	report, err = status.BuildReport(context.Background(), statusConfig())
	if err != nil {
		t.Fatalf("BuildReport (round 2): %v", err)
	}

	stale := entryFor(t, report, filepath.Join(targetRoot, ".gitconfig"))
	if stale.State != status.StateStale || stale.Repair != "writ upgrade" {
		t.Errorf("stale entry = %+v, want stale (attributed via the recorded identity) with repair 'writ upgrade'", stale)
	}

	// Modified: locally edit the redeployed target — the recorded identity attributes it.
	if err := os.WriteFile(filepath.Join(targetRoot, ".gitconfig"), []byte("my local edits"), 0o644); err != nil {
		t.Fatal(err)
	}
	report, err = status.BuildReport(context.Background(), statusConfig())
	if err != nil {
		t.Fatalf("BuildReport (modified round): %v", err)
	}
	modified := entryFor(t, report, filepath.Join(targetRoot, ".gitconfig"))
	if modified.State != status.StateModified || modified.Repair != "writ upgrade --force" {
		t.Errorf("modified entry = %+v, want modified with repair 'writ upgrade --force'", modified)
	}

	// Orphan: delete the link's source.
	if err := os.Remove(filepath.Join(sourceRoot, "myproj", ".zshrc")); err != nil {
		t.Fatal(err)
	}
	report, err = status.BuildReport(context.Background(), statusConfig())
	if err != nil {
		t.Fatalf("BuildReport (round 3): %v", err)
	}
	orphan := entryFor(t, report, linkPath)
	if orphan.State != status.StateOrphan || orphan.Repair != "writ decommission" {
		t.Errorf("orphan entry = %+v, want orphan with repair 'writ decommission'", orphan)
	}
}

// TestBuildReport_LayerLink pins the layers section: a registered link-mode layer reports its target.
func TestBuildReport_LayerLink(t *testing.T) {

	sourceRoot, _, _ := deployFixture(t)

	layersDir := cli.WritLayersDir()
	if err := os.MkdirAll(layersDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(sourceRoot, filepath.Join(layersDir, "personal")); err != nil {
		t.Fatal(err)
	}

	report, err := status.BuildReport(context.Background(), statusConfig())
	if err != nil {
		t.Fatalf("BuildReport: %v", err)
	}

	for _, layer := range report.Layers {
		if layer.Name != "personal" {
			continue
		}
		if layer.State != "link" {
			t.Errorf("personal layer state = %s, want link", layer.State)
		}
		resolved, _ := filepath.EvalSymlinks(sourceRoot) //nolint:errcheck // fixture path exists
		if layer.Target != resolved {
			t.Errorf("personal layer target = %s, want %s", layer.Target, resolved)
		}
		return
	}
	t.Error("personal layer not reported")
}

// TestBuildReport_MissingIndexIsHardError pins the settled ruling: no index, no report.
func TestBuildReport_MissingIndexIsHardError(t *testing.T) {

	deployFixture(t)

	if err := os.Remove(cli.IndexPath()); err != nil {
		t.Fatal(err)
	}

	_, err := status.BuildReport(context.Background(), statusConfig())
	if err == nil || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("BuildReport over a missing index = %v, want the hard error", err)
	}
}
