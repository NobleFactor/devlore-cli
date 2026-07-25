// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starcode

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

func testdataDir(t *testing.T) string {

	t.Helper()

	dir, err := filepath.Abs("testdata")
	if err != nil {
		t.Fatalf("abs testdata: %v", err)
	}

	return dir
}

func TestCaptureAllStar(t *testing.T) {

	root := testdataDir(t)

	provider := &Provider{
		ProviderBase: op.NewProviderBase(&op.RuntimeEnvironment{
			ResourceCatalog: op.NewResourceCatalog(),
			Root:            fsroot.OpenWritableUnconfined(root),
		}),
		Root: root,
	}

	sources, err := provider.Capture("*.star", false)
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}

	if sources.Count() == 0 {
		t.Fatal("expected at least one file")
	}

	for _, path := range sources.Paths() {
		ext := filepath.Ext(path)
		if ext != ".star" {
			t.Errorf("unexpected extension %q in %s", ext, path)
		}
	}
}

func TestCaptureRecursive(t *testing.T) {

	root := testdataDir(t)

	provider := &Provider{
		ProviderBase: op.NewProviderBase(
			&op.RuntimeEnvironment{
				ResourceCatalog: op.NewResourceCatalog(),
				Root:            fsroot.OpenWritableUnconfined(root),
			}),
		Root: root,
	}

	sources, err := provider.Capture("**/*.star", false)
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}

	if sources.Count() == 0 {
		t.Fatal("expected at least one file")
	}
}

func TestCaptureEmptyPattern(t *testing.T) {

	tempDir := t.TempDir()

	provider := &Provider{
		ProviderBase: op.NewProviderBase(
			&op.RuntimeEnvironment{
				Root:            fsroot.OpenWritableUnconfined(tempDir),
				ResourceCatalog: op.NewResourceCatalog(),
			}),
		Root: tempDir,
	}

	sources, err := provider.Capture("*.star", false)
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}

	if sources.Count() != 0 {
		t.Errorf("expected 0 files, got %d", sources.Count())
	}
}

func TestCaptureGitignore(t *testing.T) {

	tempDir := t.TempDir()
	gitDir := filepath.Join(tempDir, ".git")

	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}

	writeFile(t, filepath.Join(tempDir, ".gitignore"), "ignored.star\n")
	writeFile(t, filepath.Join(tempDir, "keep.star"), "x = 1\n")
	writeFile(t, filepath.Join(tempDir, "ignored.star"), "y = 2\n")

	provider := &Provider{
		ProviderBase: op.NewProviderBase(
			&op.RuntimeEnvironment{
				ResourceCatalog: op.NewResourceCatalog(),
				Root:            fsroot.OpenWritableUnconfined(tempDir),
			}),
		Root: tempDir,
	}

	// Excluding git-ignored files (default): ignored.star is filtered out.

	sources, err := provider.Capture("*.star", false)
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}

	if sources.Count() != 1 {
		t.Errorf("expected 1 file (git-ignored excluded), got %d", sources.Count())
	}

	// Including git-ignored files: ignored.star is captured too.

	sources, err = provider.Capture("*.star", true)
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if sources.Count() != 2 {
		t.Errorf("expected 2 files (git ignored included), got %d", sources.Count())
	}
}

func TestSourcesPaths(t *testing.T) {

	root := testdataDir(t)

	provider := &Provider{
		ProviderBase: op.NewProviderBase(&op.RuntimeEnvironment{
			ResourceCatalog: op.NewResourceCatalog(),
			Root:            fsroot.OpenWritableUnconfined(root),
		}),
		Root: root,
	}

	sources, err := provider.Capture("*.star", false)
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}

	paths := sources.Paths()

	if len(paths) != sources.Count() {
		t.Fatalf("Paths length %d != Count %d", len(paths), sources.Count())
	}

	for _, path := range paths {
		if filepath.IsAbs(path) {
			t.Errorf("Paths() should return relative paths, got %s", path)
		}
	}
}

func TestSourcesFilesAreSorted(t *testing.T) {

	root := testdataDir(t)

	provider := &Provider{
		ProviderBase: op.NewProviderBase(&op.RuntimeEnvironment{
			ResourceCatalog: op.NewResourceCatalog(),
			Root:            fsroot.OpenWritableUnconfined(root),
		}),
		Root: root,
	}

	sources, err := provider.Capture("*.star", false)
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}

	for i := 1; i < len(sources.Files); i++ {
		if sources.Files[i] < sources.Files[i-1] {
			t.Errorf("files not sorted: %s before %s", sources.Files[i-1], sources.Files[i])
		}
	}
}

func TestMatchRecursivePatternSuffix(t *testing.T) {

	matched, err := matchRecursivePattern("**/*.star", "sub/test.star")
	if err != nil {
		t.Fatalf("matchRecursivePattern: %v", err)
	}

	if !matched {
		t.Error("expected match for sub/test.star against **/*.star")
	}
}

func TestMatchRecursivePatternNoDoubleStar(t *testing.T) {

	matched, err := matchRecursivePattern("*.star", "test.star")
	if err != nil {
		t.Fatalf("matchRecursivePattern: %v", err)
	}

	if matched {
		t.Error("expected no match for pattern without **")
	}
}

func TestMatchRecursivePatternEmptySuffix(t *testing.T) {

	matched, err := matchRecursivePattern("**", "any/path.star")
	if err != nil {
		t.Fatalf("matchRecursivePattern: %v", err)
	}

	if !matched {
		t.Error("expected match when suffix is empty (matches everything)")
	}
}

func TestMatchRecursivePatternNoMatch(t *testing.T) {

	matched, err := matchRecursivePattern("**/*.text", "sub/test.star")
	if err != nil {
		t.Fatalf("matchRecursivePattern: %v", err)
	}

	if matched {
		t.Error("expected no match for .star file against **/*.text")
	}
}

func writeFile(t *testing.T, path, content string) {

	t.Helper()

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
