// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package tree

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/segment"
)

// plant writes the same file, `.bashrc`, into each named directory of a layer, its content the directory's name.
//
// Parameters:
//   - `t`: the test.
//   - `root`: the layer's root.
//   - `dirs`: the directories.
func plant(t *testing.T, root string, dirs ...string) {

	t.Helper()
	for _, dir := range dirs {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, dir, ".bashrc"), []byte(dir), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// ubuntuArm64 is the owner's machine: Ubuntu, whose lineage is Debian, on arm64.
//
// Returns:
//   - `segment.Segments`: its built-in segments.
func ubuntuArm64() segment.Segments {

	return segment.Segments{
		{Name: "OS", Value: "Linux"},
		{Name: "DISTRO", Value: "Ubuntu", Lineage: []string{"Debian"}},
		{Name: "ARCH", Value: "arm64"},
	}
}

// winningDir returns the directory the only file of a build came from.
//
// Parameters:
//   - `t`: the test.
//   - `result`: the build.
//
// Returns:
//   - `string`: the directory's name.
func winningDir(t *testing.T, result *BuildResult) string {

	t.Helper()
	if len(result.Files) != 1 {
		t.Fatalf("got %d files, want 1", len(result.Files))
	}
	return filepath.Base(filepath.Dir(result.Files[0].Source))
}

func TestProcessingPipeline(t *testing.T) {
	tests := []struct {
		filename   string
		targetName string
		actions    []string
	}{
		{"foo", "foo", []string{"file.link"}},
		{"foo.tmpl", "foo", []string{"template.render_bytes", "file.copy"}},
		// Another tool's template carries `.template`; writ links it untouched (#974).
		{"foo.template", "foo.template", []string{"file.link"}},
		{"foo.age", "foo.age", []string{"file.link"}},
		{"foo.sops", "foo", []string{"encryption.decrypt", "file.copy"}},
		{"foo.tmpl.sops", "foo", []string{"encryption.decrypt", "template.render_bytes", "file.copy"}},
		{".bashrc", ".bashrc", []string{"file.link"}},
		{".bashrc.tmpl", ".bashrc", []string{"template.render_bytes", "file.copy"}},
		{"config.yaml.tmpl.sops", "config.yaml", []string{"encryption.decrypt", "template.render_bytes", "file.copy"}},
		{"packages-manifest.yaml", "packages-manifest.yaml", []string{"manifest.resolve"}},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			name, actions := ProcessingPipeline(tt.filename)
			if name != tt.targetName {
				t.Errorf("name = %q, want %q", name, tt.targetName)
			}
			if len(actions) != len(tt.actions) {
				t.Errorf("actions = %v, want %v", actions, tt.actions)
				return
			}
			for i := range actions {
				if actions[i] != tt.actions[i] {
					t.Errorf("actions[%d] = %q, want %q", i, actions[i], tt.actions[i])
				}
			}
		})
	}
}

func TestBuild(t *testing.T) {
	// Create temp directory with test structure
	tmpDir := t.TempDir()

	// Create project directories
	dirs := []string{
		"all",
		"all.Darwin",
		"all.Unix",
		"noblefactor",
		"noblefactor.Unix",
	}

	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(tmpDir, d), 0o755); err != nil {
			t.Fatalf("failed to create dir %s: %v", d, err)
		}
	}

	// Create test files
	files := map[string]string{
		"all/.bashrc":                    "bashrc content",
		"all/.config/test.yaml":          "test config",
		"all.Darwin/.config/darwin.yaml": "darwin config",
		"all.Unix/.config/unix.yaml":     "unix config",
		"noblefactor/.ssh/config":        "ssh config",
		"noblefactor.Unix/script.tmpl":   "template content",
	}

	for path, content := range files {
		fullPath := filepath.Join(tmpDir, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatalf("failed to create dir for %s: %v", path, err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to write %s: %v", path, err)
		}
	}

	// Build tree for Darwin
	darwinSegs := segment.Segments{
		{Name: "OS", Value: "Darwin"},
		{Name: "DISTRO", Value: ""},
		{Name: "ARCH", Value: "arm64"},
	}

	targetDir := t.TempDir()

	result, err := Build(BuildConfig{
		SourceRoot: tmpDir,
		TargetRoot: targetDir,
		Projects:   []string{"all", "noblefactor"},
		Segments:   darwinSegs,
	})

	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Verify matched directories
	expectedDirs := 5 // all, all.Darwin, all.Unix, noblefactor, noblefactor.Unix
	if len(result.MatchedDirs) != expectedDirs {
		t.Errorf("got %d matched dirs, want %d", len(result.MatchedDirs), expectedDirs)
		for _, m := range result.MatchedDirs {
			t.Logf("  matched: %s", filepath.Base(m.Path))
		}
	}

	// Verify files were found
	expectedFiles := 6 // all the files we created
	if len(result.Files) != expectedFiles {
		t.Errorf("got %d nodes, want %d", len(result.Files), expectedFiles)
		for _, n := range result.Files {
			t.Logf("  node: %s actions=%v", n.ID, n.Operations)
		}
	}

	// Verify template detection
	if result.TemplateCount() != 1 {
		t.Errorf("template count = %d, want 1", result.TemplateCount())
	}

	// Verify link count
	if result.LinkCount() != 5 {
		t.Errorf("link count = %d, want 5", result.LinkCount())
	}

	// Test output
	output := result.String()
	if output == "" {
		t.Error("String() returned empty")
	}
	t.Logf("Tree output:\n%s", output)
}

