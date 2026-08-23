// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package file

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
)

// The plan-space little language (#584, ruled 2026-08-20): `foo/bar` ≡ `/foo/bar`; volume, UNC, and
// backslash spellings refuse; `@name/…` is reserved for the named multi-root design; escapes and the bare
// root refuse. One table, every rule.

func TestNormalizePlanSpacePath_TheLittleLanguage(t *testing.T) {

	accepted := []struct {
		name string
		in   string
		want string
	}{
		{"bare rel", "foo/bar", "foo/bar"},
		{"anchored spelling names the same rel", "/foo/bar", "foo/bar"},
		{"dot segments collapse", "a/./b/../c.txt", "a/c.txt"},
		{"duplicate slashes collapse", "a//b", "a/b"},
		{"colon beyond the drive position rides as a path", "https://example.com/x", "https:/example.com/x"},
	}

	for _, tc := range accepted {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizePlanSpacePath(tc.in)
			if err != nil {
				t.Fatalf("NormalizePlanSpacePath(%q): unexpected refusal: %v", tc.in, err)
			}
			if got != tc.want {
				t.Fatalf("NormalizePlanSpacePath(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}

	refused := []struct {
		name   string
		in     string
		reason string
	}{
		{"drive letter, backslash form", `C:\x`, "volume"},
		{"drive letter, slash form", "C:/x", "volume"},
		{"drive-relative", "C:x", "volume"},
		{"UNC, backslash form", `\\server\share`, "UNC"},
		{"UNC, slash form", "//server/share", "UNC"},
		{"bare backslash separator", `foo\bar`, "backslash"},
		{"root-qualified is reserved", "@config/foo", "root-qualified"},
		{"escape", "../secrets", "escapes"},
		{"escape after cleaning", "a/../../secrets", "escapes"},
		{"the root itself", "/", "root"},
		{"empty", "", "root"},
		{"dot", ".", "root"},
	}

	for _, tc := range refused {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NormalizePlanSpacePath(tc.in)
			if err == nil {
				t.Fatalf("NormalizePlanSpacePath(%q): expected refusal, got acceptance", tc.in)
			}
			if !strings.Contains(err.Error(), tc.reason) {
				t.Fatalf("NormalizePlanSpacePath(%q): refusal %q does not name %q", tc.in, err, tc.reason)
			}
		})
	}
}

// The runtime dialect (phase 4 PR 3/#611, ruled 2026-08-22): rels and the plan-space refusals as
// authored; a machine-absolute under the bound root rebases to its rel; outside the root — other
// volumes and UNC spellings included — refuses as confinement. On unix a leading slash reads as
// machine-absolute (the dialect sharpening): the two readings agree under the root, and an
// out-of-root absolute refuses rather than silently confining.

func TestNormalizeRuntimePath_TheRuntimeDialect(t *testing.T) {

	rootDir := t.TempDir()
	root, err := fsroot.OpenConfined(rootDir)
	if err != nil {
		t.Fatalf("fsroot.OpenConfined: %v", err)
	}
	t.Cleanup(func() { _ = root.Close() })

	t.Run("bare rel passes as authored", func(t *testing.T) {
		got, err := NormalizeRuntimePath(root, "foo/bar")
		if err != nil || got != "foo/bar" {
			t.Fatalf("= %q, %v; want foo/bar, nil", got, err)
		}
	})

	t.Run("machine-absolute under the root rebases to its rel", func(t *testing.T) {
		got, err := NormalizeRuntimePath(root, filepath.Join(rootDir, "sub", "report.txt"))
		if err != nil || got != "sub/report.txt" {
			t.Fatalf("= %q, %v; want sub/report.txt, nil", got, err)
		}
	})

	t.Run("machine-absolute outside the root refuses as confinement", func(t *testing.T) {
		_, err := NormalizeRuntimePath(root, filepath.Dir(rootDir))
		if err == nil || !strings.Contains(err.Error(), "outside the run's root") {
			t.Fatalf("err = %v, want the confinement refusal", err)
		}
	})

	t.Run("an escape refuses as authored", func(t *testing.T) {
		_, err := NormalizeRuntimePath(root, "../outside.txt")
		if err == nil || !strings.Contains(err.Error(), "escapes") {
			t.Fatalf("err = %v, want the escape refusal", err)
		}
	})

	t.Run("the reserved root-qualified spelling refuses as authored", func(t *testing.T) {
		_, err := NormalizeRuntimePath(root, "@name/x")
		if err == nil || !strings.Contains(err.Error(), "root-qualified") {
			t.Fatalf("err = %v, want the reservation refusal", err)
		}
	})

	t.Run("a foreign volume refuses on every platform", func(t *testing.T) {
		_, err := NormalizeRuntimePath(root, `Q:\x`)
		if err == nil {
			t.Fatal("want a refusal for a foreign volume spelling")
		}
	})

	t.Run("a UNC spelling refuses on every platform", func(t *testing.T) {
		_, err := NormalizeRuntimePath(root, `\\server\share`)
		if err == nil {
			t.Fatal("want a refusal for a UNC spelling")
		}
	})

	t.Run("a leading slash reads per platform", func(t *testing.T) {
		got, err := NormalizeRuntimePath(root, "/definitely/outside")
		if runtime.GOOS == "windows" {
			// Not machine-absolute on Windows (no volume): the plan-space anchored reading holds.
			if err != nil || got != "definitely/outside" {
				t.Fatalf("= %q, %v; want the anchored rel on windows", got, err)
			}
			return
		}
		// Machine-absolute on unix: the machine reading wins, and this one is outside the root.
		if err == nil || !strings.Contains(err.Error(), "outside the run's root") {
			t.Fatalf("err = %v, want the confinement refusal on unix", err)
		}
	})
}
