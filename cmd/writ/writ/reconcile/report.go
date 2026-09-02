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

// State classifies one inventory entry against the live filesystem.
type State int

const (
	// StateLinked means the symlink exists and resolves to its source.
	StateLinked State = iota

	// StateCopied means the copied file is present (and matches a fresh result when comparable).
	StateCopied

	// StateMissing means the deployed target is gone.
	StateMissing

	// StateConflict means something else occupies the target (wrong kind, wrong link endpoint, unreadable).
	StateConflict

	// StateOrphan means the target exists but its source is gone.
	StateOrphan

	// StateModifiedOrStale means a comparable copied target differs from a fresh result and the run predates
	// the step-48 recorded identity — a source change and a local edit are indistinguishable.
	StateModifiedOrStale

	// StateStale means the target is unchanged since deployment (its digest equals the recorded as-deployed
	// identity) and the source moved; `writ upgrade` regenerates it freely.
	StateStale

	// StateModified means the target was edited locally after deployment (its digest differs from the
	// recorded identity); `writ upgrade --force` overwrites.
	StateModified
)

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
	case StateMissing:
		return "missing"
	case StateConflict:
		return "conflict"
	case StateOrphan:
		return "orphan"
	case StateModifiedOrStale:
		return "modified-or-stale"
	case StateStale:
		return "stale"
	case StateModified:
		return "modified"
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
