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

func TestNewResource(t *testing.T) {
	r := NewResource("callable", "file.Reducer/myfn")
	if r.ContentType != "callable" {
		t.Errorf("ContentType = %q, want %q", r.ContentType, "callable")
	}
	if r.Qualifier != "file.Reducer/myfn" {
		t.Errorf("Qualifier = %q, want %q", r.Qualifier, "file.Reducer/myfn")
	}
	if r.URI() != "mem:callable/file.Reducer/myfn" {
		t.Errorf("URI() = %q, want %q", r.URI(), "mem:callable/file.Reducer/myfn")
	}
}

func TestNewResource_NoQualifier(t *testing.T) {
	r := NewResource("json", "")
	if r.URI() != "mem:json" {
		t.Errorf("URI() = %q, want %q", r.URI(), "mem:json")
	}
}

func TestNewResourceWithData(t *testing.T) {
	data := []byte("hello world")
	r := NewResourceWithData("template", "greeting", data)
	if string(r.Data) != "hello world" {
		t.Errorf("Data = %q, want %q", r.Data, "hello world")
	}
	if r.Hash == "" {
		t.Error("Hash is empty after NewResourceWithData")
	}
	// Verify hash is correct.
	h := sha256.Sum256(data)
	want := hex.EncodeToString(h[:])
	if r.Hash != want {
		t.Errorf("Hash = %q, want %q", r.Hash, want)
	}
}

func TestComputeHash_Empty(t *testing.T) {
	r := NewResource("json", "config")
	r.ComputeHash()
	if r.Hash != "" {
		t.Errorf("Hash = %q, want empty for nil Data", r.Hash)
	}
}

func TestComputeHash_Deterministic(t *testing.T) {
	r1 := NewResourceWithData("json", "a", []byte("same"))
	r2 := NewResourceWithData("json", "b", []byte("same"))
	if r1.Hash != r2.Hash {
		t.Errorf("same data different hash: %q vs %q", r1.Hash, r2.Hash)
	}
}

func TestComputeHash_DifferentData(t *testing.T) {
	r1 := NewResourceWithData("json", "a", []byte("one"))
	r2 := NewResourceWithData("json", "a", []byte("two"))
	if r1.Hash == r2.Hash {
		t.Error("different data produced same hash")
	}
}

func TestResourceURI_OpaqueScheme(t *testing.T) {
	r := NewResource("callable", "file.Reducer/myfn")
	if r.Scheme() != "mem" {
		t.Errorf("Scheme() = %q, want %q", r.Scheme(), "mem")
	}
	if r.Opaque() != "callable/file.Reducer/myfn" {
		t.Errorf("Opaque() = %q, want %q", r.Opaque(), "callable/file.Reducer/myfn")
	}
}

func TestConstructorRoundTrip(t *testing.T) {
	r, err := op.Construct[Resource]("mem:callable/file.Reducer/myfn")
	if err != nil {
		t.Fatalf("Construct: %v", err)
	}
	if r.ContentType != "callable" {
		t.Errorf("ContentType = %q, want %q", r.ContentType, "callable")
	}
	if r.Qualifier != "file.Reducer/myfn" {
		t.Errorf("Qualifier = %q, want %q", r.Qualifier, "file.Reducer/myfn")
	}
	if r.URI() != "mem:callable/file.Reducer/myfn" {
		t.Errorf("URI() = %q, want %q", r.URI(), "mem:callable/file.Reducer/myfn")
	}
}

func TestConstructorInvalidPrefix(t *testing.T) {
	_, err := op.Construct[Resource]("file:///tmp/foo")
	if err == nil {
		t.Fatal("expected error for non-mem URI")
	}
}

func TestConstructorEmptyContentType(t *testing.T) {
	_, err := op.Construct[Resource]("mem:")
	if err == nil {
		t.Fatal("expected error for empty content type")
	}
}

func TestConstructorWrongType(t *testing.T) {
	_, err := op.Construct[Resource](42)
	if err == nil {
		t.Fatal("expected error for non-string")
	}
}

func TestConstructorSimpleContentType(t *testing.T) {
	r, err := op.Construct[Resource]("mem:json")
	if err != nil {
		t.Fatalf("Construct: %v", err)
	}
	if r.ContentType != "json" {
		t.Errorf("ContentType = %q, want %q", r.ContentType, "json")
	}
	if r.Qualifier != "" {
		t.Errorf("Qualifier = %q, want empty", r.Qualifier)
	}
}
