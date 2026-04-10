// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package appnet

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func mustResource(t *testing.T, raw string) *Resource {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("url.Parse(%q): %v", raw, err)
	}
	return &Resource{SourceURL: u}
}

func TestDownloadSuccess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello world"))
	}))
	defer ts.Close()

	p := &Provider{}
	data, err := p.Download(mustResource(t, ts.URL))
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
	_, err := p.Download(mustResource(t, ts.URL))
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
	_, err := p.Download(mustResource(t, ts.URL))
	if err == nil {
		t.Fatal("Download() expected error for 500, got nil")
	}
	if !strings.Contains(err.Error(), "HTTP 500") {
		t.Errorf("error = %q, want message containing 'HTTP 500'", err)
	}
}

func TestDownloadInvalidURL(t *testing.T) {
	p := &Provider{}
	// A URL that url.Parse accepts but HTTP cannot connect to.
	_, err := p.Download(mustResource(t, "http://invalid.test:0/bad"))
	if err == nil {
		t.Fatal("Download() expected error for invalid URL, got nil")
	}
}
