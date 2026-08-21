// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package file

import (
	"path/filepath"
	"strings"
	"testing"
)

// The constructor input domain (paths-only ruling, 2026-08-09): the input is a filesystem path, plus the one
// internal round-trip — the provider's own emitted identity specific ("file://" + path) handed back by
// catalog rehydration. No URI grammar exists on the input side.

func TestDiscoverRegular_PathAndOwnSpecific_SameIdentity(t *testing.T) {

	tmp := t.TempDir()
	p := testProvider(t, tmp)
	path := filepath.Join(tmp, "notes.txt")

	fromPath, err := DiscoverRegular(p.RuntimeEnvironment(), path)
	if err != nil {
		t.Fatalf("DiscoverRegular(path): %v", err)
	}

	fromSpecific, err := DiscoverRegular(p.RuntimeEnvironment(), "file://"+path)
	if err != nil {
		t.Fatalf("DiscoverRegular(own specific): %v", err)
	}

	if fromPath.URI() != fromSpecific.URI() {
		t.Fatalf("identity diverged:\n path:     %s\n specific: %s", fromPath.URI(), fromSpecific.URI())
	}
}

func TestDiscoverRegular_WindowsShapedPath_IsAPath(t *testing.T) {

	// `D:\...` used to die as `expected file scheme, got "d"` — url.Parse reads the drive letter as a
	// one-letter URI scheme. Under the paths-only domain there is no scheme grammar to collide with.
	// On Windows the input borrows the fixture root's own volume — a foreign volume cannot be made
	// root-relative (#392) — while on Unix VolumeName is empty and the literal "D:" shape survives.
	tmp := t.TempDir()
	p := testProvider(t, tmp)

	volume := filepath.VolumeName(tmp)
	if volume == "" {
		volume = "D:"
	}

	if _, err := DiscoverRegular(p.RuntimeEnvironment(), volume+`\a\b.txt`); err != nil {
		t.Fatalf("windows-shaped path rejected: %v", err)
	}
}

func TestDiscoverRegular_ForeignSchemeString_IsAPath(t *testing.T) {

	// The former dual grammar validated URI schemes; the paths-only domain treats any string as a path.
	p := testProvider(t, t.TempDir())

	resource, err := DiscoverRegular(p.RuntimeEnvironment(), "https://example.com/x")
	if err != nil {
		t.Fatalf("foreign-scheme string rejected: %v", err)
	}

	// Path canonicalization collapses the "//" — the string rides as the relative path
	// "https:/example.com/x" under the root, proving no scheme grammar intervened. Identity is the rel
	// itself (#584), so it sits immediately after the file: marker, slash-canonical on every platform.
	if !strings.Contains(resource.URI(), "file:https:/example.com/x") {
		t.Fatalf("expected the string carried as a path in the identity, got %s", resource.URI())
	}
}
