// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package file

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// Name and Parent project a slash-form surface, but Join returns OS-native paths. On Windows they
// are therefore routinely handed backslashes, and slashpath.Base finds no separator in one — it
// answered the whole path. file.name(file.join(a, b)) returned the entire path rather than the last
// element, which made the knowledge indexer treat every directory as unrecognized there while
// passing on Unix.
//
// The conversion is filepath.ToSlash rather than a blind ReplaceAll, and that distinction is the
// point: a backslash is a legal character in a Unix filename, so it must not be read as a separator
// off Windows. The platform-specific case below is guarded for the same reason.
func TestName_AcceptsNativePaths(t *testing.T) {
	p := &Provider{}

	cases := []struct{ name, in, want string }{
		{"slash form", "knowledge/packages/slots", "slots"},
		{"joined for this platform", filepath.Join("knowledge", "packages", "slots"), "slots"},
		{"bare element", "slots", "slots"},
	}

	if runtime.GOOS == "windows" {
		cases = append(cases, struct{ name, in, want string }{
			"native windows path", `C:\tmp\reg\knowledge\packages\slots`, "slots",
		})
	} else {
		// Off Windows a backslash is part of the name, not a separator.
		cases = append(cases, struct{ name, in, want string }{
			"backslash is a filename character", `weird\name`, `weird\name`,
		})
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := p.Name(tc.in); got != tc.want {
				t.Errorf("Name(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestParent_AcceptsNativePaths(t *testing.T) {
	p := &Provider{}

	if got, want := p.Parent("knowledge/packages/slots"), "knowledge/packages"; got != want {
		t.Errorf("Parent(slash) = %q, want %q", got, want)
	}

	joined := filepath.Join("knowledge", "packages", "slots")
	if got, want := p.Parent(joined), "knowledge/packages"; got != want {
		t.Errorf("Parent(%q) = %q, want %q", joined, got, want)
	}
}

// TestPathSeam_ProducersFeedConsumers is the test whose absence let the same defect land four times
// in seventeen days (#395, #548, #600, and the indexer failure on #719).
//
// The provider speaks two path dialects. Join, Glob, Resolve, WalkTree and Link are OS-native;
// Name and Parent are slash-form. Every unit test writes its inputs in the dialect of the function
// under test, so each passes in isolation on every platform — and the defect lives only where a
// producer's output reaches a consumer's input. Nothing tested that handoff.
//
// This does. It is deliberately about composition rather than any single function: if a new path
// producer is added in the native dialect, feeding it here is what makes the mismatch fail a build
// rather than surface a fortnight later on a Windows leg.
func TestPathSeam_ProducersFeedConsumers(t *testing.T) {
	tmp := t.TempDir()
	p := testProvider(t, tmp)

	if err := os.MkdirAll(filepath.Join(tmp, "knowledge", "packages", "slots"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "knowledge", "packages", "slots", "homebrew.yaml"), []byte("x: 1\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	t.Run("Name(Join(...)) returns the last element", func(t *testing.T) {
		joined := p.Join("knowledge", "packages", "slots")
		if got := p.Name(joined); got != "slots" {
			t.Errorf("Name(Join(...)) = %q, want %q — the join produced %q", got, "slots", joined)
		}
	})

	t.Run("Parent(Join(...)) drops one element", func(t *testing.T) {
		joined := p.Join("knowledge", "packages", "slots")
		if got := p.Parent(joined); got != "knowledge/packages" {
			t.Errorf("Parent(Join(...)) = %q, want %q", got, "knowledge/packages")
		}
	})

	t.Run("Name(glob result) returns file names, not whole paths", func(t *testing.T) {
		matches, err := p.Glob(p.Join("knowledge", "packages", "slots", "*"), false)
		if err != nil {
			t.Fatalf("glob: %v", err)
		}
		if len(matches) == 0 {
			t.Fatal("glob matched nothing; the fixture is wrong, not the code")
		}
		for _, m := range matches {
			// The Starlark surface reaches a match's path exactly this way.
			path := m.Path().Abs()
			if got := p.Name(path); got != "homebrew.yaml" {
				t.Errorf("Name(%q) = %q, want %q", path, got, "homebrew.yaml")
			}
		}
	})
}
