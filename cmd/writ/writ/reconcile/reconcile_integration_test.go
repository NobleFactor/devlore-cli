// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package reconcile_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/NobleFactor/devlore-cli/cmd/internal/cli"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/deploy"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/reconcile"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/segment"

	// Blank-import the op inventory so provider registration runs for planning and graph loading.
	"github.com/NobleFactor/devlore-cli/cmd/internal/devlore"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/inventory"
)

// fixtureSegments is the segment set shared by the deploy fixture and the reconcile configs.
var fixtureSegments = segment.Segments{{Name: "OS", Value: "Darwin"}}

// deployFixture runs one real deploy (a plain link and a template) and returns the roots and template source.
func deployFixture(t *testing.T) (sourceRoot, targetRoot, templateSource string) {

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

	if _, err := deploy.Execute(context.Background(), cfg); err != nil {
		t.Fatalf("deploy fixture: %v", err)
	}

	return sourceRoot, targetRoot, templateSource
}

// reconcileConfig returns the baseline reconcile configuration for the fixture.
func reconcileConfig() *reconcile.Config {
	return &reconcile.Config{Segments: fixtureSegments}
}

// entryFor finds the report entry for a target.
func entryFor(t *testing.T, report *reconcile.Report, target string) reconcile.Entry {

	t.Helper()
	for i := range report.Entries {
		if report.Entries[i].Target == target {
			return report.Entries[i]
		}
	}
	t.Fatalf("no entry for %s; entries: %+v", target, report.Entries)
	return reconcile.Entry{}
}

