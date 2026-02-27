// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starcode

import (
	"os"
	"path/filepath"
	"testing"
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
	p := &Provider{Root: root}

	sources, err := p.Capture("*.star", false, false)
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
	p := &Provider{Root: root}

	sources, err := p.Capture("**/*.star", false, false)
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}

	if sources.Count() == 0 {
		t.Fatal("expected at least one file")
	}
}

func TestCaptureExcludesBzl(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, filepath.Join(tmp, "test.star"), "x = 1\n")
	writeFile(t, filepath.Join(tmp, "build.bzl"), "y = 2\n")

	p := &Provider{Root: tmp}

	// Without include_bzl
	sources, err := p.Capture("*", false, false)
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if sources.Count() != 1 {
		t.Errorf("expected 1 file without bzl, got %d", sources.Count())
	}

	// With include_bzl
	sources, err = p.Capture("*", false, true)
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if sources.Count() != 2 {
		t.Errorf("expected 2 files with bzl, got %d", sources.Count())
	}
}

func TestCaptureEmptyPattern(t *testing.T) {
	tmp := t.TempDir()
	p := &Provider{Root: tmp}

	sources, err := p.Capture("*.star", false, false)
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if sources.Count() != 0 {
		t.Errorf("expected 0 files, got %d", sources.Count())
	}
}

func TestCaptureGitignore(t *testing.T) {
	tmp := t.TempDir()

	gitDir := filepath.Join(tmp, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(tmp, ".gitignore"), "ignored.star\n")
	writeFile(t, filepath.Join(tmp, "keep.star"), "x = 1\n")
	writeFile(t, filepath.Join(tmp, "ignored.star"), "y = 2\n")

	p := &Provider{Root: tmp}

	// With gitignore enabled
	sources, err := p.Capture("*.star", true, false)
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if sources.Count() != 1 {
		t.Errorf("expected 1 file (gitignore active), got %d", sources.Count())
	}

	// Without gitignore
	sources, err = p.Capture("*.star", false, false)
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if sources.Count() != 2 {
		t.Errorf("expected 2 files (gitignore inactive), got %d", sources.Count())
	}
}

func TestSourcesPaths(t *testing.T) {
	root := testdataDir(t)
	p := &Provider{Root: root}

	sources, err := p.Capture("*.star", false, false)
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
	p := &Provider{Root: root}

	sources, err := p.Capture("*.star", false, false)
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
	matched, err := matchRecursivePattern("**/*.bzl", "sub/test.star")
	if err != nil {
		t.Fatalf("matchRecursivePattern: %v", err)
	}
	if matched {
		t.Error("expected no match for .star file against **/*.bzl")
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