func TestBuildWithCollisions(t *testing.T) {
	// Create temp directory with overlapping files
	tmpDir := t.TempDir()

	// Create directories with different specificity
	dirs := []string{
		"all",        // specificity 0
		"all.Darwin", // specificity 1
	}

	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(tmpDir, d), 0o755); err != nil {
			t.Fatalf("failed to create dir %s: %v", d, err)
		}
	}

	// Create the same file in both directories
	// all/.bashrc (less specific)
	if err := os.WriteFile(filepath.Join(tmpDir, "all", ".bashrc"), []byte("generic"), 0o644); err != nil {
		t.Fatal(err)
	}
	// all.Darwin/.bashrc (more specific - should win)
	if err := os.WriteFile(filepath.Join(tmpDir, "all.Darwin", ".bashrc"), []byte("darwin specific"), 0o644); err != nil {
		t.Fatal(err)
	}

	darwinSegs := segment.Segments{
		{Name: "OS", Value: "Darwin"},
		{Name: "DISTRO", Value: ""},
		{Name: "ARCH", Value: "arm64"},
	}

	targetDir := t.TempDir()

	result, err := Build(BuildConfig{
		SourceRoot: tmpDir,
		TargetRoot: targetDir,
		Projects:   []string{"all"},
		Segments:   darwinSegs,
	})

	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Should have exactly 1 node (collision resolved)
	if len(result.Files) != 1 {
		t.Errorf("got %d nodes, want 1 (collision should resolve to single node)", len(result.Files))
	}

	// Should have 1 collision recorded
	if len(result.Collisions) != 1 {
		t.Errorf("got %d collisions, want 1", len(result.Collisions))
	}

	// The winner should be the more specific one (all.Darwin)
	if len(result.Files) > 0 {
		node := result.Files[0]
		// The winner source should contain "all.Darwin"
		if !strings.Contains(node.Source, "all.Darwin") {
			t.Errorf("winner should be from all.Darwin, got source %s", node.Source)
		}
	}

	// Verify collision details
	if len(result.Collisions) > 0 {
		c := result.Collisions[0]
		if c.Target != ".bashrc" {
			t.Errorf("collision target = %q, want %q", c.Target, ".bashrc")
		}
		if c.WinnerDir != "all.Darwin" || c.LoserDir != "all" {
			t.Errorf("collision = %s over %s, want all.Darwin over all", c.WinnerDir, c.LoserDir)
		}
	}

	// Verify output includes collision warning
	output := result.String()
	if !strings.Contains(output, "Collisions (1)") {
		t.Error("output should contain collision warning")
	}

	t.Logf("Tree output:\n%s", output)
}

// TestPackagesManifestFiles tests that only valid manifest filenames are recognized.
// This test ensures that legacy filenames like "packages.manifest" are rejected.
func TestPackagesManifestFiles(t *testing.T) {
	// Verify the list contains only the expected valid filenames
	expected := map[string]bool{
		"packages-manifest.yaml": true,
		"packages-manifest.json": true,
	}

	if len(PackagesManifestFiles) != len(expected) {
		t.Errorf("PackagesManifestFiles has %d entries, want %d", len(PackagesManifestFiles), len(expected))
	}

	for _, f := range PackagesManifestFiles {
		if !expected[f] {
			t.Errorf("unexpected filename in PackagesManifestFiles: %q", f)
		}
	}

	// Verify legacy filenames are NOT in the list
	legacyFiles := []string{
		"packages.manifest",
		"packages.yaml",
		"manifest.yaml",
		"packages.json",
	}

	for _, legacy := range legacyFiles {
		for _, valid := range PackagesManifestFiles {
			if legacy == valid {
				t.Errorf("legacy filename %q should NOT be in PackagesManifestFiles", legacy)
			}
		}
	}
}

