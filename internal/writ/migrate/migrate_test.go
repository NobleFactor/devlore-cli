// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package migrate

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const fixtureDir = "testdata/fixture"

func fixtureRoot(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs(fixtureDir)
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

func TestDetect(t *testing.T) {
	root := fixtureRoot(t)
	system, err := Detect(root)
	if err != nil {
		t.Fatal(err)
	}
	if system != SystemScriptBased {
		t.Errorf("Detect() = %q, want %q", system, SystemScriptBased)
	}
}

func TestDetectTuckr(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Hooks.toml"), []byte("[hooks]"), 0644); err != nil {
		t.Fatal(err)
	}
	system, err := Detect(dir)
	if err != nil {
		t.Fatal(err)
	}
	if system != SystemTuckr {
		t.Errorf("Detect() = %q, want %q", system, SystemTuckr)
	}
}

func TestDetectStow(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".stow-local-ignore"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	system, err := Detect(dir)
	if err != nil {
		t.Fatal(err)
	}
	if system != SystemStow {
		t.Errorf("Detect() = %q, want %q", system, SystemStow)
	}
}

func TestDetectChezmoi(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "dot_config"), 0755); err != nil {
		t.Fatal(err)
	}
	system, err := Detect(dir)
	if err != nil {
		t.Fatal(err)
	}
	if system != SystemChezmoi {
		t.Errorf("Detect() = %q, want %q", system, SystemChezmoi)
	}
}

func TestDetectYadm(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".bashrc##os.Linux"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	system, err := Detect(dir)
	if err != nil {
		t.Fatal(err)
	}
	if system != SystemYadm {
		t.Errorf("Detect() = %q, want %q", system, SystemYadm)
	}
}

func TestDetectBareGit(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "HEAD"), []byte("ref: refs/heads/main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "objects"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "refs"), 0755); err != nil {
		t.Fatal(err)
	}
	system, err := Detect(dir)
	if err != nil {
		t.Fatal(err)
	}
	if system != SystemBareGit {
		t.Errorf("Detect() = %q, want %q", system, SystemBareGit)
	}
}

func TestDetectNative(t *testing.T) {
	dir := t.TempDir()
	// Create Home/<project> structure
	if err := os.MkdirAll(filepath.Join(dir, "Home", "all"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Home", "all", ".bashrc"), []byte("# bashrc"), 0644); err != nil {
		t.Fatal(err)
	}
	system, err := Detect(dir)
	if err != nil {
		t.Fatal(err)
	}
	if system != SystemNative {
		t.Errorf("Detect() = %q, want %q", system, SystemNative)
	}
}

func TestDetectNativeSystem(t *testing.T) {
	dir := t.TempDir()
	// Create System/<project> structure (no Home)
	if err := os.MkdirAll(filepath.Join(dir, "System", "base", "etc"), 0755); err != nil {
		t.Fatal(err)
	}
	system, err := Detect(dir)
	if err != nil {
		t.Fatal(err)
	}
	if system != SystemNative {
		t.Errorf("Detect() = %q, want %q", system, SystemNative)
	}
}

func TestInventory(t *testing.T) {
	root := fixtureRoot(t)
	entries, err := Inventory(root)
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) == 0 {
		t.Fatal("Inventory returned no entries")
	}

	// Verify project/platform parsing
	projectPlatformCases := map[string]struct{ project, platform string }{
		"all-Darwin":        {"all", "Darwin"},
		"all-Unix":          {"all", "Unix"},
		"all-Linux":         {"all", "Linux"},
		"noblefactor-Unix":  {"noblefactor", "Unix"},
		"microsoft-Windows": {"microsoft", "Windows"},
	}

	for _, e := range entries {
		// Extract the top-level directory from the relative path
		topDir := strings.SplitN(e.RelPath, string(filepath.Separator), 2)[0]
		if expected, ok := projectPlatformCases[topDir]; ok {
			if e.Project != expected.project {
				t.Errorf("entry %s: project = %q, want %q", e.RelPath, e.Project, expected.project)
			}
			if e.Platform != expected.platform {
				t.Errorf("entry %s: platform = %q, want %q", e.RelPath, e.Platform, expected.platform)
			}
			delete(projectPlatformCases, topDir) // only check once per dir
		}
	}

	// Verify base projects (no platform)
	for _, e := range entries {
		if strings.HasPrefix(e.RelPath, "all/") || strings.HasPrefix(e.RelPath, "noblefactor/") {
			if e.Platform != "" {
				t.Errorf("entry %s: platform = %q, want empty", e.RelPath, e.Platform)
			}
		}
	}
}

