// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package gitignore

import (
	"os"
	"path/filepath"
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
	if err := tracker.Push("src"); err != nil {
		t.Fatalf("Push(src): %v", err)
	}

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
	if err := tracker.Push("a"); err != nil {
		t.Fatalf("Push(a): %v", err)
	}
	ignored, _ = tracker.IsIgnored("a/test.tmp", false)
	if !ignored {
		t.Error("expected a/test.tmp to be ignored after Push(a)")
	}

	// Push b: auto-pops a, *.tmp should no longer be ignored in b
	if err := tracker.Push("b"); err != nil {
		t.Fatalf("Push(b): %v", err)
	}
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
