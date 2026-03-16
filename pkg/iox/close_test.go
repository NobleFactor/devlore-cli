// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package iox

import (
	"errors"
	"testing"
)

type errCloser struct{ err error }

func (c errCloser) Close() error { return c.err }

func TestClose_NilErr(t *testing.T) {

	var err error
	Close(&err, errCloser{nil})
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestClose_SingleError(t *testing.T) {

	want := errors.New("close failed")
	var err error
	Close(&err, errCloser{want})
	if !errors.Is(err, want) {
		t.Fatalf("expected %v, got %v", want, err)
	}
}

func TestClose_JoinsMultipleErrors(t *testing.T) {

	e1 := errors.New("first")
	e2 := errors.New("second")
	var err error
	Close(&err, errCloser{e1}, errCloser{e2})
	if !errors.Is(err, e1) || !errors.Is(err, e2) {
		t.Fatalf("expected both errors, got %v", err)
	}
}

func TestClose_PreservesExistingError(t *testing.T) {

	existing := errors.New("existing")
	closeErr := errors.New("close")
	err := existing
	Close(&err, errCloser{closeErr})
	if !errors.Is(err, existing) || !errors.Is(err, closeErr) {
		t.Fatalf("expected both errors, got %v", err)
	}
}

func TestClose_SkipsNilCloser(t *testing.T) {

	var err error
	Close(&err, nil, errCloser{nil})
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestClose_NoClosers(t *testing.T) {

	var err error
	Close(&err)
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestClose_MixedNilAndErrors(t *testing.T) {

	want := errors.New("only")
	var err error
	Close(&err, errCloser{nil}, nil, errCloser{want}, errCloser{nil})
	if !errors.Is(err, want) {
		t.Fatalf("expected %v, got %v", want, err)
	}
}
