// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"strings"
	"testing"
)

func TestGenerateNodeID_NoComponents(t *testing.T) {
	id := GenerateNodeID("node")
	if !strings.HasPrefix(id, "node-") {
		t.Errorf("GenerateNodeID(node) = %q, want prefix node-", id)
	}
	// Should be "node-<number>"
	parts := strings.Split(id, "-")
	if len(parts) != 2 {
		t.Errorf("GenerateNodeID(node) = %q, want format node-<number>", id)
	}
}

func TestGenerateNodeID_WithComponents(t *testing.T) {
	id := GenerateNodeID("phase", "install", "brew")
	if !strings.HasPrefix(id, "phase-install-brew-") {
		t.Errorf("GenerateNodeID(phase,install,brew) = %q, want prefix phase-install-brew-", id)
	}
	// Should be "phase-install-brew-<number>"
	parts := strings.Split(id, "-")
	if len(parts) != 4 {
		t.Errorf("GenerateNodeID(phase,install,brew) = %q, want 4 dash-separated parts", id)
	}
}

func TestGenerateNodeID_SingleComponent(t *testing.T) {
	id := GenerateNodeID("do", "link")
	if !strings.HasPrefix(id, "do-link-") {
		t.Errorf("GenerateNodeID(do,link) = %q, want prefix do-link-", id)
	}
}

func TestGenerateNodeID_Uniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := GenerateNodeID("test")
		if seen[id] {
			t.Fatalf("duplicate ID generated: %q", id)
		}
		seen[id] = true
	}
}

func TestGenerateNodeID_UniqueAcrossPrefixes(t *testing.T) {
	id1 := GenerateNodeID("alpha")
	id2 := GenerateNodeID("beta")
	if id1 == id2 {
		t.Errorf("IDs should differ: %q vs %q", id1, id2)
	}
}
