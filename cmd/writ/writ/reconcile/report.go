// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package reconcile

import (
	"encoding/json"

	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/readback"
)

// Report is the four-section reconcile report.
type Report struct {

	// Layers is the registered layer tree — the "where from".
	Layers []Layer `json:"layers"`

	// Entries is the classified deployed inventory, sorted by target.
	Entries []Entry `json:"entries"`

	// Packages records the package operations writ's runs performed, fact-of-record.
	Packages []readback.PackageRecord `json:"packages,omitempty"`

	// Health is the store's self-report: folded runs and missing-piece findings.
	Health Health `json:"health"`
}

// Layer is one conventional layer's registration status.
type Layer struct {

	// Name is the layer name: "base", "team", or "personal".
	Name string `json:"name"`

	// Path is the layer's location under the writ layers directory.
	Path string `json:"path"`

	// State is "absent", "directory", "link", or "broken-link".
	State string `json:"state"`

	// Target is the resolved link target when State is "link".
	Target string `json:"target,omitempty"`
}

// Entry is one classified inventory row.
type Entry struct {

	// Target is the absolute deployed path.
	Target string `json:"target"`

	// Source is the absolute source path the target was deployed from.
	Source string `json:"source"`

	// Project is the owning project.
	Project string `json:"project"`

	// Layer is the contributing layer, or "" in single-source mode.
	Layer string `json:"layer,omitempty"`

	// Scope is the target scope ("system" / "home", or "" for unscoped runs).
	Scope string `json:"scope,omitempty"`

	// Action is the target-producing action name.
	Action string `json:"action"`

	// State is the classification against the live filesystem.
	State State `json:"state"`

	// Repair names the lifecycle command that repairs the finding, or "" when none applies.
	Repair string `json:"repair,omitempty"`

	// Message elaborates the classification for human readers.
	Message string `json:"message,omitempty"`
}

// Health is the store's self-report.
type Health struct {

	// Runs is the number of traces folded into the inventory.
	Runs int `json:"runs"`

	// Findings are the missing-piece detections (index entries whose documents are gone, documents the index
	// never recorded).
	Findings []string `json:"findings,omitempty"`
}

// State classifies one record entry against the system. The record is the desired state (#923, ruled
// 2026-09-23): every word names how the system, or the record's reference to its source, stands against it.
type State int

const (
	// StateLinked means the symlink is as recorded and its referent's content is as recorded.
	StateLinked State = 0

	// StateCopied means the copied file is as recorded and its source's content is as recorded.
	StateCopied State = 1

	// StateAbsent means the record says a target is there and it is not. Repair: deploy.
	StateAbsent State = 2

	// StateChanged means the target is there but is not what the record says: not the recorded symlink, or a
	// copy whose content digest moved. Repair: deploy for a link, `upgrade --force` for a copy.
	StateChanged State = 3

	// StateDangling means the source the record names does not resolve -- a link whose referent is gone, or a
	// copy whose source is gone: a reference that outlives its referent. Reconcile reports it and leaves it; the
	// repair is a new record, which only deploy makes.
	StateDangling State = 4

	// StateStale means the deployed thing is as recorded, its source resolves, and the source's content no
	// longer matches the recorded source digest: a derivative behind an origin that still exists. Repair: upgrade.
	StateStale State = 5
)

// HasDrift reports whether any entry stands against the record: a state other than [StateLinked] or
// [StateCopied]. It is what `writ reconcile`'s exit status reads (#756); the store's health findings are its
// self-report and are not drift.
//
// Returns:
//   - `bool`: true when at least one entry is absent, changed, dangling or stale.
func (r *Report) HasDrift() bool {
	return r.DriftCount() > 0
}

// DriftCount counts the entries that stand against the record.
//
// Returns:
//   - `int`: the number of entries whose state is other than [StateLinked] or [StateCopied].
func (r *Report) DriftCount() int {
	count := 0
	for i := range r.Entries {
		if r.Entries[i].State != StateLinked && r.Entries[i].State != StateCopied {
			count++
		}
	}
	return count
}

// Label returns the machine-readable classification name.
//
// Returns:
//   - `string`: the lowercase label.
func (s State) Label() string {
	switch s {
	case StateLinked:
		return "linked"
	case StateCopied:
		return "copied"
	case StateAbsent:
		return "absent"
	case StateChanged:
		return "changed"
	case StateDangling:
		return "dangling"
	case StateStale:
		return "stale"
	default:
		return "unknown"
	}
}

// MarshalJSON encodes the state as its label.
//
// Returns:
//   - `[]byte`: the JSON-encoded label.
//   - `error`: any error from [json.Marshal].
func (s State) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.Label())
}
