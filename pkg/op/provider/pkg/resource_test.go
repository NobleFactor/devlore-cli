// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package pkg

import (
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/platform"
)

func resCtx(managerName string) *op.RuntimeEnvironment {
	mgr := &mockPackageManager{
		name:      managerName,
		installed: make(map[string]bool),
		versions:  make(map[string]string),
	}
	return &op.RuntimeEnvironment{
		Platform: &mockPlatform{
			defaultPM: mgr,
			available: map[string]platform.PackageManager{managerName: mgr},
		},
	}
}

// testActivation returns an [op.ActivationRecord] suitable for production-claim test calls. SiteID is
// derived from the test name; Runtime is the resCtx-built environment (carries Platform; Catalog is nil
// so the unlinked candidate is returned).
func testActivation(t *testing.T, managerName string) *op.ActivationRecord {
	t.Helper()
	return &op.ActivationRecord{
		Runtime: resCtx(managerName),
		SiteID:  "test:" + t.Name(),
	}
}

// --- NewResource ---

func TestNewResource(t *testing.T) {
	r, err := NewResource(testActivation(t, "apt"), "jq")
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}
	if r.Name != "jq" {
		t.Errorf("Name = %q, want %q", r.Name, "jq")
	}
	if r.Type != "apt" {
		t.Errorf("Type = %q, want %q", r.Type, "apt")
	}
	if r.Version != "" {
		t.Errorf("Version = %q, want empty", r.Version)
	}
}

func TestNewResource_WithPrefix(t *testing.T) {
	r, err := NewResource(testActivation(t, "brew"), "brew:jq")
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}
	if r.Name != "jq" {
		t.Errorf("Name = %q, want %q", r.Name, "jq")
	}
	if r.Type != "brew" {
		t.Errorf("Type = %q, want %q", r.Type, "brew")
	}
}

// --- URI ---

func TestResourceURI(t *testing.T) {
	r, err := NewResource(testActivation(t, "brew"), "jq")
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}
	if got := r.ReachabilityURI(); got != "pkg:brew/jq" {
		t.Errorf("ReachabilityURI() = %q, want %q", got, "pkg:brew/jq")
	}
}

// --- Interface guards ---

func TestResourceImplementsInterface(t *testing.T) {
	var _ op.Resource = (*Resource)(nil)
}

func TestReceiptImplementsInterface(t *testing.T) {
	var _ op.Receipt = (*Receipt)(nil)
}
