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
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/NobleFactor/devlore-cli/cmd/internal/devlore"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/deploy"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/readback"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/segment"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/tree"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/file"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/template"
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
//   - `error`: non-nil when the run index is missing or the fold fails.
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

	data := deploy.RenderData(cfg.Segments, cfg.Vars)

	wanted := make(map[string]bool, len(cfg.Projects))
	for _, p := range cfg.Projects {
		wanted[p] = true
	}

	//nolint:gocritic // rangeValCopy: map values are unaddressable; the per-iteration copy is the read.
	for _, entry := range inventory.Entries {
		if len(wanted) > 0 && !wanted[entry.Project] {
			continue
		}
		report.Entries = append(report.Entries, classifyEntry(entry, data))
	}

	sort.Slice(report.Entries, func(i, j int) bool { return report.Entries[i].Target < report.Entries[j].Target })

	return report, nil
}

// region HELPER FUNCTIONS

// classifyEntry classifies one deployed entry against the live filesystem.
//
// Linked entries verify the symlink and its endpoints; copied entries compare content against a fresh
// in-process result of the current source and attribute differences through the run's recorded as-deployed
// identity (step 48): target-digest ≠ recorded → modified; target unchanged + fresh differs → stale; no
// recorded identity → modified-or-stale (indeterminate). Encrypted chains attribute through the recorded
// SOURCE digest (the encrypted bytes hash without decrypting) when the run cataloged the source.
//
// Parameters:
//   - `entry`: the folded inventory entry.
//   - `data`: the render data for the freshness comparison.
//
// Returns:
//   - `Entry`: the classified report entry, with its repair pointer.
func classifyEntry(entry readback.Entry, data map[string]any) Entry {

	classified := Entry{
		Target:  entry.Target,
		Source:  entry.Source,
		Project: entry.Project,
		Layer:   entry.Layer,
		Scope:   entry.Scope,
		Action:  entry.Action,
	}

	if entry.Action == string(file.Link) {
		classifyLink(&classified)
		return classified
	}

	classifyCopied(&classified, recordedPair{target: entry.RecordedDigest, source: entry.RecordedSourceDigest}, data)
	return classified
}

// recordedPair carries an entry's step-48 recorded content identities into the copied classification.
type recordedPair struct {

	// target is the as-deployed digest of the target, or "" for pre-capture runs.
	target string

	// source is the recorded digest of the source (encrypted chains), or "" when not cataloged.
	source string
}

// classifyLink fills the classification for a linked entry.
//
// Parameters:
//   - `classified`: the report entry to fill; Target and Source are already set.
func classifyLink(classified *Entry) {

	info, err := os.Lstat(classified.Target)
	if errors.Is(err, os.ErrNotExist) {
		classified.State = StateMissing
		classified.Repair = "writ deploy"
		classified.Message = "symlink not present"
		return
	}
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		classified.State = StateConflict
		classified.Message = "target exists but is not a symlink"
		return
	}

	resolvedTarget, err := filepath.EvalSymlinks(classified.Target)
	if err != nil {
		if _, sourceErr := os.Lstat(classified.Source); errors.Is(sourceErr, os.ErrNotExist) {
			classified.State = StateOrphan
			classified.Repair = "writ decommission"
			classified.Message = "symlink points at a deleted source"
			return
		}
		classified.State = StateConflict
		classified.Message = "symlink cannot be resolved"
		return
	}

	resolvedSource, err := filepath.EvalSymlinks(classified.Source)
	if err != nil {
		classified.State = StateOrphan
		classified.Repair = "writ decommission"
		classified.Message = "source no longer exists"
		return
	}

	if resolvedTarget != resolvedSource {
		classified.State = StateConflict
		classified.Message = "symlink points at " + resolvedTarget
		return
	}

	classified.State = StateLinked
}

// classifyCopied fills the classification for a copied entry (template render, sops decrypt, plain copy).
//
// Parameters:
//   - `classified`: the report entry to fill; Target, Source, and Action are already set.
//   - `recorded`: the entry's step-48 recorded content identities (empty fields = pre-capture run).
//   - `data`: the render data for the freshness comparison.
func classifyCopied(classified *Entry, recorded recordedPair, data map[string]any) {

	if _, err := os.Lstat(classified.Target); errors.Is(err, os.ErrNotExist) {
		classified.State = StateMissing
		classified.Repair = "writ deploy"
		classified.Message = "file not present"
		return
	}

	source, err := os.ReadFile(classified.Source)
	if err != nil {
		classified.State = StateOrphan
		classified.Repair = "writ decommission"
		classified.Message = "source no longer readable"
		return
	}

	current, err := os.ReadFile(classified.Target)
	if err != nil {
		classified.State = StateConflict
		classified.Message = "target cannot be read"
		return
	}

	// Local-edit attribution (step 48): the recorded as-deployed digest tells target-modified apart from
	// source-changed. Absent identity (a pre-capture run) leaves the indeterminate class.
	targetUnchanged := false
	if recorded.target != "" {
		if readback.ContentDigest(current) != recorded.target {
			classified.State = StateModified
			classified.Repair = "writ upgrade --force"
			classified.Message = "locally modified since deployment"
			return
		}
		targetUnchanged = true
	}

	_, operations := tree.ProcessingPipeline(filepath.Base(classified.Source))
	pipeline := strings.Join(operations, "+")

	var fresh []byte
	switch pipeline {
	case "template.render_bytes+file.copy":
		provider := &template.Provider{}
		rendered, renderErr := provider.RenderText(string(source), data)
		if renderErr != nil {
			classified.State = StateModifiedOrStale
			classified.Repair = "writ upgrade"
			classified.Message = "current source fails to render: " + renderErr.Error()
			return
		}
		fresh = []byte(rendered)
	case "file.link":
		fresh = source
	default:
		// Encrypted chains: the fresh result is not computable, but the ENCRYPTED source's bytes are hashable
		// when the run cataloged the source — source movement attributes without decrypting.
		if targetUnchanged && recorded.source != "" {
			if readback.ContentDigest(source) == recorded.source {
				classified.State = StateCopied
				return
			}
			classified.State = StateStale
			classified.Repair = "writ upgrade"
			classified.Message = "encrypted source changed since deployment"
			return
		}
		classified.State = StateCopied
		classified.Message = "encrypted; content not compared"
		return
	}

	if bytes.Equal(current, fresh) {
		classified.State = StateCopied
		return
	}

	if targetUnchanged {
		classified.State = StateStale
		classified.Repair = "writ upgrade"
		classified.Message = "source changed since deployment"
		return
	}

	classified.State = StateModifiedOrStale
	classified.Repair = "writ upgrade"
	classified.Message = "differs from a fresh result (this run predates the recorded content identity)"
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