func TestClassify(t *testing.T) {
	root := fixtureRoot(t)
	entries, err := Inventory(root)
	if err != nil {
		t.Fatal(err)
	}
	Classify(entries)

	expectations := map[string]FileClass{
		"all/.config/git/config.all":                             ClassStaticConfig,
		"all-Darwin/.zprofile":                                   ClassStaticConfig,
		"all-Darwin/Library/Fonts/SFMono-Regular.otf":            ClassFont,
		"all-Unix/.bashrc":                                       ClassStaticConfig,
		"all-Unix/.Personal-secrets/gnupg/key.txt":               ClassSecret,
		"noblefactor/.ssh/config":                                ClassStaticConfig,
		"microsoft-Windows/local/bin/Initialize-SshIdentity.ps1": ClassStaticConfig,
	}

	// Scripts (require executable bit)
	execExpectations := map[string]FileClass{
		"all/local/bin/git-new-workspace":                  ClassScript,
		"all-Darwin/local/bin/Install-Tuckr":               ClassLifecycleScript,
		"all-Unix/local/bin/New-SshAuthenticationKey":      ClassScript,
		"all-Linux/local/bin/Install-Rust":                 ClassLifecycleScript,
		"all-Linux/local/bin/Install-Tuckr":                ClassLifecycleScript,
		"noblefactor-Unix/.local/bin/Install-Docker":       ClassLifecycleScript,
		"noblefactor-Unix/.local/bin/Install-Dependencies": ClassLifecycleScript,
	}

	for relPath, expected := range expectations {
		found := false
		for _, e := range entries {
			if e.RelPath == relPath {
				found = true
				if e.Class != expected {
					t.Errorf("classify %s: got %q, want %q", relPath, e.Class, expected)
				}
				break
			}
		}
		if !found {
			t.Errorf("classify: entry %s not found in inventory", relPath)
		}
	}

	for relPath, expected := range execExpectations {
		found := false
		for _, e := range entries {
			if e.RelPath == relPath {
				found = true
				if !e.IsExecutable {
					t.Skipf("classify %s: not executable (test environment may not preserve permissions)", relPath)
				}
				if e.Class != expected {
					t.Errorf("classify %s: got %q, want %q", relPath, e.Class, expected)
				}
				break
			}
		}
		if !found {
			t.Errorf("classify: entry %s not found in inventory", relPath)
		}
	}
}

func TestMapping(t *testing.T) {
	root := fixtureRoot(t)
	mappings, err := BuildMappings(root)
	if err != nil {
		t.Fatal(err)
	}

	expected := map[string]string{
		"all-Darwin":        "all.Darwin",
		"all-Linux":         "all.Linux",
		"all-Unix":          "all.Unix",
		"microsoft-Windows": "microsoft.Windows",
		"noblefactor-Unix":  "noblefactor.Unix",
	}

	if len(mappings) != len(expected) {
		t.Errorf("BuildMappings: got %d mappings, want %d", len(mappings), len(expected))
	}

	for _, m := range mappings {
		want, ok := expected[m.SourceDir]
		if !ok {
			t.Errorf("unexpected mapping: %s → %s", m.SourceDir, m.TargetDir)
			continue
		}
		if m.TargetDir != want {
			t.Errorf("mapping %s: got %q, want %q", m.SourceDir, m.TargetDir, want)
		}
	}
}

