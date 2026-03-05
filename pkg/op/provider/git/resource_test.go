// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package git

import (
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

func TestResourceURI(t *testing.T) {
	r := &Resource{ClonePath: "/tmp/repo"}
	uri := r.URI()
	if uri == "" {
		t.Error("URI() returned empty string")
	}
	if r.Scheme() != op.SchemeGit {
		t.Errorf("Scheme() = %q, want %q", r.Scheme(), op.SchemeGit)
	}
}

func TestResourceImplementsInterface(t *testing.T) {
	var _ op.Resource = (*Resource)(nil)
}

func TestTombstoneImplementsInterface(t *testing.T) {
	var _ op.Tombstone = (*Tombstone)(nil)
}

func TestConstructorRoundTrip(t *testing.T) {
	r, err := op.Construct[Resource]("/tmp/myrepo")
	if err != nil {
		t.Fatalf("Construct: %v", err)
	}
	if r.ClonePath != "/tmp/myrepo" {
		t.Errorf("ClonePath = %q, want %q", r.ClonePath, "/tmp/myrepo")
	}
}