// TestProcessingPipeline_ManifestFilenames tests that only valid manifest filenames
// trigger the manifest.resolve action, and legacy filenames are treated as regular files.
func TestProcessingPipeline_ManifestFilenames(t *testing.T) {
	tests := []struct {
		filename    string
		wantResolve bool // true if should get manifest-resolve
		description string
	}{
		{"packages-manifest.yaml", true, "valid YAML manifest"},
		{"packages-manifest.json", true, "valid JSON manifest"},
		{"packages.manifest", false, "legacy filename must be rejected"},
		{"packages.yaml", false, "invalid manifest name"},
		{"manifest.yaml", false, "invalid manifest name"},
		{"packages-manifest.yml", false, "wrong extension"},
		{"PACKAGES-MANIFEST.YAML", false, "case-sensitive check"},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			_, actions := ProcessingPipeline(tt.filename)
			hasResolve := hasAction(actions, "manifest.resolve")
			if hasResolve != tt.wantResolve {
				t.Errorf("ProcessingPipeline(%q) manifest-resolve = %v, want %v (%s)",
					tt.filename, hasResolve, tt.wantResolve, tt.description)
			}
		})
	}
}

func TestActionHelpers(t *testing.T) {
	tests := []struct {
		actions     []string
		hasCopy     bool
		hasManifest bool
	}{
		{[]string{"file.link"}, false, false},
		{[]string{"template.render_bytes", "file.copy"}, true, false},
		{[]string{"encryption.decrypt", "file.copy"}, true, false},
		{[]string{"encryption.decrypt", "template.render_bytes", "file.copy"}, true, false},
		{[]string{"manifest.resolve"}, false, true},
	}

	for _, tt := range tests {
		if got := hasAction(tt.actions, "file.copy"); got != tt.hasCopy {
			t.Errorf("hasAction(%v, file.copy) = %v, want %v", tt.actions, got, tt.hasCopy)
		}
		if got := hasAction(tt.actions, "manifest.resolve"); got != tt.hasManifest {
			t.Errorf("hasAction(%v, manifest.resolve) = %v, want %v", tt.actions, got, tt.hasManifest)
		}
	}
}

