// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package git

import (
	"path/filepath"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// The constructor input domain (paths-only ruling, 2026-08-09): a filesystem path, plus the provider's own
// emitted identity specific ("file://" + path) on the catalog-rehydration round-trip. No URI grammar on the
// input side.

func TestDiscoverResource_PathAndOwnSpecific_SameIdentity(t *testing.T) {

	runtimeEnvironment := &op.RuntimeEnvironment{Root: fsroot.OpenWritableUnconfined("/")}
	path := filepath.Join(t.TempDir(), "repo")

	fromPath, err := DiscoverResource(runtimeEnvironment, path)
	if err != nil {
		t.Fatalf("DiscoverResource(path): %v", err)
	}

	fromSpecific, err := DiscoverResource(runtimeEnvironment, "file://"+path)
	if err != nil {
		t.Fatalf("DiscoverResource(own specific): %v", err)
	}

	if fromPath.URI() != fromSpecific.URI() {
		t.Fatalf("identity diverged:\n path:     %s\n specific: %s", fromPath.URI(), fromSpecific.URI())
	}
}

func TestDiscoverResource_WindowsShapedPath_IsAPath(t *testing.T) {

	// `D:\...` used to die as `expected file scheme, got "d"`; the paths-only domain has no scheme
	// grammar to collide with.
	runtimeEnvironment := &op.RuntimeEnvironment{Root: fsroot.OpenWritableUnconfined("/")}

	if _, err := DiscoverResource(runtimeEnvironment, `D:\repos\x`); err != nil {
		t.Fatalf("windows-shaped path rejected: %v", err)
	}
}
