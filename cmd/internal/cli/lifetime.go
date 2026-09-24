// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"

	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
	"github.com/NobleFactor/devlore-cli/pkg/iox"
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// A lifetime is one `writ deploy` invocation, across every scope it ran, together with the upgrades,
// reconciliations and adoptions that follow it, until a later deploy replaces it or a decommission ends it
// (ruled 2026-09-22 and 2026-09-23; devlore-cli#913, #922). The record a deployment leaves is the fold of the
// current lifetime's runs and nothing else: deploy replaces the record, upgrade updates it, decommission removes
// it, reconcile restores the system to it.
//
// The lifetime is a document of its own under [LifetimesDir], `<id>.yaml`, and `current` names the current one.
// The run index stays a detection hint; the lifetime document is the truth about membership, so a trace on disk
// that no lifetime names belongs to none. A store with no `lifetimes/` directory has no current lifetime, which
// is the not-found `writ reconcile` answers on a machine never deployed.

// LifetimeCurrent, LifetimeReplaced and LifetimeEnded are the states a lifetime passes through: current from the
// first run it records; replaced when a later deploy records its first run; ended when a decommission ends it.
const (
	LifetimeCurrent  = "current"
	LifetimeReplaced = "replaced"
	LifetimeEnded    = "ended"
)

// RunOperationDeploy, RunOperationUpgrade, RunOperationReconcile, RunOperationAdopt and RunOperationDecommission
// name the operation that wrote a lifetime's run. Deploy opens a lifetime; the others write into the current one.
const (
	RunOperationDeploy       = "deploy"
	RunOperationUpgrade      = "upgrade"
	RunOperationReconcile    = "reconcile"
	RunOperationAdopt        = "adopt"
	RunOperationDecommission = "decommission"
)

// lifetimesDirname is the lifetimes directory's name under the store root, and currentFilename the name of the
// file that holds the current lifetime's id -- a plain file, not a symlink, so the current pointer needs no link
// privilege on Windows.
const (
	lifetimesDirname = "lifetimes"
	currentFilename  = "current"
)

// LifetimeRun is one run a lifetime owns: the trace of one scope's graph, written by one operation.
type LifetimeRun struct {

	// At is the UTC moment the trace was written.
	At time.Time `json:"at" yaml:"at"`

	// Operation is the lifecycle operation that wrote the run: one of the RunOperation* constants.
	Operation string `json:"operation" yaml:"operation"`

	// GraphChecksum is the run's graph identity, the join key into [GraphsDir] and [TracesDir].
	GraphChecksum string `json:"graph_checksum" yaml:"graph_checksum"`

	// TraceFile is the trace's filename within its per-graph traces subdirectory.
	TraceFile string `json:"trace_file" yaml:"trace_file"`
}

// Lifetime is the document of one lifetime.
type Lifetime struct {

	// ID is the lifetime's UUIDv7, minted by the deploy invocation that opened it; its leading 48 bits are the
	// moment it was minted.
	ID string `json:"id" yaml:"id"`

	// Opened is the UTC moment the lifetime was minted.
	Opened time.Time `json:"opened" yaml:"opened"`

	// Closed is the UTC moment the lifetime stopped being current: replaced or ended. Zero while current.
	Closed time.Time `json:"closed,omitempty" yaml:"closed,omitempty"`

	// State is [LifetimeCurrent], [LifetimeReplaced] or [LifetimeEnded].
	State string `json:"state" yaml:"state"`

	// Runs is every run the lifetime owns, in write order.
	Runs []LifetimeRun `json:"runs" yaml:"runs"`
}

// LifetimesDir returns the lifetimes directory under the store root.
//
// Returns:
//   - `string`: the absolute path of `lifetimes` under [StoreHome].
func LifetimesDir() string {
	return storePath(lifetimesDirname)
}

// NewLifetime mints a lifetime that is not yet on disk.
//
// A deploy mints after its pre-flight passes and before its first scope runs; the lifetime reaches the store,
// and replaces the current one, when its first run is appended -- so a deploy that fails before it writes any
// trace replaces nothing.
//
// Returns:
//   - `*Lifetime`: the minted lifetime, state [LifetimeCurrent], no runs, not persisted.
func NewLifetime() *Lifetime {

	now := time.Now().UTC()
	return &Lifetime{ID: uuid.Must(uuid.NewV7()).String(), Opened: now, State: LifetimeCurrent}
}

// CurrentLifetime loads the current lifetime.
//
// Returns:
//   - `*Lifetime`: the lifetime `current` names.
//   - `error`: [os.ErrNotExist] when the store has no current lifetime; any read or decode failure otherwise.
func CurrentLifetime() (*Lifetime, error) {

	data, err := os.ReadFile(filepath.Join(LifetimesDir(), currentFilename))
	if err != nil {
		return nil, err
	}

	return LoadLifetime(strings.TrimSpace(string(data)))
}

// LoadLifetime loads one lifetime by id.
//
// Parameters:
//   - `id`: the lifetime's id.
//
// Returns:
//   - `*Lifetime`: the decoded document.
//   - `error`: [os.ErrNotExist] when no such lifetime is on disk; any read or decode failure otherwise.
func LoadLifetime(id string) (*Lifetime, error) {

	data, err := os.ReadFile(lifetimePath(id))
	if err != nil {
		return nil, err
	}

	var lifetime Lifetime
	if err := yaml.Unmarshal(data, &lifetime); err != nil {
		return nil, fmt.Errorf("decode lifetime %s: %w", id, err)
	}

	return &lifetime, nil
}