func TestBuildMultiSource(t *testing.T) {
	// Create base and personal layer directories
	baseDir := t.TempDir()
	personalDir := t.TempDir()
	targetDir := t.TempDir()

	// Create project in base layer
	if err := os.MkdirAll(filepath.Join(baseDir, "all"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "all", ".bashrc"), []byte("base bashrc"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create project in personal layer with different file
	if err := os.MkdirAll(filepath.Join(personalDir, "all"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(personalDir, "all", ".zshrc"), []byte("personal zshrc"), 0o644); err != nil {
		t.Fatal(err)
	}

	segs := segment.Segments{
		{Name: "OS", Value: "Darwin"},
	}

	sources := []LayerSource{
		{Layer: "base", Path: baseDir, Order: 0, SourceRoot: baseDir, TargetRoot: targetDir},
		{Layer: "personal", Path: personalDir, Order: 2, SourceRoot: personalDir, TargetRoot: targetDir},
	}

	result, err := Build(BuildConfig{
		Sources:  sources,
		Projects: []string{"all"},
		Segments: segs,
	})

	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Should have 2 nodes (one from each layer)
	if len(result.Files) != 2 {
		t.Errorf("got %d nodes, want 2", len(result.Files))
		for _, f := range result.Files {
			t.Logf("  file: %s layer=%s", f.ID, f.Layer)
		}
	}

	// Verify layer is set
	for _, f := range result.Files {
		if f.Layer == "" {
			t.Errorf("file %s missing layer", f.ID)
		}
	}

	// No collisions (different files)
	if len(result.Collisions) != 0 {
		t.Errorf("got %d collisions, want 0", len(result.Collisions))
	}
}

func TestBuildMultiSourceLayerPrecedence(t *testing.T) {
	// Create base and personal layer directories
	baseDir := t.TempDir()
	personalDir := t.TempDir()
	targetDir := t.TempDir()

	// Create same file in both layers
	if err := os.MkdirAll(filepath.Join(baseDir, "all"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "all", ".bashrc"), []byte("base bashrc"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(filepath.Join(personalDir, "all"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(personalDir, "all", ".bashrc"), []byte("personal bashrc"), 0o644); err != nil {
		t.Fatal(err)
	}

	segs := segment.Segments{
		{Name: "OS", Value: "Darwin"},
	}

	sources := []LayerSource{
		{Layer: "base", Path: baseDir, Order: 0, SourceRoot: baseDir, TargetRoot: targetDir},
		{Layer: "personal", Path: personalDir, Order: 2, SourceRoot: personalDir, TargetRoot: targetDir},
	}

	result, err := Build(BuildConfig{
		Sources:  sources,
		Projects: []string{"all"},
		Segments: segs,
	})

	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Should have 1 node (collision resolved)
	if len(result.Files) != 1 {
		t.Errorf("got %d nodes, want 1", len(result.Files))
	}

	// Should have 1 collision
	if len(result.Collisions) != 1 {
		t.Errorf("got %d collisions, want 1", len(result.Collisions))
	}

	// Personal layer should win
	if len(result.Files) > 0 {
		f := result.Files[0]
		if f.Layer != "personal" {
			t.Errorf("winner layer = %s, want personal", f.Layer)
		}
		if !strings.Contains(f.Source, personalDir) {
			t.Errorf("winner source should be from personal layer, got %s", f.Source)
		}
	}

	// Verify collision details
	if len(result.Collisions) > 0 {
		c := result.Collisions[0]
		if c.WinnerLayer != "personal" {
			t.Errorf("collision winner layer = %s, want personal", c.WinnerLayer)
		}
		if c.LoserLayer != "base" {
			t.Errorf("collision loser layer = %s, want base", c.LoserLayer)
		}
	}
}

func TestBuildMultiSourceSpecificityWithinLayer(t *testing.T) {
	// Create single layer with different specificities
	layerDir := t.TempDir()
	targetDir := t.TempDir()

	// Create all (specificity 0) and all.Darwin (specificity 1)
	if err := os.MkdirAll(filepath.Join(layerDir, "all"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(layerDir, "all.Darwin"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Same file in both
	if err := os.WriteFile(filepath.Join(layerDir, "all", ".bashrc"), []byte("generic"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(layerDir, "all.Darwin", ".bashrc"), []byte("darwin"), 0o644); err != nil {
		t.Fatal(err)
	}

	segs := segment.Segments{
		{Name: "OS", Value: "Darwin"},
	}

	sources := []LayerSource{
		{Layer: "personal", Path: layerDir, Order: 2, SourceRoot: layerDir, TargetRoot: targetDir},
	}

	result, err := Build(BuildConfig{
		Sources:  sources,
		Projects: []string{"all"},
		Segments: segs,
	})

	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Should have 1 node (collision resolved)
	if len(result.Files) != 1 {
		t.Errorf("got %d nodes, want 1", len(result.Files))
	}

	// More specific (all.Darwin) should win
	if len(result.Files) > 0 {
		node := result.Files[0]
		if !strings.Contains(node.Source, "all.Darwin") {
			t.Errorf("winner should be from all.Darwin, got %s", node.Source)
		}
	}

	// Verify collision details
	if len(result.Collisions) != 1 {
		t.Errorf("got %d collisions, want 1", len(result.Collisions))
	} else {
		c := result.Collisions[0]
		if c.WinnerDir != "all.Darwin" || c.LoserDir != "all" {
			t.Errorf("collision = %s over %s, want all.Darwin over all", c.WinnerDir, c.LoserDir)
		}
	}
}

func TestBuildMultiSourceLayerBeatsSpecificity(t *testing.T) {
	// Layer precedence should beat specificity
	baseDir := t.TempDir()
	personalDir := t.TempDir()
	targetDir := t.TempDir()

	// ProviderBase layer with high specificity (all.Darwin)
	if err := os.MkdirAll(filepath.Join(baseDir, "all.Darwin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "all.Darwin", ".bashrc"), []byte("base darwin"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Personal layer with low specificity (all)
	if err := os.MkdirAll(filepath.Join(personalDir, "all"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(personalDir, "all", ".bashrc"), []byte("personal generic"), 0o644); err != nil {
		t.Fatal(err)
	}

	segs := segment.Segments{
		{Name: "OS", Value: "Darwin"},
	}

	sources := []LayerSource{
		{Layer: "base", Path: baseDir, Order: 0, SourceRoot: baseDir, TargetRoot: targetDir},
		{Layer: "personal", Path: personalDir, Order: 2, SourceRoot: personalDir, TargetRoot: targetDir},
	}

	result, err := Build(BuildConfig{
		Sources:  sources,
		Projects: []string{"all"},
		Segments: segs,
	})

	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Should have 1 node
	if len(result.Files) != 1 {
		t.Errorf("got %d nodes, want 1", len(result.Files))
	}

	// Personal layer should win despite lower specificity
	if len(result.Files) > 0 {
		f := result.Files[0]
		if f.Layer != "personal" {
			t.Errorf("winner layer = %s, want personal (layer beats specificity)", f.Layer)
		}
	}

	// Verify collision
	if len(result.Collisions) != 1 {
		t.Errorf("got %d collisions, want 1", len(result.Collisions))
	} else {
		c := result.Collisions[0]
		// Personal's all wins over base's more specific all.Darwin: layers are processed left to right (Q4)
		if c.WinnerDir != "all" || c.LoserDir != "all.Darwin" {
			t.Errorf("collision = %s over %s, want personal's all over base's all.Darwin", c.WinnerDir, c.LoserDir)
		}
		if c.WinnerLayer != "personal" {
			t.Errorf("winner layer = %s, want personal", c.WinnerLayer)
		}
		if c.LoserLayer != "base" {
			t.Errorf("loser layer = %s, want base", c.LoserLayer)
		}
	}
}

// --- The one selector API's ordering (#944) ---

// TestBuild_TheMostSpecificLinkWins pins "the most specific link in the chain wins" and the ruled order of application
// (Q3), in a single-source build and within one layer of a multi-source build.
func TestBuild_TheMostSpecificLinkWins(t *testing.T) {

	dirs := []string{"common", "common.Unix", "common.Linux", "common.Debian", "common.Debian.arm64", "common.Ubuntu",
		"common.Linux.arm64"}

	single := t.TempDir()
	plant(t, single, dirs...)
	result, err := Build(BuildConfig{SourceRoot: single, TargetRoot: t.TempDir(), Projects: []string{"common"},
		Segments: ubuntuArm64()})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got := winningDir(t, result); got != "common.Ubuntu" {
		t.Errorf("single source: .bashrc from %s, want common.Ubuntu", got)
	}

	var applied []string
	for _, m := range result.MatchedDirs {
		applied = append(applied, filepath.Base(m.Path))
	}
	want := []string{"common", "common.Unix", "common.Linux", "common.Linux.arm64", "common.Debian",
		"common.Debian.arm64", "common.Ubuntu"}
	if !reflect.DeepEqual(applied, want) {
		t.Errorf("order of application:\n got %v\nwant %v", applied, want)
	}

	layerDir := t.TempDir()
	plant(t, layerDir, dirs...)
	result, err = Build(BuildConfig{
		Sources:  []LayerSource{{Layer: "personal", Order: 2, SourceRoot: layerDir, TargetRoot: t.TempDir()}},
		Projects: []string{"common"},
		Segments: ubuntuArm64(),
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got := winningDir(t, result); got != "common.Ubuntu" {
		t.Errorf("multi-source: .bashrc from %s, want common.Ubuntu", got)
	}
}

// TestBuild_DarwinBeatsUnix: the chain ranks Darwin above Unix; the suffix count tied them, and Unix won.
func TestBuild_DarwinBeatsUnix(t *testing.T) {

	root := t.TempDir()
	plant(t, root, "common.Unix", "common.Darwin")
	darwin := segment.Segments{{Name: "OS", Value: "Darwin"}, {Name: "ARCH", Value: "arm64"}}

	result, err := Build(BuildConfig{SourceRoot: root, TargetRoot: t.TempDir(), Projects: []string{"common"},
		Segments: darwin})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got := winningDir(t, result); got != "common.Darwin" {
		t.Errorf(".bashrc from %s, want common.Darwin", got)
	}
}

// TestBuild_ProjectsApplyInOrder pins Q27: projects in the order given, then the platform ranking within each.
func TestBuild_ProjectsApplyInOrder(t *testing.T) {

	root := t.TempDir()
	plant(t, root, "common.Ubuntu", "noblefactor", "thenobles")

	result, err := Build(BuildConfig{SourceRoot: root, TargetRoot: t.TempDir(),
		Projects: []string{"common", "noblefactor", "thenobles"}, Segments: ubuntuArm64()})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got := winningDir(t, result); got != "thenobles" {
		t.Errorf(".bashrc from %s, want thenobles, the last project named", got)
	}

	result, err = Build(BuildConfig{SourceRoot: root, TargetRoot: t.TempDir(),
		Projects: []string{"common", "thenobles", "noblefactor"}, Segments: ubuntuArm64()})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got := winningDir(t, result); got != "noblefactor" {
		t.Errorf(".bashrc from %s, want noblefactor, the last project named", got)
	}
}

// TestBuild_TheCollisionSaysWhatDecided: a collision carries both directories' ranks and names the part that put the
// winner after the loser (Requirement 2).
func TestBuild_TheCollisionSaysWhatDecided(t *testing.T) {

	desktop := append(ubuntuArm64(), segment.Segment{Name: "ROLE", Value: "desktop", Values: []string{"desktop"}})

	tests := []struct {
		name     string
		base     []string
		personal []string
		projects []string
		segments segment.Segments
		reason   string
	}{
		{"the OS word", []string{"common.Debian", "common.Ubuntu"}, nil, []string{"common"}, ubuntuArm64(),
			"a more specific OS word"},
		{"the architecture", []string{"common.Debian", "common.Debian.arm64"}, nil, []string{"common"},
			ubuntuArm64(), "it names the architecture"},
		{"an extra", []string{"common.Debian", "common.Debian.desktop"}, nil, []string{"common"}, desktop,
			"it names the ROLE segment"},
		{"the project", []string{"common.Ubuntu", "noblefactor"}, nil, []string{"common", "noblefactor"},
			ubuntuArm64(), "a later project"},
		{"the layer", []string{"common.Ubuntu"}, []string{"common"}, []string{"common"}, ubuntuArm64(),
			"a later layer"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := t.TempDir()
			plant(t, base, tt.base...)
			sources := []LayerSource{{Layer: "base", Order: 0, SourceRoot: base, TargetRoot: t.TempDir()}}
			if tt.personal != nil {
				personal := t.TempDir()
				plant(t, personal, tt.personal...)
				sources = append(sources, LayerSource{Layer: "personal", Order: 2, SourceRoot: personal,
					TargetRoot: sources[0].TargetRoot})
			}

			result, err := Build(BuildConfig{Sources: sources, Projects: tt.projects, Segments: tt.segments})
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			if len(result.Collisions) != 1 {
				t.Fatalf("got %d collisions, want 1", len(result.Collisions))
			}
			c := result.Collisions[0]
			if c.Reason != tt.reason {
				t.Errorf("Reason = %q, want %q", c.Reason, tt.reason)
			}
			if tt.personal == nil && c.WinnerRank.Compare(c.LoserRank) <= 0 {
				t.Errorf("WinnerRank %+v does not rank after LoserRank %+v", c.WinnerRank, c.LoserRank)
			}
			if summary := result.String(); !strings.Contains(summary, tt.reason) {
				t.Errorf("the summary does not give the reason %q:\n%s", tt.reason, summary)
			}
		})
	}
}

// TestBuild_GrammarErrorsRefuseTheBuild: a malformed directory name in any layer refuses the build before a file is
// read, and every one is listed (Q19).
func TestBuild_GrammarErrorsRefuseTheBuild(t *testing.T) {

	base, personal := t.TempDir(), t.TempDir()
	plant(t, base, "common", "common.Debain")
	plant(t, personal, "common.Linux.Debian", "common.arm64.Debian", "common.Fedora")

	_, err := Build(BuildConfig{
		Sources: []LayerSource{
			{Layer: "base", Order: 0, SourceRoot: base, TargetRoot: t.TempDir()},
			{Layer: "personal", Order: 2, SourceRoot: personal, TargetRoot: t.TempDir()},
		},
		Projects: []string{"common"},
		Segments: ubuntuArm64(),
	})

	var refusal *segment.Refusal
	if !errors.As(err, &refusal) {
		t.Fatalf("Build: %v, want a *segment.Refusal", err)
	}
	var names []string
	for _, entry := range refusal.Entries {
		names = append(names, entry.Err.Name)
	}
	want := []string{"common.Debain", "common.Linux.Debian", "common.arm64.Debian"}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("refused %v, want %v; common.Fedora is another machine's, and silent", names, want)
	}
}
