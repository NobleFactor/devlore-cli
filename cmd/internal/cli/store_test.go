// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// TestWriteTrace_IndexSurvivesLinkFailure pins the ordering fixed for #438: the run index is appended before
// the `latest.yaml` convenience link, so a link that cannot be created never costs the index its entry.
//
// The failure it guards is silent rather than loud. Every caller of [WriteTrace] warns instead of failing, so
// before the reorder a machine that cannot create symlinks — an ordinary Windows user, lacking Developer Mode
// and administrator rights — wrote the trace to disk, lost the index entry, and reported success.
//
// The link failure is forced portably by occupying `latest.yaml` with a non-empty directory: the best-effort
// Remove cannot unlink it, so Symlink fails on every platform rather than only where privileges are short.
func TestWriteTrace_IndexSurvivesLinkFailure(t *testing.T) {

	t.Setenv("XDG_STATE_HOME", t.TempDir())

	const checksum = "sha256:5eaf00d5eaf00d5eaf00d5eaf00d5eaf00d5eaf00d5eaf00d5eaf00d5eaf00d5"
	trace := &op.Trace{GraphChecksum: checksum}

	// Occupy the link name with a directory that cannot be removed.
	directory := filepath.Join(TracesDir(), safeChecksum(checksum))
	occupied := filepath.Join(directory, "latest.yaml")
	if err := os.MkdirAll(filepath.Join(occupied, "occupant"), 0o750); err != nil { //nolint:gosec // G301: test fixture
		t.Fatalf("preparing the occupied link name: %v", err)
	}

	path, err := WriteTrace(trace)
	if err == nil {
		t.Fatal("WriteTrace succeeded; the occupied link name should have failed the symlink")
	}
	if path != "" {
		t.Errorf("path = %q on failure, want empty", path)
	}

	// The trace document is durable even though the link failed.
	written, globErr := filepath.Glob(filepath.Join(directory, "*.yaml"))
	if globErr != nil {
		t.Fatalf("globbing the trace directory: %v", globErr)
	}
	var traceFiles []string
	for _, candidate := range written {
		if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
			traceFiles = append(traceFiles, candidate)
		}
	}
	if len(traceFiles) != 1 {
		t.Fatalf("trace files = %v, want exactly one", traceFiles)
	}

	// The index recorded it. This is the assertion that fails if the link is ever moved back ahead of the
	// index append.
	entries, err := ReadIndex()
	if err != nil {
		t.Fatalf("ReadIndex: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("index entries = %d, want 1 — the index lost a trace that is on disk", len(entries))
	}
	if entries[0].GraphChecksum != checksum {
		t.Errorf("index GraphChecksum = %q, want %q", entries[0].GraphChecksum, checksum)
	}
	if entries[0].Event != IndexEventTrace {
		t.Errorf("index Event = %q, want %q", entries[0].Event, IndexEventTrace)
	}
	if got, want := entries[0].TraceFile, filepath.Base(traceFiles[0]); got != want {
		t.Errorf("index TraceFile = %q, want %q", got, want)
	}
}
