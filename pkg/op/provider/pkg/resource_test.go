// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package pkg

import (
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

func resCtx(managerName string) *op.ExecutionContext {
	mgr := &mockPackageManager{
		name:      managerName,
		installed: make(map[string]bool),
		versions:  make(map[string]string),
	}
	return &op.ExecutionContext{
		Platform: &op.Platform{
			PackageManager:  mgr,
			PackageManagers: map[string]op.PackageManager{managerName: mgr},
		},
	}
}

// --- NewResource ---

func TestNewResource(t *testing.T) {
	ctx := resCtx("apt")
	r, err := NewResource(ctx, "jq")
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
	ctx := resCtx("brew")
	r, err := NewResource(ctx, "brew:jq")
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
	ctx := resCtx("brew")
	r, err := NewResource(ctx, "jq")
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
