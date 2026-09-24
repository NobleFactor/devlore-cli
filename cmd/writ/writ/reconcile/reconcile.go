// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

// Package reconcile reports what should be present, where it should have come from, and what's missing or
// different. Phase-8 step 47 named this command `writ status` because "reconcile" promised a mutation it
// did not perform; #762 returned the name once repair was chartered, and #774 landed it.
//
// Reconcile produces a report: today it mutates nothing, and each finding names the lifecycle command that repairs it
// (missing → `writ deploy`; stale → `writ upgrade`; modified → `writ upgrade --force`; orphan →
// `writ decommission`). The report has four sections: the registered layer tree (the "where from"), the
// deployed inventory per scope (the fold, classified against the live filesystem), the package operations
// writ's runs performed (fact-of-record), and store health (the run index's missing-piece detection). A missing
// run index is a hard error per the settled design — reconcile refuses to report from silence. Drift attribution
// (stale vs. modified) reads the run's recorded as-deployed content identity (step 48); runs traced before the
// capture report differing targets as modified-or-stale (indeterminate). Document-signature verification is
// `writ verify` (step 46).
package reconcile

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"

	"github.com/NobleFactor/devlore-cli/cmd/internal/devlore"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/readback"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/segment"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/file"
)

// Config carries the resolved settings for one reconcile report.
type Config struct {

	// Projects filters the inventory section; empty reports every project.
	Projects []string

	// Verbose narrates store detail via the shared console narrator.
	Verbose bool

	// Segments are the platform/custom segments for the freshness comparison.
	Segments segment.Segments

	// Vars are the user-configured template variables for the freshness comparison.
	Vars map[string]any
}

// BuildReport derives the four-section reconcile report from the store and the live filesystem.
//
// The report is the command's result and is rendered by the shared pipeline: every presentation is a
// presentation of its JSON, so there is no text renderer here and no format decision.
//
// Parameters:
//   - `ctx`: the context for the store fold.
//   - `cfg`: the resolved reconcile configuration.
//
// Returns:
//   - `*Report`: the assembled report.
//   - `error`: non-nil when the store has no current deployment (not-found) or the fold fails.
func BuildReport(ctx context.Context, cfg *Config) (*Report, error) {

	inventory, err := readback.Fold(ctx)
	if err != nil {
		return nil, err
	}

	report := &Report{
		Layers:   layerStatuses(),
		Packages: inventory.Packages,
		Health: Health{
			Runs:     inventory.Runs,
			Findings: inventory.Findings,
		},
	}

	wanted := make(map[string]bool, len(cfg.Projects))
	for _, p := range cfg.Projects {
		wanted[p] = true
	}

	//nolint:gocritic // rangeValCopy: map values are unaddressable; the per-iteration copy is the read.
	for _, entry := range inventory.Entries {
		if len(wanted) > 0 && !wanted[entry.Project] {
			continue
		}
		report.Entries = append(report.Entries, classifyEntry(entry))
	}

	sort.Slice(report.Entries, func(i, j int) bool { return report.Entries[i].Target < report.Entries[j].Target })

	return report, nil
}

// region HELPER FUNCTIONS

// classifyEntry classifies one record entry against the system, the record as the reference (#923).
//
// The occupant is judged by [readback.Entry.AsRecorded] -- what the record wrote -- then the source by the
// recorded source digest. No word is decided by consulting the layer checkout beyond the source the record names.
//
// Parameters:
//   - `entry`: the folded record entry.
//
// Returns:
//   - `Entry`: the classified report entry, with the repair it names.
func classifyEntry(entry readback.Entry) Entry {

	classified := Entry{
		Target:  entry.Target,
		Source:  entry.Source,
		Project: entry.Project,
		Layer:   entry.Layer,
		Scope:   entry.Scope,
		Action:  entry.Action,
	}

	if entry.Action == string(file.Link) {
		classifyLink(&classified, entry)
		return classified
	}

	classifyCopied(&classified, entry)
	return classified
}

// classifyLink judges a linked entry: absent, changed, dangling, stale, or linked.
//
// Parameters:
//   - `classified`: the report entry to fill; Target and Source are already set.
//   - `entry`: the record entry, with its recorded digests.
func classifyLink(classified *Entry, entry readback.Entry) {

	if _, err := os.Lstat(classified.Target); errors.Is(err, os.ErrNotExist) {
		classified.State = StateAbsent
		classified.Repair = "writ deploy"
		classified.Message = "symlink not present"
		return
	}

	if !entry.AsRecorded() {
		classified.State = StateChanged
		classified.Repair = "writ deploy"
		classified.Message = "not the symlink the record wrote"
		return
	}

	referent, err := os.ReadFile(classified.Target)
	if err != nil {
		classified.State = StateDangling
		classified.Repair = "writ deploy"
		classified.Message = "the recorded source does not resolve"
		return
	}

	if entry.RecordedSourceDigest != "" && readback.ContentDigest(referent) != entry.RecordedSourceDigest {
		classified.State = StateStale
		classified.Repair = "writ upgrade"
		classified.Message = "source changed since deployment"
		return
	}

	classified.State = StateLinked
}

// classifyCopied judges a copied entry: absent, changed, dangling, stale, or copied.
//
// The recorded target digest tells a local edit; the recorded source digest tells a source that moved, for a
// template's source and an encrypted source alike, since the source's bytes hash without rendering or decrypting.
//
// Parameters:
//   - `classified`: the report entry to fill; Target and Source are already set.
//   - `entry`: the record entry, with its recorded digests.
func classifyCopied(classified *Entry, entry readback.Entry) {

	if _, err := os.Lstat(classified.Target); errors.Is(err, os.ErrNotExist) {
		classified.State = StateAbsent
		classified.Repair = "writ deploy"
		classified.Message = "file not present"
		return
	}

	if entry.RecordedDigest != "" && !entry.AsRecorded() {
		classified.State = StateChanged
		classified.Repair = "writ upgrade --force"
		classified.Message = "not the content the record wrote"
		return
	}

	source, err := os.ReadFile(classified.Source)
	if err != nil {
		classified.State = StateDangling
		classified.Repair = "writ deploy"
		classified.Message = "the recorded source does not resolve"
		return
	}
	if entry.RecordedSourceDigest != "" && readback.ContentDigest(source) != entry.RecordedSourceDigest {
		classified.State = StateStale
		classified.Repair = "writ upgrade"
		classified.Message = "source changed since deployment"
		return
	}

	classified.State = StateCopied
}

// layerStatuses reports the registered layer tree under [devlore.WritLayersDir].
//
// Returns:
//   - `[]Layer`: one status per conventional layer (base, team, personal), in precedence order.
func layerStatuses() []Layer {

	var layers []Layer

	for _, name := range []string{"base", "team", "personal"} {

		path := filepath.Join(devlore.WritLayersDir(), name)
		layer := Layer{Name: name, Path: path}

		info, err := os.Lstat(path)
		switch {
		case errors.Is(err, os.ErrNotExist):
			layer.State = "absent"
		case err != nil:
			layer.State = "broken-link"
		case info.Mode()&os.ModeSymlink != 0:
			target, resolveErr := filepath.EvalSymlinks(path)
			if resolveErr != nil {
				layer.State = "broken-link"
			} else {
				layer.State = "link"
				layer.Target = target
			}
		case info.IsDir():
			layer.State = "directory"
		default:
			layer.State = "broken-link"
		}

		layers = append(layers, layer)
	}

	return layers
}

// endregion
