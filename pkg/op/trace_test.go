// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"bytes"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestLoadTrace_RoundTrip(t *testing.T) {

	trace := &Trace{GraphChecksum: "sha256:0123"}
	if err := trace.StampChecksum(); err != nil {
		t.Fatalf("StampChecksum: %v", err)
	}

	data, err := yaml.Marshal(trace)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	loaded, err := LoadTrace(data)
	if err != nil {
		t.Fatalf("LoadTrace: %v", err)
	}
	if loaded.GraphChecksum != trace.GraphChecksum {
		t.Errorf("GraphChecksum = %q, want %q", loaded.GraphChecksum, trace.GraphChecksum)
	}
	if loaded.Checksum != trace.Checksum {
		t.Errorf("Checksum = %q, want %q", loaded.Checksum, trace.Checksum)
	}
}

func TestLoadTrace_StampIsIdempotent(t *testing.T) {

	trace := &Trace{GraphChecksum: "sha256:0123"}
	if err := trace.StampChecksum(); err != nil {
		t.Fatalf("StampChecksum: %v", err)
	}
	first := trace.Checksum

	if err := trace.StampChecksum(); err != nil {
		t.Fatalf("StampChecksum (second): %v", err)
	}
	if trace.Checksum != first {
		t.Errorf("restamp changed checksum: %q -> %q", first, trace.Checksum)
	}
}

func TestLoadTrace_RefusesMissingChecksum(t *testing.T) {

	data, err := yaml.Marshal(&Trace{GraphChecksum: "sha256:0123"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	_, err = LoadTrace(data)
	if err == nil || !strings.Contains(err.Error(), "no checksum") {
		t.Fatalf("LoadTrace = %v, want no-checksum error", err)
	}
}

func TestLoadTrace_RefusesTamperedDocument(t *testing.T) {

	trace := &Trace{GraphChecksum: "sha256:0123"}
	if err := trace.StampChecksum(); err != nil {
		t.Fatalf("StampChecksum: %v", err)
	}

	data, err := yaml.Marshal(trace)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	tampered := bytes.Replace(data, []byte("sha256:0123"), []byte("sha256:4567"), 1)
	if bytes.Equal(tampered, data) {
		t.Fatal("tamper had no effect on the document bytes")
	}

	_, err = LoadTrace(tampered)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("LoadTrace = %v, want checksum-mismatch error", err)
	}
}
