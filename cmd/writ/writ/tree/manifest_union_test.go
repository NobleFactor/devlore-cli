// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package tree

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/segment"
)

// writeManifest puts a packages-manifest.yaml naming one package under `dir`.
//
// Parameters:
//   - `t`: the test.
//   - `dir`: the directory to create and write into.
//   - `pkg`: the package the manifest claims.
func writeManifest(t *testing.T, dir, pkg string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "packages:\n  - name: " + pkg + "\n"
	if err := os.WriteFile(filepath.Join(dir, "packages-manifest.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// darwinSegments is the segment set the union tests match against.
func darwinSegments() segment.Segments {
	return segment.Segments{{Name: "OS", Value: "Darwin"}}
}

// TestBuild_ManifestsAtTwoSpecificitiesBothSurvive is #814: a manifest lands nowhere in the filesystem, so the
// overlay rule that keeps one file per target path must not apply to it. Both of a layer's manifests contribute.
func TestBuild_ManifestsAtTwoSpecificitiesBothSurvive(t *testing.T) {

	layerDir, targetDir := t.TempDir(), t.TempDir()
	writeManifest(t, filepath.Join(layerDir, "all"), "general")
	writeManifest(t, filepath.Join(layerDir, "all.Darwin"), "specific")

	result, err := Build(BuildConfig{
		Sources:  []LayerSource{{Layer: "personal", Path: layerDir, Order: 2, SourceRoot: layerDir, TargetRoot: targetDir}},
		Projects: []string{"all"},
		Segments: darwinSegments(),
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if len(result.Manifests) != 2 {
		t.Fatalf("got %d manifests, want 2 -- both contribute; a manifest is not a file that lands somewhere", len(result.Manifests))
	}
	if !strings.Contains(result.Manifests[0].Source, filepath.Join("all", "")) ||
		strings.Contains(result.Manifests[0].Source, "all.Darwin") {
		t.Errorf("first manifest is %s; contribution order is general before specific", result.Manifests[0].Source)
	}
	for _, collision := range result.Collisions {
		if strings.Contains(collision.Target, "packages-manifest") {
			t.Errorf("a manifest was recorded as a collision: %+v", collision)
		}
	}
	for _, file := range result.Files {
		if strings.Contains(file.ID, "packages-manifest") {
			t.Errorf("a manifest is still in Files: %s", file.ID)
		}
	}
}

// TestBuild_ManifestsInTwoLayersBothSurvive is the layer half: base and team each contribute, base first.
func TestBuild_ManifestsInTwoLayersBothSurvive(t *testing.T) {

	baseDir, teamDir, targetDir := t.TempDir(), t.TempDir(), t.TempDir()
	writeManifest(t, filepath.Join(baseDir, "all"), "from-base")
	writeManifest(t, filepath.Join(teamDir, "all"), "from-team")

	result, err := Build(BuildConfig{
		Sources: []LayerSource{
			{Layer: "base", Path: baseDir, Order: 1, SourceRoot: baseDir, TargetRoot: targetDir},
			{Layer: "team", Path: teamDir, Order: 2, SourceRoot: teamDir, TargetRoot: targetDir},
		},
		Projects: []string{"all"},
		Segments: darwinSegments(),
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if len(result.Manifests) != 2 {
		t.Fatalf("got %d manifests, want 2 -- every layer contributes", len(result.Manifests))
	}
	if result.Manifests[0].Layer != "base" || result.Manifests[1].Layer != "team" {
		t.Errorf("layers are %q then %q; contribution order is base before team",
			result.Manifests[0].Layer, result.Manifests[1].Layer)
	}
}

// TestBuild_AFileStillCollides guards the other side: the bypass is for manifests only, and an ordinary file at one
// target path in two places still resolves to a single winner with a collision recorded.
func TestBuild_AFileStillCollides(t *testing.T) {

	layerDir, targetDir := t.TempDir(), t.TempDir()
	for _, arm := range []struct{ dir, body string }{{"all", "generic"}, {"all.Darwin", "darwin"}} {
		if err := os.MkdirAll(filepath.Join(layerDir, arm.dir), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(layerDir, arm.dir, ".bashrc"), []byte(arm.body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	result, err := Build(BuildConfig{
		Sources:  []LayerSource{{Layer: "personal", Path: layerDir, Order: 2, SourceRoot: layerDir, TargetRoot: targetDir}},
		Projects: []string{"all"},
		Segments: darwinSegments(),
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if len(result.Files) != 1 || len(result.Collisions) != 1 {
		t.Fatalf("got %d files and %d collisions, want 1 and 1 -- files still overlay", len(result.Files), len(result.Collisions))
	}
	if !strings.Contains(result.Files[0].Source, "all.Darwin") {
		t.Errorf("winner is %s; the more specific file wins", result.Files[0].Source)
	}
}
