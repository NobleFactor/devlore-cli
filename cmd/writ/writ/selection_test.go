// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package writ

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/cmd/internal/cli"
	"github.com/NobleFactor/devlore-cli/cmd/internal/devlore"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/deploy"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/segment"

	// The deploy plans through the provider registry, which the inventory's init() populates.
	_ "github.com/NobleFactor/devlore-cli/pkg/op/inventory"
)

// registerLayers points the XDG homes at a temporary root and registers `names` as base, team and personal in
// that order, each a repository directory named for itself with a Home holding `common` and the given projects.
func registerLayers(t *testing.T, names []string, projects map[string][]string) {

	t.Helper()

	root := t.TempDir()
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))

	if err := os.MkdirAll(devlore.WritLayersDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	for i, name := range names {
		repository := filepath.Join(root, "Workspace", name)
		for _, project := range append([]string{"common"}, projects[name]...) {
			if err := os.MkdirAll(filepath.Join(repository, "Home", project), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.Symlink(repository, filepath.Join(devlore.WritLayersDir(), LayerOrder[i])); err != nil {
			t.Fatal(err)
		}
	}
}

// TestResolveSelection_Implicit pins the implicit set (#850): common, then one project per configured layer
// repository in layer order, whether or not any layer carries a project by that name.
func TestResolveSelection_Implicit(t *testing.T) {

	registerLayers(t, []string{"noblefactor-ops", "devlore-cli", "personal"}, map[string][]string{"personal": {"devlore-cli"}})

	selection, err := resolveSelection(context.Background(), nil)
	if err != nil {
		t.Fatalf("resolveSelection: %v", err)
	}
	want := []string{"common", "noblefactor-ops", "devlore-cli", "personal"}
	if !slices.Equal(selection.Implicit, want) {
		t.Errorf("Implicit = %v, want %v", selection.Implicit, want)
	}
	if len(selection.Recorded) != 0 || len(selection.Named) != 0 {
		t.Errorf("Recorded = %v, Named = %v on a bare deploy with nothing recorded; want none", selection.Recorded, selection.Named)
	}
	if got := selection.Narration(); got != "common, noblefactor-ops, devlore-cli, personal (implicit)" {
		t.Errorf("Narration = %q", got)
	}
}

// TestResolveSelection_Named pins the named kind: a name adds once, a name already implicit is not repeated, and a
// name no registered layer carries is refused with the name in the message, exit 64.
func TestResolveSelection_Named(t *testing.T) {

	registerLayers(t, []string{"noblefactor-ops", "devlore-cli", "personal"}, map[string][]string{"personal": {"thenobles", "thenobles.Darwin"}})

	selection, err := resolveSelection(context.Background(), []string{"thenobles", "devlore-cli", "thenobles"})
	if err != nil {
		t.Fatalf("resolveSelection: %v", err)
	}
	if !slices.Equal(selection.Named, []string{"thenobles"}) {
		t.Errorf("Named = %v, want [thenobles]: devlore-cli is implicit and thenobles adds once", selection.Named)
	}
	if got := selection.Narration(); got != "common, noblefactor-ops, devlore-cli, personal (implicit); thenobles (named)" {
		t.Errorf("Narration = %q", got)
	}

	_, err = resolveSelection(context.Background(), []string{"noblefamily"})
	if err == nil {
		t.Fatal("an unknown project was accepted")
	}
	if !strings.Contains(err.Error(), `"noblefamily"`) {
		t.Errorf("the refusal does not name the project: %v", err)
	}
	if code := cli.ExitCode(err); code != cli.ExitUsage {
		t.Errorf("ExitCode = %d, want %d", code, cli.ExitUsage)
	}
}

// TestResolveSelection_Recorded pins the recorded kind (#850, ruled 2026-09-26: the record is the selection): a
// project a deploy put in place stays selected by a later bare deploy, and an implicit project is never repeated
// as recorded.
func TestResolveSelection_Recorded(t *testing.T) {

	registerLayers(t, []string{"noblefactor-ops", "devlore-cli", "personal"}, map[string][]string{"personal": {"thenobles"}})
	personal := filepath.Join(devlore.WritLayersDir(), "personal")
	if err := os.WriteFile(filepath.Join(personal, "Home", "thenobles", ".tnrc"), []byte("tn"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(personal, "Home", "common", ".commonrc"), []byte("common"), 0o644); err != nil {
		t.Fatal(err)
	}

	targetRoot := t.TempDir()
	if _, err := deploy.Execute(context.Background(), &deploy.Config{
		SourceRoot: filepath.Join(personal, "Home"),
		TargetRoot: targetRoot,
		Projects:   []string{"common", "thenobles"},
		Segments:   segment.Segments{{Name: "OS", Value: "Darwin"}},
	}); err != nil {
		t.Fatalf("deploy: %v", err)
	}

	selection, err := resolveSelection(context.Background(), nil)
	if err != nil {
		t.Fatalf("resolveSelection: %v", err)
	}
	if !slices.Equal(selection.Recorded, []string{"thenobles"}) {
		t.Errorf("Recorded = %v, want [thenobles]: the record holds it, and common is implicit", selection.Recorded)
	}
	if got := selection.Narration(); got != "common, noblefactor-ops, devlore-cli, personal (implicit); thenobles (recorded)" {
		t.Errorf("Narration = %q", got)
	}
}