// AppendRun records a run on the lifetime and persists the document.
//
// The first run persisted for a [LifetimeCurrent] lifetime makes it the current one: the previous current, if
// any, becomes [LifetimeReplaced] with `Closed` set. A lifetime that is replaced or ended refuses the run: it is
// no longer the record, and nothing writes into history.
//
// Parameters:
//   - `run`: the run to record; `At` is set to now when zero.
//
// Returns:
//   - `error`: when the lifetime is not current, or the store cannot be written.
func (l *Lifetime) AppendRun(run LifetimeRun) (err error) {

	if l.State != LifetimeCurrent {
		return fmt.Errorf("lifetime %s is %s: it no longer records runs", l.ID, l.State)
	}

	if run.At.IsZero() {
		run.At = time.Now().UTC()
	}
	l.Runs = append(l.Runs, run)

	stateRoot, err := OpenTree(StoreHome())
	if err != nil {
		return err
	}
	defer iox.Close(&err, stateRoot)

	if err := stateRoot.MkdirAll(stateRoot.NewPath(LifetimesDir()), 0o750); err != nil {
		return fmt.Errorf("create lifetimes directory: %w", err)
	}

	if err := l.replacePrevious(stateRoot); err != nil {
		return err
	}

	if err := l.save(stateRoot); err != nil {
		return err
	}

	return writeAtomically(stateRoot, filepath.Join(LifetimesDir(), currentFilename), []byte(l.ID+"\n"))
}

// RequireCurrentLifetime loads the current lifetime for an operation that writes into it.
//
// Upgrade, reconcile, adopt and decommission write into the current deployment; with none there is nothing to
// write into, and the answer is not-found -- [ExitNoInput], the same code `writ reconcile` gives on a machine never
// deployed (#756).
//
// Parameters:
//   - `operation`: the RunOperation* constant naming the caller, for the message.
//
// Returns:
//   - `*Lifetime`: the current lifetime.
//   - `error`: an [ExitNoInput]-coded error when there is no current lifetime; any other read failure as is.
func RequireCurrentLifetime(operation string) (*Lifetime, error) {

	lifetime, err := CurrentLifetime()
	if errors.Is(err, os.ErrNotExist) {
		return nil, ExitWith(ExitNoInput,
			fmt.Errorf("%s: no current deployment to write into: nothing has been deployed, or it was decommissioned",
				operation))
	}
	if err != nil {
		return nil, err
	}

	return lifetime, nil
}

// WriteLifetimeTrace persists a run's trace and records the run on its lifetime.
//
// [WriteTrace] then [Lifetime.AppendRun]: the trace is in the store before the lifetime names it, so a crash
// between the two leaves a trace no lifetime owns -- which the fold does not read -- rather than a lifetime naming
// a trace that is not there.
//
// Parameters:
//   - `lifetime`: the lifetime the run belongs to; for a deploy, the one it minted, for the others, the current.
//   - `operation`: the RunOperation* constant naming the writer.
//   - `trace`: the run's trace.
//
// Returns:
//   - `string`: the trace's path, as [WriteTrace] returns it.
//   - `error`: when the trace cannot be written, or the lifetime refuses the run.
func WriteLifetimeTrace(lifetime *Lifetime, operation string, trace *op.Trace) (string, error) {

	path, err := WriteTrace(trace)
	if err != nil {
		return "", err
	}

	run := LifetimeRun{Operation: operation, GraphChecksum: trace.GraphChecksum, TraceFile: filepath.Base(path)}
	if err := lifetime.AppendRun(run); err != nil {
		return path, fmt.Errorf("record %s run on lifetime %s: %w", operation, lifetime.ID, err)
	}

	return path, nil
}

// replacePrevious marks the current lifetime replaced when `l` is about to take its place -- once, on `l`'s first
// persisted run. A previous current that is `l` itself, or none, changes nothing.
//
// Parameters:
//   - `stateRoot`: the open store root.
//
// Returns:
//   - `error`: when the previous lifetime cannot be read or rewritten.
func (l *Lifetime) replacePrevious(stateRoot fsroot.Dir) error {

	if len(l.Runs) != 1 {
		return nil
	}

	previous, err := CurrentLifetime()
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if previous.ID == l.ID || previous.State != LifetimeCurrent {
		return nil
	}

	previous.State = LifetimeReplaced
	previous.Closed = time.Now().UTC()

	return previous.save(stateRoot)
}

// save writes the lifetime's document under [LifetimesDir], atomically.
//
// Parameters:
//   - `stateRoot`: the open store root.
//
// Returns:
//   - `error`: when the document cannot be encoded or written.
func (l *Lifetime) save(stateRoot fsroot.Dir) error {

	data, err := yaml.Marshal(l)
	if err != nil {
		return fmt.Errorf("encode lifetime %s: %w", l.ID, err)
	}

	return writeAtomically(stateRoot, lifetimePath(l.ID), data)
}

// lifetimePath returns the document path of the lifetime with `id`.
//
// Parameters:
//   - `id`: the lifetime's id.
//
// Returns:
//   - `string`: the absolute path of `<id>.yaml` under [LifetimesDir].
func lifetimePath(id string) string {
	return filepath.Join(LifetimesDir(), id+".yaml")
}

// writeAtomically writes `data` to `path` through a sibling temporary file and a rename, so a reader never sees a
// torn document.
//
// Parameters:
//   - `stateRoot`: the open store root.
//   - `path`: the absolute destination path under the store.
//   - `data`: the bytes to write.
//
// Returns:
//   - `error`: when the temporary file cannot be written or renamed into place.
func writeAtomically(stateRoot fsroot.Dir, path string, data []byte) error {

	temporary := stateRoot.NewPath(path + ".tmp")
	if err := stateRoot.WriteFile(temporary, data, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}

	if err := stateRoot.Rename(temporary, stateRoot.NewPath(path)); err != nil {
		return fmt.Errorf("place %s: %w", path, err)
	}

	return nil
}
