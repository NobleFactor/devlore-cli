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

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// TestLifetime_NoCurrentIsNotFound pins the never-deployed answer: a store with no lifetimes has no current one,
// and the error is os.ErrNotExist so callers can tell it from a broken store.
func TestLifetime_NoCurrentIsNotFound(t *testing.T) {

	t.Setenv("XDG_STATE_HOME", t.TempDir())

	if _, err := CurrentLifetime(); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("CurrentLifetime on an empty store: err = %v, want os.ErrNotExist", err)
	}
}

// TestLifetime_RoundTrip pins the document: runs append in order, the document reloads intact by id, and
// `current` names it from its first run.
func TestLifetime_RoundTrip(t *testing.T) {

	t.Setenv("XDG_STATE_HOME", t.TempDir())

	lifetime := NewLifetime()
	if lifetime.State != LifetimeCurrent || lifetime.ID == "" || len(lifetime.Runs) != 0 {
		t.Fatalf("NewLifetime = %+v, want current, an id, no runs", lifetime)
	}
	if _, err := CurrentLifetime(); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a minted lifetime reached the store before its first run: err = %v", err)
	}

	first := LifetimeRun{Operation: RunOperationDeploy, GraphChecksum: "sha256:aa11", TraceFile: "a.yaml"}
	second := LifetimeRun{Operation: RunOperationDeploy, GraphChecksum: "sha256:bb22", TraceFile: "b.yaml"}
	for _, run := range []LifetimeRun{first, second} {
		if err := lifetime.AppendRun(run); err != nil {
			t.Fatalf("AppendRun(%s): %v", run.GraphChecksum, err)
		}
	}

	current, err := CurrentLifetime()
	if err != nil {
		t.Fatalf("CurrentLifetime: %v", err)
	}
	if current.ID != lifetime.ID || current.State != LifetimeCurrent || !current.Closed.IsZero() {
		t.Errorf("current = %+v, want %s, current, not closed", current, lifetime.ID)
	}
	if len(current.Runs) != 2 || current.Runs[0].GraphChecksum != "sha256:aa11" || current.Runs[1].TraceFile != "b.yaml" {
		t.Errorf("runs = %+v, want the two appended, in order", current.Runs)
	}
	if current.Runs[0].At.IsZero() {
		t.Error("a run's At was not stamped")
	}

	loaded, err := LoadLifetime(lifetime.ID)
	if err != nil {
		t.Fatalf("LoadLifetime: %v", err)
	}
	if loaded.ID != current.ID || len(loaded.Runs) != len(current.Runs) {
		t.Errorf("LoadLifetime = %+v, want what CurrentLifetime returned", loaded)
	}
}

// TestLifetime_DeployReplacesTheCurrent pins the replacement: a later lifetime's first run makes it current and
// marks the previous one replaced, closed now, its runs kept; the previous one then refuses runs.
func TestLifetime_DeployReplacesTheCurrent(t *testing.T) {

	t.Setenv("XDG_STATE_HOME", t.TempDir())

	older := NewLifetime()
	deploy := LifetimeRun{Operation: RunOperationDeploy, GraphChecksum: "sha256:aa11", TraceFile: "a.yaml"}
	upgrade := LifetimeRun{Operation: RunOperationUpgrade, GraphChecksum: "sha256:aa11", TraceFile: "b.yaml"}
	for _, run := range []LifetimeRun{deploy, upgrade} {
		if err := older.AppendRun(run); err != nil {
			t.Fatalf("older.AppendRun(%s): %v", run.Operation, err)
		}
	}

	newer := NewLifetime()
	before := time.Now().UTC()
	third := LifetimeRun{Operation: RunOperationDeploy, GraphChecksum: "sha256:cc33", TraceFile: "c.yaml"}
	if err := newer.AppendRun(third); err != nil {
		t.Fatalf("newer.AppendRun: %v", err)
	}

	current, err := CurrentLifetime()
	if err != nil {
		t.Fatalf("CurrentLifetime: %v", err)
	}
	if current.ID != newer.ID {
		t.Fatalf("current = %s, want the newer %s", current.ID, newer.ID)
	}

	replaced, err := LoadLifetime(older.ID)
	if err != nil {
		t.Fatalf("LoadLifetime(older): %v", err)
	}
	if replaced.State != LifetimeReplaced || replaced.Closed.Before(before) {
		t.Errorf("older = {state %s, closed %v}, want replaced, closed at or after %v",
			replaced.State, replaced.Closed, before)
	}
	if len(replaced.Runs) != 2 {
		t.Errorf("older kept %d runs, want its 2 (history is kept; pruning deletes)", len(replaced.Runs))
	}

	if err := replaced.AppendRun(upgrade); err == nil {
		t.Error("a replaced lifetime accepted a run; want a refusal")
	}
}

