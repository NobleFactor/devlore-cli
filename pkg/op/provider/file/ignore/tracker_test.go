// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package ignore

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// =============================================================================
// Test: NewTracker
// =============================================================================

func TestNewTracker(t *testing.T) {
	t.Run("creates tracker from temp dir", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, root, ".gitignore", "*.log\nbuild/\n")

		tracker, err := NewTracker(root)
		if err != nil {
			t.Fatalf("NewTracker() error = %v", err)
		}

		absRoot, _ := filepath.Abs(root)
		if tracker.Root() != absRoot {
			t.Errorf("Root() = %q, want %q", tracker.Root(), absRoot)
		}
	})

	t.Run("works without gitignore", func(t *testing.T) {
		root := t.TempDir()

		tracker, err := NewTracker(root)
		if err != nil {
			t.Fatalf("NewTracker() error = %v", err)
		}

		// Nothing should be ignored
		ignored, _ := tracker.IsIgnored("anyfile.txt", false)
		if ignored {
			t.Error("expected anyfile.txt to not be ignored with no .gitignore")
		}
	})
}

// =============================================================================
// Test: IsIgnored
// =============================================================================

func TestIsIgnored(t *testing.T) {
	t.Run("basic patterns", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, root, ".gitignore", "*.log\nbuild/\n")

		tracker, err := NewTracker(root)
		if err != nil {
			t.Fatalf("NewTracker() error = %v", err)
		}

		tests := []struct {
			path    string
			isDir   bool
			ignored bool
		}{
			{"debug.log", false, true},
			{"app.log", false, true},
			{"main.go", false, false},
			{"build", true, true},
			{"src", true, false},
		}

		for _, tt := range tests {
			ignored, _ := tracker.IsIgnored(tt.path, tt.isDir)
			if ignored != tt.ignored {
				t.Errorf("IsIgnored(%q, %v) = %v, want %v", tt.path, tt.isDir, ignored, tt.ignored)
			}
		}
	})

	t.Run("negation patterns", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, root, ".gitignore", "*.log\n!important.log\n")

		tracker, err := NewTracker(root)
		if err != nil {
			t.Fatalf("NewTracker() error = %v", err)
		}

		ignored, _ := tracker.IsIgnored("debug.log", false)
		if !ignored {
			t.Error("expected debug.log to be ignored")
		}

		ignored, _ = tracker.IsIgnored("important.log", false)
		if ignored {
			t.Error("expected important.log to NOT be ignored (negation)")
		}
	})
}

// =============================================================================
// Test: Nested Gitignore
// =============================================================================

func TestNestedGitignore(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, ".gitignore", "*.log\n")
	mkdirAll(t, root, "src")
	writeFile(t, root, "src/.gitignore", "!debug.log\n")

	tracker, err := NewTracker(root)
	if err != nil {
		t.Fatalf("NewTracker() error = %v", err)
	}

	// Root level: *.log should be ignored
	ignored, _ := tracker.IsIgnored("app.log", false)
	if !ignored {
		t.Error("expected app.log at root to be ignored")
	}

	// Push src directory to load its .gitignore
	tracker.Push("src")

	// Inside src: debug.log should NOT be ignored (negation in src/.gitignore)
	ignored, _ = tracker.IsIgnored("src/debug.log", false)
	if ignored {
		t.Error("expected src/debug.log to NOT be ignored (subdirectory negation)")
	}

	// Inside src: other .log files should still be ignored (from root .gitignore)
	ignored, _ = tracker.IsIgnored("src/other.log", false)
	if !ignored {
		t.Error("expected src/other.log to be ignored (root pattern still applies)")
	}
}

// =============================================================================
// Test: WalkTree
// =============================================================================