func TestScriptAnalysis(t *testing.T) {
	root := fixtureRoot(t)
	entries, err := Inventory(root)
	if err != nil {
		t.Fatal(err)
	}
	Classify(entries)
	scripts := AnalyzeScripts(entries)

	// Find Install-Tuckr analysis (the Darwin one)
	var tuckrAnalysis *ScriptAnalysis
	for i, s := range scripts {
		if s.Name == "Install-Tuckr" && strings.Contains(s.RelPath, "Darwin") {
			tuckrAnalysis = &scripts[i]
			break
		}
	}

	if tuckrAnalysis == nil {
		// Might not have executable bit in test env
		t.Skip("Install-Tuckr not found in script analyses (may lack executable bit)")
	}

	if tuckrAnalysis.Phase != "install" {
		t.Errorf("Install-Tuckr phase: got %q, want %q", tuckrAnalysis.Phase, "install")
	}
	if tuckrAnalysis.PackageManager != "cargo" {
		t.Errorf("Install-Tuckr manager: got %q, want %q", tuckrAnalysis.PackageManager, "cargo")
	}
	if len(tuckrAnalysis.PackageNames) != 1 || tuckrAnalysis.PackageNames[0] != "tuckr" {
		t.Errorf("Install-Tuckr packages: got %v, want [tuckr]", tuckrAnalysis.PackageNames)
	}
}

func TestPlanGeneration(t *testing.T) {
	root := fixtureRoot(t)
	opts := Options{
		SourceRoot: root,
		Format:     "text",
	}

	plan, err := BuildPlan(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}

	if plan.System != SystemScriptBased {
		t.Errorf("plan.System = %q, want %q", plan.System, SystemScriptBased)
	}
	if plan.Stats.TotalFiles == 0 {
		t.Error("plan.Stats.TotalFiles = 0, want > 0")
	}
	if plan.Stats.Renames != 5 {
		t.Errorf("plan.Stats.Renames = %d, want 5", plan.Stats.Renames)
	}
	if plan.Stats.Projects < 3 {
		t.Errorf("plan.Stats.Projects = %d, want >= 3", plan.Stats.Projects)
	}
}