// TestBuildReport_CleanDeployment pins the healthy shape: linked + copied entries, three absent layers, one
// folded run, no findings.
func TestBuildReport_CleanDeployment(t *testing.T) {

	_, targetRoot, _ := deployFixture(t)

	report, err := reconcile.BuildReport(context.Background(), reconcileConfig())
	if err != nil {
		t.Fatalf("BuildReport: %v", err)
	}

	link := entryFor(t, report, filepath.Join(targetRoot, ".zshrc"))
	if link.State != reconcile.StateLinked || link.Repair != "" {
		t.Errorf("link entry = %+v, want linked with no repair", link)
	}

	rendered := entryFor(t, report, filepath.Join(targetRoot, ".gitconfig"))
	if rendered.State != reconcile.StateCopied || rendered.Repair != "" {
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

// TestBuildReport_Classifications pins the six words and the repair each names (#923): absent, changed, stale,
// dangling, for a link and for a copy, the record as the reference.
func TestBuildReport_Classifications(t *testing.T) {

	sourceRoot, targetRoot, templateSource := deployFixture(t)
	rendered := filepath.Join(targetRoot, ".gitconfig")
	linkPath := filepath.Join(targetRoot, ".zshrc")

	// absent: the rendered copy is removed. changed: the link is replaced by a real file.
	if err := os.Remove(rendered); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(linkPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(linkPath, []byte("hand-written"), 0o644); err != nil {
		t.Fatal(err)
	}

	report, err := reconcile.BuildReport(context.Background(), reconcileConfig())
	if err != nil {
		t.Fatalf("BuildReport: %v", err)
	}
	if absent := entryFor(t, report, rendered); absent.State != reconcile.StateAbsent || absent.Repair != "writ deploy" {
		t.Errorf("absent entry = %+v, want absent with repair 'writ deploy'", absent)
	}
	if changed := entryFor(t, report, linkPath); changed.State != reconcile.StateChanged || changed.Repair != "writ deploy" {
		t.Errorf("changed link = %+v, want changed with repair 'writ deploy'", changed)
	}

	// stale: redeploy, then move the template's source under the copy and the link's source under the link.
	if err := os.Remove(linkPath); err != nil {
		t.Fatal(err)
	}
	redeploy := &deploy.Config{
		SourceRoot: sourceRoot,
		TargetRoot: targetRoot,
		Projects:   []string{"myproj"},
		Segments:   fixtureSegments,
	}
	if _, err := deploy.Execute(context.Background(), redeploy); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(templateSource, []byte("os={{ .Segments.OS }} v2"), 0o644); err != nil {
		t.Fatal(err)
	}
	linkSource := filepath.Join(sourceRoot, "myproj", ".zshrc")
	if err := os.WriteFile(linkSource, []byte("plain zsh, edited in the checkout"), 0o644); err != nil {
		t.Fatal(err)
	}

	report, err = reconcile.BuildReport(context.Background(), reconcileConfig())
	if err != nil {
		t.Fatalf("BuildReport (stale round): %v", err)
	}
	if stale := entryFor(t, report, rendered); stale.State != reconcile.StateStale || stale.Repair != "writ upgrade" {
		t.Errorf("stale copy = %+v, want stale with repair 'writ upgrade'", stale)
	}
	// A link's referent IS the checkout, and the record holds no source digest for a link (the link's source is a
	// path slot, not a cataloged resource), so an edit under a link is git's business and the link stays linked.
	if linked := entryFor(t, report, linkPath); linked.State != reconcile.StateLinked {
		t.Errorf("link whose referent was edited = %+v, want linked (the record holds no source digest for a link)", linked)
	}

	// changed copy: a local edit of the copy; the recorded target digest attributes it.
	if err := os.WriteFile(rendered, []byte("my local edits"), 0o644); err != nil {
		t.Fatal(err)
	}
	report, err = reconcile.BuildReport(context.Background(), reconcileConfig())
	if err != nil {
		t.Fatalf("BuildReport (changed round): %v", err)
	}
	if changed := entryFor(t, report, rendered); changed.State != reconcile.StateChanged || changed.Repair != "writ upgrade --force" {
		t.Errorf("changed copy = %+v, want changed with repair 'writ upgrade --force'", changed)
	}

	// dangling: the sources are removed under both -- a reference that outlives its referent, for both kinds.
	if err := os.Remove(linkSource); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(templateSource); err != nil {
		t.Fatal(err)
	}
	report, err = reconcile.BuildReport(context.Background(), reconcileConfig())
	if err != nil {
		t.Fatalf("BuildReport (dangling round): %v", err)
	}
	if dangling := entryFor(t, report, linkPath); dangling.State != reconcile.StateDangling || dangling.Repair != "writ deploy" {
		t.Errorf("dangling link = %+v, want dangling with repair 'writ deploy'", dangling)
	}
	// the copy was locally edited above; restore it so the source's absence is what decides
	if err := os.WriteFile(rendered, []byte("os=Darwin"), 0o644); err != nil {
		t.Fatal(err)
	}
	report, err = reconcile.BuildReport(context.Background(), reconcileConfig())
	if err != nil {
		t.Fatalf("BuildReport (dangling copy round): %v", err)
	}
	if dangling := entryFor(t, report, rendered); dangling.State != reconcile.StateDangling || dangling.Repair != "writ deploy" {
		t.Errorf("dangling copy = %+v, want dangling with repair 'writ deploy'", dangling)
	}
}

// TestBuildReport_Words pins the vocabulary: six labels, and only these.
func TestBuildReport_Words(t *testing.T) {

	want := map[reconcile.State]string{
		reconcile.StateLinked:   "linked",
		reconcile.StateCopied:   "copied",
		reconcile.StateAbsent:   "absent",
		reconcile.StateChanged:  "changed",
		reconcile.StateDangling: "dangling",
		reconcile.StateStale:    "stale",
	}
	for state, label := range want {
		if state.Label() != label {
			t.Errorf("State(%d).Label() = %q, want %q", state, state.Label(), label)
		}
	}
	if got := reconcile.State(6).Label(); got != "unknown" {
		t.Errorf("a seventh state labels %q; the vocabulary is six words", got)
	}
}

// TestBuildReport_LayerLink pins the layers section: a registered link-mode layer reports its target.
func TestBuildReport_LayerLink(t *testing.T) {

	sourceRoot, _, _ := deployFixture(t)

	layersDir := devlore.WritLayersDir()
	if err := os.MkdirAll(layersDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(sourceRoot, filepath.Join(layersDir, "personal")); err != nil {
		t.Fatal(err)
	}

	report, err := reconcile.BuildReport(context.Background(), reconcileConfig())
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

// TestBuildReport_NoLifetimeIsNotFound pins the ruled answer (#922, #756): with no current lifetime there is no
// record to compare against, and the report is a not-found error, never an empty report.
func TestBuildReport_NoLifetimeIsNotFound(t *testing.T) {

	deployFixture(t)

	if err := os.Remove(filepath.Join(cli.LifetimesDir(), "current")); err != nil {
		t.Fatal(err)
	}

	_, err := reconcile.BuildReport(context.Background(), reconcileConfig())
	if err == nil {
		t.Fatal("BuildReport with no current lifetime = nil error, want not-found")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("error %v does not unwrap to os.ErrNotExist", err)
	}
}
