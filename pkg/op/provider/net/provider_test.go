// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package net //nolint:revive // package name is domain-specific

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDownloadSuccess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello world"))
	}))
	defer ts.Close()

	p := &Provider{}
	data, err := p.Download(ts.URL)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}

	want := []byte("hello world")
	if !bytes.Equal(data, want) {
		t.Errorf("Download() = %q, want %q", data, want)
	}
}

func TestDownloadNotFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	p := &Provider{}
	_, err := p.Download(ts.URL)
	if err == nil {
		t.Fatal("Download() expected error for 404, got nil")
	}
	if !strings.Contains(err.Error(), "HTTP 404") {
		t.Errorf("error = %q, want message containing 'HTTP 404'", err)
	}
}

func TestDownloadServerError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	p := &Provider{}
	_, err := p.Download(ts.URL)
	if err == nil {
		t.Fatal("Download() expected error for 500, got nil")
	}
	if !strings.Contains(err.Error(), "HTTP 500") {
		t.Errorf("error = %q, want message containing 'HTTP 500'", err)
	}
}

func TestDownloadInvalidURL(t *testing.T) {
	p := &Provider{}
	_, err := p.Download("://bad")
	if err == nil {
		t.Fatal("Download() expected error for invalid URL, got nil")
	}
}
