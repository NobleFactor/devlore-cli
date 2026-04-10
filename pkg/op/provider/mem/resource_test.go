// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package mem

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

func TestResourceImplementsInterface(t *testing.T) {
	var _ op.Resource = (*Resource)(nil)
}

func newRes(t *testing.T, spec ResourceSpec) *Resource {
	t.Helper()
	r, err := NewResource(&op.ExecutionContext{}, spec)
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}
	return r
}

func TestNewResource(t *testing.T) {
	r := newRes(t, ResourceSpec{ContentType: "callable", Namespace: "file.Reducer", Name: "myfn"})
	if r.ContentType != "callable" {
		t.Errorf("ContentType = %q, want %q", r.ContentType, "callable")
	}
	if r.Namespace != "file.Reducer" {
		t.Errorf("Namespace = %q, want %q", r.Namespace, "file.Reducer")
	}
	if r.Name != "myfn" {
		t.Errorf("Name = %q, want %q", r.Name, "myfn")
	}
	if r.URI() != "mem:callable/file.Reducer/myfn" {
		t.Errorf("URI() = %q, want %q", r.URI(), "mem:callable/file.Reducer/myfn")
	}
}

func TestNewResource_NoNamespace(t *testing.T) {
	r := newRes(t, ResourceSpec{ContentType: "json", Name: "config"})
	if r.URI() != "mem:json/config" {
		t.Errorf("URI() = %q, want %q", r.URI(), "mem:json/config")
	}
}

func TestNewResource_ContentTypeOnly(t *testing.T) {
	r := newRes(t, ResourceSpec{ContentType: "json"})
	if r.URI() != "mem:json" {
		t.Errorf("URI() = %q, want %q", r.URI(), "mem:json")
	}
}

func TestNewResource_WithData(t *testing.T) {
	data := []byte("hello world")
	r := newRes(t, ResourceSpec{ContentType: "template", Name: "greeting", Data: data})
	if string(r.Data) != "hello world" {
		t.Errorf("Data = %q, want %q", r.Data, "hello world")
	}
	if r.Hash == "" {
		t.Error("Hash is empty after NewResource with Data")
	}
	h := sha256.Sum256(data)
	want := hex.EncodeToString(h[:])
	if r.Hash != want {
		t.Errorf("Hash = %q, want %q", r.Hash, want)
	}
}

func TestComputeHash_Empty(t *testing.T) {
	r := newRes(t, ResourceSpec{ContentType: "json", Name: "config"})
	r.ComputeHash()
	if r.Hash != "" {
		t.Errorf("Hash = %q, want empty for nil Data", r.Hash)
	}
}

func TestComputeHash_Deterministic(t *testing.T) {
	r1 := newRes(t, ResourceSpec{ContentType: "json", Name: "a", Data: []byte("same")})
	r2 := newRes(t, ResourceSpec{ContentType: "json", Name: "b", Data: []byte("same")})
	if r1.Hash != r2.Hash {
		t.Errorf("same data different hash: %q vs %q", r1.Hash, r2.Hash)
	}
}

func TestComputeHash_DifferentData(t *testing.T) {
	r1 := newRes(t, ResourceSpec{ContentType: "json", Name: "a", Data: []byte("one")})
	r2 := newRes(t, ResourceSpec{ContentType: "json", Name: "a", Data: []byte("two")})
	if r1.Hash == r2.Hash {
		t.Error("different data produced same hash")
	}
}

func TestResourceURI_OpaqueScheme(t *testing.T) {
	r := newRes(t, ResourceSpec{ContentType: "callable", Namespace: "file.Reducer", Name: "myfn"})
	if r.Scheme() != "mem" {
		t.Errorf("Scheme() = %q, want %q", r.Scheme(), "mem")
	}
	if r.Opaque() != "callable/file.Reducer/myfn" {
		t.Errorf("Opaque() = %q, want %q", r.Opaque(), "callable/file.Reducer/myfn")
	}
}

func TestNewResource_EmptyContentType(t *testing.T) {
	_, err := NewResource(&op.ExecutionContext{}, ResourceSpec{})
	if err == nil {
		t.Fatal("expected error for empty content type")
	}
}

func TestNewResource_WrongType(t *testing.T) {
	_, err := NewResource(&op.ExecutionContext{}, 42)
	if err == nil {
		t.Fatal("expected error for non-ResourceSpec")
	}
}