// TestLifetime_NoTornDocument pins the atomic write: no `.tmp` sibling survives a save, and the directory holds
// exactly the document and `current`.
func TestLifetime_NoTornDocument(t *testing.T) {

	t.Setenv("XDG_STATE_HOME", t.TempDir())

	lifetime := NewLifetime()
	run := LifetimeRun{Operation: RunOperationDeploy, GraphChecksum: "sha256:aa11", TraceFile: "a.yaml"}
	if err := lifetime.AppendRun(run); err != nil {
		t.Fatalf("AppendRun: %v", err)
	}

	entries, err := os.ReadDir(LifetimesDir())
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".tmp") {
			t.Errorf("temporary file survived: %s", entry.Name())
		}
	}
	if len(entries) != 2 {
		t.Errorf("lifetimes dir holds %d entries, want 2 (the document and current)", len(entries))
	}
}

// TestRequireCurrentLifetime_NotFoundIs66 pins the coded refusal: an operation that writes into the current
// deployment, with none, exits ExitNoInput -- the same code reconcile gives on a machine never deployed.
func TestRequireCurrentLifetime_NotFoundIs66(t *testing.T) {

	t.Setenv("XDG_STATE_HOME", t.TempDir())

	_, err := RequireCurrentLifetime(RunOperationUpgrade)
	if err == nil {
		t.Fatal("RequireCurrentLifetime with no lifetime = nil error, want not-found")
	}
	if code := ExitCode(err); code != ExitNoInput {
		t.Errorf("ExitCode = %d, want %d (EX_NOINPUT)", code, ExitNoInput)
	}
	if !strings.Contains(err.Error(), RunOperationUpgrade) {
		t.Errorf("error %q does not name the operation", err)
	}

	lifetime := NewLifetime()
	deployed := LifetimeRun{Operation: RunOperationDeploy, GraphChecksum: "sha256:aa11", TraceFile: "a.yaml"}
	if err := lifetime.AppendRun(deployed); err != nil {
		t.Fatalf("AppendRun: %v", err)
	}
	found, err := RequireCurrentLifetime(RunOperationUpgrade)
	if err != nil || found.ID != lifetime.ID {
		t.Errorf("RequireCurrentLifetime after a deploy = (%v, %v), want the current lifetime", found, err)
	}
}

// TestWriteLifetimeTrace_RecordsTheRun pins the seam every writer uses: the trace lands in the store and the
// lifetime names it by graph checksum and trace file, so the fold can find it.
func TestWriteLifetimeTrace_RecordsTheRun(t *testing.T) {

	t.Setenv("XDG_STATE_HOME", t.TempDir())

	const checksum = "sha256:5eaf00d5eaf00d5eaf00d5eaf00d5eaf00d5eaf00d5eaf00d5eaf00d5eaf00d5"
	lifetime := NewLifetime()

	path, err := WriteLifetimeTrace(lifetime, RunOperationDeploy, &op.Trace{GraphChecksum: checksum})
	if err != nil {
		t.Fatalf("WriteLifetimeTrace: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the trace is not at %s: %v", path, err)
	}

	current, err := CurrentLifetime()
	if err != nil {
		t.Fatalf("CurrentLifetime: %v", err)
	}
	if len(current.Runs) != 1 {
		t.Fatalf("runs = %+v, want the one written", current.Runs)
	}
	run := current.Runs[0]
	if run.Operation != RunOperationDeploy || run.GraphChecksum != checksum || run.TraceFile != filepath.Base(path) {
		t.Errorf("run = %+v, want deploy, %s, %s", run, checksum, filepath.Base(path))
	}
	if filepath.Join(TracesDir(), safeChecksum(checksum), run.TraceFile) != path {
		t.Errorf("the lifetime's run does not locate the trace: %s", path)
	}
}
