// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package status

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/readback"
)

// Report is the four-section status report.
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

// String returns the entry-row indicator for the text report.
//
// Returns:
//   - `string`: the one-glyph indicator.
func (s State) String() string {
	switch s {
	case StateLinked:
		return "✓"
	case StateCopied:
		return "✓"
	case StateMissing:
		return "✗"
	case StateConflict:
		return "⚠"
	case StateOrphan:
		return "?"
	case StateModifiedOrStale:
		return "M"
	case StateStale:
		return "↑"
	case StateModified:
		return "M"
	default:
		return "!"
	}
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

// region HELPER FUNCTIONS

// presentJSON emits the report as indented JSON on stdout.
//
// Parameters:
//   - `report`: the report to emit.
//
// Returns:
//   - `error`: non-nil when encoding fails.
func presentJSON(report *Report) error {

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

// presentText emits the four-section human-readable report on stdout.
//
// Parameters:
//   - `report`: the report to emit.
//
// Returns:
//   - `error`: always nil; present for symmetry with [presentJSON].
func presentText(report *Report) error {

	fmt.Println("Layers:")
	for _, layer := range report.Layers {
		switch layer.State {
		case "link":
			fmt.Printf("  %-8s → %s\n", layer.Name, layer.Target)
		case "directory":
			fmt.Printf("  %-8s %s\n", layer.Name, layer.Path)
		case "absent":
			fmt.Printf("  %-8s (absent)\n", layer.Name)
		default:
			fmt.Printf("  %-8s (%s) %s\n", layer.Name, layer.State, layer.Path)
		}
	}
	fmt.Println()

	if len(report.Entries) == 0 {
		fmt.Println("No deployed files recorded.")
	} else {
		presentEntries(report.Entries)
	}

	if len(report.Packages) > 0 {
		fmt.Printf("Packages via writ: %d operation(s)\n", len(report.Packages))
		for _, record := range report.Packages {
			fmt.Printf("  %s %s (%s)\n", record.Action, record.UnitID, record.At.Format("2006-01-02"))
		}
		fmt.Println()
	}

	fmt.Printf("Store: %d run(s) folded", report.Health.Runs)
	if len(report.Health.Findings) > 0 {
		fmt.Printf(", %d finding(s) — findings may be incomplete:\n", len(report.Health.Findings))
		for _, finding := range report.Health.Findings {
			fmt.Printf("  ! %s\n", finding)
		}
	} else {
		fmt.Println(", store healthy")
	}

	return nil
}

// presentEntries prints the inventory grouped by project, with the per-state summary line.
//
// Parameters:
//   - `entries`: the classified entries, already sorted by target.
func presentEntries(entries []Entry) {

	byProject := make(map[string][]Entry)
	for i := range entries {
		entry := &entries[i]
		project := entry.Project
		if project == "" {
			project = "(unknown)"
		}
		byProject[project] = append(byProject[project], *entry)
	}

	projects := make([]string, 0, len(byProject))
	for project := range byProject {
		projects = append(projects, project)
	}
	sort.Strings(projects)

	for _, project := range projects {
		fmt.Printf("%s:\n", project)
		group := byProject[project]
		for i := range group {
			entry := &group[i]
			line := fmt.Sprintf("  %s %s", entry.State, entry.Target)
			if entry.Message != "" {
				line += " (" + entry.Message + ")"
			}
			if entry.Repair != "" {
				line += " — repair: " + entry.Repair
			}
			fmt.Println(line)
		}
		fmt.Println()
	}

	tally := make(map[State]int)
	for i := range entries {
		tally[entries[i].State]++
	}

	ok := tally[StateLinked] + tally[StateCopied]
	fmt.Printf("%d file(s): %d ok", len(entries), ok)
	for _, pair := range []struct {
		state State
		label string
	}{
		{StateMissing, "missing"},
		{StateConflict, "conflict"},
		{StateOrphan, "orphan"},
		{StateStale, "stale"},
		{StateModified, "modified"},
		{StateModifiedOrStale, "modified-or-stale"},
	} {
		if n := tally[pair.state]; n > 0 {
			fmt.Printf(", %d %s", n, pair.label)
		}
	}
	fmt.Println()
	fmt.Println()
}

// endregion
