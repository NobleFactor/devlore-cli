// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package pkg

import (
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

func TestResourceURI(t *testing.T) {
	r := &Resource{Name: "vim"}
	uri := r.URI()
	if uri == "" {
		t.Error("URI() returned empty string")
	}
	if r.Scheme() != op.SchemePackage {
		t.Errorf("Scheme() = %q, want %q", r.Scheme(), op.SchemePackage)
	}
	if r.Path() != "vim" {
		t.Errorf("Path() = %q, want %q", r.Path(), "vim")
	}
}

func TestResourceImplementsInterface(t *testing.T) {
	var _ op.Resource = (*Resource)(nil)
}

func TestTombstoneImplementsInterface(t *testing.T) {
	var _ op.Tombstone = (*Tombstone)(nil)
}

func TestConstructorRoundTrip(t *testing.T) {
	r, err := op.Construct[Resource]("nginx")
	if err != nil {
		t.Fatalf("Construct: %v", err)
	}
	if r.Name != "nginx" {
		t.Errorf("Name = %q, want %q", r.Name, "nginx")
	}
}
