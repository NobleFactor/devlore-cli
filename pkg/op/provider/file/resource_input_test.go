// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

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
	p := testProvider(t, t.TempDir())

	if _, err := DiscoverRegular(p.RuntimeEnvironment(), `D:\a\b.txt`); err != nil {
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
	// "https:/example.com/x" under the root, proving no scheme grammar intervened.
	if !strings.Contains(resource.URI(), "/https:/example.com/x") {
		t.Fatalf("expected the string carried as a path in the identity, got %s", resource.URI())
	}
}