func TestExecution(t *testing.T) {
	// Copy fixture to a temp directory for destructive testing
	tmpDir := t.TempDir()
	copyDir(t, fixtureRoot(t), tmpDir)

	opts := Options{
		SourceRoot: tmpDir,
	}
	plan, err := BuildPlan(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := Execute(&buf, plan); err != nil {
		t.Fatalf("Execute failed: %v\nOutput: %s", err, buf.String())
	}

	// Verify renames happened
	for _, m := range plan.Mappings {
		srcPath := filepath.Join(tmpDir, m.SourceDir)
		dstPath := filepath.Join(tmpDir, m.TargetDir)

		if exists(srcPath) {
			t.Errorf("source dir still exists after rename: %s", m.SourceDir)
		}
		if !exists(dstPath) {
			t.Errorf("target dir does not exist after rename: %s", m.TargetDir)
		}
	}

	// Verify files preserved
	if !exists(filepath.Join(tmpDir, "all.Darwin", "Library", "Fonts", "SFMono-Regular.otf")) {
		t.Error("font file not preserved after rename")
	}

	// Verify marker written
	markerPath := filepath.Join(tmpDir, ".writ-migrated")
	if !exists(markerPath) {
		t.Fatal(".writ-migrated marker not written")
	}

	data, err := os.ReadFile(markerPath)
	if err != nil {
		t.Fatal(err)
	}
	var marker MigratedMarker
	if err := yaml.Unmarshal(data, &marker); err != nil {
		t.Fatalf("marker YAML parse: %v", err)
	}
	if marker.System != SystemScriptBased {
		t.Errorf("marker.System = %q, want %q", marker.System, SystemScriptBased)
	}
	if len(marker.Mappings) != 5 {
		t.Errorf("marker.Mappings has %d entries, want 5", len(marker.Mappings))
	}
}

func TestExecutionConflict(t *testing.T) {
	tmpDir := t.TempDir()
	copyDir(t, fixtureRoot(t), tmpDir)

	// Create a conflicting target directory
	if err := os.Mkdir(filepath.Join(tmpDir, "all.Darwin"), 0755); err != nil {
		t.Fatal(err)
	}

	opts := Options{SourceRoot: tmpDir}
	plan, err := BuildPlan(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	err = Execute(&buf, plan)
	if err == nil {
		t.Fatal("Execute should fail when target exists")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error = %q, want to contain 'already exists'", err.Error())
	}
}

func TestAlreadyMigrated(t *testing.T) {
	tmpDir := t.TempDir()
	copyDir(t, fixtureRoot(t), tmpDir)

	// Write a marker to simulate prior migration
	if err := os.WriteFile(filepath.Join(tmpDir, ".writ-migrated"), []byte("timestamp: now\n"), 0644); err != nil {
		t.Fatal(err)
	}

	opts := Options{SourceRoot: tmpDir}
	_, err := BuildPlan(context.Background(), opts)
	if err == nil {
		t.Fatal("BuildPlan should fail when already migrated")
	}
	if !strings.Contains(err.Error(), "already migrated") {
		t.Errorf("error = %q, want to contain 'already migrated'", err.Error())
	}
}

func TestFormatText(t *testing.T) {
	root := fixtureRoot(t)
	opts := Options{SourceRoot: root}
	plan, err := BuildPlan(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := FormatPlan(&buf, plan, "text"); err != nil {
		t.Fatal(err)
	}

	output := buf.String()

	requiredSections := []string{
		"Migration Plan",
		"Source:",
		"System: script-based",
		"Summary:",
		"Directory renames",
		"TODOs after migration",
	}
	for _, section := range requiredSections {
		if !strings.Contains(output, section) {
			t.Errorf("text output missing section %q", section)
		}
	}

	// Verify mapping display
	if !strings.Contains(output, "all-Darwin") || !strings.Contains(output, "all.Darwin") {
		t.Error("text output missing mapping display")
	}
}

func TestFormatYAML(t *testing.T) {
	root := fixtureRoot(t)
	opts := Options{SourceRoot: root}
	plan, err := BuildPlan(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := FormatPlan(&buf, plan, "yaml"); err != nil {
		t.Fatal(err)
	}

	// Verify round-trip
	var parsed MigrationPlan
	if err := yaml.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("YAML round-trip failed: %v", err)
	}
	if parsed.System != plan.System {
		t.Errorf("YAML round-trip: system = %q, want %q", parsed.System, plan.System)
	}
	if parsed.Stats.TotalFiles != plan.Stats.TotalFiles {
		t.Errorf("YAML round-trip: total_files = %d, want %d", parsed.Stats.TotalFiles, plan.Stats.TotalFiles)
	}
}

func TestFormatJSON(t *testing.T) {
	root := fixtureRoot(t)
	opts := Options{SourceRoot: root}
	plan, err := BuildPlan(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := FormatPlan(&buf, plan, "json"); err != nil {
		t.Fatal(err)
	}

	// Verify it's valid JSON
	var parsed MigrationPlan
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("JSON round-trip failed: %v", err)
	}
	if parsed.System != plan.System {
		t.Errorf("JSON round-trip: system = %q, want %q", parsed.System, plan.System)
	}
}

func TestParseProjectPlatform(t *testing.T) {
	cases := []struct {
		name     string
		project  string
		platform string
	}{
		{"all", "all", ""},
		{"all-Darwin", "all", "Darwin"},
		{"all-Linux", "all", "Linux"},
		{"all-Unix", "all", "Unix"},
		{"all-Windows", "all", "Windows"},
		{"all-Debian", "all", "Debian"},
		{"noblefactor", "noblefactor", ""},
		{"noblefactor-Unix", "noblefactor", "Unix"},
		{"microsoft-Windows", "microsoft", "Windows"},
		{"thenobles-Darwin", "thenobles", "Darwin"},
		{"my-project-Darwin", "my-project", "Darwin"},
		{"no-match-here", "no-match-here", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			project, platform := parseProjectPlatform(tc.name)
			if project != tc.project {
				t.Errorf("parseProjectPlatform(%q): project = %q, want %q", tc.name, project, tc.project)
			}
			if platform != tc.platform {
				t.Errorf("parseProjectPlatform(%q): platform = %q, want %q", tc.name, platform, tc.platform)
			}
		})
	}
}

// copyDir recursively copies src to dst.
func copyDir(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relPath, _ := filepath.Rel(src, path)
		targetPath := filepath.Join(dst, relPath)

		if d.IsDir() {
			return os.MkdirAll(targetPath, 0755)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		return os.WriteFile(targetPath, data, info.Mode())
	})
	if err != nil {
		t.Fatalf("copyDir %s → %s: %v", src, dst, err)
	}
}
