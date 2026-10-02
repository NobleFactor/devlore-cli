// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package cli

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
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

	lifetime := NextLifetime(nil, nil)
	if lifetime.State != LifetimeCurrent || lifetime.ID == "" || len(lifetime.Runs) != 0 {
		t.Fatalf("NextLifetime(nil, nil) = %+v, want current, an id, no runs", lifetime)
	}
	if _, err := CurrentLifetime(); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a minted lifetime reached the store before its first run: err = %v", err)
	}

	first := LifetimeRun{
		Operation: RunOperationDeploy, Scope: "home", GraphChecksum: "sha256:aa11", TraceFile: "a.yaml",
	}
	second := LifetimeRun{
		Operation: RunOperationDeploy, Scope: "home", GraphChecksum: "sha256:bb22", TraceFile: "b.yaml",
	}
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
	if len(current.Runs) != 2 || current.Runs[0].GraphChecksum != "sha256:aa11" ||
		current.Runs[1].TraceFile != "b.yaml" {
		t.Errorf("runs = %+v, want the two appended, in order", current.Runs)
	}
	if current.Runs[0].At.IsZero() {
		t.Error("a run's At was not stamped")
	}
	if current.Runs[0].Scope != "home" {
		t.Errorf("a run's scope = %q after the round trip, want home", current.Runs[0].Scope)
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

	older := NextLifetime(nil, nil)
	deploy := LifetimeRun{
		Operation: RunOperationDeploy, Scope: "home", GraphChecksum: "sha256:aa11", TraceFile: "a.yaml",
	}
	upgrade := LifetimeRun{
		Operation: RunOperationUpgrade, Scope: "home", GraphChecksum: "sha256:aa11", TraceFile: "b.yaml",
	}
	for _, run := range []LifetimeRun{deploy, upgrade} {
		if err := older.AppendRun(run); err != nil {
			t.Fatalf("older.AppendRun(%s): %v", run.Operation, err)
		}
	}

	newer := NextLifetime(older, nil)
	before := time.Now().UTC()
	third := LifetimeRun{
		Operation: RunOperationDeploy, Scope: "home", GraphChecksum: "sha256:cc33", TraceFile: "c.yaml",
	}
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

// TestNextLifetime_BareDeployCarriesNothing pins the bare deploy (#926): it runs every scope, so its lifetime
// carries no run forward -- the runs of lifetimes written before runs recorded a scope among them.
func TestNextLifetime_BareDeployCarriesNothing(t *testing.T) {

	current := &Lifetime{ID: "current", State: LifetimeCurrent, Runs: []LifetimeRun{
		{Operation: RunOperationDeploy, Scope: "home", GraphChecksum: "sha256:aa11", TraceFile: "a.yaml"},
		{Operation: RunOperationDeploy, Scope: "system", GraphChecksum: "sha256:bb22", TraceFile: "b.yaml"},
		{Operation: RunOperationDeploy, GraphChecksum: "sha256:cc33", TraceFile: "c.yaml"},
	}}

	if next := NextLifetime(current, nil); len(next.Runs) != 0 {
		t.Errorf("a bare deploy carried %+v forward, want nothing", next.Runs)
	}
}

// TestNextLifetime_ScopedDeployCarriesTheOtherScopes pins the scoped deploy (#926): its lifetime carries forward,
// by reference, every run of the scopes it was not asked to run, in their order, and none of its own scopes'.
func TestNextLifetime_ScopedDeployCarriesTheOtherScopes(t *testing.T) {

	home := LifetimeRun{Operation: RunOperationDeploy, Scope: "home", GraphChecksum: "sha256:aa11", TraceFile: "a.yaml"}
	system := LifetimeRun{
		Operation: RunOperationDeploy, Scope: "system", GraphChecksum: "sha256:bb22", TraceFile: "b.yaml",
	}
	staging := LifetimeRun{
		Operation: RunOperationUpgrade, Scope: "staging", GraphChecksum: "sha256:cc33", TraceFile: "c.yaml",
	}
	current := &Lifetime{ID: "current", State: LifetimeCurrent, Runs: []LifetimeRun{home, system, staging}}

	next := NextLifetime(current, []string{"home"})
	if want := []LifetimeRun{system, staging}; !slices.Equal(next.Runs, want) {
		t.Errorf("carried %+v, want %+v", next.Runs, want)
	}
	if next.ID == current.ID || next.State != LifetimeCurrent {
		t.Errorf("next = {id %s, state %s}, want a new current lifetime", next.ID, next.State)
	}
}

// TestLifetime_CarriedLifetimeReplacesTheCurrent pins the switch for a lifetime that starts with carried runs: its
// first save, not its first run, makes it current and replaces the one it carried from.
func TestLifetime_CarriedLifetimeReplacesTheCurrent(t *testing.T) {

	t.Setenv("XDG_STATE_HOME", t.TempDir())

	older := NextLifetime(nil, nil)
	for _, run := range []LifetimeRun{
		{Operation: RunOperationDeploy, Scope: "home", GraphChecksum: "sha256:aa11", TraceFile: "a.yaml"},
		{Operation: RunOperationDeploy, Scope: "system", GraphChecksum: "sha256:bb22", TraceFile: "b.yaml"},
	} {
		if err := older.AppendRun(run); err != nil {
			t.Fatalf("older.AppendRun: %v", err)
		}
	}

	newer := NextLifetime(older, []string{"home"})
	run := LifetimeRun{Operation: RunOperationDeploy, Scope: "home", GraphChecksum: "sha256:cc33", TraceFile: "c.yaml"}
	if err := newer.AppendRun(run); err != nil {
		t.Fatalf("newer.AppendRun: %v", err)
	}

	current, err := CurrentLifetime()
	if err != nil || current.ID != newer.ID {
		t.Fatalf("CurrentLifetime = (%v, %v), want the newer %s", current, err, newer.ID)
	}
	if len(current.Runs) != 2 || current.Runs[0].Scope != "system" || current.Runs[1].GraphChecksum != "sha256:cc33" {
		t.Errorf("current's runs = %+v, want system's carried, then home's new run", current.Runs)
	}
	replaced, err := LoadLifetime(older.ID)
	if err != nil || replaced.State != LifetimeReplaced {
		t.Errorf("older = (%v, %v), want it replaced", replaced, err)
	}
}

// TestLifetime_NoTornDocument pins the atomic write: no `.tmp` sibling survives a save, and the directory holds
// exactly the document and `current`.
func TestLifetime_NoTornDocument(t *testing.T) {

	t.Setenv("XDG_STATE_HOME", t.TempDir())

	lifetime := NextLifetime(nil, nil)
	run := LifetimeRun{Operation: RunOperationDeploy, Scope: "home", GraphChecksum: "sha256:aa11", TraceFile: "a.yaml"}
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

	lifetime := NextLifetime(nil, nil)
	deployed := LifetimeRun{
		Operation: RunOperationDeploy, Scope: "home", GraphChecksum: "sha256:aa11", TraceFile: "a.yaml",
	}
	if err := lifetime.AppendRun(deployed); err != nil {
		t.Fatalf("AppendRun: %v", err)
	}
	found, err := RequireCurrentLifetime(RunOperationUpgrade)
	if err != nil || found.ID != lifetime.ID {
		t.Errorf("RequireCurrentLifetime after a deploy = (%v, %v), want the current lifetime", found, err)
	}
}

// TestWriteLifetimeTrace_RecordsTheRun pins the seam every writer uses: the trace lands in the store and the
// lifetime names it by graph checksum, trace file and scope, so the fold can find it and a deploy can replace it.
func TestWriteLifetimeTrace_RecordsTheRun(t *testing.T) {

	t.Setenv("XDG_STATE_HOME", t.TempDir())

	const checksum = "sha256:5eaf00d5eaf00d5eaf00d5eaf00d5eaf00d5eaf00d5eaf00d5eaf00d5eaf00d5"
	lifetime := NextLifetime(nil, nil)

	path, err := WriteLifetimeTrace(lifetime, RunOperationDeploy, "home", &op.Trace{GraphChecksum: checksum})
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
	if run.Scope != "home" {
		t.Errorf("run's scope = %q, want home", run.Scope)
	}
	if filepath.Join(TracesDir(), safeChecksum(checksum), run.TraceFile) != path {
		t.Errorf("the lifetime's run does not locate the trace: %s", path)
	}
}

// TestWriteLifetimeTrace_FailedScopeKeepsItsRuns pins the failure rule (#926): a deploy's run that failed keeps its
// scope's previous runs ahead of it, while a scope that succeeded is replaced by its new run alone.
func TestWriteLifetimeTrace_FailedScopeKeepsItsRuns(t *testing.T) {

	t.Setenv("XDG_STATE_HOME", t.TempDir())

	homeBefore := LifetimeRun{
		Operation: RunOperationDeploy, Scope: "home", GraphChecksum: "sha256:aa11", TraceFile: "a.yaml",
	}
	systemBefore := LifetimeRun{
		Operation: RunOperationDeploy, Scope: "system", GraphChecksum: "sha256:bb22", TraceFile: "b.yaml",
	}
	current := &Lifetime{ID: "current", State: LifetimeCurrent, Runs: []LifetimeRun{homeBefore, systemBefore}}

	next := NextLifetime(current, nil)

	const succeeded = "sha256:5eaf00d5eaf00d5eaf00d5eaf00d5eaf00d5eaf00d5eaf00d5eaf00d5eaf00d5"
	if _, err := WriteLifetimeTrace(next, RunOperationDeploy, "home", &op.Trace{GraphChecksum: succeeded}); err != nil {
		t.Fatalf("WriteLifetimeTrace(home): %v", err)
	}
	const failed = "sha256:fa11edfa11edfa11edfa11edfa11edfa11edfa11edfa11edfa11edfa11edfa11"
	failure := &op.Trace{GraphChecksum: failed, RunStatus: op.RunStatus{Condition: op.ConditionExecutionFailed}}
	if _, err := WriteLifetimeTrace(next, RunOperationDeploy, "system", failure); err != nil {
		t.Fatalf("WriteLifetimeTrace(system): %v", err)
	}

	checksums := make([]string, len(next.Runs))
	for i, run := range next.Runs {
		checksums[i] = run.GraphChecksum
	}
	if want := []string{succeeded, "sha256:bb22", failed}; !slices.Equal(checksums, want) {
		t.Errorf("runs = %v, want home's new run, then system's previous run ahead of its failed one: %v",
			checksums, want)
	}
}
