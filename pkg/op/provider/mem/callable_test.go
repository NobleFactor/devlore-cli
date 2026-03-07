// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package mem

import (
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

func TestCallableImplementsResource(t *testing.T) {
	var _ op.Resource = (*Callable)(nil)
}

func TestNewCallable(t *testing.T) {
	c := NewCallable("file.Reducer", "count_python_files")
	if c.FuncType != "file.Reducer" {
		t.Errorf("FuncType = %q, want %q", c.FuncType, "file.Reducer")
	}
	if c.Name != "count_python_files" {
		t.Errorf("Name = %q, want %q", c.Name, "count_python_files")
	}
	if c.FuncName != "_callable" {
		t.Errorf("FuncName = %q, want %q", c.FuncName, "_callable")
	}
	if c.ContentType != "callable" {
		t.Errorf("ContentType = %q, want %q", c.ContentType, "callable")
	}
}

func TestCallableURI(t *testing.T) {
	c := NewCallable("file.Reducer", "count_python_files")
	want := "mem:callable/file.Reducer/count_python_files"
	if got := c.URI(); got != want {
		t.Errorf("URI() = %q, want %q", got, want)
	}
}

func TestCallableURI_Lambda(t *testing.T) {
	c := NewCallable("file.Reducer", "file.walk_tree.fn")
	want := "mem:callable/file.Reducer/file.walk_tree.fn"
	if got := c.URI(); got != want {
		t.Errorf("URI() = %q, want %q", got, want)
	}
}

func TestCallableURI_OpaqueScheme(t *testing.T) {
	c := NewCallable("Predicate", "is_large")
	if c.Scheme() != "mem" {
		t.Errorf("Scheme() = %q, want %q", c.Scheme(), "mem")
	}
	if c.Opaque() != "callable/Predicate/is_large" {
		t.Errorf("Opaque() = %q, want %q", c.Opaque(), "callable/Predicate/is_large")
	}
}

func TestCallableSetSource(t *testing.T) {
	c := NewCallable("file.Reducer", "myfn")
	source := []byte(`def _callable(initial, resource, path):
    return initial + [resource]
`)
	c.SetSource(source)
	if string(c.Data) != string(source) {
		t.Errorf("Data = %q, want %q", c.Data, source)
	}
	if c.Hash == "" {
		t.Error("Hash is empty after SetSource")
	}
}

func TestCallableSetSource_HashChanges(t *testing.T) {
	c := NewCallable("file.Reducer", "myfn")
	c.SetSource([]byte("version 1"))
	hash1 := c.Hash
	c.SetSource([]byte("version 2"))
	hash2 := c.Hash
	if hash1 == hash2 {
		t.Error("hash did not change after SetSource with different data")
	}
}

func TestCallableFn_PanicsBeforeInit(t *testing.T) {
	c := NewCallable("file.Reducer", "myfn")
	defer func() {
		if r := recover(); r == nil {
			t.Error("Fn() did not panic before Init")
		}
	}()
	c.Fn()
}
