// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package status reports what should be present, where it should have come from, and what's missing or
// different (phase-8 step 47 slice 3 — `writ status` replaces `writ reconcile`).
//
// Status is report-only: it mutates nothing, and each finding names the lifecycle command that repairs it
// (missing → `writ deploy`; modified-or-stale → `writ upgrade`; orphan → `writ decommission`). The report has
// four sections: the registered layer tree (the "where from"), the deployed inventory per scope (the fold,
// classified against the live filesystem), the package operations writ's runs performed (fact-of-record), and
// store health (the run index's missing-piece detection). A missing run index is a hard error per the settled
// design — status refuses to report from silence. Until step 48 records as-deployed content identity, a
// differing copied target reports as modified-or-stale (indeterminate); the receipt-signature check arrives
// with step 46.
package status

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/deploy"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/readback"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/segment"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/tree"
	"github.com/NobleFactor/devlore-cli/internal/cli"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/template"
)

// Config carries the resolved settings for one status report.
type Config struct {

	// Projects filters the inventory section; empty reports every project.
	Projects []string

	// JSON emits the report as JSON instead of human-readable text.
	JSON bool

	// Verbose narrates store detail via [cli.Note].
	Verbose bool

	// Segments are the platform/custom segments for the freshness comparison.
	Segments segment.Segments

	// Vars are the user-configured template variables for the freshness comparison.
	Vars map[string]any
}

// Execute builds the status report and presents it.
//
// Parameters:
//   - `ctx`: the context for the store fold.
//   - `cfg`: the resolved status configuration.
//
// Returns:
//   - `error`: non-nil when the run index is missing, the fold fails, or presentation fails.
func Execute(ctx context.Context, cfg *Config) error {

	report, err := BuildReport(ctx, cfg)
	if err != nil {
		return err
	}

	if cfg.JSON {
		return presentJSON(report)
	}
	return presentText(report)
}

// BuildReport derives the four-section status report from the store and the live filesystem.
//
// Parameters:
//   - `ctx`: the context for the store fold.
//   - `cfg`: the resolved status configuration.
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
// in-process result of the current source (the interim posture: a difference is modified-or-stale,
// indeterminate until step 48 records as-deployed identity; encrypted entries are not compared).
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

	if entry.Action == "file.link" {
		classifyLink(&classified)
		return classified
	}

	classifyCopied(&classified, data)
	return classified
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
//   - `data`: the render data for the freshness comparison.
func classifyCopied(classified *Entry, data map[string]any) {

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
		classified.State = StateCopied
		classified.Message = "encrypted; content not compared"
		return
	}

	current, err := os.ReadFile(classified.Target)
	if err != nil {
		classified.State = StateConflict
		classified.Message = "target cannot be read"
		return
	}

	if bytes.Equal(current, fresh) {
		classified.State = StateCopied
		return
	}

	classified.State = StateModifiedOrStale
	classified.Repair = "writ upgrade"
	classified.Message = "differs from a fresh result (source change and local edits are indistinguishable until step 48)"
}

// layerStatuses reports the registered layer tree under [cli.WritLayersDir].
//
// Returns:
//   - `[]Layer`: one status per conventional layer (base, team, personal), in precedence order.
func layerStatuses() []Layer {

	var layers []Layer

	for _, name := range []string{"base", "team", "personal"} {

		path := filepath.Join(cli.WritLayersDir(), name)
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
