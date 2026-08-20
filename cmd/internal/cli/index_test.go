// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/NobleFactor/devlore-cli/cmd/internal/devlore"
	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
)

// stateRootForTest opens the devlore state tree the way a store write does.
//
// The root is owned by the caller in production ([WriteGraph] / [WriteTrace]), so a test that appends must own
// one too — exercising the real ownership shape rather than a bespoke one.
//
// Parameters:
//   - `t`: the test harness.
//
// Returns:
//   - `fsroot.Dir`: the state tree, closed at test end.
func stateRootForTest(t *testing.T) fsroot.Dir {

	t.Helper()

	root, err := OpenTree(devlore.StateHome())
	if err != nil {
		t.Fatalf("OpenTree(%s): %v", devlore.StateHome(), err)
	}
	t.Cleanup(func() { _ = root.Close() })

	return root
}

// TestIndexAppendReadRoundTrip pins the NDJSON round trip: appended entries read back in order with their
// fields intact.
func TestIndexAppendReadRoundTrip(t *testing.T) {

	t.Setenv("XDG_STATE_HOME", t.TempDir())
	stateRoot := stateRootForTest(t)

	first := IndexEntry{
		At:            time.Date(2026, 7, 15, 18, 30, 14, 0, time.UTC),
		Event:         IndexEventGraph,
		Tool:          "writ",
		Scope:         "home",
		GraphChecksum: "sha256:aa11",
	}
	second := IndexEntry{
		At:            time.Date(2026, 7, 15, 18, 30, 21, 0, time.UTC),
		Event:         IndexEventTrace,
		GraphChecksum: "sha256:aa11",
		TraceFile:     "20260715T183021Z.yaml",
	}

	if err := appendIndexEntry(stateRoot, first); err != nil {
		t.Fatalf("append first: %v", err)
	}
	if err := appendIndexEntry(stateRoot, second); err != nil {
		t.Fatalf("append second: %v", err)
	}

	entries, err := ReadIndex()
	if err != nil {
		t.Fatalf("ReadIndex: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(entries))
	}
	if entries[0] != first {
		t.Errorf("entries[0] = %+v, want %+v", entries[0], first)
	}
	if entries[1] != second {
		t.Errorf("entries[1] = %+v, want %+v", entries[1], second)
	}
}

// TestIndexTornLastLine pins torn-write tolerance: a truncated final line is skipped, everything before it
// still reads.
func TestIndexTornLastLine(t *testing.T) {

	t.Setenv("XDG_STATE_HOME", t.TempDir())
	stateRoot := stateRootForTest(t)

	entry := IndexEntry{At: time.Now().UTC().Truncate(time.Second), Event: IndexEventGraph, GraphChecksum: "sha256:bb22"}
	if err := appendIndexEntry(stateRoot, entry); err != nil {
		t.Fatalf("append: %v", err)
	}

	file, err := os.OpenFile(IndexPath(), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(`{"at":"2026-07-15T18:30:2`); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	entries, err := ReadIndex()
	if err != nil {
		t.Fatalf("ReadIndex over torn index: %v", err)
	}
	if len(entries) != 1 || entries[0].GraphChecksum != "sha256:bb22" {
		t.Errorf("entries = %+v, want the one intact entry", entries)
	}
}

// TestIndexMissingIsAnError pins the settled ruling: a missing index file is an error the caller can
// distinguish via errors.Is(err, os.ErrNotExist), not a silent empty read.
func TestIndexMissingIsAnError(t *testing.T) {

	t.Setenv("XDG_STATE_HOME", filepath.Join(t.TempDir(), "absent"))

	entries, err := ReadIndex()
	if err == nil {
		t.Fatalf("ReadIndex on a missing index = %v entries, nil error; want an error", len(entries))
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("error %v does not unwrap to os.ErrNotExist", err)
	}
	if !strings.Contains(err.Error(), "index.ndjson") {
		t.Errorf("error %v does not name the index path", err)
	}
}
