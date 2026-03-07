// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package pkg

import (
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

func TestNewResource(t *testing.T) {
	r := NewResource("jq")
	if r.Name != "jq" {
		t.Errorf("Name = %q, want %q", r.Name, "jq")
	}
	if r.Type != "" {
		t.Errorf("Type = %q, want empty", r.Type)
	}
	if r.Version != "" {
		t.Errorf("Version = %q, want empty", r.Version)
	}
}

func TestNewTypedResource(t *testing.T) {
	r := NewTypedResource("jq", "brew")
	if r.Name != "jq" {
		t.Errorf("Name = %q, want %q", r.Name, "jq")
	}
	if r.Type != "brew" {
		t.Errorf("Type = %q, want %q", r.Type, "brew")
	}
}

func TestResourceURI_WithType(t *testing.T) {
	r := NewTypedResource("jq", "brew")
	want := "pkg:brew/jq"
	if got := r.URI(); got != want {
		t.Errorf("URI() = %q, want %q", got, want)
	}
}

func TestResourceURI_WithoutType(t *testing.T) {
	r := NewResource("jq")
	want := "pkg:/jq"
	if got := r.URI(); got != want {
		t.Errorf("URI() = %q, want %q", got, want)
	}
}

func TestResourceURI_WithVersion(t *testing.T) {
	r := Resource{Name: "jq", Type: "brew", Version: "1.7"}
	r.SetURI(r.buildURI())
	want := "pkg:brew/jq@1.7"
	if got := r.URI(); got != want {
		t.Errorf("URI() = %q, want %q", got, want)
	}
}

func TestResourceURI_Winget(t *testing.T) {
	r := Resource{Name: "Microsoft.VisualStudioCode", Type: "winget"}
	r.SetURI(r.buildURI())
	want := "pkg:winget/Microsoft/VisualStudioCode"
	if got := r.URI(); got != want {
		t.Errorf("URI() = %q, want %q", got, want)
	}
}

func TestResourceURI_ParsesAsOpaque(t *testing.T) {
	r := NewTypedResource("jq", "brew")
	if r.Scheme() != "pkg" {
		t.Errorf("Scheme() = %q, want pkg", r.Scheme())
	}
	if r.Opaque() != "brew/jq" {
		t.Errorf("Opaque() = %q, want brew/jq", r.Opaque())
	}
}

func TestPurl_ConvergesWithURI(t *testing.T) {
	r := NewTypedResource("jq", "brew")
	if r.URI() != r.Purl() {
		t.Errorf("URI() = %q != Purl() = %q — should converge", r.URI(), r.Purl())
	}
}

func TestPurl_WithVersion(t *testing.T) {
	r := Resource{Name: "jq", Type: "brew", Version: "1.7"}
	r.SetURI(r.buildURI())
	want := "pkg:brew/jq@1.7"
	if got := r.Purl(); got != want {
		t.Errorf("Purl() = %q, want %q", got, want)
	}
}

func TestPurl_Winget(t *testing.T) {
	r := Resource{Name: "Microsoft.VisualStudioCode", Type: "winget"}
	r.SetURI(r.buildURI())
	want := "pkg:winget/Microsoft/VisualStudioCode"
	if got := r.Purl(); got != want {
		t.Errorf("Purl() = %q, want %q", got, want)
	}
}

func TestPurl_WingetWithVersion(t *testing.T) {
	r := Resource{Name: "Microsoft.VisualStudioCode", Type: "winget", Version: "1.85"}
	r.SetURI(r.buildURI())
	want := "pkg:winget/Microsoft/VisualStudioCode@1.85"
	if got := r.Purl(); got != want {
		t.Errorf("Purl() = %q, want %q", got, want)
	}
}

func TestPurl_WingetNoDot(t *testing.T) {
	r := Resource{Name: "curl", Type: "winget"}
	r.SetURI(r.buildURI())
	want := "pkg:winget/curl"
	if got := r.Purl(); got != want {
		t.Errorf("Purl() = %q, want %q", got, want)
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