func TestWalkTree(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, ".gitignore", "*.log\nvendor/\n")
	writeFile(t, root, "main.go", "package main")
	writeFile(t, root, "debug.log", "some log")
	mkdirAll(t, root, "src")
	writeFile(t, root, "src/app.go", "package src")
	mkdirAll(t, root, "vendor")
	writeFile(t, root, "vendor/lib.go", "package vendor")

	tracker, err := NewTracker(root)
	if err != nil {
		t.Fatalf("NewTracker() error = %v", err)
	}

	var walked []string
	err = WalkTree(WalkOptions{
		Root:    root,
		Tracker: tracker,
		Callback: func(path string, isDir bool) error {
			walked = append(walked, path)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("WalkTree() error = %v", err)
	}

	sort.Strings(walked)

	// Should include: main.go, src, src/app.go
	// Should NOT include: debug.log, vendor, vendor/lib.go
	assertContains(t, walked, "main.go")
	assertContains(t, walked, "src")
	assertContains(t, walked, filepath.Join("src", "app.go"))
	assertNotContains(t, walked, "debug.log")
	assertNotContains(t, walked, "vendor")
	assertNotContains(t, walked, filepath.Join("vendor", "lib.go"))
}

func TestWalkTreeDirsAndFiles(t *testing.T) {
	root := t.TempDir()
	mkdirAll(t, root, "a")
	writeFile(t, root, "a/file.txt", "hello")
	mkdirAll(t, root, "b")
	writeFile(t, root, "b/file.txt", "world")

	var dirs, files []string
	err := WalkTree(WalkOptions{
		Root: root,
		Callback: func(path string, isDir bool) error {
			if isDir {
				dirs = append(dirs, path)
			} else {
				files = append(files, path)
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("WalkTree() error = %v", err)
	}

	sort.Strings(dirs)
	sort.Strings(files)

	if len(dirs) != 2 {
		t.Errorf("expected 2 directories, got %d: %v", len(dirs), dirs)
	}
	if len(files) != 2 {
		t.Errorf("expected 2 files, got %d: %v", len(files), files)
	}

	assertContains(t, dirs, "a")
	assertContains(t, dirs, "b")
	assertContains(t, files, filepath.Join("a", "file.txt"))
	assertContains(t, files, filepath.Join("b", "file.txt"))
}

func TestWalkTreeSkipStop(t *testing.T) {
	root := t.TempDir()
	mkdirAll(t, root, "a")
	writeFile(t, root, "a/file.txt", "a")
	mkdirAll(t, root, "b")
	writeFile(t, root, "b/file.txt", "b")
	mkdirAll(t, root, "c")
	writeFile(t, root, "c/file.txt", "c")

	t.Run("skip directory", func(t *testing.T) {
		var walked []string
		err := WalkTree(WalkOptions{
			Root: root,
			Callback: func(path string, isDir bool) error {
				walked = append(walked, path)
				if isDir && path == "b" {
					return filepath.SkipDir
				}
				return nil
			},
		})
		if err != nil {
			t.Fatalf("WalkTree() error = %v", err)
		}

		// "b" is yielded (but then skipped), "b/file.txt" is NOT yielded
		assertContains(t, walked, "b")
		assertNotContains(t, walked, filepath.Join("b", "file.txt"))
		assertContains(t, walked, filepath.Join("a", "file.txt"))
		assertContains(t, walked, filepath.Join("c", "file.txt"))
	})

	t.Run("stop walk", func(t *testing.T) {
		var walked []string
		err := WalkTree(WalkOptions{
			Root: root,
			Callback: func(path string, isDir bool) error {
				walked = append(walked, path)
				if isDir && path == "b" {
					return ErrWalkStopped
				}
				return nil
			},
		})
		if err != nil {
			t.Fatalf("WalkTree() error = %v (should be nil for ErrWalkStopped)", err)
		}

		// Walk stopped at "b", so "c" and beyond should not be present
		assertContains(t, walked, "a")
		assertContains(t, walked, "b")
		assertNotContains(t, walked, "c")
	})
}

func TestWalkTreeNestedGitignore(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, ".gitignore", "*.tmp\n")
	mkdirAll(t, root, "sub")
	writeFile(t, root, "sub/.gitignore", "*.bak\n")
	writeFile(t, root, "sub/keep.go", "package sub")
	writeFile(t, root, "sub/remove.tmp", "temp")
	writeFile(t, root, "sub/remove.bak", "backup")
	writeFile(t, root, "keep.go", "package main")
	writeFile(t, root, "remove.tmp", "temp")

	tracker, err := NewTracker(root)
	if err != nil {
		t.Fatalf("NewTracker() error = %v", err)
	}

	var walked []string
	err = WalkTree(WalkOptions{
		Root:    root,
		Tracker: tracker,
		Callback: func(path string, isDir bool) error {
			if !isDir {
				walked = append(walked, path)
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("WalkTree() error = %v", err)
	}

	sort.Strings(walked)

	assertContains(t, walked, "keep.go")
	assertContains(t, walked, filepath.Join("sub", "keep.go"))
	assertNotContains(t, walked, "remove.tmp")
	assertNotContains(t, walked, filepath.Join("sub", "remove.tmp"))
	assertNotContains(t, walked, filepath.Join("sub", "remove.bak"))
}

func TestWalkTreeSkipsGitDir(t *testing.T) {
	root := t.TempDir()
	mkdirAll(t, root, ".git/objects")
	writeFile(t, root, ".git/HEAD", "ref: refs/heads/main")
	writeFile(t, root, "file.go", "package main")

	var walked []string
	err := WalkTree(WalkOptions{
		Root: root,
		Callback: func(path string, isDir bool) error {
			walked = append(walked, path)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("WalkTree() error = %v", err)
	}

	assertContains(t, walked, "file.go")
	assertNotContains(t, walked, ".git")
	assertNotContains(t, walked, filepath.Join(".git", "HEAD"))
}

// =============================================================================
// Test: Push auto-pops siblings
// =============================================================================

func TestPushAutoPop(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, ".gitignore", "*.log\n")
	mkdirAll(t, root, "a")
	writeFile(t, root, "a/.gitignore", "*.tmp\n")
	mkdirAll(t, root, "b")
	writeFile(t, root, "b/.gitignore", "*.bak\n")

	tracker, err := NewTracker(root)
	if err != nil {
		t.Fatalf("NewTracker() error = %v", err)
	}

	// Root: *.log is ignored
	ignored, _ := tracker.IsIgnored("test.log", false)
	if !ignored {
		t.Error("expected test.log to be ignored at root")
	}

	// Push a: *.tmp should now also be ignored
	tracker.Push("a")
	ignored, _ = tracker.IsIgnored("a/test.tmp", false)
	if !ignored {
		t.Error("expected a/test.tmp to be ignored after Push(a)")
	}

	// Push b: auto-pops a, *.tmp should no longer be ignored in b
	tracker.Push("b")
	ignored, _ = tracker.IsIgnored("b/test.bak", false)
	if !ignored {
		t.Error("expected b/test.bak to be ignored after Push(b)")
	}
}

// =============================================================================
// Helpers
// =============================================================================

func writeFile(t *testing.T, root, relPath, content string) {
	t.Helper()
	path := filepath.Join(root, relPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mkdirAll(t *testing.T, root, relPath string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, relPath), 0o755); err != nil {
		t.Fatal(err)
	}
}

func assertContains(t *testing.T, items []string, want string) {
	t.Helper()
	for _, item := range items {
		if item == want {
			return
		}
	}
	t.Errorf("expected %v to contain %q", items, want)
}

func assertNotContains(t *testing.T, items []string, notWant string) {
	t.Helper()
	for _, item := range items {
		if item == notWant {
			t.Errorf("expected %v to NOT contain %q", items, notWant)
			return
		}
	}
}
